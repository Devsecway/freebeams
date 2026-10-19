package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"

	"freeworship/internal/app/core"
	"freeworship/internal/control"
	"freeworship/internal/models"
	"freeworship/internal/render"
)

// Settings: the values the Options dialog edits (core.Settings) are
// authoritative state on the App. Events change them and persist each under
// its own key; render projects them onto the live monitor, the output, the
// logo, the interface theme and the dialog.

// loadSettings reads the persisted settings at startup (synchronously, like
// the layout: they decide what the first frame looks like) and applies the
// logo to the presenter.
func (a *App) loadSettings() {
	a.settings = core.DefaultSettings(core.EWImagesDir)
	if a.st != nil {
		get := func(key string, dst any) {
			if _, err := a.st.GetSetting(a.ctx, key, dst); err != nil {
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
		get(core.SettingRecent, &a.recent)
	}
	a.settings = a.settings.Normalized(core.EWImagesDir)
	a.pres.SetLogo(a.settings.Logo)
	log.Printf("[freeworship] settings: transition %s; output screen %d; logo %s; %s interface",
		a.settings.Transition, a.settings.OutputScreen, core.LogoDescription(a.settings.Logo), a.settings.InterfaceColor)
}

// saveSetting persists one setting off the main thread.
func (a *App) saveSetting(key string, v any) {
	st := a.st
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
	a.settings.OutputScreen = i
	a.settingsVer++
	a.saveSetting(core.SettingOutputScreen, i)
	return nil
}

// setLogo changes the logo screen's background.
func (a *App) setLogo(bg models.Background) {
	if bg.Kind == "" {
		bg = core.LogoBackground(core.EWImagesDir)
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
	default:
		return false
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
			a.setLogo(core.LogoFromPath(path, core.EWImagesDir))
			return nil
		}
		var bg models.Background
		if err := json.Unmarshal(raw, &bg); err != nil {
			return fmt.Errorf("set-setting %s: want an image path or a background object: %w", key, err)
		}
		a.setLogo(bg)
	case core.SettingInterfaceColor, "interfaceColor":
		var c string
		if err := json.Unmarshal(raw, &c); err != nil {
			return fmt.Errorf("set-setting %s: %w", key, err)
		}
		return a.setInterfaceColor(c)
	default:
		return fmt.Errorf("set-setting: unknown key %q (transition, output.screen, output.logo, ui.interfaceColor)", key)
	}
	return nil
}
