package core

import (
	"fmt"
	"strings"
	"time"

	"freeworship/internal/models"
)

// Setting key for the one-off media registration on libraries that finished
// the Phase 1 import before the media library existed.
const SettingMediaImportDone = "import.easyworship.media.done"

// CacheEnv overrides the media cache directory (thumbnails, transcodes).
const CacheEnv = "FREEWORSHIP_CACHE"

// MediaCaps is what the machine can do with videos.
type MediaCaps struct {
	FFmpeg bool // ffmpeg and ffprobe found: videos get poster frames and metadata
	Theora bool // ffmpeg has the libtheora encoder: videos can be transcoded for playback
}

// MediaDeck builds the deck for a library media item: one slide showing the
// media, with a video's source path replaced by its playable file through
// lookup (nil: no substitution). ItemID is empty (library content).
func MediaDeck(m models.Media, lookup func(string) (string, bool)) models.Deck {
	s := m.Slide()
	bg := models.ResolveVideo(*s.Background, lookup)
	s.Background = &bg
	return models.Deck{Title: m.Title, Theme: models.DefaultMediaTheme(), Slides: []models.Slide{s}}
}

// MediaItem makes a schedule item for a media item. The slide snapshot
// keeps the source path (not the transcode), so the schedule stays portable;
// ResolveDeck substitutes the playable file when the item is shown.
func MediaItem(m models.Media) models.ScheduleItem {
	sub := "Image"
	if m.Kind == models.MediaVideo {
		sub = "Video"
		if m.Duration > 0 {
			sub += " · " + FormatDuration(m.Duration)
		}
	}
	return models.ScheduleItem{
		Kind:     models.ItemMedia,
		Title:    m.Title,
		Subtitle: sub,
		MediaID:  m.ID,
		Theme:    models.DefaultMediaTheme(),
		Slides:   []models.Slide{m.Slide()},
	}
}

// ResolveDeck returns d with every video background's path replaced by the
// playable file lookup finds (see models.ResolveVideo). d is not modified;
// a deck without video backgrounds is returned as is.
func ResolveDeck(d *models.Deck, lookup func(string) (string, bool)) *models.Deck {
	if d == nil || lookup == nil {
		return d
	}
	var out *models.Deck
	for i, s := range d.Slides {
		if s.Background == nil || s.Background.Kind != models.BackgroundVideo {
			continue
		}
		bg := models.ResolveVideo(*s.Background, lookup)
		if bg == *s.Background {
			continue
		}
		if out == nil {
			cp := *d
			cp.Slides = cloneSlides(d.Slides)
			out = &cp
		}
		out.Slides[i].Background = &bg
	}
	if out == nil {
		return d
	}
	return out
}

// StillBackground returns the background a thumbnail draws for bg: a video
// becomes its poster image (or the plain color without one). Other kinds are
// returned unchanged. nil stays nil.
func StillBackground(bg *models.Background) *models.Background {
	if bg == nil || bg.Kind != models.BackgroundVideo {
		return bg
	}
	still := models.Background{Kind: models.BackgroundColor, Color: bg.Color}
	if bg.Poster != "" {
		still = models.Background{Kind: models.BackgroundImage, Color: bg.Color, Path: bg.Poster}
	}
	return &still
}

// ThumbBackground is what the media grid draws for an item: its thumbnail
// file when made, else the image itself for a still, else the plain color.
func ThumbBackground(m models.Media) models.Background {
	bg := models.Background{Kind: models.BackgroundColor, Color: models.Hex(0x2A2A2A)}
	switch {
	case m.ThumbPath != "":
		bg.Kind, bg.Path = models.BackgroundImage, m.ThumbPath
	case m.Kind == models.MediaImage && m.Path != "":
		bg.Kind, bg.Path = models.BackgroundImage, m.Path
	}
	return bg
}

// FormatDuration formats a duration as m:ss or h:mm:ss ("" for zero).
func FormatDuration(d time.Duration) string {
	if d <= 0 {
		return ""
	}
	secs := int((d + time.Second/2) / time.Second)
	h, m, s := secs/3600, (secs%3600)/60, secs%60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

// MediaBadge is the short text on a media tile's right: a video's duration
// when it can play, its conversion progress while it is being transcoded
// (progress in (0,1)), or why it cannot play yet (failed: its derivation
// failed). Images have none.
func MediaBadge(m models.Media, playable bool, caps MediaCaps, progress float64, failed bool) string {
	if m.Kind != models.MediaVideo {
		return ""
	}
	switch {
	case progress > 0 && progress < 1:
		return fmt.Sprintf("Converting %d%%", int(progress*100))
	case playable:
		if d := FormatDuration(m.Duration); d != "" {
			return d
		}
		return "Video"
	case !caps.FFmpeg:
		return "Needs ffmpeg"
	case !caps.Theora:
		return "Needs libtheora"
	case failed:
		return "Unavailable"
	}
	return "Queued"
}

// MediaFooter is the Media tab's footer text: the counts for the listed
// collection, and what is holding videos back.
func MediaFooter(videos, images int, kind models.MediaKind, caps MediaCaps, working bool) string {
	var parts []string
	switch kind {
	case models.MediaVideo:
		parts = append(parts, plural(videos, "video", "videos"))
	case models.MediaImage:
		parts = append(parts, plural(images, "image", "images"))
	default:
		parts = append(parts, plural(videos+images, "item", "items"))
		if videos+images > 0 {
			parts = append(parts, plural(videos, "video", "videos"), plural(images, "image", "images"))
		}
	}
	switch {
	case working:
		parts = append(parts, "preparing media…")
	case videos > 0 && kind != models.MediaImage && !caps.FFmpeg:
		parts = append(parts, "ffmpeg not found: videos cannot play")
	case videos > 0 && kind != models.MediaImage && !caps.Theora:
		parts = append(parts, "ffmpeg has no Theora encoder: videos cannot play")
	}
	return strings.Join(parts, " · ")
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

// FindMedia returns the index of the item in list whose title matches title
// (exact, then prefix, then substring, case-insensitively), else the one
// whose path ends with title; -1 when none.
func FindMedia(list []models.Media, title string) int {
	t := strings.ToLower(strings.TrimSpace(title))
	if t == "" {
		return -1
	}
	for _, match := range []func(string) bool{
		func(s string) bool { return s == t },
		func(s string) bool { return strings.HasPrefix(s, t) },
		func(s string) bool { return strings.Contains(s, t) },
	} {
		for i, m := range list {
			if match(strings.ToLower(m.Title)) {
				return i
			}
		}
	}
	for i, m := range list {
		if strings.HasSuffix(strings.ToLower(m.Path), t) {
			return i
		}
	}
	return -1
}

// MediaIndexOf returns the index of the item with id in list, or -1.
func MediaIndexOf(list []models.Media, id int64) int {
	for i, m := range list {
		if m.ID == id {
			return i
		}
	}
	return -1
}

// Playables maps every video's source path to its playable file, for
// ResolveDeck / MediaDeck lookups. Videos without one are left out.
func Playables(list []models.Media) map[string]string {
	out := map[string]string{}
	for _, m := range list {
		if p := m.PlayablePath(); p != "" {
			out[m.Path] = p
		}
	}
	return out
}
