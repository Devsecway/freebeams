package app

import (
	"encoding/json"
	"errors"
	"fmt"

	"freebeams/internal/app/core"
	"freebeams/internal/control"
	"freebeams/internal/models"
	"freebeams/internal/render"
	"freebeams/internal/ui"

	"graphics.gd/classdb/Control"
)

// Video and audio playback: the transport bars under the Preview Output and
// Live Output monitors, the play / pause / seek / volume / loop control
// actions and the "playback" part of /state.
//
// State machine (CLAUDE.md): what the operator wants of each monitor's media
// is app state (playbackState, one core.Playback per monitor). Events (the
// transport bar's callbacks, control actions, a file reaching its end, a
// new item arriving on a monitor) mutate it; renderPlayback projects it onto
// the shared native player of the monitor's role (render.Current) and reads
// the player back for the transport bar, every frame while a file is open.

// playbackState is the operator's playback per monitor.
type playbackState struct {
	live    core.Playback
	preview core.Playback

	// resource is the resource preview's mini transport.
	resource core.Playback
}

// monitorRoles lists the views with a transport bar: the two output
// monitors and the resource preview.
var monitorRoles = []render.VideoRole{render.RoleLive, render.RolePreview, render.RoleResource}

// transportBar is a transport view: the monitors' Transport or the
// resource preview's MiniTransport.
type transportBar interface {
	AsControl() Control.Instance
	SetState(ui.TransportState)
	Callbacks() *ui.TransportCallbacks
}

// roleIndex is the role's slot in the view cache's memos.
func roleIndex(role render.VideoRole) int {
	switch role {
	case render.RolePreview:
		return 1
	case render.RoleResource:
		return 2
	}
	return 0
}

// targetRole maps a control action's target to its player role.
func targetRole(t core.Target) render.VideoRole {
	switch t {
	case core.TargetPreview:
		return render.RolePreview
	case core.TargetResource:
		return render.RoleResource
	}
	return render.RoleLive
}

// initPlayback picks the media player and sets the starting volume. Called
// from Ready, before buildUI.
func (a *App) initPlayback() {
	a.selectMediaPlayer()
	a.pb.live.Volume = 1
	a.pb.preview.Volume = 1
	a.pb.resource.Volume = 1
}

// playback returns the state for a monitor.
func (a *App) playback(role render.VideoRole) *core.Playback {
	switch role {
	case render.RoleLive:
		return &a.pb.live
	case render.RoleResource:
		return &a.pb.resource
	}
	return &a.pb.preview
}

// playbackFrame is the monitor's frame (what it shows).
func (a *App) playbackFrame(role render.VideoRole) models.Frame {
	switch role {
	case render.RoleLive:
		return a.pres.LiveFrame()
	case render.RoleResource:
		f, _, _ := a.resourcePreviewFrame()
		return f
	}
	return a.pres.PreviewFrame()
}

// transport is the monitor's transport bar.
func (a *App) transport(role render.VideoRole) transportBar {
	switch role {
	case render.RoleLive:
		return a.win.LiveTransport
	case render.RoleResource:
		return a.win.ResourceTransport
	}
	return a.win.PreviewTransport
}

// wirePlayback gives the monitor views their player roles and connects the
// transport bars (called from buildUI).
func (a *App) wirePlayback() {
	a.liveView.SetVideoRole(render.RoleLive)       // shares the output's player
	a.previewView.SetVideoRole(render.RolePreview) // its own, paused
	a.resView.SetVideoRole(render.RoleResource)    // its own too: previews a clip off the output
	for _, role := range monitorRoles {
		t := a.transport(role).Callbacks()
		t.OnPlayPause = func() { a.event(func() { a.togglePlay(role) }) }
		t.OnSeek = func(frac float64) {
			a.event(func() { a.seekTo(role, core.SeekFraction(frac, a.playbackDuration(role))) })
		}
		t.OnVolume = func(v float64) { a.event(func() { a.playback(role).Volume = core.ClampVolume(v) }) }
		t.OnLoop = func(on bool) { a.event(func() { a.playback(role).Loop = on }) }
		t.OnSkip = func(dir int) {
			a.event(func() {
				if dir < 0 {
					a.seekTo(role, 0)
				} else {
					a.seekTo(role, a.playbackDuration(role))
				}
			})
		}
	}
}

// ---- events -------------------------------------------------------------------------

// playbackEvents turns what happened since the last frame into state: a new
// item on a monitor gets its defaults (live autoplays, preview waits), a
// file that reached its end with looping off is no longer playing. It
// reports whether anything changed. Called every frame from Process.
func (a *App) playbackEvents() bool {
	if render.AnyMediaActive() {
		render.PumpMedia() // a file held under Black has no layer pumping it (render/hold.go)
	}
	changed := false
	for _, role := range monitorRoles {
		pb := a.playback(role)
		next, c := core.AdvancePlayback(*pb, a.playbackFrame(role), role == render.RoleLive)
		if c {
			*pb = next
			changed = true
		}
		if src := render.Source(role, pb.Key); src != nil && src.TakeEnded() && pb.Playing {
			pb.Playing = false
			changed = true
		}
	}
	return changed
}

// togglePlay plays or pauses the monitor's media.
func (a *App) togglePlay(role render.VideoRole) {
	pb := a.playback(role)
	if pb.Key == "" {
		return
	}
	pb.Playing = !pb.Playing
}

