package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"freebeams/internal/app/core"
	"freebeams/internal/control"
	"freebeams/internal/models"
	"freebeams/internal/render"
	"freebeams/internal/ui"

	"graphics.gd/classdb/Control"
)

// The stage display (EasyWorship's Foldback output): a text-only confidence
// monitor for the musicians and the speaker in a window of its own
// (render.Stage), previewable in the Live Output monitor (the output
// selector). Its settings and the service timer are authoritative state here;
// core.BuildStageFrame projects them, with the presenter and the schedule,
// onto a models.StageFrame that both views draw.

// stageState is the App's stage display state.
type stageState struct {
	settings core.StageSettings
	snap     core.StageSettings // when the Options dialog opened (Cancel restores it)
	timer    core.StageTimer
	// monitor: the Live Output monitor previews the Foldback output.
	monitor bool
	// warning is the Options dialog's message about a refused display.
	warning string
	// note is the stage message (Live ▸ Stage Message…), normalized.
	note string

	// ---- view
	win       *render.Stage
	view      *render.StageView // the Live Output monitor's Foldback view
	shownVer  int               // settingsVer last projected onto the dialog's section
	shownWarn string
}

// liveMonitorContent is the Live Output monitor's content: the live SlideView
// with the Foldback StageView over it, shown when the operator selects the
// Foldback output. The live view stays visible underneath, so a live video
// keeps its player (render.RoleLive) while the monitor shows the stage.
func (a *App) liveMonitorContent() Control.Instance {
	holder := Control.New()
	holder.SetMouseFilter(Control.MouseFilterIgnore)
	holder.SetClipContents(true)
	lv := a.liveView.AsControl()
	lv.SetAnchorsAndOffsetsPreset(Control.PresetFullRect)
	holder.AsNode().AddChild(lv.AsNode())
	a.stage.view = render.NewStageView()
	sv := a.stage.view.AsControl()
	sv.SetAnchorsAndOffsetsPreset(Control.PresetFullRect)
	sv.AsCanvasItem().SetVisible(false)
	holder.AsNode().AddChild(sv.AsNode())
	return holder
}

// wireStage creates the stage window manager and connects the stage's UI
// (called from buildUI, after liveMonitorContent).
func (a *App) wireStage() {
	a.stage.settings = core.DefaultStageSettings()
	a.stage.win = render.NewStage(a.AsNode())
	a.stage.win.OnClosed = func() {
		log.Printf("[freebeams] stage display window closed")
		a.event(func() { a.fail("stage display", a.setStageEnabled(false)) })
	}
	a.stage.win.OnInput = a.outputInput // hotkeys while the windowed stage has focus, as on the output
	a.win.OnMonitorOutput = func(foldback bool) { a.event(func() { a.stage.monitor = foldback }) }
	a.win.Settings.OnStage = func(v ui.StageValues) { a.event(func() { a.applyStageValues(v) }) }
	a.stage.shownVer = -1
	a.win.StageMessage.OnShow = func(text string) { a.event(func() { a.setStageNote(text) }) }
	a.win.StageMessage.OnClear = func() { a.event(func() { a.setStageNote("") }) }
}

// setStageNote puts a message up on the stage display ("" takes it down).
func (a *App) setStageNote(text string) {
	text = core.NormalizeStageNote(text)
	if text == a.stage.note {
		return
	}
	a.stage.note = text
	if text == "" {
		log.Printf("[freebeams] stage message cleared")
	} else {
		log.Printf("[freebeams] stage message: %q", text)
	}
}

// openStageMessage opens Live ▸ Stage Message… with text in its box.
func (a *App) openStageMessage(text string) {
	if a.win.DialogOpen() {
		return // one dialog at a time from the menu (as Options… does)
	}
	a.win.StageMessage.Open(text, a.stage.note != "")
}

// loadStageSettings reads the persisted stage settings (startup).
func (a *App) loadStageSettings() {
	st := core.DefaultStageSettings()
	if a.st != nil {
		if _, err := a.st.GetSetting(a.ctx(), core.SettingStage, &st); err != nil {
			a.fail("read setting "+core.SettingStage, err)
		}
	}
	a.stage.settings = st.Normalized()
	s := a.stage.settings
	log.Printf("[freebeams] stage display: %s; screen %d; next %v, clock %v (24h %v), timer %v, text %.0f%%",
		map[bool]string{true: "on", false: "off"}[s.Enabled], s.Screen, s.ShowNext, s.ShowClock, s.Clock24h, s.ShowTimer, s.FontScale*100)
}

// ---- events -------------------------------------------------------------------------

// screenLayout returns the number of displays and the primary's index.
func screenLayout() (n, primary int) {
	screens := render.Screens()
	for _, s := range screens {
		if s.Primary {
			primary = s.Index
		}
	}
	return len(screens), primary
}

