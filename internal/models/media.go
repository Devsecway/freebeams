package models

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// MediaKind classifies a media library item.
type MediaKind string

const (
	MediaImage MediaKind = "image"
	MediaVideo MediaKind = "video"
	// MediaAudio is a sound file: an item with no picture of its own that
	// plays over a black slide.
	MediaAudio MediaKind = "audio"
)

// imageExts, videoExts and audioExts are the file types the media library
// accepts (lower case, with the dot). Godot decodes every image kind
// natively. Videos and audio play through the platform's media framework
// where there is one (AVFoundation on macOS, see internal/render); elsewhere
// Godot plays Ogg Theora only and the other videos are transcoded first (see
// internal/media).
var (
	imageExts = map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".webp": true, ".bmp": true}
	videoExts = map[string]bool{".mp4": true, ".mov": true, ".m4v": true, ".ogv": true, ".webm": true}
	audioExts = map[string]bool{".mp3": true, ".m4a": true, ".aac": true, ".wav": true, ".aiff": true, ".aif": true, ".flac": true, ".ogg": true}
)

// Plays reports whether the kind is media that plays over time (a video or
// an audio file) rather than a still.
func (k MediaKind) Plays() bool { return k == MediaVideo || k == MediaAudio }

// MediaKindOf classifies a file by its extension. ok is false for files the
// library does not accept.
func MediaKindOf(path string) (kind MediaKind, ok bool) {
	ext := strings.ToLower(filepath.Ext(path))
	switch {
	case imageExts[ext]:
		return MediaImage, true
	case videoExts[ext]:
		return MediaVideo, true
	case audioExts[ext]:
		return MediaAudio, true
	}
	return "", false
}

// PlayableVideoExt is the only video container Godot decodes (Ogg Theora).
const PlayableVideoExt = ".ogv"

// IsPlayableVideo reports whether path is a video file Godot can play as is.
func IsPlayableVideo(path string) bool {
	return path != "" && strings.EqualFold(filepath.Ext(path), PlayableVideoExt)
}

// MediaTitle derives a display title from a file path: the file name without
// its extension, with underscores read as spaces and whitespace collapsed.
func MediaTitle(path string) string {
	base := filepath.Base(path)
	base = strings.TrimSuffix(base, filepath.Ext(base))
	base = strings.ReplaceAll(base, "_", " ")
	return strings.Join(strings.Fields(base), " ")
}

// Media is one item of the media library: a still image, a motion
// background video or an audio file on disk, plus what the app derived from
// it (a thumbnail, and for videos a playable transcode where one is needed).
type Media struct {
	ID   int64     `json:"id"`
	Path string    `json:"path"` // absolute path of the source file
	Kind MediaKind `json:"kind"`
	// Title is shown in the library and the schedule (defaults to MediaTitle(Path)).
	Title string `json:"title"`
	// Duration of a video or audio file; 0 for stills.
	Duration time.Duration `json:"duration"`
	Width    int           `json:"width"`  // pixels; 0 when unknown
	Height   int           `json:"height"` // pixels; 0 when unknown
	// ThumbPath is a small still of the item (a video's poster frame); "" until made.
	ThumbPath string `json:"thumbPath,omitempty"`
	// TranscodedPath is the playable (.ogv) copy of a video; "" until made, and
	// always "" for stills.
	TranscodedPath string    `json:"transcodedPath,omitempty"`
	Added          time.Time `json:"added"`

	// Aspect and Repeat are the item's Aspect Ratio ▸ and Repeating ▸ menu
	// choices (how it fills the slide; whether a video loops). "" reads as
	// the defaults (AspectFill, RepeatLoop).
	Aspect MediaAspect `json:"aspect,omitempty"`
	Repeat MediaRepeat `json:"repeat,omitempty"`

	// Tags are the item's keywords (the Properties dialog), comma-separated
	// as NormalizeTags writes them; the library search matches them.
	Tags string `json:"tags,omitempty"`

	// Volume is the item's own playback volume in percent (the Properties
	// dialog's Volume, EasyWorship's Form_VideoProperties TrackBar_Volume):
	// 1..100, or 0 for none, which lets the monitor's volume carry over from
	// the previous item. Mute starts the item silent; the operator's
	// transport can still raise it for that playback. Videos and audio only.
	Volume int  `json:"volume,omitempty"`
	Mute   bool `json:"mute,omitempty"`
}

