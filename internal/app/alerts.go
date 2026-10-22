package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"freebeams/internal/app/core"
	"freebeams/internal/control"
	"freebeams/internal/models"
	"freebeams/internal/render"
	"freebeams/internal/ui"
)

// Alerts: the toolbar's Alerts ▾ menu, the saved alerts (store), the live
// overlay (core.Alerts: message alerts, nursery codes, the countdown and the
// clock) and its styles (Options ▸ Alerts, a.settings.Alerts). The live state
// is authoritative and ticked once per frame with the wall clock; the overlay
// layer of the Live monitor and of the output is a projection of
// core.Alerts.Frame, set every frame (renderOverlay) without marking the app
// dirty, so a scrolling ticker never keeps wait-idle from answering.

// alertsState is the app's alert state.
type alertsState struct {
	live   core.Alerts
	saved  []models.Alert // the saved alerts, by name
	ver    int            // bumped when saved changes
	mgrSel int            // the Alert Manager's selected row (-1: none)
	// fill is the alert whose tokens the Fill Alert Text Tokens dialog asks
	// for (a saved alert, or a one-off from the editor's Show).
	fill *models.Alert
}

func (a *App) alertNow() time.Time { return time.Now() }

// initAlerts loads the saved alerts and the overlay's font metrics.
func (a *App) initAlerts() {
	a.al.mgrSel = -1
	a.al.live.Measure = render.MeasureAlertText
	a.loadSavedAlerts(nil)
}

// loadSavedAlerts reads the saved alerts off the main thread.
func (a *App) loadSavedAlerts(cmd *control.Command) {
	st := a.st()
	if st == nil {
		a.answer(cmd, nil)
		return
	}
	a.async(cmd, func(ctx context.Context) func() error {
		list, err := st.ListAlerts(ctx)
		return func() error {
			if err != nil {
				a.fail("list alerts", err)
				return err
			}
			a.setSavedAlerts(list, 0)
			return nil
		}
	})
}

// setSavedAlerts replaces the list, selecting id in the manager (0 keeps the
// selection in range).
func (a *App) setSavedAlerts(list []models.Alert, id int64) {
	a.al.saved = list
	a.al.ver++
	if id != 0 {
		a.al.mgrSel = findAlertID(list, id)
	}
	if a.al.mgrSel >= len(list) {
		a.al.mgrSel = len(list) - 1
	}
}

func findAlertID(list []models.Alert, id int64) int {
	for i, al := range list {
		if al.ID == id {
			return i
		}
	}
	return -1
}

// findAlert finds a saved alert by id, else by name (case-insensitive).
func (a *App) findAlert(id int64, name string) int {
	if id != 0 {
		return findAlertID(a.al.saved, id)
	}
	for i, al := range a.al.saved {
		if strings.EqualFold(al.Name, strings.TrimSpace(name)) {
			return i
		}
	}
	return -1
}

// ---- events -----------------------------------------------------------------------

// saveAlert inserts or updates a saved alert (the editor's OK).
func (a *App) saveAlert(al models.Alert, cmd *control.Command) {
	st := a.st()
	if st == nil {
		a.answer(cmd, errors.New("save alert: the library is unavailable"))
		return
	}
	a.async(cmd, func(ctx context.Context) func() error {
		err := st.SaveAlert(ctx, &al)
		var list []models.Alert
		if err == nil {
			list, err = st.ListAlerts(ctx)
		}
		return func() error {
			if err != nil {
				if cmd == nil {
					a.alert("Save Alert", err)
				} else {
					a.fail("save alert", err)
				}
				return err
			}
			a.setSavedAlerts(list, al.ID)
			return nil
		}
	})
}

// deleteAlert removes a saved alert, asking first unless confirm is set.
func (a *App) deleteAlert(i int, confirm bool, cmd *control.Command) {
	if i < 0 || i >= len(a.al.saved) {
		a.answer(cmd, fmt.Errorf("delete-alert: no saved alert %d", i))
		return
	}
	al := a.al.saved[i]
	if !confirm {
		a.win.Confirm.Ask("Delete Alert", "Delete the alert \""+al.Name+"\"?", func() {
			a.event(func() { a.deleteAlert(findAlertID(a.al.saved, al.ID), true, nil) })
		})
		a.answer(cmd, nil)
		return
	}
	st := a.st()
	if st == nil {
		a.answer(cmd, errors.New("delete alert: the library is unavailable"))
		return
	}
	a.async(cmd, func(ctx context.Context) func() error {
		err := st.DeleteAlert(ctx, al.ID)
		var list []models.Alert
		if err == nil {
			list, err = st.ListAlerts(ctx)
		}
		return func() error {
			if err != nil {
				a.fail("delete alert", err)
				return err
			}
			a.setSavedAlerts(list, 0)
			return nil
		}
	})
}

