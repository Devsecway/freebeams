package app

import (
	"time"

	"freeworship/internal/models"
)

// Transitions: the configured models.Transition (settings.Transition) drives
// the live monitor and the projector output. The preview monitor keeps the
// default crossfade and thumbnails cut (render.SlideView with duration 0).

// renderTransition projects the configured transition onto the output's
// view. SetTransition only stores the kind and duration, so it is idempotent
// and cheap enough to run on every render pass; the view is created when the
// output opens, so this runs after the open/close switch in render (the live
// monitor is set right before its SetFrame).
func (a *App) renderTransition(t models.Transition) {
	if v := a.out.View(); v != nil {
		v.SetTransition(t)
	}
}

// msDuration converts a control-API duration (milliseconds) to a Duration.
func msDuration(ms int) time.Duration { return time.Duration(ms) * time.Millisecond }
