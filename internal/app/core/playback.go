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

	// Own reports a playback that started at its item's own volume or Mute
	// (Background.Volume / Mute). The operator's volume changes stay with
	// it: the next item without a volume of its own starts at Carry, the
	// volume the monitor had before (CarriedVolume).
	Own   bool
	Carry float64

	// Hidden records whether Black or Logo covered the file when the app
	// last looked (MonitorMedia's shown, inverted; CoverPlayback acts on
	// its changes). BlackPaused reports that the Pause on Black option
	// paused the file then, so leaving Black plays it again.
	Hidden      bool
	BlackPaused bool
}

// NewPlayback is a monitor's playback before anything played: full volume.
func NewPlayback() Playback { return Playback{Volume: 1} }

// CarriedVolume is the volume the monitor's next item without a volume of
// its own starts at: the operator's volume, unless this playback runs at
// its item's own volume (then the volume from before it).
func (pb Playback) CarriedVolume() float64 {
	if pb.Own {
		return pb.Carry
	}
	return pb.Volume
}

// SetPlaying is the operator's play / pause: it also forgets a pause the
// Pause on Black option made, so leaving Black keeps the operator's choice.
func (pb *Playback) SetPlaying(on bool) {
	pb.Playing, pb.BlackPaused = on, false
}

// StartVolume is the volume bg starts at on a monitor whose carried volume
// is carry: muted 0, else the item's own volume (percent), else carry. own
// reports that the item decided it (Mute or a volume of its own).
func StartVolume(bg models.Background, carry float64) (v float64, own bool) {
	switch {
	case bg.Mute:
		return 0, true
	case bg.Volume > 0:
		return ClampVolume(float64(models.ClampMediaVolume(bg.Volume)) / models.MaxMediaVolume), true
	}
	return ClampVolume(carry), false
}

// PlaybackDefaults is the playback a monitor starts with when bg (a video or
// audio background) arrives: live autoplays, preview waits paused at 0; the
// item's Loop is the default; the volume is the item's (StartVolume: Mute,
// else its own volume) or carries over from the previous playback (the app
// starts each monitor at 1.0; an item with a volume of its own does not
// pass it on). A seek request that was never applied stays counted
// (SeekSeq) so render does not replay it.
func PlaybackDefaults(bg models.Background, live bool, prev Playback) Playback {
	carry := prev.CarriedVolume()
	v, own := StartVolume(bg, carry)
	pb := Playback{Key: bg.Path, Playing: live, Loop: bg.Loop, Volume: v, SeekSeq: prev.SeekSeq, Own: own}
	if own {
		pb.Carry = carry // only kept while the item decides the volume
	}
	return pb
}

// NoPlayback is the playback of a monitor that shows no video or audio: the
// carried volume and the seek count carry over for the next item.
func NoPlayback(prev Playback) Playback {
	return Playback{Volume: prev.CarriedVolume(), SeekSeq: prev.SeekSeq}
}

// CoverPlayback applies Black and Logo to a monitor's playback (after
// AdvancePlayback): hidden is whether they cover its file now. With
// pauseOnBlack (the Pause on Black option, live only) covering a playing
// file pauses it (BlackPaused), and uncovering plays it again if that
// pause is still the one Black made; an operator's play or pause in
// between (SetPlaying) wins. Without the option Black leaves the file
// playing (EasyWorship's behaviour), but a pause Black made earlier is
// still undone. Only a change of hidden acts, so the operator can play a
// file under Black. changed reports a new state.
func CoverPlayback(pb Playback, hidden, pauseOnBlack bool) (next Playback, changed bool) {
	if pb.Key == "" {
		hidden = false
	}
	if hidden == pb.Hidden {
		return pb, false
	}
	pb.Hidden = hidden
	switch {
	case hidden && pauseOnBlack && pb.Playing:
		pb.Playing, pb.BlackPaused = false, true
	case !hidden && pb.BlackPaused:
		pb.Playing, pb.BlackPaused = true, false
	}
	return pb, true
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
	if f.Slide == nil {
		return models.Background{}, false, false
	}
	bg = f.Theme.Background
	if f.Slide.Background != nil {
		bg = *f.Slide.Background
	}
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

	// ItemVolume reports a playback that started at its item's own volume
	// or Mute (Playback.Own); BlackPaused one that the Pause on Black option
	// paused and leaving Black plays again.
	ItemVolume  bool `json:"itemVolume,omitempty"`
	BlackPaused bool `json:"blackPaused,omitempty"`
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