// stageScreen is the display the stage belongs on right now, and whether its
// configured display had to be given up because the output covers it.
func (a *App) stageScreen() (int, bool) {
	n, primary := screenLayout()
	return core.StageScreenFor(a.stage.settings.Screen, n, primary, a.outputScreen())
}

// setStageSettings changes the stage settings. A display the output covers
// (or one that is not connected) is refused: the dialog shows why and keeps
// the previous choice.
func (a *App) setStageSettings(s core.StageSettings) error {
	s = s.Normalized()
	n, primary := screenLayout()
	var err error
	switch {
	case s.Screen >= n:
		err = fmt.Errorf("stage display: display %d is not connected (%d display(s))", s.Screen+1, n)
	case core.StageConflict(s.Screen, a.outputScreen(), primary):
		err = errors.New(core.StageConflictMessage(s.Screen))
	}
	if err != nil {
		a.stage.warning = err.Error()
		a.settingsVer++ // re-project the dialog: the refused choice snaps back
		return err
	}
	a.stage.warning = ""
	if s == a.stage.settings {
		return nil
	}
	a.stage.settings = s
	a.settingsVer++
	a.saveSetting(core.SettingStage, s)
	return nil
}

// setStageEnabled shows or hides the stage display (Live menu, `stage {on}`,
// closing its window).
func (a *App) setStageEnabled(on bool) error {
	s := a.stage.settings
	s.Enabled = on
	return a.setStageSettings(s)
}

// applyStageValues takes a change from the Options dialog's Stage Display
// section.
func (a *App) applyStageValues(v ui.StageValues) {
	s := core.StageSettings{Enabled: v.Enabled, Screen: v.Screen, ShowNext: v.ShowNext, ShowClock: v.ShowClock,
		Clock24h: v.Clock24h, ShowTimer: v.ShowTimer, FontScale: v.FontScale}
	if err := a.setStageSettings(s); err != nil {
		a.fail("stage display", err)
	}
}

// stageAllowsOutputScreen refuses an output display that the enabled stage
// display covers (setOutputScreen).
func (a *App) stageAllowsOutputScreen(setting int) error {
	if !a.stage.settings.Enabled {
		return nil
	}
	n, primary := screenLayout()
	out := core.OutputScreenFor(setting, n, primary)
	if core.StageConflict(a.stage.settings.Screen, out, primary) {
		return errors.New(core.StageConflictMessage(out))
	}
	return nil
}

// onLiveMenu runs the Live menu's items (onMenu passes the ones it does not
// know).
func (a *App) onLiveMenu(act ui.MenuAction) {
	switch act {
	case ui.MenuGoLive:
		a.goLive()
	case ui.MenuGoBack:
		a.goBack()
	case ui.MenuLogo:
		a.pres.ToggleLogo()
	case ui.MenuBlack:
		a.pres.ToggleBlack()
	case ui.MenuClear:
		a.pres.ToggleClear()
	case ui.MenuShowLive:
		a.setOutput(!a.pres.State.OutputEnabled)
	case ui.MenuStageDisplay:
		if err := a.setStageEnabled(!a.stage.settings.Enabled); err != nil {
			a.alert("Stage Display", err)
		}
	case ui.MenuStageMessage:
		a.openStageMessage(a.stage.note)
	}
}

// ---- view ---------------------------------------------------------------------------

// stageFrame projects the state onto the stage display at now.
func (a *App) stageFrame(now time.Time) models.StageFrame {
	return core.BuildStageFrame(core.StageInput{Presenter: a.pres.State, Schedule: a.sched, Settings: a.stage.settings,
		Timer: a.stage.timer, Now: now, Note: a.stage.note})
}

// stageValues is the dialog's projection of the stage settings.
func (a *App) stageValues() ui.StageValues {
	s := a.stage.settings
	return ui.StageValues{Enabled: s.Enabled, Screen: s.Screen, ShowNext: s.ShowNext, ShowClock: s.ShowClock,
		Clock24h: s.Clock24h, ShowTimer: s.ShowTimer, FontScale: s.FontScale, Warning: a.stage.warning}
}

// renderStage projects the stage state (part of render): the window on its
// display, the frame on the window and the Live Output monitor, the output
// selector, the Live menu and the dialog's section.
func (a *App) renderStage() {
	s := a.stage.settings
	screen, _ := a.stageScreen()
	win := a.stage.win
	switch {
	case s.Enabled && !win.IsOpen():
		win.Open(screen)
	case !s.Enabled && win.IsOpen():
		win.Close()
	case s.Enabled && win.Screen() != screen:
		win.Open(screen) // the setting or the displays changed: move it
	}
	a.tickStage()
	if sv := a.stage.view.AsCanvasItem(); sv.Visible() != a.stage.monitor {
		sv.SetVisible(a.stage.monitor)
	}
	a.win.SetMonitorOutput(a.stage.monitor)
	st := a.pres.State
	a.win.Menus.SetLiveState(ui.LiveMenuState{Logo: st.Mode == models.ModeLogo, Black: st.Mode == models.ModeBlack,
		Clear: st.Mode == models.ModeClear, Live: st.OutputEnabled, Stage: s.Enabled})
	if a.settingsVer != a.stage.shownVer || a.stage.warning != a.stage.shownWarn {
		a.stage.shownVer, a.stage.shownWarn = a.settingsVer, a.stage.warning
		a.win.Settings.SetStage(a.stageValues())
	}
}