// setPlaying plays or pauses the monitor's media.
func (a *App) setPlaying(role render.VideoRole, on bool) {
	if pb := a.playback(role); pb.Key != "" {
		pb.Playing = on
	}
}

// seekTo asks for a seek to secs on the monitor's media.
func (a *App) seekTo(role render.VideoRole, secs float64) {
	pb := a.playback(role)
	if pb.Key == "" {
		return
	}
	pb.SeekTo = max(secs, 0)
	pb.SeekSeq++
}

// playbackDuration is the monitor's file duration as the player reports it
// (0 while loading).
func (a *App) playbackDuration(role render.VideoRole) float64 {
	src := render.Source(role, a.playback(role).Key)
	if src == nil {
		return 0
	}
	return src.State().Duration
}

// ---- render ---------------------------------------------------------------------------

// renderPlayback projects the playback state onto the players and the
// players onto the transport bars. Idempotent; the only thing it writes is
// the view cache's memo of the applied seek.
func (a *App) renderPlayback() {
	for _, role := range monitorRoles {
		pb := a.playback(role)
		idx := roleIndex(role)
		// The player stays open for the file the state is about, also while
		// Black or Logo hides it (render/hold.go): its position goes on.
		render.Hold(role, pb.Key)
		src := render.Source(role, pb.Key)
		if src != nil {
			src.SetLoop(pb.Loop)
			src.SetVolume(pb.Volume)
			if pb.Playing != src.Playing() {
				if pb.Playing {
					src.Play()
				} else {
					src.Pause()
				}
			}
			if pb.SeekSeq != a.view.seekApplied[idx] {
				src.Seek(pb.SeekTo)
			}
		}
		a.view.seekApplied[idx] = pb.SeekSeq // a seek with no player open is dropped
		st := a.playbackStateDoc(role)
		t := a.transport(role)
		if t.AsControl().AsCanvasItem().Visible() != st.Media {
			t.AsControl().AsCanvasItem().SetVisible(st.Media)
		}
		if st.Media {
			t.SetState(ui.TransportState{Playing: st.Playing, Position: st.Position, Duration: st.Duration,
				Loop: st.Loop, Volume: st.Volume, Enabled: !st.Failed})
		}
	}
}

// playbackStateDoc is one monitor's playback for /state and the transport.
func (a *App) playbackStateDoc(role render.VideoRole) core.PlaybackState {
	st := core.PlaybackState{Native: render.NativeVideoSupported(), Player: render.MediaPlayer()}
	frame := a.playbackFrame(role)
	bg, shown, ok := core.MonitorMedia(frame)
	if !ok {
		return st
	}
	pb := a.playback(role)
	st.Media, st.Hidden = true, !shown
	st.Title = frame.Slide.Label
	st.Playing, st.Loop, st.Volume = pb.Playing, pb.Loop, pb.Volume
	if src := render.Source(role, bg.Path); src != nil {
		p := src.State()
		st.Position, st.Duration = p.Position, p.Duration
		st.HasVideo, st.HasAudio, st.Failed = p.HasVideo, p.HasAudio, p.Failed
	} else if !render.NativeVideoSupported() {
		st.Failed = true // nothing can play it here (poster only)
	}
	return st
}

// playbackStates is the /state document's "playback" object.
func (a *App) playbackStates() core.PlaybackStates {
	return core.PlaybackStates{Live: a.playbackStateDoc(render.RoleLive), Preview: a.playbackStateDoc(render.RolePreview),
		Resource: a.playbackStateDoc(render.RoleResource)}
}

// ---- control actions ------------------------------------------------------------------

// handlePlaybackCommand runs the transport actions (play, pause, toggle-play,
// seek, volume, loop), each with "target": "live" (default) or "preview";
// false when the action is not one of them.
func (a *App) handlePlaybackCommand(cmd *control.Command, args actionArgs) bool {
	switch cmd.Action {
	case "play", "pause", "toggle-play", "seek", "volume", "loop":
	default:
		return false
	}
	target, ok := core.PlaybackTarget(args.Target)
	if !ok {
		cmd.RespondErr(core.PlaybackError(cmd.Action, args.Target))
		return true
	}
	role := targetRole(target)
	if a.playback(role).Key == "" {
		cmd.RespondErr(fmt.Errorf("%s: the %s monitor shows no video or audio", cmd.Action, target))
		return true
	}
	switch cmd.Action {
	case "play":
		a.setPlaying(role, true)
	case "pause":
		a.setPlaying(role, false)
	case "toggle-play":
		a.togglePlay(role)
	case "seek":
		if args.Seconds == nil {
			cmd.RespondErr(errors.New("seek: \"seconds\" is required"))
			return true
		}
		a.seekTo(role, *args.Seconds)
	case "volume":
		var v float64
		if len(args.Value) == 0 || json.Unmarshal(args.Value, &v) != nil {
			cmd.RespondErr(errors.New("volume: \"value\" must be a number from 0 to 1"))
			return true
		}
		a.playback(role).Volume = core.ClampVolume(v)
	case "loop":
		if args.On == nil {
			cmd.RespondErr(errors.New("loop: \"on\" is required"))
			return true
		}
		a.playback(role).Loop = *args.On
	}
	a.answer(cmd, nil)
	return true
}
