package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"runtime/debug"

	"freebeams/internal/app/core"
	"freebeams/internal/control"
	"freebeams/internal/models"
	"freebeams/internal/render"
	"freebeams/internal/ui"

	"graphics.gd/classdb/DisplayServer"
)

// Settings: the values the Options dialog edits (core.Settings) are
// authoritative state on the App. Events change them and persist each under
// its own key; render projects them onto the live monitor, the output, the
// logo, the interface theme and the dialog. The dialog itself is a projection
// too: settingsOpen says whether it is up, and a change made in it goes
// through the same setters as the control API and the menus.

// loadSettings reads the persisted settings at startup (synchronously, like
// the layout: they decide what the first frame looks like) and applies the
// logo to the presenter.
func (a *App) loadSettings() {
	a.logoDir = a.defaultLogoDir()
	a.settings = core.DefaultSettings(a.logoDir)
	if a.st() != nil {
		get := func(key string, dst any) {
			if _, err := a.st().GetSetting(a.ctx(), key, dst); err != nil {
				a.fail("read setting "+key, err)
			}
		}
		get(core.SettingTransition, &a.settings.Transition)
		get(core.SettingOutputScreen, &a.settings.OutputScreen)
		var logo models.Background
		get(core.SettingLogo, &logo)
		if logo.Kind != "" {
			a.settings.Logo = logo
		}
		get(core.SettingInterfaceColor, &a.settings.InterfaceColor)
		get(core.SettingAlertStyles, &a.settings.Alerts)
		get(core.SettingHotkeys, &a.settings.Hotkeys) // hotkeys.go
		get(core.SettingRecent, &a.recent)
	}
	a.settings = a.settings.Normalized(a.logoDir)
	a.pres.SetLogo(a.settings.Logo)
	a.keys.SetKeymap(a.settings.Hotkeys)
	log.Printf("[freebeams] settings: transition %s; output screen %d; logo %s; %s interface",
		a.settings.Transition, a.settings.OutputScreen, core.LogoDescription(a.settings.Logo), a.settings.InterfaceColor)
}

// saveSetting persists one setting off the main thread.
func (a *App) saveSetting(key string, v any) {
	st := a.st()
	if st == nil {
		return
	}
	a.async(nil, func(ctx context.Context) func() error {
		err := st.SetSetting(ctx, key, v)
		return func() error {
			a.fail("save setting "+key, err)
			return err
		}
	})
}

// outputScreen is the display the output belongs on right now: the configured
// one when it is connected, else the first non-primary display.
func (a *App) outputScreen() int {
	screens := render.Screens()
	primary := 0
	for _, s := range screens {
		if s.Primary {
			primary = s.Index
		}
	}
	return core.OutputScreenFor(a.settings.OutputScreen, len(screens), primary)
}

// ---- events ----------------------------------------------------------------------

// setTransition changes the live transition (kind and duration).
func (a *App) setTransition(t models.Transition) {
	t = t.Normalized()
	if a.settings.Transition == t {
		return
	}
	a.settings.Transition = t
	a.settingsVer++
	a.saveSetting(core.SettingTransition, t)
}

// setOutputScreen selects the output display (core.AutoScreen or an index of
// a connected display).
func (a *App) setOutputScreen(i int) error {
	if i < 0 {
		i = core.AutoScreen
	} else if n := len(render.Screens()); i >= n {
		return fmt.Errorf("output screen %d: only %d display(s) connected", i, n)
	}
	if a.settings.OutputScreen == i {
		return nil
	}
	if err := a.stageAllowsOutputScreen(i); err != nil { // never the stage display's screen (stage.go)
		return err
	}
	a.settings.OutputScreen = i
	a.settingsVer++
	a.saveSetting(core.SettingOutputScreen, i)
	return nil
}

// setLogo changes the logo screen's background.
func (a *App) setLogo(bg models.Background) {
	if bg.Kind == "" {
		bg = core.LogoBackground(a.logoDir)
	}
	if a.settings.Logo == bg {
		return
	}
	a.settings.Logo = bg
	a.settingsVer++
	a.pres.SetLogo(bg)
	a.saveSetting(core.SettingLogo, bg)
}

// setInterfaceColor switches between the dark and light interface.
func (a *App) setInterfaceColor(color string) error {
	c, err := core.ParseInterfaceColor(color)
	if err != nil {
		return err
	}
	if a.settings.InterfaceColor == c {
		return nil
	}
	a.settings.InterfaceColor = c
	a.settingsVer++
	a.saveSetting(core.SettingInterfaceColor, c)
	return nil
}

