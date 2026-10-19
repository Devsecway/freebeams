package models

import (
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

// Background is the slide background this item provides. A still covers the
// slide. A video background names the source file (so a schedule saved on
// one computer still identifies the clip on another) with the thumbnail as
// its poster; the app substitutes the playable file when it has one (see
// ResolveVideo). A video loops by default (motion backgrounds); an audio
// file plays once over a black slide.
func (m Media) Background() Background {
	bg := Background{Color: RGBA{A: 1}, Path: m.Path}
	switch m.Kind {
	case MediaVideo:
		bg.Kind = BackgroundVideo
		bg.Poster = m.ThumbPath
		bg.Loop = true
	case MediaAudio:
		bg.Kind = BackgroundAudio
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