// tickStage refreshes the frame of whatever shows the stage (the clock and
// the timer change on their own). It runs every frame from Process: a frame
// equal to the shown one costs a comparison.
func (a *App) tickStage() {
	if !a.stage.win.IsOpen() && !a.stage.monitor {
		return
	}
	f := a.stageFrame(time.Now())
	a.stage.win.SetFrame(f)
	if a.stage.monitor {
		a.stage.view.SetFrame(f)
	}
}

// stageStateDoc is /state.stage.
func (a *App) stageStateDoc() core.StageState {
	now := time.Now()
	_, conflict := a.stageScreen()
	w := a.stage.win
	return core.StageStateDoc(a.stageFrame(now), a.stage.settings, a.stage.timer, now, core.StageWindow{
		Open: w.IsOpen(), Fullscreen: w.Fullscreen(), Focused: w.Focused(), Conflict: conflict && a.stage.settings.Enabled,
		Monitor: a.stage.monitor, Screen: w.Screen(),
	})
}

// closeStage closes the stage window at shutdown.
func (a *App) closeStage() {
	if a.stage.win == nil {
		return
	}
	a.stage.win.OnClosed = nil
	a.stage.win.OnInput = nil
	a.stage.win.Close()
}

// ---- control ------------------------------------------------------------------------

// handleStageCommand runs the stage display's actions; false when the action
// is not one of them.
//
//	stage {on?, screen?}             show / hide (toggle without on), choose the display (-1 automatic)
//	stage-timer {action}             start | stop | reset | toggle the service timer
//	stage-monitor {on?}              the Live Output monitor previews the Foldback output (toggle without on)
//	stage-native                     what the OS says about the stage window (macOS)
//	stage-note {text}                put a stage message up ("" takes it down)
//	stage-note {mode: "open", text?} open Live ▸ Stage Message… (text, else the current message, in the box)
//	stage-note {mode: "clear"}       the dialog's Clear when it is open, else take the message down
func (a *App) handleStageCommand(cmd *control.Command, args actionArgs) bool {
	switch cmd.Action {
	case "stage":
		s := a.stage.settings
		switch {
		case args.On != nil:
			s.Enabled = *args.On
		case args.Screen == nil:
			s.Enabled = !s.Enabled
		}
		if args.Screen != nil {
			s.Screen = *args.Screen
		}
		a.answer(cmd, a.setStageSettings(s))
	case "stage-timer":
		a.answer(cmd, a.stage.timer.Apply(args.Action, time.Now()))
	case "stage-monitor":
		on := !a.stage.monitor
		if args.On != nil {
			on = *args.On
		}
		a.stage.monitor = on
		a.answer(cmd, nil)
	case "stage-note":
		switch args.Mode {
		case "":
			a.setStageNote(args.Text)
		case "open":
			if a.win.DialogOpen() {
				cmd.RespondErr(errors.New("stage-note: another dialog is open"))
				return true
			}
			text := args.Text
			if text == "" {
				text = a.stage.note
			}
			a.openStageMessage(text)
		case "clear":
			if !a.win.StageMessage.Clear() {
				a.setStageNote("")
			}
		default:
			cmd.RespondErr(fmt.Errorf("stage-note: unknown mode %q (open|clear, or none with text)", args.Mode))
			return true
		}
		a.answer(cmd, nil)
	case "stage-native":
		st, ok := a.stage.win.NativeState()
		if !ok {
			cmd.RespondErr(errors.New("stage-native: the stage display is closed or not probeable here"))
			return true
		}
		a.replies = append(a.replies, func() {
			cmd.RespondJSON(struct {
				render.NativeWindowState
				State core.State `json:"state"`
			}{st, a.state()})
		})
	default:
		return false
	}
	return true
}

// applyStageSetting is set-setting for the stage key: a JSON object whose
// fields replace the current ones (the others are kept).
func (a *App) applyStageSetting(raw json.RawMessage) error {
	s := a.stage.settings
	if err := json.Unmarshal(raw, &s); err != nil {
		return fmt.Errorf("set-setting %s: want an object like {\"enabled\":true,\"clock24h\":true}: %w", core.SettingStage, err)
	}
	return a.setStageSettings(s)
}
