package core

import (
	"time"

	"freebeams/internal/models"
)

// Live alerts: the message alert queue, the nursery badge, the countdown and
// the clock overlay. Alerts is authoritative app state; the app ticks it once
// per frame with the current time (injected, so the timeline is testable) and
// projects Frame onto the overlay layer of the Live monitor and the output.

// SettingAlertStyles is the setting that holds models.AlertStyles.
const SettingAlertStyles = "settings.alerts"

// AlertRun is a message alert on the queue: a saved alert or a one-off
// message, with its operator tokens filled.
type AlertRun struct {
	Run     int   // this showing's id (1, 2, …)
	AlertID int64 // the saved alert (0: a one-off message)
	Name    string
	Text    string // {countdown} / {clock} are resolved while it shows
	Display models.AlertDisplay
	Align   models.HAlign
	End     models.AlertEnd
	Count   int
	Seconds int
	// Location overrides Options ▸ Alerts ▸ Screen Location: "top",
	// "bottom" or "" (the setting).
	Location string

	started time.Time
	running bool
}

// RunFromAlert makes a run of a saved alert whose tokens were filled into text.
func RunFromAlert(a models.Alert, text string) AlertRun {
	a = a.Normalized()
	return AlertRun{AlertID: a.ID, Name: a.Name, Text: text, Display: a.Display, Align: a.Align,
		End: a.End, Count: a.Count, Seconds: a.Seconds}
}

// CountdownRequest starts a countdown: to Target, or for Seconds from now.
type CountdownRequest struct {
	Target   time.Time
	Seconds  float64
	Caption  string
	Position models.AlertPosition // "": Options ▸ Alerts
	End      models.CountdownEnd  // "": Options ▸ Alerts
}

type countdownRun struct {
	active  bool
	target  time.Time
	caption string
	pos     models.AlertPosition
	end     models.CountdownEnd
}

// textCache keeps one formatted string per key, so the per-frame projection
// allocates only when the text really changes (once a second for a clock).
type textCache struct {
	key  int64
	fmt  string
	text string
	set  bool
}

type widthCache struct {
	text  string
	style models.AlertBoxStyle
	width float64
	set   bool
}

type dynCache struct {
	src, cd, clk, out string
	set               bool
}

// Alerts is the live alert state.
type Alerts struct {
	// Measure returns a message's width in canvas units in a style (the
	// renderer's font metrics). Nil uses a rough estimate.
	Measure func(text string, st models.AlertBoxStyle) float64

	runs    []AlertRun
	nextRun int
	nursery models.NurseryQueue
	cd      countdownRun
	clockOn bool
	ver     int

	width   widthCache
	cdText  textCache
	clkText textCache
	dyn     dynCache
}

// Version changes whenever the alerts change other than by the passing of
// time (the toolbar state, the dialogs, /state).
func (a *Alerts) Version() int { return a.ver }

func (a *Alerts) changed() { a.ver++ }

// ---- message alerts ------------------------------------------------------------

// Show queues a message alert; it starts now when nothing else is showing.
// It returns the run id.
func (a *Alerts) Show(r AlertRun, now time.Time) int {
	a.nextRun++
	r.Run = a.nextRun
	r.running = false
	if r.Count < 1 {
		r.Count = 1
	}
	a.runs = append(a.runs, r)
	if len(a.runs) == 1 {
		a.runs[0].start(now)
	}
	a.changed()
	return r.Run
}

func (r *AlertRun) start(now time.Time) { r.started, r.running = now, true }

// Runs returns a copy of the queue (the first one is on screen).
func (a *Alerts) Runs() []AlertRun { return append([]AlertRun(nil), a.runs...) }

// ClearMessages removes every message alert.
func (a *Alerts) ClearMessages() bool {
	if len(a.runs) == 0 {
		return false
	}
	a.runs = nil
	a.changed()
	return true
}

// ClearMessage removes one run (the next one starts when it was on screen).
func (a *Alerts) ClearMessage(run int, now time.Time) bool {
	for i, r := range a.runs {
		if r.Run == run {
			a.runs = append(a.runs[:i], a.runs[i+1:]...)
			if i == 0 && len(a.runs) > 0 {
				a.runs[0].start(now)
			}
			a.changed()
			return true
		}
	}
	return false
}

// Next ends the message on screen and starts the next one (EasyWorship's
// Next button).
func (a *Alerts) Next(now time.Time) bool {
	if len(a.runs) == 0 {
		return false
	}
	return a.ClearMessage(a.runs[0].Run, now)
}

// ---- nursery ---------------------------------------------------------------------------

// AddNursery shows a code on the nursery badge.
func (a *Alerts) AddNursery(code string, now time.Time) bool {
	if !a.nursery.Add(code, now) {
		return false
	}
	a.changed()
	return true
}