// showAlert fills an alert's tokens from values and puts it on the queue.
func (a *App) showAlert(al models.Alert, values map[string]string, location string) (int, error) {
	al = al.Normalized()
	if al.Message == "" {
		return 0, errors.New("the alert has no message")
	}
	text, missing := models.FillAlertTokens(al.Message, values)
	if len(missing) > 0 {
		return 0, fmt.Errorf("the alert needs a value for %s", strings.Join(missing, ", "))
	}
	run := core.RunFromAlert(al, text)
	run.Location = location
	return a.al.live.Show(run, a.alertNow()), nil
}

// useAlert shows an alert as the Alerts ▾ menu and the manager's Use do:
// an alert with tokens asks for their values first.
func (a *App) useAlert(al models.Alert) {
	if toks := models.AlertTokens(al.Message); len(toks) > 0 {
		cp := al
		a.al.fill = &cp
		title := al.Name
		if title == "" {
			title = "Alert"
		}
		a.win.AlertFill.Open(title, al.Message, toks)
		return
	}
	if _, err := a.showAlert(al, nil, ""); err != nil {
		a.alert("Alert", err)
	}
}

// fillUsed is the Fill dialog's USE.
func (a *App) fillUsed(values map[string]string) {
	al := a.al.fill
	a.al.fill = nil
	if al == nil {
		return
	}
	if _, err := a.showAlert(*al, values, ""); err != nil {
		a.alert("Alert", err)
	}
}

// startCountdownForm starts the Countdown dialog's countdown.
func (a *App) startCountdownForm(f ui.CountdownForm) error {
	now := a.alertNow()
	req := core.CountdownRequest{Caption: f.Caption, Position: f.Position, End: f.End}
	if f.ToTime {
		at, err := models.ParseClockTime(f.Time, now)
		if err != nil {
			return err
		}
		req.Target = at
	} else {
		secs := f.Minutes*60 + f.Seconds
		if secs <= 0 {
			return errors.New("the countdown needs a length")
		}
		req.Seconds = float64(secs)
	}
	a.al.live.StartCountdown(req, now)
	return nil
}

// onAlertsMenu runs an Alerts ▾ entry (i: the saved alert for AlertsMenuAlert).
func (a *App) onAlertsMenu(k ui.AlertsMenuKind, i int) error {
	switch k {
	case ui.AlertsMenuAlert:
		if i < 0 || i >= len(a.al.saved) {
			return fmt.Errorf("no saved alert %d", i)
		}
		a.useAlert(a.al.saved[i])
	case ui.AlertsMenuNew:
		a.win.AlertEditor.Open(models.Alert{Display: models.AlertScroll, End: models.AlertEndCount, Count: 3})
	case ui.AlertsMenuManager:
		a.win.AlertManager.SetAlerts(a.al.saved, a.al.mgrSel)
		a.win.AlertManager.Open()
	case ui.AlertsMenuNursery:
		a.win.Nursery.SetCodes(a.al.live.NurseryCodes())
		a.win.Nursery.Open()
	case ui.AlertsMenuCountdown:
		st := a.settings.Alerts
		a.win.Countdown.Open(st.CountdownPosition, st.CountdownEnd)
	case ui.AlertsMenuClock:
		a.al.live.SetClock(!a.al.live.ClockOn())
	case ui.AlertsMenuClear:
		a.al.live.ClearAll()
	}
	return nil
}

// setAlertStyles changes Options ▸ Alerts.
func (a *App) setAlertStyles(st models.AlertStyles) {
	st = st.Normalized()
	if a.settings.Alerts == st {
		return
	}
	a.settings.Alerts = st
	a.settingsVer++
	a.saveSetting(core.SettingAlertStyles, st)
}

// ---- view -------------------------------------------------------------------------

