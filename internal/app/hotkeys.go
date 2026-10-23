package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"

	"freebeams/internal/app/core"
	"freebeams/internal/control"
	"freebeams/internal/models"
	"freebeams/internal/store"
	"freebeams/internal/ui"

	"graphics.gd/classdb/Input"
	"graphics.gd/classdb/InputEvent"
)

// Options ▸ Hotkeys and Options ▸ Themes. The keymap is a setting
// (core.Settings.Hotkeys, persisted under core.SettingHotkeys) applied live
// to a.keys. "Add Key…" starts a capture: the dialog routes the next key
// press to hotkeyCaptured, which adds it, refuses a reserved key, or asks
// whether to take a key another action has. The default themes per item
// type are the Themes tab's "Set as Default" state (themes.go).

// optionsState is the Options dialog's editor state (main thread only).
type optionsState struct {
	capture string // the hotkey action capturing a key ("" when none)
	status  string // the line under the hotkey table
	refused bool   // status is a refusal
	ver     int    // bumped when capture or status change

	ewHotkeys bool // EasyWorship's HotKeys.2.ini exists (Import from EasyWorship)

	// Render caches: what the dialog shows (render writes only these).
	shownVer, shownSettings, shownThemes int
	shownInit                            bool

	// progressWaiters are wait-transition commands (serveProgressWaiters).
	progressWaiters []progressWaiter
}

// progressWaiter answers a wait-transition command once the transition has
// reached at least progress, or fails it after a number of frames.
type progressWaiter struct {
	cmd      *control.Command
	progress float64
	frames   int
}

// maxProgressWaitFrames bounds a wait-transition (about 10 s at 60 fps).
const maxProgressWaitFrames = 600

func init() {
	// F1–F12 are bindable (core.KnownKey).
	for i := 0; i < 12; i++ {
		namedKeys[Input.KeyF1+Input.Key(i)] = fmt.Sprintf("f%d", i+1)
	}
}

// loadHotkeys reads the stored keymap (loadSettings): core.SettingHotkeys,
// or, when only an older build has saved one, core.SettingHotkeysLegacy
// migrated (there "ctrl" meant Ctrl or ⌘; on a Mac it becomes both). The
// migrated keymap is written under the new key on its first change. nil
// means the defaults.
func (a *App) loadHotkeys() core.Keymap {
	var km core.Keymap
	found, err := a.st().GetSetting(a.ctx(), core.SettingHotkeys, &km)
	if err != nil {
		a.fail("read setting "+core.SettingHotkeys, err)
	}
	if found {
		return km
	}
	var legacy core.Keymap
	found, err = a.st().GetSetting(a.ctx(), core.SettingHotkeysLegacy, &legacy)
	if err != nil {
		a.fail("read setting "+core.SettingHotkeysLegacy, err)
	}
	if !found {
		return nil
	}
	km = core.MigrateKeymap(legacy)
	log.Printf("[freebeams] hotkeys: migrated %s (⌘ and Control told apart: %v)", core.SettingHotkeysLegacy, core.MacModifiers())
	return km
}

// ---- events ----------------------------------------------------------------------

// setHotkeys replaces the keymap: applied to the hotkeys at once, persisted.
func (a *App) setHotkeys(km core.Keymap) {
	km = km.Normalized()
	if a.settings.Hotkeys.Equal(km) {
		return
	}
	a.settings.Hotkeys = km
	a.keys.SetKeymap(km)
	a.settingsVer++
	a.saveSetting(core.SettingHotkeys, km)
}

// hotkeyStatus sets the line under the hotkey table.
func (a *App) hotkeyStatus(text string, refused bool) {
	a.opt.status, a.opt.refused = text, refused
	a.opt.ver++
}

// startHotkeyCapture makes the next key press a new key of action id.
func (a *App) startHotkeyCapture(id string) {
	a.opt.capture = id
	a.hotkeyStatus(fmt.Sprintf("Press the new key for %s… (Esc cancels)", core.HotkeyLabel(id)), false)
}

// endHotkeyCapture stops a capture (the status line is kept).
func (a *App) endHotkeyCapture() {
	if a.opt.capture != "" {
		a.opt.capture = ""
		a.opt.ver++
	}
}