// RemoveNursery takes a code off the badge.
func (a *Alerts) RemoveNursery(code string) bool {
	if !a.nursery.Remove(code) {
		return false
	}
	a.changed()
	return true
}

// ClearNursery removes every code.
func (a *Alerts) ClearNursery() bool {
	if !a.nursery.Clear() {
		return false
	}
	a.changed()
	return true
}

// NurseryCodes lists the codes on the badge, oldest first.
func (a *Alerts) NurseryCodes() []string { return a.nursery.Codes() }

// ---- countdown and clock ----------------------------------------------------------------

// StartCountdown starts (or replaces) the countdown.
func (a *Alerts) StartCountdown(req CountdownRequest, now time.Time) {
	target := req.Target
	if target.IsZero() {
		target = now.Add(time.Duration(req.Seconds * float64(time.Second)))
	}
	a.cd = countdownRun{active: true, target: target, caption: req.Caption, pos: req.Position, end: req.End}
	a.changed()
}

// StopCountdown removes the countdown.
func (a *Alerts) StopCountdown() bool {
	if !a.cd.active {
		return false
	}
	a.cd = countdownRun{}
	a.changed()
	return true
}

// CountdownActive reports whether a countdown is shown.
func (a *Alerts) CountdownActive() bool { return a.cd.active }

// SetClock shows or hides the clock overlay.
func (a *Alerts) SetClock(on bool) bool {
	if a.clockOn == on {
		return false
	}
	a.clockOn = on
	a.changed()
	return true
}

// ClockOn reports whether the clock overlay is on.
func (a *Alerts) ClockOn() bool { return a.clockOn }

// ClearAll removes every alert, the nursery codes, the countdown and the clock.
func (a *Alerts) ClearAll() bool {
	c := a.ClearMessages()
	c = a.ClearNursery() || c
	c = a.StopCountdown() || c
	c = a.SetClock(false) || c
	return c
}

// Active reports whether anything is shown (the Alerts button's state).
func (a *Alerts) Active() bool {
	return len(a.runs) > 0 || a.nursery.Len() > 0 || a.cd.active || a.clockOn
}

func (a *Alerts) countdownEnd(st models.AlertStyles) models.CountdownEnd {
	if a.cd.end != "" {
		return a.cd.end
	}
	return st.CountdownEnd
}

func (a *Alerts) countdownPos(st models.AlertStyles) models.AlertPosition {
	if a.cd.pos != "" {
		return a.cd.pos
	}
	return st.CountdownPosition
}

// countdownRemaining is the time left (negative after the end).
func (a *Alerts) countdownRemaining(now time.Time) time.Duration { return a.cd.target.Sub(now) }

// countdownText formats the countdown, cached per whole second.
func (a *Alerts) countdownText(now time.Time, st models.AlertStyles) string {
	if !a.cd.active {
		return models.FormatCountdown(0, st.CountdownFormat)
	}
	rem := a.countdownRemaining(now)
	if rem < 0 && a.countdownEnd(st) != models.CountdownOverrun {
		rem = 0
	}
	// The second the text shows: positive remainders round up.
	key := int64(rem / time.Second)
	if rem > 0 && rem%time.Second != 0 {
		key++
	}
	c := &a.cdText
	if !c.set || c.key != key || c.fmt != st.CountdownFormat {
		*c = textCache{key: key, fmt: st.CountdownFormat, text: models.FormatCountdown(rem, st.CountdownFormat), set: true}
	}
	return c.text
}

// clockText formats the time of day, cached per second.
func (a *Alerts) clockText(now time.Time, st models.AlertStyles) string {
	c := &a.clkText
	key := now.Unix()
	if !c.set || c.key != key || c.fmt != st.ClockFormat {
		*c = textCache{key: key, fmt: st.ClockFormat, text: models.FormatClock(now, st.ClockFormat), set: true}
	}
	return c.text
}

// ---- the timeline -----------------------------------------------------------------------

// displayText is what the message shows now: its dynamic tokens resolved
// (cached until a value changes).
func (a *Alerts) displayText(r *AlertRun, now time.Time, st models.AlertStyles) string {
	if !models.HasDynamicTokens(r.Text) {
		return r.Text
	}
	cd, clk := a.countdownText(now, st), a.clockText(now, st)
	d := &a.dyn
	if !d.set || d.src != r.Text || d.cd != cd || d.clk != clk {
		*d = dynCache{src: r.Text, cd: cd, clk: clk, out: models.ResolveDynamicTokens(r.Text, cd, clk), set: true}
	}
	return d.out
}