// openSettings shows the Options dialog on a section (ui.SettingsPage…),
// remembering the settings so Cancel can restore them. On an open dialog it
// only switches the section.
func (a *App) openSettings(page int) {
	a.settingsPage = page
	if a.settingsOpen {
		a.win.Settings.SetPage(page) // view-only state, like the resource tab
		return
	}
	a.settingsSnap = a.settings
	a.stage.snap, a.stage.warning = a.stage.settings, "" // stage.go
	a.rm.snapEnabled, a.rm.snapPort = a.rm.enabled, a.rm.port
	a.checkEWHotkeys() // hotkeys.go
	a.settingsOpen = true
	a.settingsVer++ // the displays may have changed since the last time
}

// closeSettings hides the dialog; cancel restores the settings from when it
// opened (changes applied live meanwhile are undone through the same setters).
func (a *App) closeSettings(cancel bool) {
	a.settingsOpen = false
	a.endHotkeyCapture() // hotkeys.go
	if !cancel {
		return
	}
	s := a.settingsSnap
	a.setTransition(s.Transition)
	if err := a.setOutputScreen(s.OutputScreen); err != nil {
		a.fail("restore output screen", err)
	}
	a.setLogo(s.Logo)
	if err := a.setInterfaceColor(s.InterfaceColor); err != nil {
		a.fail("restore interface color", err)
	}
	a.fail("restore stage display", a.setStageSettings(a.stage.snap)) // stage.go
	if a.rm.srv.Get() != nil && (a.rm.port != a.rm.snapPort || a.rm.enabled != a.rm.snapEnabled) {
		a.fail("restore remote port", a.setRemotePort(a.rm.snapPort))
		a.setRemoteEnabled(a.rm.snapEnabled)
	}
	a.setAlertStyles(s.Alerts)
	a.setHotkeys(s.Hotkeys)
}

// logoFilters are the native dialog filters for Choose Image….
var logoFilters = []string{"*.png, *.jpg, *.jpeg, *.webp, *.bmp ; Images"}

// chooseLogoDialog shows the native Open dialog for the logo image.
func (a *App) chooseLogoDialog() {
	err := DisplayServer.FileDialogShow("Choose Logo Image", a.dialogDir(), "", false, DisplayServer.FileDialogModeOpenFile,
		logoFilters, func(ok bool, paths []string, _ int) {
			if !ok || len(paths) == 0 {
				return
			}
			a.event(func() { a.setLogo(core.LogoFromPath(paths[0], a.logoDir)) })
		}, 0)
	if err != nil {
		a.alert("Logo Image", fmt.Errorf("Could not show the Open dialog.\n\n%w", err))
	}
}

// notify shows an informational message in the message dialog (About). Unlike
// alert it is not an error, so nothing is logged as one.
func (a *App) notify(title, message string) {
	if len(a.alerts) < maxAlerts {
		a.alerts = append(a.alerts, core.Alert{Title: title, Message: message})
	}
}

// aboutText describes the build (Help ▸ About).
func aboutText() string {
	version := "development build"
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		version = info.Main.Version
	}
	return "FreeBeams — " + version + "\n\nA free worship presentation app: songs, scriptures, media and a projector output, " +
		"built with Go and Godot (graphics.gd). Hotkeys and layout follow EasyWorship."
}

// onMenu runs a menu item (ui.Menus.OnAction and the `menu` control action).
func (a *App) onMenu(act ui.MenuAction) {
	switch act {
	case ui.MenuNewSchedule:
		a.confirmDiscard("New Schedule", a.newSchedule)
	case ui.MenuNewSong:
		a.newSong()
	case ui.MenuOpenSchedule:
		a.confirmDiscard("Open Schedule", a.openScheduleDialog)
	case ui.MenuSaveSchedule:
		a.saveScheduleInteractive()
	case ui.MenuSaveScheduleAs:
		a.saveScheduleAsDialog()
	case ui.MenuQuit:
		a.requestQuit()
	case ui.MenuOptions:
		a.openSettings(ui.SettingsPageOutput)
	case ui.MenuInterfaceDark:
		a.fail("interface color", a.setInterfaceColor(core.ColorDark))
	case ui.MenuInterfaceLight:
		a.fail("interface color", a.setInterfaceColor(core.ColorLight))
	case ui.MenuAbout:
		a.notify("About FreeBeams", aboutText())
	default:
		a.onLiveMenu(act) // the Live menu (stage.go)
	}
}