// wireAlerts connects the Alerts ▾ menu and the alert dialogs (buildUI).
func (a *App) wireAlerts() {
	w := a.win
	w.Toolbar.SetEnabled(ui.ActionAlerts, true)
	w.Toolbar.OnAlertsMenu = func(k ui.AlertsMenuKind, i int) {
		a.event(func() { a.fail("alerts menu", a.onAlertsMenu(k, i)) })
	}
	w.Settings.OnAlertStyles = func(st models.AlertStyles) { a.event(func() { a.setAlertStyles(st) }) }

	m := w.AlertManager
	m.OnSelect = func(i int) { a.event(func() { a.al.mgrSel = i }) }
	m.OnAdd = func() { a.event(func() { a.fail("alerts menu", a.onAlertsMenu(ui.AlertsMenuNew, -1)) }) }
	m.OnDelete = func(i int) { a.event(func() { a.deleteAlert(i, false, nil) }) }
	m.OnEdit = func(i int) {
		a.event(func() {
			if i >= 0 && i < len(a.al.saved) {
				a.al.mgrSel = i
				w.AlertEditor.Open(a.al.saved[i])
			}
		})
	}
	m.OnUse = func(i int) {
		a.event(func() {
			if i < 0 || i >= len(a.al.saved) {
				return
			}
			al := a.al.saved[i]
			a.al.mgrSel = i
			w.AlertManager.Hide()
			// The token question opens once the manager is down (Godot gives
			// a window one exclusive child; dialogHost picks the host).
			a.after(1, func() { a.useAlert(al) })
		})
	}

	e := w.AlertEditor
	e.OnSave = func(al models.Alert) { a.event(func() { a.saveAlert(al, nil) }) }
	e.OnShow = func(al models.Alert) { a.event(func() { a.after(1, func() { a.useAlert(al) }) }) }

	w.AlertFill.OnUse = func(values map[string]string) { a.event(func() { a.fillUsed(values) }) }

	n := w.Nursery
	n.OnAdd = func(code string) { a.event(func() { a.al.live.AddNursery(code, a.alertNow()) }) }
	n.OnRemove = func(code string) { a.event(func() { a.al.live.RemoveNursery(code) }) }
	n.OnClear = func() { a.event(func() { a.al.live.ClearNursery() }) }

	c := w.Countdown
	c.OnStart = func(f ui.CountdownForm) {
		a.event(func() {
			if err := a.startCountdownForm(f); err != nil {
				c.SetError(err.Error())
				return
			}
			c.Hide()
		})
	}
	c.OnStop = func() { a.event(func() { a.al.live.StopCountdown() }) }
}

// tickAlerts advances the live alerts to now (before render): a finished
// message gives way to the next, nursery codes expire, a countdown ends.
func (a *App) tickAlerts() {
	if a.al.live.Tick(a.alertNow(), a.settings.Alerts) {
		a.dirty = true
	}
}

// renderAlerts projects the saved alerts and the live alert lists onto the
// Alerts ▾ menu and the dialogs (part of render).
func (a *App) renderAlerts(v *viewCache) {
	w := a.win
	clock := a.al.live.ClockOn()
	if !v.init || a.al.ver != v.alertsVer || clock != v.alertsClock {
		names := make([]string, len(a.al.saved))
		for i, al := range a.al.saved {
			names[i] = al.Name
		}
		w.Toolbar.SetAlertsMenu(names, clock)
		v.alertsClock = clock
	}
	if !v.init || a.al.ver != v.alertsVer || a.al.mgrSel != v.alertsMgrSel {
		w.AlertManager.SetAlerts(a.al.saved, a.al.mgrSel)
		v.alertsMgrSel = a.al.mgrSel
	}
	v.alertsVer = a.al.ver
	if codes := a.al.live.NurseryCodes(); !v.init || !equalStrings(codes, v.alertsNursery) {
		v.alertsNursery = codes
		w.Nursery.SetCodes(codes)
	}
}