// measure is a message's width in canvas units (cached for the last text).
func (a *Alerts) measure(text string, st models.AlertBoxStyle) float64 {
	w := &a.width
	if w.set && w.text == text && w.style == st {
		return w.width
	}
	var width float64
	if a.Measure != nil {
		width = a.Measure(text, st)
	} else {
		width = float64(len([]rune(text))) * float64(st.Size) * 0.55
	}
	*w = widthCache{text: text, style: st, width: width, set: true}
	return width
}

// scrolls reports whether a run is drawn as a ticker.
func scrolls(r *AlertRun, width float64, st models.AlertStyles) bool {
	switch r.Display {
	case models.AlertScroll:
		return true
	case models.AlertAuto:
		return !models.BannerFits(width, st.Message.Size)
	}
	return false
}

// runTiming is a running message's position on its timeline.
type runTiming struct {
	text      string
	scroll    bool
	x         float64
	pass      int           // 0-based
	elapsed   time.Duration // since it started
	remaining time.Duration // -1: until removed
	done      bool
}

func (a *Alerts) timing(r *AlertRun, now time.Time, st models.AlertStyles) runTiming {
	t := runTiming{text: a.displayText(r, now, st), elapsed: now.Sub(r.started), remaining: -1}
	if t.elapsed < 0 {
		t.elapsed = 0
	}
	width := a.measure(t.text, st.Message)
	t.scroll = scrolls(r, width, st)
	var total time.Duration // the time a Display Count takes
	if t.scroll {
		m := models.Marquee{Width: models.CanvasWidth, TextWidth: width, Speed: float64(st.ScrollSpeed)}
		t.x, t.pass = m.At(t.elapsed)
		total = time.Duration(r.Count) * m.PassDuration()
	} else {
		total = time.Duration(r.Count) * time.Duration(st.StaticSeconds) * time.Second
		t.pass = int(t.elapsed / (time.Duration(max(st.StaticSeconds, 1)) * time.Second))
	}
	switch r.End {
	case models.AlertEndCount:
		t.remaining = max(total-t.elapsed, 0)
		t.done = t.elapsed >= total
	case models.AlertEndDuration:
		d := time.Duration(r.Seconds) * time.Second
		t.remaining = max(d-t.elapsed, 0)
		t.done = t.elapsed >= d
	case models.AlertEndToken:
		if a.cd.active {
			t.remaining = max(a.countdownRemaining(now), 0)
		} else {
			t.remaining = 0
		}
		t.done = t.remaining <= 0
	}
	return t
}

// Tick advances the alerts to now: a finished message gives way to the next
// one, nursery codes expire (auto-remove) and a countdown set to hide ends.
// It reports whether anything changed.
func (a *Alerts) Tick(now time.Time, st models.AlertStyles) bool {
	changed := false
	for len(a.runs) > 0 {
		r := &a.runs[0]
		if !r.running {
			r.start(now)
		}
		if !a.timing(r, now, st).done {
			break
		}
		a.runs = a.runs[1:]
		if len(a.runs) > 0 {
			a.runs[0].start(now)
		}
		changed = true
	}
	if a.nursery.Expire(now, time.Duration(st.NurseryAutoRemove)*time.Second) {
		changed = true
	}
	if a.cd.active && a.countdownEnd(st) == models.CountdownHide && a.countdownRemaining(now) <= 0 {
		a.cd = countdownRun{}
		changed = true
	}
	if changed {
		a.changed()
	}
	return changed
}

// Hidden reports whether the overlays are hidden by the output's mode
// (Options ▸ Alerts ▸ hide on Black).
func Hidden(st models.AlertStyles, mode models.OutputMode) bool {
	return st.HideOnBlack && mode == models.ModeBlack
}

// Frame is what the overlay layer shows at now. hidden blanks everything
// (Hidden). Call Tick first.
func (a *Alerts) Frame(now time.Time, st models.AlertStyles, hidden bool) models.OverlayFrame {
	var f models.OverlayFrame
	if hidden {
		return f
	}
	if len(a.runs) > 0 {
		r := &a.runs[0]
		t := a.timing(r, now, st)
		top := st.MessageTop
		switch r.Location {
		case "top":
			top = true
		case "bottom":
			top = false
		}
		f.Message = models.MessageOverlay{Visible: true, Text: t.text, Scroll: t.scroll, X: t.x, Align: r.Align, Top: top}
	}
	if a.nursery.Len() > 0 {
		f.Nursery = models.BoxOverlay{Visible: true, Text: a.nursery.Text(), Position: st.NurseryPosition}
	}
	if a.cd.active {
		f.Countdown = models.BoxOverlay{Visible: true, Text: a.countdownText(now, st), Caption: a.cd.caption, Position: a.countdownPos(st)}
	}
	if a.clockOn {
		f.Clock = models.BoxOverlay{Visible: true, Text: a.clockText(now, st), Position: st.ClockPosition}
	}
	return f
}