// openRecent opens a schedule from File ▸ Open Recent.
func (a *App) openRecent(path string) {
	a.confirmDiscard("Open Schedule", func() { a.openSchedule(path) })
}

// ---- view -----------------------------------------------------------------------

// wireSettings connects the Options dialog and the menus to their event
// handlers (called from buildUI).
func (a *App) wireSettings() {
	d := a.win.Settings
	a.logoView = render.NewSlideView()
	a.logoView.SetTransitionDuration(0)
	d.LogoPreview.SetContent(a.logoView.AsControl())
	d.OnTransition = func(t models.Transition) { a.event(func() { a.setTransition(t) }) }
	d.OnOutputScreen = func(i int) { a.event(func() { a.fail("output screen", a.setOutputScreen(i)) }) }
	d.OnChooseLogo = func() { a.event(a.chooseLogoDialog) }
	d.OnDefaultLogo = func() { a.event(func() { a.setLogo(core.LogoBackground(a.logoDir)) }) }
	d.OnImportCopyrighted = func() { a.event(func() { a.importCopyrightedBibles(nil, false) }) }
	d.OnOK = func() { a.event(func() { a.closeSettings(false) }) }
	d.OnCancel = func() { a.event(func() { a.closeSettings(true) }) }

	a.wireOptions() // Hotkeys and Themes (hotkeys.go)

	m := a.win.Menus
	m.OnAction = func(act ui.MenuAction) { a.event(func() { a.onMenu(act) }) }
	m.OnOpenRecent = func(path string) { a.event(func() { a.openRecent(path) }) }
}

// settingsValues is the dialog's projection of the settings.
func (a *App) settingsValues() ui.SettingsValues {
	v := ui.SettingsValues{
		Transition:   a.settings.Transition,
		OutputScreen: a.settings.OutputScreen,
		Logo:         core.LogoDescription(a.settings.Logo),
		LibraryPath:  "Library unavailable",
		Alerts:       a.settings.Alerts,
	}
	if a.st() != nil {
		v.LibraryPath = a.st().Path()
	}
	v.RemoteEnabled, v.RemotePort = a.rm.enabled, a.rm.port
	v.RemoteStatus, _ = a.remoteStatusText()
	return v
}

// screenRows lists the connected displays for the dialog's output picker.
func screenRows() []ui.ScreenRow {
	screens := render.Screens()
	rows := make([]ui.ScreenRow, 0, len(screens))
	for _, s := range screens {
		rows = append(rows, ui.ScreenRow{Index: s.Index, Label: core.ScreenLabel(s.Index, s.Size[0], s.Size[1], s.Primary)})
	}
	return rows
}