// MaxMediaVolume is a media item's full volume in percent.
const MaxMediaVolume = 100

// ClampMediaVolume keeps an item volume inside 0 (none) .. MaxMediaVolume.
func ClampMediaVolume(v int) int { return min(max(v, 0), MaxMediaVolume) }

// MediaProperties is what the media item's Properties dialog edits
// (EasyWorship's Form_VideoProperties: Title, Aspect Ratio, Repeating, Tags,
// Volume / Mute).
type MediaProperties struct {
	Title  string      `json:"title"`
	Tags   string      `json:"tags"`
	Aspect MediaAspect `json:"aspect"`
	Repeat MediaRepeat `json:"repeat"`

	// Volume is the item's own volume in percent (0 = none: the monitor's
	// volume carries over); Mute starts it silent (see Media.Volume).
	Volume int  `json:"volume"`
	Mute   bool `json:"mute"`
}

// Properties returns the item's current properties (effective Repeating).
func (m Media) Properties() MediaProperties {
	return MediaProperties{Title: m.Title, Tags: m.Tags, Aspect: m.Aspect.Normalized(), Repeat: m.RepeatMode(),
		Volume: m.Volume, Mute: m.Mute}
}

// WithProperties returns m with p applied: a blank title keeps m's, the tags
// are normalized, an invalid aspect or repeat keeps m's, the volume is
// clamped to 0..MaxMediaVolume (a still keeps no volume or mute).
func (m Media) WithProperties(p MediaProperties) Media {
	if t := strings.TrimSpace(p.Title); t != "" {
		m.Title = t
	}
	m.Tags = NormalizeTags(p.Tags)
	if p.Aspect.Valid() {
		m.Aspect = p.Aspect
	}
	if p.Repeat.Valid() {
		m.Repeat = p.Repeat
	}
	m.Volume, m.Mute = ClampMediaVolume(p.Volume), p.Mute
	if !m.Kind.Plays() {
		m.Volume, m.Mute = 0, false
	}
	return m
}

// NormalizeTags cleans a tag list typed by the operator: commas or new
// lines separate tags, each is trimmed with inner whitespace collapsed,
// empty ones and case-insensitive repeats are dropped (the first spelling
// wins); the result is joined with ", ".
func NormalizeTags(s string) string {
	seen := map[string]bool{}
	var out []string
	for _, t := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == '\n' || r == '\r' || r == ';' }) {
		t = strings.Join(strings.Fields(t), " ")
		k := strings.ToLower(t)
		if t == "" || seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, t)
	}
	return strings.Join(out, ", ")
}

// MediaAspect is how a media item is scaled to the slide (the tile menu's
// Aspect Ratio ▸). It is also the slide background's Fit.
type MediaAspect string

const (
	AspectFill    MediaAspect = "fill"    // cover the canvas, cropping the overflow (default)
	AspectFit     MediaAspect = "fit"     // fit inside the canvas, letterboxed
	AspectStretch MediaAspect = "stretch" // stretch to the canvas, ignoring the aspect ratio
)

// MediaAspects lists the aspect choices in menu order.
var MediaAspects = []MediaAspect{AspectFill, AspectFit, AspectStretch}

// Valid reports whether a is a known aspect ("" is not: use Normalized).
func (a MediaAspect) Valid() bool {
	return a == AspectFill || a == AspectFit || a == AspectStretch
}

// Normalized returns a, or AspectFill for "" and unknown values.
func (a MediaAspect) Normalized() MediaAspect {
	if a.Valid() {
		return a
	}
	return AspectFill
}

// ParseMediaAspect reads "fill" / "fit" / "stretch" (case-insensitive).
func ParseMediaAspect(s string) (MediaAspect, error) {
	a := MediaAspect(strings.ToLower(strings.TrimSpace(s)))
	if !a.Valid() {
		return "", fmt.Errorf("unknown aspect ratio %q (fill|fit|stretch)", s)
	}
	return a, nil
}

