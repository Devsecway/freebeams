package core

import (
	"errors"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"

	"freebeams/internal/models"
)

// Restyling one item (the Preview Output monitor's context menu, UI-SPEC
// §3.4): the item in Preview gets its own copy of its theme, changed by the
// menu's Change Theme ▸, Background ▸ and Text ▸ entries or the item theme
// editor. The copy keeps the saved theme's ID and Name and is marked
// Theme.Custom while its look differs from that theme; Revert to Global
// Theme takes the saved theme back. The saved theme in the library is never
// changed. The events live in app/previewstyle.go.

// Font size limits and the Increase Font / Decrease Font step, in canvas
// units (the theme editor's Size field allows 8..400).
const (
	FontSizeStep = 8
	MinFontSize  = 16
	MaxFontSize  = 400
	minShrink    = 8 // the smallest Font.MinSize
)

// Restylable reports whether the theme decides how deck d looks: it has a
// slide with text (a song, a scripture or a blank slide). Media and
// presentation slides bring their own backgrounds and have no text, so the
// menu's restyling entries are disabled for them.
func Restylable(d *models.Deck) bool {
	if d == nil {
		return false
	}
	for _, s := range d.Slides {
		switch s.Kind {
		case models.SlideSong, models.SlideScripture, models.SlideBlank:
			return true
		}
	}
	return false
}

// DeckKind is the item type of deck d judged by its slides (song or
// scripture), for the default theme a revert falls back to; "" otherwise.
func DeckKind(d *models.Deck) models.ItemKind {
	if d == nil {
		return ""
	}
	for _, s := range d.Slides {
		switch s.Kind {
		case models.SlideSong:
			return models.ItemSong
		case models.SlideScripture:
			return models.ItemScripture
		}
	}
	return ""
}

// BaseTheme is the saved theme an item's copy t is based on: the library's
// current version of the theme with t's ID, else def (the default theme for
// the item's type: the theme was deleted, or the item came from elsewhere).
func BaseTheme(t models.Theme, list []models.Theme, def models.Theme) models.Theme {
	if i := ThemeIndexOf(list, t.ID); i >= 0 {
		return list[i]
	}
	return def
}

// markCustom marks t Custom when its look differs from base. t keeps its
// own ID and Name (the theme it was made from, even one deleted since).
func markCustom(t, base models.Theme) models.Theme {
	t.Custom = !t.SameLook(base)
	return t
}

// Customize returns t as an item's copy of base, Custom when the look
// differs (the item theme editor's OK / Apply).
func Customize(t, base models.Theme) models.Theme { return markCustom(t, base) }

// StyleEdit is one change to an item's look. Nil / zero fields are left
// alone; the Reset fields take that part from the base theme first.
type StyleEdit struct {
	ResetBackground bool
	Background      *models.Background // Select Media… / Solid Color…

	ResetText  bool
	FontFamily *string  // "" is the platform default
	FontSize   *float32 // canvas units
	FontStep   int      // +1 Increase Font, -1 Decrease Font
	FontColor  *models.RGBA
	Bold       *bool
	Italic     *bool
	Outline    *bool
	Shadow     *bool
	Align      *models.HAlign
}

// textFields copies the text part of a theme (everything but the background).
func textFields(dst *models.Theme, src models.Theme) {
	dst.Font, dst.Outline, dst.Shadow = src.Font, src.Outline, src.Shadow
	dst.HAlign, dst.VAlign, dst.Margins = src.HAlign, src.VAlign, src.Margins
	dst.FooterSize, dst.FooterColor = src.FooterSize, src.FooterColor
}

// setFontSize sets the maximum font size (clamped) and scales the auto-fit
// floor with it, so a slide that was shrunk to fit grows or shrinks too.
func setFontSize(f *models.Font, size float32) {
	size = float32(math.Round(float64(min(max(size, MinFontSize), MaxFontSize))))
	if f.Size > 0 && f.MinSize > 0 {
		f.MinSize = float32(math.Round(float64(f.MinSize * size / f.Size)))
	}
	f.Size = size
	f.MinSize = min(max(f.MinSize, minShrink), f.Size)
}