// renderOverlay projects the live alerts onto the Live monitor's and the
// output's overlay layers. It runs every frame (a ticker moves, the countdown
// and the clock change each second) and writes only view memos.
func (a *App) renderOverlay() {
	now, st := a.alertNow(), a.settings.Alerts
	hidden := core.Hidden(st, a.pres.State.Mode)
	f := a.al.live.Frame(now, st, hidden)
	a.liveView.SetOverlay(f, st)
	a.out.SetOverlay(f, st)
	moving := f.Message.Visible && f.Message.Scroll
	if moving != a.view.alertsAnim {
		a.view.alertsAnim = moving
		if moving {
			render.AcquireAnimating()
		} else {
			render.ReleaseAnimating()
		}
	}
	if w := a.win.Countdown; w.Visible() {
		running, status := a.al.live.CountdownActive(), ""
		if running {
			cs := a.al.live.State(now, st, hidden, nil).Countdown
			status = "Running: " + cs.Text
			if cs.Caption != "" {
				status = "Running: " + cs.Caption + " " + cs.Text
			}
		}
		if running != a.view.alertsCdRunning || status != a.view.alertsCdStatus {
			a.view.alertsCdRunning, a.view.alertsCdStatus = running, status
			w.SetStatus(running, status)
		}
	}
}

// alertsStateDoc is /state.alerts.
func (a *App) alertsStateDoc() core.AlertsState {
	st := a.settings.Alerts
	return a.al.live.State(a.alertNow(), st, core.Hidden(st, a.pres.State.Mode), a.al.saved)
}

// ---- control ------------------------------------------------------------------------

// alertFromArgs builds the alert an `alert` action shows: a saved one (id or
// name) or a one-off message (text), with the display options overridden.
func (a *App) alertFromArgs(args actionArgs) (models.Alert, error) {
	var al models.Alert
	switch {
	case args.ID != 0 || args.Name != "":
		i := a.findAlert(args.ID, args.Name)
		if i < 0 {
			return al, fmt.Errorf("no saved alert matches id %d / name %q", args.ID, args.Name)
		}
		al = a.al.saved[i]
		if args.Text != "" {
			al.Message = args.Text
		}
	case strings.TrimSpace(args.Text) != "":
		al = models.Alert{Message: args.Text, Display: models.AlertScroll, End: models.AlertEndCount, Count: 3}
	default:
		return al, errors.New("give the text of a message, or the id or name of a saved alert")
	}
	if args.Display != "" {
		d, err := models.ParseAlertDisplay(args.Display)
		if err != nil {
			return al, err
		}
		al.Display = d
	}
	if args.Align != "" {
		switch h := models.HAlign(strings.ToLower(args.Align)); h {
		case models.AlignLeft, models.AlignCenter, models.AlignRight:
			al.Align = h
		default:
			return al, fmt.Errorf("unknown align %q (left|center|right)", args.Align)
		}
	}
	if args.Count != nil {
		al.End, al.Count = models.AlertEndCount, *args.Count
	}
	if args.Seconds != nil {
		al.End, al.Seconds = models.AlertEndDuration, int(*args.Seconds)
	}
	if args.End != "" {
		switch e := models.AlertEnd(strings.ToLower(args.End)); e {
		case models.AlertEndCount, models.AlertEndManual, models.AlertEndToken, models.AlertEndDuration:
			al.End = e
		default:
			return al, fmt.Errorf("unknown end %q (count|manual|token|duration)", args.End)
		}
	}
	return al.Normalized(), nil
}

// countdownFromArgs reads a countdown request: {seconds | minutes | time,
// caption, position, end}.
func (a *App) countdownFromArgs(args actionArgs, now time.Time) (core.CountdownRequest, error) {
	req := core.CountdownRequest{Caption: args.Caption}
	switch {
	case args.Time != "":
		at, err := models.ParseClockTime(args.Time, now)
		if err != nil {
			return req, err
		}
		req.Target = at
	case args.Seconds != nil || args.Minutes != nil:
		var secs float64
		if args.Seconds != nil {
			secs += *args.Seconds
		}
		if args.Minutes != nil {
			secs += *args.Minutes * 60
		}
		if secs <= 0 {
			return req, errors.New("the countdown needs a positive length")
		}
		req.Seconds = secs
	default:
		return req, errors.New("give seconds, minutes or a time of day")
	}
	if args.Position != "" {
		p, err := models.ParseAlertPosition(args.Position)
		if err != nil {
			return req, err
		}
		req.Position = p
	}
	if args.End != "" {
		switch e := models.CountdownEnd(strings.ToLower(args.End)); e {
		case models.CountdownHide, models.CountdownHold, models.CountdownOverrun:
			req.End = e
		default:
			return req, fmt.Errorf("unknown end %q (hide|hold|overrun)", args.End)
		}
	}
	return req, nil
}

