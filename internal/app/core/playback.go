package core

import (
	"fmt"
	"math"
	"time"

	"freebeams/internal/models"
)

// Video and audio playback (the transport bars under the Preview Output and
// Live Output monitors). The authoritative state is the app's per-monitor
// Playback (what the operator wants); what the player is actually doing is
// read back every frame into a PlaybackState for the transport and /state.

// Playback is what the operator asked of one monitor's media: the app's
// state, projected onto the player by render. Key identifies the media it
// applies to (the background's path); a new item gets fresh defaults
// (PlaybackDefaults).
type Playback struct {
	Key     string  // the media path these settings are for ("" = nothing plays)
	Playing bool    // want playing (false = paused)
	Loop    bool    // start again at the end
	Volume  float64 // 0..1
	SeekSeq int     // bumped by every seek request; render applies SeekTo once per bump
	SeekTo  float64 // seconds
}

// PlaybackDefaults is the playback a monitor starts with when bg (a video or
// audio background) arrives: live autoplays, preview waits paused at 0; the
// item's Loop is the default; the volume carries over from the previous
// playback (the app starts each monitor at 1.0). A seek request that was
// never applied stays counted (SeekSeq) so render does not replay it.
func PlaybackDefaults(bg models.Background, live bool, prev Playback) Playback {
	return Playback{Key: bg.Path, Playing: live, Loop: bg.Loop, Volume: prev.Volume, SeekSeq: prev.SeekSeq}
}

// NoPlayback is the playback of a monitor that shows no video or audio: the
// volume and the seek count carry over for the next item.
func NoPlayback(prev Playback) Playback {
	return Playback{Volume: prev.Volume, SeekSeq: prev.SeekSeq}
}

// FrameMedia returns the video or audio background a frame shows, if any: a
// media slide's background, or the theme's motion background behind a text
// slide, in the normal and clear modes (clear only hides the text). Black
// and logo show no media, so nothing plays.
func FrameMedia(f models.Frame) (models.Background, bool) {
	if f.Slide == nil {
		return models.Background{}, false
	}
	switch f.Mode {
	case models.ModeBlack, models.ModeLogo:
		return models.Background{}, false
	}
	bg := f.Theme.Background
	if f.Slide.Background != nil {
		bg = *f.Slide.Background
	}
	if !bg.Kind.Plays() || bg.Path == "" {
		return models.Background{}, false
	}
	return bg, true
}

// MonitorMedia returns the video or audio a monitor keeps playing for frame
// f: FrameMedia's, and also the media slide under Black and Logo. Black and
// Logo cover the output but leave the file playing (EasyWorship's
// behaviour: the sound goes on, and coming back shows the clip where it has
// got to), so shown is false then. ok is false when the frame's slide is no
// video or audio at all.
func MonitorMedia(f models.Frame) (bg models.Background, shown, ok bool) {
	if f.Slide == nil || f.Slide.Background == nil {
		return models.Background{}, false, false
	}
	bg = *f.Slide.Background
	if !bg.Kind.Plays() || bg.Path == "" {
		return models.Background{}, false, false
	}
	switch f.Mode {
	case models.ModeBlack, models.ModeLogo:
		return bg, false, true
	}
	return bg, true, true
}

// AdvancePlayback is a monitor's playback after its frame became f: the
// same file (also under Black / Logo, and back) keeps everything the
// operator set, play / pause, loop, volume and the seek count, so the
// player's position carries on; a new file gets PlaybackDefaults; a frame
// with no video or audio NoPlayback. changed reports a new Key.
func AdvancePlayback(pb Playback, f models.Frame, live bool) (next Playback, changed bool) {
	bg, _, ok := MonitorMedia(f)
	key := ""
	if ok {
		key = bg.Path
	}
	if key == pb.Key {
		return pb, false
	}
	if ok {
		return PlaybackDefaults(bg, live, pb), true
	}
	return NoPlayback(pb), true
}