// RestyleTheme applies edit e to t, an item's theme based on base (see
// BaseTheme), and returns the new copy (Custom when it no longer looks like
// base). An outline or shadow switched on that has no size of its own takes
// the default song theme's.
func RestyleTheme(t, base models.Theme, e StyleEdit) models.Theme {
	if e.ResetBackground {
		t.Background = base.Background
	}
	if e.Background != nil {
		t.Background = *e.Background
	}
	if e.ResetText {
		textFields(&t, base)
	}
	if e.FontFamily != nil {
		t.Font.Family = strings.TrimSpace(*e.FontFamily)
	}
	if e.FontSize != nil {
		setFontSize(&t.Font, *e.FontSize)
	}
	if e.FontStep != 0 {
		setFontSize(&t.Font, t.Font.Size+float32(e.FontStep*FontSizeStep))
	}
	if e.FontColor != nil {
		t.Font.Color = *e.FontColor
	}
	if e.Bold != nil {
		t.Font.Bold = *e.Bold
	}
	if e.Italic != nil {
		t.Font.Italic = *e.Italic
	}
	def := models.DefaultSongTheme()
	if e.Outline != nil {
		t.Outline.Enabled = *e.Outline
		if t.Outline.Enabled && t.Outline.Size <= 0 {
			t.Outline.Size, t.Outline.Color = def.Outline.Size, def.Outline.Color
		}
	}
	if e.Shadow != nil {
		t.Shadow.Enabled = *e.Shadow
		if t.Shadow.Enabled && t.Shadow.OffsetX == 0 && t.Shadow.OffsetY == 0 && t.Shadow.Blur == 0 {
			t.Shadow = def.Shadow
		}
	}
	if e.Align != nil {
		t.HAlign = *e.Align
	}
	return markCustom(t, base)
}

// SolidBackground is a plain color background (Background ▸ Solid Color…).
func SolidBackground(c models.RGBA) models.Background {
	c.A = 1
	return models.Background{Kind: models.BackgroundColor, Color: c}
}

// ErrAudioBackground refuses an audio file as a slide background.
var ErrAudioBackground = errors.New("an audio file cannot be a background: choose an image or a video")

// MediaBackground is the theme background a library item provides
// (Background ▸ Select Media…): an image covers the slide, a video loops
// behind the text (scaled by the item's Aspect Ratio). Audio is refused.
func MediaBackground(m models.Media) (models.Background, error) {
	if m.Kind == models.MediaAudio {
		return models.Background{}, ErrAudioBackground
	}
	bg := m.Background()
	if bg.Kind == models.BackgroundVideo {
		bg.Loop = true // a motion background runs for as long as the item shows
	}
	return bg, nil
}

// BackgroundMedia lists the library items that can be a background
// (images and videos), in list order, for the Select Media… picker.
func BackgroundMedia(list []models.Media) []models.Media {
	var out []models.Media
	for _, m := range list {
		if m.Kind == models.MediaImage || m.Kind == models.MediaVideo {
			out = append(out, m)
		}
	}
	return out
}

// SameContent reports whether a and b are the same item with the same
// slides, whatever their themes (the restyled preview of a library item and
// a freshly built deck of it).
func SameContent(a, b *models.Deck) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.ItemID == b.ItemID && a.Title == b.Title && reflect.DeepEqual(a.Slides, b.Slides)
}

// CarryStyle gives fresh, a newly built deck of a library item, the restyled
// theme of old when old is the same item restyled on its own: the override
// goes with the item when it is shown, sent live or added to the schedule
// again. Otherwise fresh is returned unchanged.
func CarryStyle(old *models.Deck, fresh models.Deck) models.Deck {
	if old != nil && old.Theme.Custom && SameContent(old, &fresh) {
		fresh.Theme = old.Theme
	}
	return fresh
}

// ParseHexColor reads "#rrggbb" or "#rrggbbaa" (the # is optional).
func ParseHexColor(s string) (models.RGBA, error) {
	h := strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(h) != 6 && len(h) != 8 {
		return models.RGBA{}, fmt.Errorf("color %q: want #rrggbb or #rrggbbaa", s)
	}
	v, err := strconv.ParseUint(h, 16, 32)
	if err != nil {
		return models.RGBA{}, fmt.Errorf("color %q: want #rrggbb or #rrggbbaa", s)
	}
	if len(h) == 6 {
		return models.Hex(uint32(v)), nil
	}
	c := models.Hex(uint32(v >> 8))
	c.A = float32(v&0xFF) / 255
	return c, nil
}

