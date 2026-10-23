package app

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"freebeams/internal/app/core"
	"freebeams/internal/models"
	"freebeams/internal/render"
)

// Transitions: the configured models.Transition (settings.Transition) drives
// the live monitor and the projector output, and the Black, Clear and Logo
// transitions (settings.Screens, "same" by default) the changes into and out
// of those screens. The preview monitor keeps the default crossfade and
// thumbnails cut (render.SlideView with duration 0).

// applyTransitions projects the configured transitions onto a live view.
// The setters only store them (the view picks one when a change starts), so
// this is idempotent and cheap enough to run on every render pass.
func (a *App) applyTransitions(v *render.SlideView) {
	v.SetTransition(a.settings.Transition)
	v.SetScreenTransitions(a.settings.Screens)
}

// renderTransition projects the transitions onto the output's view. The
// view is created when the output opens, so this runs after the open/close
// switch in render (the live monitor is set right before its SetFrame).
func (a *App) renderTransition() {
	if v := a.out.View(); v != nil {
		a.applyTransitions(v)
	}
}

// liveTransition is the transition running on the output (the live
// monitor when the output is closed); nil when none runs (/state).
func (a *App) liveTransition() *models.Transition {
	v := a.liveView
	if o := a.out.View(); o != nil {
		v = o
	}
	if v == nil {
		return nil
	}
	if t, ok := v.RunningTransition(); ok {
		return &t
	}
	return nil
}

// setScreenTransition changes the Black, Clear or Logo transition.
func (a *App) setScreenTransition(mode models.OutputMode, t models.ScreenTransition) {
	next := a.settings.Screens.With(mode, t)
	if next == a.settings.Screens {
		return
	}
	a.settings.Screens = next
	a.settingsVer++
	st, _ := next.Get(mode)
	a.saveSetting(core.SettingScreenTransition(mode), st)
}

// screenTransitionCommand is `transition {mode, kind, duration}` for a
// screen: kind "same" follows the slide transition; a duration alone keeps
// the screen's kind (the slide's when it was "same").
func (a *App) screenTransitionCommand(mode models.OutputMode, args actionArgs) error {
	cur, _ := a.settings.Screens.Get(mode)
	if strings.EqualFold(strings.TrimSpace(args.Kind), models.SameAsSlide) {
		a.setScreenTransition(mode, models.SameTransition())
		return nil
	}
	t := cur.Resolve(a.settings.Transition)
	if args.Kind != "" {
		k, err := models.ParseTransitionKind(args.Kind)
		if err != nil {
			return fmt.Errorf("transition: %w", err)
		}
		t.Kind = k
	}
	if args.Duration != nil {
		t.Duration = msDuration(*args.Duration)
	}
	if args.Kind == "" && args.Duration == nil {
		return errors.New("transition: kind or duration is required")
	}
	a.setScreenTransition(mode, models.OwnTransition(t))
	return nil
}

// msDuration converts a control-API duration (milliseconds) to a Duration.
func msDuration(ms int) time.Duration { return time.Duration(ms) * time.Millisecond }