// hotkeyCaptured takes a key press made while capturing: Escape cancels,
// anything else is offered to addHotkey.
func (a *App) hotkeyCaptured(ev core.KeyEvent) {
	id := a.opt.capture
	if id == "" || ev.Echo {
		return
	}
	a.endHotkeyCapture()
	if ev.Key == "escape" && !ev.Ctrl && !ev.Meta && !ev.Alt && !ev.Shift {
		a.hotkeyStatus("", false)
		return
	}
	a.addHotkey(id, core.ChordOf(ev))
}

// addHotkey binds c to action id. A reserved key or one the action already
// has is refused on the status line; a key another action has asks first.
func (a *App) addHotkey(id string, c core.Chord) {
	km, err := a.settings.Hotkeys.WithKey(id, c, false)
	var conflict *core.ConflictError
	switch {
	case errors.As(err, &conflict):
		a.hotkeyStatus(fmt.Sprintf("%s is assigned to %s: reassign it or cancel.", c.Label(), core.HotkeyLabel(conflict.Action)), false)
		a.win.Confirm.Ask("Reassign Key", fmt.Sprintf("%s is assigned to %s.\nAssign it to %s instead?",
			c.Label(), core.HotkeyLabel(conflict.Action), core.HotkeyLabel(id)), func() {
			a.event(func() { a.reassignHotkey(id, c) })
		})
	case err != nil:
		a.hotkeyStatus(err.Error()+".", true)
		log.Printf("[freebeams] hotkey refused: %v", err)
	default:
		a.setHotkeys(km)
		a.hotkeyStatus(fmt.Sprintf("%s added to %s.", c.Label(), core.HotkeyLabel(id)), false)
	}
}

// reassignHotkey moves c to action id (the Reassign question's OK).
func (a *App) reassignHotkey(id string, c core.Chord) {
	km, err := a.settings.Hotkeys.WithKey(id, c, true)
	if err != nil {
		a.hotkeyStatus(err.Error()+".", true)
		return
	}
	a.setHotkeys(km)
	a.hotkeyStatus(fmt.Sprintf("%s now belongs to %s.", c.Label(), core.HotkeyLabel(id)), false)
}

// removeHotkey removes key i of the action in table row.
func (a *App) removeHotkey(row, i int) {
	acts := core.HotkeyActions()
	if row < 0 || row >= len(acts) {
		return
	}
	id := acts[row].ID
	keys := a.settings.Hotkeys[id]
	if i < 0 || i >= len(keys) {
		return
	}
	a.setHotkeys(a.settings.Hotkeys.WithoutKey(id, keys[i]))
	a.hotkeyStatus(fmt.Sprintf("%s removed from %s.", keys[i].Label(), core.HotkeyLabel(id)), false)
}

// resetHotkeys restores EasyWorship's keys.
func (a *App) resetHotkeys() {
	a.endHotkeyCapture()
	a.setHotkeys(core.DefaultKeymap())
	a.hotkeyStatus("The EasyWorship default keys are restored.", false)
}

// checkEWHotkeys looks for EasyWorship's HotKeys.2.ini (in the background).
func (a *App) checkEWHotkeys() {
	a.async(nil, func(context.Context) func() error {
		_, err := os.Stat(core.EWHotkeysPath)
		return func() error {
			if ok := err == nil; ok != a.opt.ewHotkeys {
				a.opt.ewHotkeys = ok
				a.opt.ver++
			}
			return nil
		}
	})
}

// importHotkeys reads EasyWorship's hotkeys (path "" is its usual place).
func (a *App) importHotkeys(path string, cmd *control.Command) {
	if path == "" {
		path = core.EWHotkeysPath
	}
	a.endHotkeyCapture()
	a.async(cmd, func(context.Context) func() error {
		b, err := os.ReadFile(path)
		return func() error {
			if err != nil {
				a.hotkeyStatus("Could not read EasyWorship's hotkeys: "+err.Error(), true)
				return fmt.Errorf("import hotkeys: %w", err)
			}
			km, skipped, err := core.ParseEWHotkeys(string(b), a.settings.Hotkeys)
			if err != nil {
				a.hotkeyStatus("Could not read EasyWorship's hotkeys: "+err.Error(), true)
				return fmt.Errorf("import hotkeys %s: %w", path, err)
			}
			a.setHotkeys(km)
			msg := "Imported EasyWorship's hotkeys."
			if len(skipped) > 0 {
				msg = fmt.Sprintf("Imported EasyWorship's hotkeys; skipped %d key(s) FreeBeams cannot use.", len(skipped))
				log.Printf("[freebeams] hotkey import skipped: %q", skipped)
			}
			a.hotkeyStatus(msg, false)
			return nil
		}
	})
}