// MediaRepeat is whether a video plays again when it ends (the tile menu's
// Repeating ▸). Stills ignore it.
type MediaRepeat string

const (
	RepeatLoop MediaRepeat = "loop" // play again from the start (default)
	RepeatOnce MediaRepeat = "once" // play once and hold the last frame
)

// MediaRepeats lists the repeat choices in menu order.
var MediaRepeats = []MediaRepeat{RepeatLoop, RepeatOnce}

// Valid reports whether r is a known repeat mode ("" is not: use Normalized).
func (r MediaRepeat) Valid() bool { return r == RepeatLoop || r == RepeatOnce }

// Normalized returns r, or RepeatLoop for "" and unknown values.
func (r MediaRepeat) Normalized() MediaRepeat {
	if r.Valid() {
		return r
	}
	return RepeatLoop
}

// ParseMediaRepeat reads "loop" / "once" (case-insensitive).
func ParseMediaRepeat(s string) (MediaRepeat, error) {
	r := MediaRepeat(strings.ToLower(strings.TrimSpace(s)))
	if !r.Valid() {
		return "", fmt.Errorf("unknown repeat mode %q (loop|once)", s)
	}
	return r, nil
}

// RepeatMode is the item's effective Repeating choice: Repeat when set,
// else the default for its kind: videos loop (motion backgrounds), audio
// files play once.
func (m Media) RepeatMode() MediaRepeat {
	if m.Repeat.Valid() {
		return m.Repeat
	}
	if m.Kind == MediaAudio {
		return RepeatOnce
	}
	return RepeatLoop
}

// PlayablePath is the file the renderer can play for a video: the transcode
// when there is one, else the source if it is already .ogv, else "". Stills
// return "".
func (m Media) PlayablePath() string {
	if m.Kind != MediaVideo {
		return ""
	}
	if m.TranscodedPath != "" {
		return m.TranscodedPath
	}
	if IsPlayableVideo(m.Path) {
		return m.Path
	}
	return ""
}

// Background is the slide background this item provides, scaled by its
// Aspect (Fit) and looping by its effective Repeat (Loop, see RepeatMode),
// a video or audio file starting at its Volume / Mute. A
// still covers the slide. A video background names the source file (so a
// schedule saved on one computer still identifies the clip on another) with
// the thumbnail as its poster; the app substitutes the playable file when it
// has one (see ResolveVideo). An audio file plays over a black slide.
func (m Media) Background() Background {
	bg := Background{Color: RGBA{A: 1}, Path: m.Path, Fit: m.Aspect.Normalized()}
	switch m.Kind {
	case MediaVideo:
		bg.Kind = BackgroundVideo
		bg.Poster = m.ThumbPath
		bg.Loop = m.RepeatMode() == RepeatLoop
		bg.Volume, bg.Mute = ClampMediaVolume(m.Volume), m.Mute
	case MediaAudio:
		bg.Kind = BackgroundAudio
		bg.Loop = m.RepeatMode() == RepeatLoop
		bg.Volume, bg.Mute = ClampMediaVolume(m.Volume), m.Mute
	default:
		bg.Kind = BackgroundImage
	}
	return bg
}

// Slide is the single slide a media item shows: the background and nothing
// else, labelled with the item's title.
func (m Media) Slide() Slide {
	bg := m.Background()
	return Slide{Kind: SlideMedia, Label: m.Title, Background: &bg}
}

// ResolveVideo returns bg with a video's Path replaced by the playable file
// that lookup finds for its source path (a transcode made since the slide
// was built). A background that is not a video, or that lookup has nothing
// for, is returned unchanged.
func ResolveVideo(bg Background, lookup func(source string) (playable string, ok bool)) Background {
	if bg.Kind != BackgroundVideo || lookup == nil {
		return bg
	}
	if p, ok := lookup(bg.Path); ok && p != "" {
		bg.Path = p
	}
	return bg
}

// DefaultMediaTheme is the theme media items are shown with: a plain black
// background that the media covers.
func DefaultMediaTheme() Theme {
	t := DefaultSongTheme()
	t.Name = "Media"
	t.Background = Background{Kind: BackgroundColor, Color: RGBA{A: 1}}
	return t
}