// PlaybackState is one monitor's playback as reported to the transport bar
// and GET /state: the operator's settings plus what the player reports.
type PlaybackState struct {
	Playing  bool    `json:"playing"`
	Position float64 `json:"position"` // seconds
	Duration float64 `json:"duration"` // seconds; 0 until the file is open
	Loop     bool    `json:"loop"`
	Volume   float64 `json:"volume"`
	HasVideo bool    `json:"hasVideo"`
	HasAudio bool    `json:"hasAudio"`
	Title    string  `json:"title,omitempty"` // the media item's title
	Native   bool    `json:"native"`          // a media player is in use: AVFoundation or ffmpeg (false: Theora / nothing)
	// Media reports whether the monitor shows a video or audio item at all
	// (the transport bar is shown only then).
	Media bool `json:"media"`
	// Failed reports a file the player could not open (the poster is shown).
	Failed bool `json:"failed,omitempty"`

	// Player is the media player backend (avfoundation, ffmpeg, theora).
	Player string `json:"player"`

	// Hidden reports a file that plays on under Black or Logo (MonitorMedia):
	// the output does not show it, the sound and the position go on.
	Hidden bool `json:"hidden,omitempty"`
}

// PlaybackStates is the /state document's "playback" object.
type PlaybackStates struct {
	Live    PlaybackState `json:"live"`
	Preview PlaybackState `json:"preview"`

	// Resource is the resource preview's mini transport (a clip previewed
	// before it goes anywhere near the output).
	Resource PlaybackState `json:"resource"`
}

// TransportTime formats the transport bar's time as "m:ss / m:ss" ("0:00 /
// 0:30"; hours when a file is that long). Position is clamped into the
// duration; an unknown duration shows "0:00".
func TransportTime(position, duration float64) string {
	if duration < 0 || math.IsNaN(duration) || math.IsInf(duration, 0) {
		duration = 0
	}
	if position < 0 || math.IsNaN(position) || math.IsInf(position, 0) {
		position = 0
	}
	if duration > 0 && position > duration {
		position = duration
	}
	return clockTime(position) + " / " + clockTime(duration)
}

// clockTime formats whole seconds as m:ss (h:mm:ss from an hour); 0 is "0:00".
func clockTime(secs float64) string {
	if secs <= 0 {
		return "0:00"
	}
	s := FormatDuration(time.Duration(math.Floor(secs)) * time.Second)
	if s == "" {
		return "0:00"
	}
	return s
}

// SeekFraction converts a slider fraction (0..1) to seconds of a duration.
func SeekFraction(frac, duration float64) float64 {
	if duration <= 0 {
		return 0
	}
	return min(max(frac, 0), 1) * duration
}

// ClampVolume keeps a volume inside 0..1.
func ClampVolume(v float64) float64 {
	if math.IsNaN(v) {
		return 1
	}
	return min(max(v, 0), 1)
}

// Target is a transport a playback control action drives.
type Target int

const (
	TargetLive     Target = iota // the Live Output monitor and the projector
	TargetPreview                // the Preview Output monitor
	TargetResource               // the resource preview's mini transport
)

// String is the target's name in the control API.
func (t Target) String() string {
	switch t {
	case TargetPreview:
		return "preview"
	case TargetResource:
		return "resource"
	}
	return "live"
}

// PlaybackTarget resolves a control action's "target" ("live" by default,
// "preview" or "resource"). ok is false for anything else.
func PlaybackTarget(target string) (t Target, ok bool) {
	switch target {
	case "", "live":
		return TargetLive, true
	case "preview":
		return TargetPreview, true
	case "resource":
		return TargetResource, true
	}
	return TargetLive, false
}

// PlaybackError is the control API's complaint about a target.
func PlaybackError(action, target string) error {
	return fmt.Errorf("%s: target must be \"live\", \"preview\" or \"resource\", got %q", action, target)
}