// countdownFormArgs fills the Countdown dialog from a countdown-form action.
func countdownFormArgs(args actionArgs) (ui.CountdownForm, error) {
	f := ui.CountdownForm{ToTime: args.Time != "", Time: args.Time, Caption: args.Caption}
	if args.Minutes != nil {
		f.Minutes = int(*args.Minutes)
	}
	if args.Seconds != nil {
		f.Seconds = int(*args.Seconds)
	}
	if args.Position != "" {
		p, err := models.ParseAlertPosition(args.Position)
		if err != nil {
			return f, err
		}
		f.Position = p
	}
	if args.End != "" {
		f.End = models.CountdownEnd(strings.ToLower(args.End))
	}
	return f, nil
}

// handleAlertsCommand runs the alert actions; false when the action is not
// one of them.
func (a *App) handleAlertsCommand(cmd *control.Command, args actionArgs) bool {
	ok := func() { a.answer(cmd, nil) }
	refuse := func(err error) { cmd.RespondErr(fmt.Errorf("%s: %w", cmd.Action, err)) }
	w := a.win
	now := a.alertNow()
	switch cmd.Action {
	case "alert":
		// Show a message alert: {text} or {id | name, values}, with optional
		// display (ticker|static|auto), location (top|bottom), align, count,
		// seconds and end.
		al, err := a.alertFromArgs(args)
		if err != nil {
			refuse(err)
			return true
		}
		loc := strings.ToLower(args.Location)
		if loc != "" && loc != "top" && loc != "bottom" {
			refuse(fmt.Errorf("unknown location %q (top|bottom)", args.Location))
			return true
		}
		if _, err := a.showAlert(al, args.Values, loc); err != nil {
			refuse(err)
			return true
		}
		ok()
	case "alert-next":
		a.al.live.Next(now)
		ok()
	case "alert-clear":
		// {run}: one message; {kind}: all (default), messages, nursery,
		// countdown or clock.
		if args.Run != nil {
			if !a.al.live.ClearMessage(*args.Run, now) {
				refuse(fmt.Errorf("no message alert %d", *args.Run))
				return true
			}
			ok()
			return true
		}
		switch strings.ToLower(args.Kind) {
		case "", "all":
			a.al.live.ClearAll()
		case "messages", "message":
			a.al.live.ClearMessages()
		case "nursery":
			a.al.live.ClearNursery()
		case "countdown":
			a.al.live.StopCountdown()
		case "clock":
			a.al.live.SetClock(false)
		default:
			refuse(fmt.Errorf("unknown kind %q (all|messages|nursery|countdown|clock)", args.Kind))
			return true
		}
		ok()
	case "nursery":
		if !a.al.live.AddNursery(args.Code, now) {
			refuse(fmt.Errorf("code %q is empty or already shown", args.Code))
			return true
		}
		ok()
	case "nursery-remove":
		if !a.al.live.RemoveNursery(args.Code) {
			refuse(fmt.Errorf("code %q is not shown", args.Code))
			return true
		}
		ok()
	case "countdown":
		req, err := a.countdownFromArgs(args, now)
		if err != nil {
			refuse(err)
			return true
		}
		a.al.live.StartCountdown(req, now)
		ok()
	case "countdown-stop":
		a.al.live.StopCountdown()
		ok()
	case "clock":
		on := !a.al.live.ClockOn()
		if args.On != nil {
			on = *args.On
		}
		a.al.live.SetClock(on)
		ok()
	case "save-alert":
		if args.Alert == nil {
			refuse(errors.New("alert is required"))
			return true
		}
		a.saveAlert(*args.Alert, cmd)
	case "delete-alert":
		a.deleteAlert(a.findAlert(args.ID, args.Name), args.Confirm, cmd)
	case "alerts-menu":
		// Run an Alerts ▾ entry as a click does: {item: alert|new|manager|
		// nursery|countdown|clock|clear, index | name}; {item: "open"} pops
		// the menu up, {item: "close"} closes it. The reply carries the
		// menu's entries.
		var err error
		switch item := strings.ToLower(args.Item); item {
		case "open":
			if !w.Toolbar.PopupAlerts() {
				err = errors.New("the menu is not built yet")
			}
		case "close":
			w.Toolbar.HideAlertsMenu()
		default:
			k, found := ui.ParseAlertsMenu(item)
			if !found {
				refuse(fmt.Errorf("unknown item %q", args.Item))
				return true
			}
			i := -1
			if k == ui.AlertsMenuAlert {
				switch {
				case args.Index != nil:
					i = *args.Index
				default:
					i = a.findAlert(args.ID, args.Name)
				}
			}
			err = a.onAlertsMenu(k, i)
		}
		if err != nil {
			refuse(err)
			return true
		}
		a.replies = append(a.replies, func() {
			entries, clock := w.Toolbar.AlertsMenuEntries()
			cmd.RespondJSON(struct {
				Entries []string   `json:"entries"`
				Clock   bool       `json:"clock"`
				Open    bool       `json:"open"`
				State   core.State `json:"state"`
			}{entries, clock, w.Toolbar.AlertsMenuVisible(), a.state()})
		})
	case "alert-manager":
		// The Alert Manager's buttons: {index, mode: select (default) | use |
		// edit | delete}.
		if !w.AlertManager.Visible() {
			refuse(errors.New("the Alert Manager is not open"))
			return true
		}
		if args.Index == nil || *args.Index < 0 || *args.Index >= len(a.al.saved) {
			refuse(errArgs)
			return true
		}
		i := *args.Index
		m := w.AlertManager
		switch args.Mode {
		case "", "select":
			m.OnSelect(i)
		case "use":
			m.OnUse(i)
		case "edit":
			m.OnEdit(i)
		case "delete":
			m.OnDelete(i)
		default:
			refuse(fmt.Errorf("unknown mode %q (select|use|edit|delete)", args.Mode))
			return true
		}
		ok()
	case "alert-editor":
		// Type into the open New / Edit Alert dialog ({alert}); {mode: "show"}
		// presses Show (OK is `accept`).
		e := w.AlertEditor
		if !e.Visible() {
			refuse(errors.New("the alert editor is not open"))
			return true
		}
		if args.Alert != nil {
			e.SetFields(*args.Alert)
		}
		if args.Mode == "show" {
			e.PressShow()
		}
		a.replies = append(a.replies, func() {
			cmd.RespondJSON(struct {
				Error string     `json:"error"`
				State core.State `json:"state"`
			}{e.Error(), a.state()})
		})
	case "alert-fill":
		// Type token values into the open Fill Alert Text Tokens dialog
		// (USE is `accept`). The reply carries its preview and tokens.
		f := w.AlertFill
		if !f.Visible() {
			refuse(errors.New("the Fill Alert Text Tokens dialog is not open"))
			return true
		}
		f.SetValues(args.Values)
		a.replies = append(a.replies, func() {
			cmd.RespondJSON(struct {
				Tokens  []string   `json:"tokens"`
				Preview string     `json:"preview"`
				Error   string     `json:"error"`
				State   core.State `json:"state"`
			}{f.Tokens(), f.Preview(), f.Error(), a.state()})
		})
	case "nursery-type":
		// Type a code into the open Nursery dialog and press Add.
		if !w.Nursery.Visible() {
			refuse(errors.New("the Nursery dialog is not open"))
			return true
		}
		w.Nursery.SetCode(args.Code)
		w.Nursery.PressAdd()
		ok()
	case "countdown-form":
		// Fill the open Countdown dialog (Start is `accept`).
		if !w.Countdown.Visible() {
			refuse(errors.New("the Countdown dialog is not open"))
			return true
		}
		f, err := countdownFormArgs(args)
		if err != nil {
			refuse(err)
			return true
		}
		w.Countdown.SetForm(f)
		a.replies = append(a.replies, func() {
			cmd.RespondJSON(struct {
				Error string     `json:"error"`
				State core.State `json:"state"`
			}{w.Countdown.Error(), a.state()})
		})
	default:
		return false
	}
	return true
}

// applyAlertStyles is set-setting {key: "alerts"}: a full or partial
// models.AlertStyles merged over the current one.
func (a *App) applyAlertStyles(key string, raw json.RawMessage) error {
	st := a.settings.Alerts
	if err := json.Unmarshal(raw, &st); err != nil {
		return fmt.Errorf("set-setting %s: %w", key, err)
	}
	a.setAlertStyles(st)
	return nil
}