// ---- default themes (Options ▸ Themes) -------------------------------------------

// defaultThemeKindNames are the Options labels of store.DefaultThemeKinds.
var defaultThemeKindNames = map[models.ItemKind]string{models.ItemSong: "Songs", models.ItemScripture: "Scriptures"}

// parseDefaultThemeKind reads an item kind that has a default theme.
func parseDefaultThemeKind(s string) (models.ItemKind, error) {
	k := strings.ToLower(strings.TrimSpace(s))
	switch k {
	case "songs":
		k = string(models.ItemSong)
	case "scriptures", "bible", "verse", "verses":
		k = string(models.ItemScripture)
	}
	for _, kind := range store.DefaultThemeKinds {
		if string(kind) == k {
			return kind, nil
		}
	}
	return "", fmt.Errorf("kind must be one of %v (the item types with a default theme), got %q", store.DefaultThemeKinds, s)
}

// followDefaultTheme re-themes what shows the old default of kind when t
// becomes the new one: the library deck behind the resource preview (a song
// without a theme of its own, the selected passage) and Preview when it shows
// that deck. Schedule items keep their snapshot. Called before the new
// defaults are applied (a.th.defaults still names the old one).
func (a *App) followDefaultTheme(kind models.ItemKind, t models.Theme) {
	old := a.defaultTheme(kind).ID
	var lib **models.Deck
	switch kind {
	case models.ItemSong:
		if a.song != nil && core.SongThemeName(a.song.ThemeID, a.th.list) != "" {
			return // the song has a theme of its own
		}
		lib = &a.songRes.deck
	case models.ItemScripture:
		lib = &a.scriptureDeck
	default:
		return
	}
	if !core.FollowsDefault(*lib, old) {
		return
	}
	was := **lib
	d := was
	d.Theme = t
	*lib = &d
	a.pres.RestyleDecks(func(pd *models.Deck) (models.Theme, bool) {
		if pd != a.pres.State.Preview || !core.SameDeck(pd, &was) {
			return models.Theme{}, false
		}
		return t, true
	})
}

// ---- view -----------------------------------------------------------------------

// wireOptions connects the Hotkeys and Themes sections (called from
// wireSettings).
func (a *App) wireOptions() {
	h := a.win.Settings.Hotkeys
	h.OnAdd = func(row int) {
		a.event(func() {
			if acts := core.HotkeyActions(); row >= 0 && row < len(acts) {
				a.startHotkeyCapture(acts[row].ID)
			}
		})
	}
	h.OnRemove = func(row, key int) { a.event(func() { a.removeHotkey(row, key) }) }
	h.OnReset = func() { a.event(a.resetHotkeys) }
	h.OnImport = func() { a.event(func() { a.importHotkeys("", nil) }) }
	h.OnKey = func(ev InputEvent.Instance) {
		if k, ok := keyEvent(ev); ok {
			a.event(func() { a.hotkeyCaptured(k) })
		}
	}
	a.win.Settings.DefaultThemes.OnChange = func(kind, theme int) {
		a.event(func() {
			if kind >= 0 && kind < len(store.DefaultThemeKinds) {
				a.setDefaultTheme(theme, store.DefaultThemeKinds[kind], nil)
			}
		})
	}
}

// renderOptions projects the keymap, the capture and the default themes onto
// the Options dialog (part of renderSettings).
func (a *App) renderOptions() {
	h := a.win.Settings.Hotkeys
	if !a.opt.shownInit || a.opt.shownSettings != a.settingsVer {
		bs := a.settings.Hotkeys.Bindings()
		rows := make([]ui.HotkeyRow, len(bs))
		for i, b := range bs {
			rows[i] = ui.HotkeyRow{Action: b.Action, Keys: b.Keys, Labels: b.Labels}
		}
		h.SetRows(rows)
		a.opt.shownSettings = a.settingsVer
	}
	if !a.opt.shownInit || a.opt.shownVer != a.opt.ver {
		h.SetCapturing(a.opt.capture != "")
		h.SetStatus(a.opt.status, a.opt.refused)
		h.SetImportAvailable(a.opt.ewHotkeys)
		if a.opt.capture != "" {
			for i, act := range core.HotkeyActions() {
				if act.ID == a.opt.capture && h.Selected() != i {
					h.Select(i)
				}
			}
		}
		a.opt.shownVer = a.opt.ver
	}
	if !a.opt.shownInit || a.opt.shownThemes != a.th.ver {
		rows := make([]ui.DefaultThemeRow, 0, len(store.DefaultThemeKinds))
		for _, kind := range store.DefaultThemeKinds {
			r := ui.DefaultThemeRow{Kind: defaultThemeKindNames[kind], Selected: core.ThemeIndexOf(a.th.list, a.th.defaults[kind])}
			for _, t := range a.th.list {
				r.Choices = append(r.Choices, t.Name)
			}
			rows = append(rows, r)
		}
		a.win.Settings.DefaultThemes.SetRows(rows)
		a.opt.shownThemes = a.th.ver
	}
	a.opt.shownInit = true
}