// renderSettings projects the settings onto the dialog, the menus and the
// interface theme (part of render; v is the view cache).
func (a *App) renderSettings(v *viewCache) {
	w := a.win
	if a.settingsVer != v.settingsVer {
		v.settingsVer = a.settingsVer
		w.Settings.SetValues(a.settingsValues())
		w.Settings.SetScreens(screenRows())
		w.Settings.SetImportCopyrightedAvailable(len(core.CopyrightedBiblesToImport(a.ewBibles, a.bibles)) > 0)
		w.Menus.SetInterfaceColor(a.settings.InterfaceColor == core.ColorDark)
	}
	a.renderOptions() // Hotkeys and Themes (hotkeys.go)
	a.logoView.SetFrame(models.Frame{Mode: models.ModeLogo, Logo: a.settings.Logo})
	if a.settings.InterfaceColor != v.color {
		v.color = a.settings.InterfaceColor
		if v.color == core.ColorLight {
			w.SetPalette(ui.LightPalette())
		} else {
			w.SetPalette(ui.DarkPalette())
		}
	}
	if len(a.recent) != len(v.recent) || !equalStrings(a.recent, v.recent) {
		v.recent = append(v.recent[:0:0], a.recent...)
		w.Menus.SetRecent(a.recent)
	}
	switch {
	case a.settingsOpen && !w.Settings.Visible():
		w.Settings.OpenAt(a.settingsPage)
	case !a.settingsOpen && w.Settings.Visible():
		w.Settings.Hide()
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// ---- control -----------------------------------------------------------------------

// handleSettingsCommand runs the settings actions; false when the action is
// not one of them.
func (a *App) handleSettingsCommand(cmd *control.Command, args actionArgs) bool {
	switch cmd.Action {
	case "settings":
		// GET-like: the reply's state carries the settings.
		a.answer(cmd, nil)
	case "transition":
		// {kind, duration}: either part may be left out to keep the current one.
		t := a.settings.Transition
		if args.Kind != "" {
			k, err := models.ParseTransitionKind(args.Kind)
			if err != nil {
				cmd.RespondErr(fmt.Errorf("transition: %w", err))
				return true
			}
			t.Kind = k
		}
		if args.Duration != nil {
			t.Duration = msDuration(*args.Duration)
		}
		a.setTransition(t)
		a.answer(cmd, nil)
	case "set-setting":
		a.answer(cmd, a.applySetting(args.Key, args.Value))
	case "options":
		// Open the Options dialog, optionally on a section ({page}), or with
		// {on: false} close it as OK does (`dismiss` is its Cancel).
		page := ui.SettingsPageOutput
		if args.Page != "" {
			p, ok := ui.ParseSettingsPage(args.Page)
			if !ok {
				cmd.RespondErr(fmt.Errorf("options: unknown page %q", args.Page))
				return true
			}
			page = p
		}
		if args.On == nil || *args.On {
			a.openSettings(page)
		} else {
			a.closeSettings(false)
		}
		a.answer(cmd, nil)
	case "menu":
		// Run a menu item by name (ui.MenuAction names), as a click would.
		act, ok := ui.ParseMenuAction(args.Item)
		if !ok {
			cmd.RespondErr(fmt.Errorf("menu: unknown item %q", args.Item))
			return true
		}
		a.onMenu(act)
		a.answer(cmd, nil)
	default:
		return a.handleOptionsCommand(cmd, args) // hotkeys.go
	}
	return true
}

// applySetting sets one setting from its JSON value (set-setting {key, value}).
// Keys are the stored keys or their short forms: transition, output.screen,
// output.logo (an image path, "" for the default, or a background object) and
// ui.interfaceColor.
func (a *App) applySetting(key string, raw json.RawMessage) error {
	if len(raw) == 0 {
		return errors.New("set-setting: value is required")
	}
	switch key {
	case core.SettingTransition, "transition":
		var t models.Transition
		if err := json.Unmarshal(raw, &t); err != nil {
			return fmt.Errorf("set-setting %s: %w", key, err)
		}
		a.setTransition(t)
	case core.SettingOutputScreen, "outputScreen":
		var i int
		if err := json.Unmarshal(raw, &i); err != nil {
			return fmt.Errorf("set-setting %s: %w", key, err)
		}
		return a.setOutputScreen(i)
	case core.SettingLogo, "logo":
		var path string
		if err := json.Unmarshal(raw, &path); err == nil {
			a.setLogo(core.LogoFromPath(path, a.logoDir))
			return nil
		}
		var bg models.Background
		if err := json.Unmarshal(raw, &bg); err != nil {
			return fmt.Errorf("set-setting %s: want an image path or a background object: %w", key, err)
		}
		a.setLogo(bg)
	case core.SettingRemoteEnabled, "remoteEnabled":
		var on bool
		if err := json.Unmarshal(raw, &on); err != nil {
			return fmt.Errorf("set-setting %s: %w", key, err)
		}
		a.setRemoteEnabled(on)
		return nil
	case core.SettingRemotePort, "remotePort":
		var p int
		if err := json.Unmarshal(raw, &p); err != nil {
			return fmt.Errorf("set-setting %s: %w", key, err)
		}
		return a.setRemotePort(p)
	case core.SettingInterfaceColor, "interfaceColor":
		var c string
		if err := json.Unmarshal(raw, &c); err != nil {
			return fmt.Errorf("set-setting %s: %w", key, err)
		}
		return a.setInterfaceColor(c)
	case core.SettingStage, "stage":
		return a.applyStageSetting(raw) // stage.go
	case core.SettingAlertStyles, "alerts":
		return a.applyAlertStyles(key, raw)
	default:
		return fmt.Errorf("set-setting: unknown key %q (transition, output.screen, output.logo, ui.interfaceColor, stage, remote.enabled, remote.port, alerts)", key)
	}
	return nil
}