// HexColor formats c as "#rrggbb", with an alpha byte when it is not opaque.
func HexColor(c models.RGBA) string {
	b := func(f float32) int { return int(math.Round(float64(min(max(f, 0), 1)) * 255)) }
	s := fmt.Sprintf("#%02x%02x%02x", b(c.R), b(c.G), b(c.B))
	if b(c.A) != 255 {
		s += fmt.Sprintf("%02x", b(c.A))
	}
	return s
}

// DeckStyle is the look of a deck in GET /state (preview.style /
// live.style): the fields the Preview Output menu changes.
type DeckStyle struct {
	Custom          bool    `json:"custom"`     // restyled on its own (Theme.Custom)
	Background      string  `json:"background"` // color | gradient | image | video
	BackgroundColor string  `json:"backgroundColor"`
	BackgroundPath  string  `json:"backgroundPath,omitempty"`
	Font            string  `json:"font"` // "" is the platform default
	Size            float32 `json:"size"`
	MinSize         float32 `json:"minSize"`
	Color           string  `json:"color"`
	Bold            bool    `json:"bold"`
	Italic          bool    `json:"italic"`
	Outline         bool    `json:"outline"`
	Shadow          bool    `json:"shadow"`
	Align           string  `json:"align"`
}

// StyleOf summarises t for GET /state.
func StyleOf(t models.Theme) DeckStyle {
	return DeckStyle{
		Custom:          t.Custom,
		Background:      string(t.Background.Kind),
		BackgroundColor: HexColor(t.Background.Color),
		BackgroundPath:  t.Background.Path,
		Font:            t.Font.Family,
		Size:            t.Font.Size,
		MinSize:         t.Font.MinSize,
		Color:           HexColor(t.Font.Color),
		Bold:            t.Font.Bold,
		Italic:          t.Font.Italic,
		Outline:         t.Outline.Enabled,
		Shadow:          t.Shadow.Enabled,
		Align:           string(t.HAlign),
	}
}

// StylePatch is the body of the preview-theme control action: the same
// changes as the menu, by value. Colors are "#rrggbb".
type StylePatch struct {
	Theme           *models.Theme `json:"theme"` // the whole copy (the item theme editor's OK)
	BackgroundColor string        `json:"backgroundColor"`
	ResetBackground bool          `json:"resetBackground"`
	ResetText       bool          `json:"resetText"`
	Font            *string       `json:"font"`
	Size            *float32      `json:"size"`
	Color           string        `json:"color"`
	Bold            *bool         `json:"bold"`
	Italic          *bool         `json:"italic"`
	Outline         *bool         `json:"outline"`
	Shadow          *bool         `json:"shadow"`
	Align           string        `json:"align"`
}

// ParseAlign reads left / center / right.
func ParseAlign(s string) (models.HAlign, error) {
	switch a := models.HAlign(strings.ToLower(strings.TrimSpace(s))); a {
	case models.AlignLeft, models.AlignCenter, models.AlignRight:
		return a, nil
	}
	return "", fmt.Errorf("align %q: want left, center or right", s)
}

// Edit converts the patch to a StyleEdit (Theme is applied separately).
func (p StylePatch) Edit() (StyleEdit, error) {
	e := StyleEdit{ResetBackground: p.ResetBackground, ResetText: p.ResetText, FontFamily: p.Font, FontSize: p.Size,
		Bold: p.Bold, Italic: p.Italic, Outline: p.Outline, Shadow: p.Shadow}
	if p.BackgroundColor != "" {
		c, err := ParseHexColor(p.BackgroundColor)
		if err != nil {
			return StyleEdit{}, fmt.Errorf("backgroundColor: %w", err)
		}
		bg := SolidBackground(c)
		e.Background = &bg
	}
	if p.Color != "" {
		c, err := ParseHexColor(p.Color)
		if err != nil {
			return StyleEdit{}, fmt.Errorf("color: %w", err)
		}
		e.FontColor = &c
	}
	if p.Align != "" {
		a, err := ParseAlign(p.Align)
		if err != nil {
			return StyleEdit{}, err
		}
		e.Align = &a
	}
	return e, nil
}