// optionsStateDoc is /state.options.
func (a *App) optionsStateDoc() core.OptionsState {
	return core.OptionsState{HotkeyCapture: a.opt.capture, HotkeyStatus: a.opt.status, HotkeyRefused: a.opt.refused}
}

// transitionProgress is the output's running transition (the live
// monitor's when the output is closed); 0 when none runs.
func (a *App) transitionProgress() float64 {
	v := a.liveView
	if o := a.out.View(); o != nil {
		v = o
	}
	if v == nil || !v.Transitioning() {
		return 0
	}
	return v.TransitionProgress()
}

// serveProgressWaiters answers wait-transition commands (every frame).
func (a *App) serveProgressWaiters() {
	if len(a.opt.progressWaiters) == 0 {
		return
	}
	p := a.transitionProgress()
	keep := a.opt.progressWaiters[:0]
	for _, w := range a.opt.progressWaiters {
		switch {
		case p >= w.progress:
			a.answer(w.cmd, nil)
		case p == 0:
			w.cmd.RespondErr(errors.New("wait-transition: no transition is running"))
		case w.frames >= maxProgressWaitFrames:
			w.cmd.RespondErr(fmt.Errorf("wait-transition: still at %.2f after %d frames", p, w.frames))
		default:
			w.frames++
			keep = append(keep, w)
		}
	}
	a.opt.progressWaiters = keep
}

// ---- control -----------------------------------------------------------------------

// handleOptionsCommand runs the hotkey and transition-wait actions; false
// when the action is not one of them.
func (a *App) handleOptionsCommand(cmd *control.Command, args actionArgs) bool {
	switch cmd.Action {
	case "hotkeys":
		a.answer(cmd, nil) // the reply's state carries settings.hotkeys and options
	case "set-hotkey":
		// {action, keys: ["ctrl+b", "F5"], reassign}: the action's whole list.
		id, err := core.ParseHotkeyAction(args.Action)
		if err != nil {
			cmd.RespondErr(fmt.Errorf("set-hotkey: %w", err))
			return true
		}
		chords, err := core.ParseChords(args.Keys)
		if err != nil {
			cmd.RespondErr(fmt.Errorf("set-hotkey: %w", err))
			return true
		}
		km, err := a.settings.Hotkeys.WithKeys(id, chords, args.Reassign)
		if err != nil {
			a.hotkeyStatus(err.Error()+".", errors.Is(err, core.ErrReserved))
			cmd.RespondErr(fmt.Errorf("set-hotkey: %w", err))
			return true
		}
		a.setHotkeys(km)
		a.answer(cmd, nil)
	case "reset-hotkeys":
		a.resetHotkeys()
		a.answer(cmd, nil)
	case "hotkey-capture":
		// {action}: Add Key… on that action (opens Options ▸ Hotkeys);
		// {on: false} cancels a capture.
		if args.On != nil && !*args.On {
			a.endHotkeyCapture()
			a.answer(cmd, nil)
			return true
		}
		id, err := core.ParseHotkeyAction(args.Action)
		if err != nil {
			cmd.RespondErr(fmt.Errorf("hotkey-capture: %w", err))
			return true
		}
		a.openSettings(ui.SettingsPageHotkeys)
		a.startHotkeyCapture(id)
		a.answer(cmd, nil)
	case "import-hotkeys":
		a.importHotkeys(args.Path, cmd)
	case "wait-transition":
		// {percent}: reply once the output's transition has reached that
		// progress (0..1), so a screenshot lands mid-way on any machine.
		p := args.Percent
		if p <= 0 || p >= 1 {
			cmd.RespondErr(errors.New("wait-transition: percent must be between 0 and 1"))
			return true
		}
		a.opt.progressWaiters = append(a.opt.progressWaiters, progressWaiter{cmd: cmd, progress: p})
	default:
		return false
	}
	return true
}