// Moving reports whether a ticker is scrolling (the renderer keeps
// low-processor mode off meanwhile).
func (a *Alerts) Moving(now time.Time, st models.AlertStyles, hidden bool) bool {
	if hidden || len(a.runs) == 0 {
		return false
	}
	return a.timing(&a.runs[0], now, st).scroll
}

// ---- /state ---------------------------------------------------------------------------------

// AlertRunState is one queued message alert in /state.alerts.active.
type AlertRunState struct {
	Run             int     `json:"run"`
	ID              int64   `json:"id"`
	Name            string  `json:"name"`
	Text            string  `json:"text"`    // as shown now (dynamic tokens resolved)
	Style           string  `json:"style"`   // ticker | static (what it is drawn as)
	Display         string  `json:"display"` // the alert's display method: static | scroll | auto
	Location        string  `json:"location"`
	End             string  `json:"end"`
	Passes          int     `json:"passes"`          // the Display Count
	Pass            int     `json:"pass"`            // 1-based pass (or static period) on screen
	RemainingPasses int     `json:"remainingPasses"` // passes left including this one (count alerts)
	Elapsed         float64 `json:"elapsed"`         // seconds
	Remaining       float64 `json:"remaining"`       // seconds; -1 until removed
	X               float64 `json:"x"`               // a ticker's text left edge (canvas units)
	Showing         bool    `json:"showing"`         // on screen (the first of the queue)
}

// CountdownState is /state.alerts.countdown.
type CountdownState struct {
	Active    bool    `json:"active"`
	Text      string  `json:"text"`
	Caption   string  `json:"caption"`
	Remaining float64 `json:"remaining"` // seconds (negative while overrunning)
	Position  string  `json:"position"`
	End       string  `json:"end"`
}

// ClockState is /state.alerts.clock.
type ClockState struct {
	On       bool   `json:"on"`
	Text     string `json:"text"`
	Position string `json:"position"`
}

// AlertsState is /state.alerts.
type AlertsState struct {
	Active    []AlertRunState `json:"active"`
	Message   string          `json:"message"` // the message on screen ("" when none)
	Nursery   []string        `json:"nursery"`
	Countdown CountdownState  `json:"countdown"`
	Clock     ClockState      `json:"clock"`
	Saved     []models.Alert  `json:"saved"`
	Showing   bool            `json:"showing"` // anything is on the overlay layer
	Moving    bool            `json:"moving"`  // a ticker scrolls (low-processor mode off)
	Hidden    bool            `json:"hidden"`  // hidden by Black (Options ▸ Alerts)
	Version   int             `json:"version"`
}

// State reports the alerts at now; saved are the saved alerts.
func (a *Alerts) State(now time.Time, st models.AlertStyles, hidden bool, saved []models.Alert) AlertsState {
	s := AlertsState{Active: []AlertRunState{}, Nursery: a.nursery.Codes(), Saved: saved, Hidden: hidden, Version: a.ver}
	if s.Saved == nil {
		s.Saved = []models.Alert{}
	}
	for i := range a.runs {
		r := &a.runs[i]
		rs := AlertRunState{Run: r.Run, ID: r.AlertID, Name: r.Name, Display: string(r.Display), End: string(r.End),
			Passes: r.Count, Location: r.Location, Remaining: -1}
		if rs.Location == "" {
			rs.Location = map[bool]string{true: "top", false: "bottom"}[st.MessageTop]
		}
		if i == 0 && r.running {
			t := a.timing(r, now, st)
			rs.Text, rs.Showing = t.text, true
			rs.Style = map[bool]string{true: "ticker", false: "static"}[t.scroll]
			rs.Pass, rs.Elapsed, rs.X = t.pass+1, t.elapsed.Seconds(), t.x
			if t.remaining >= 0 {
				rs.Remaining = t.remaining.Seconds()
			}
			if r.End == models.AlertEndCount {
				rs.RemainingPasses = max(r.Count-t.pass, 0)
			}
			s.Message = t.text
		} else {
			rs.Text = r.Text
			if r.End == models.AlertEndCount {
				rs.RemainingPasses = r.Count
			}
		}
		s.Active = append(s.Active, rs)
	}
	if a.cd.active {
		rem := a.countdownRemaining(now)
		s.Countdown = CountdownState{Active: true, Text: a.countdownText(now, st), Caption: a.cd.caption,
			Remaining: rem.Seconds(), Position: string(a.countdownPos(st)), End: string(a.countdownEnd(st))}
	}
	s.Clock = ClockState{On: a.clockOn, Position: string(st.ClockPosition)}
	if a.clockOn {
		s.Clock.Text = a.clockText(now, st)
	}
	f := a.Frame(now, st, hidden)
	s.Showing = f.Any()
	s.Moving = a.Moving(now, st, hidden)
	return s
}
