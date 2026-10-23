package core

import (
	"fmt"
	"strings"

	"freebeams/internal/models"
)

// Themes: the sample content the Themes tab and the theme editor render,
// theme lookups, and how a changed theme reaches the schedule and the decks.

// SampleSlide is the lyric every theme is previewed with (thumbnails, the
// resource preview and the editor's live preview): four lines and a footer.
func SampleSlide() models.Slide {
	return models.Slide{
		Kind:        models.SlideSong,
		Label:       "Verse 1",
		Group:       models.SectionVerse,
		GroupNumber: 1,
		Text:        "Amazing grace! How sweet the sound\nThat saved a wretch like me!\nI once was lost, but now am found;\nWas blind, but now I see.",
		Footer:      "Amazing Grace · John Newton · Public Domain",
	}
}

// ThemeDeck is the one-slide deck that shows a theme (SampleSlide drawn with
// it), titled with the theme's name. ItemID is empty: it is library content.
func ThemeDeck(t models.Theme) models.Deck {
	return models.Deck{Title: t.Name, Theme: t, Slides: []models.Slide{SampleSlide()}}
}

// SongDeckWith is SongDeck with an explicit theme (the configured default).
func SongDeckWith(song models.Song, theme models.Theme) models.Deck {
	d := SongDeck(song)
	d.Theme = theme
	return d
}

// ScriptureDeckWith is ScriptureDeck with an explicit theme.
func ScriptureDeckWith(p models.Passage, bibleAbbrev string, verses []models.Verse, theme models.Theme) models.Deck {
	d := ScriptureDeck(p, bibleAbbrev, verses)
	d.Theme = theme
	return d
}

// FindTheme returns the index of the theme in list whose name matches name
// (exact, then prefix, then substring, case-insensitively); -1 when none.
func FindTheme(list []models.Theme, name string) int {
	n := strings.ToLower(strings.TrimSpace(name))
	if n == "" {
		return -1
	}
	for _, match := range []func(string) bool{
		func(s string) bool { return s == n },
		func(s string) bool { return strings.HasPrefix(s, n) },
		func(s string) bool { return strings.Contains(s, n) },
	} {
		for i, t := range list {
			if match(strings.ToLower(t.Name)) {
				return i
			}
		}
	}
	return -1
}

// ThemeIndexOf returns the index of the theme with id in list, or -1 (also
// for id 0, which no stored theme has).
func ThemeIndexOf(list []models.Theme, id int64) int {
	if id == 0 {
		return -1
	}
	for i, t := range list {
		if t.ID == id {
			return i
		}
	}
	return -1
}

// CopyName is the name for a duplicate of name that is not in taken: "X
// Copy", then "X Copy 2", "X Copy 3", … (case-insensitively unique).
func CopyName(name string, taken []string) string {
	used := make(map[string]bool, len(taken))
	for _, t := range taken {
		used[strings.ToLower(strings.TrimSpace(t))] = true
	}
	base := strings.TrimSpace(name)
	if base == "" {
		base = "Theme"
	}
	cand := base + " Copy"
	for n := 2; used[strings.ToLower(cand)]; n++ {
		cand = fmt.Sprintf("%s Copy %d", base, n)
	}
	return cand
}

// ThemeBadge is the tile badge that marks the default themes: "Songs",
// "Scripture", "Songs · Scripture" or "".
func ThemeBadge(t models.Theme, defaults map[models.ItemKind]int64) string {
	if t.ID == 0 {
		return ""
	}
	var parts []string
	if defaults[models.ItemSong] == t.ID {
		parts = append(parts, "Songs")
	}
	if defaults[models.ItemScripture] == t.ID {
		parts = append(parts, "Scripture")
	}
	return strings.Join(parts, " · ")
}

// IsDefaultTheme reports whether t is the default for any item type.
func IsDefaultTheme(t models.Theme, defaults map[models.ItemKind]int64) bool {
	return ThemeBadge(t, defaults) != ""
}

// ThemeCountText is the Themes footer text ("2 themes").
func ThemeCountText(n int) string { return plural(n, "theme", "themes") }

// RestyleSchedule gives every schedule item that uses theme t (matched by
// ID) the new look, keeping the item's own slides. An item restyled on its
// own (Theme.Custom, the Preview Output menu) keeps its copy. It returns the
// number of items changed; the schedule is marked dirty when that is more
// than zero.
func RestyleSchedule(s *models.Schedule, t models.Theme) int {
	if s == nil || t.ID == 0 {
		return 0
	}
	n := 0
	for i := range s.Items {
		if s.Items[i].Theme.ID == t.ID && !s.Items[i].Theme.Custom && s.Items[i].Theme != t {
			s.Items[i].Theme = t
			n++
		}
	}
	if n > 0 {
		s.Dirty = true
	}
	return n
}

// ApplyItemTheme sets the theme of schedule item i (the per-item override)
// and marks the schedule dirty. It reports false for an index out of range.
func ApplyItemTheme(s *models.Schedule, i int, t models.Theme) bool {
	if s == nil || i < 0 || i >= len(s.Items) {
		return false
	}
	if s.Items[i].Theme != t {
		s.Items[i].Theme = t
		s.Dirty = true
	}
	return true
}

// RestyleByThemeID is a Presenter.RestyleDecks callback: decks drawn with
// theme t.ID get t, except a deck restyled on its own (Theme.Custom).
func RestyleByThemeID(t models.Theme) func(*models.Deck) (models.Theme, bool) {
	return func(d *models.Deck) (models.Theme, bool) {
		if t.ID != 0 && d.Theme.ID == t.ID && !d.Theme.Custom {
			return t, true
		}
		return models.Theme{}, false
	}
}

// RestyleByItemID is a Presenter.RestyleDecks callback: the deck of schedule
// item itemID gets t.
func RestyleByItemID(itemID string, t models.Theme) func(*models.Deck) (models.Theme, bool) {
	return func(d *models.Deck) (models.Theme, bool) {
		if itemID != "" && d.ItemID == itemID {
			return t, true
		}
		return models.Theme{}, false
	}
}

// ThemesState summarises the Themes tab for GET /state.
type ThemesState struct {
	Count            int      `json:"count"`
	Names            []string `json:"names"`
	Selected         int      `json:"selected"`
	Current          string   `json:"current,omitempty"` // name of the selected theme
	SongDefault      string   `json:"songDefault"`
	ScriptureDefault string   `json:"scriptureDefault"`
	CountText        string   `json:"countText"`
	Editor           bool     `json:"editor"`            // the theme editor is open
	Editing          string   `json:"editing,omitempty"` // name of the theme in the editor

	// The theme polish (app/themepolish.go): the editor's apply choice, the
	// edited theme's background and band, and the font search.
	ApplyToSchedule bool               `json:"applyToSchedule"`
	EditBackground  *models.Background `json:"editBackground,omitempty"`
	EditBox         *models.TextBox    `json:"editBox,omitempty"`
	EditFont        string             `json:"editFont,omitempty"`
	FontList        bool               `json:"fontList,omitempty"`    // the font list is open
	FontMatches     int                `json:"fontMatches,omitempty"` // entries it shows
}

// BuildThemesState fills a ThemesState from the listed themes and defaults.
func BuildThemesState(list []models.Theme, selected int, defaults map[models.ItemKind]int64, editorOpen bool, editing string) ThemesState {
	st := ThemesState{Count: len(list), Names: make([]string, 0, len(list)), Selected: selected,
		CountText: ThemeCountText(len(list)), Editor: editorOpen}
	for _, t := range list {
		st.Names = append(st.Names, t.Name)
	}
	if selected >= 0 && selected < len(list) {
		st.Current = list[selected].Name
	}
	if i := ThemeIndexOf(list, defaults[models.ItemSong]); i >= 0 {
		st.SongDefault = list[i].Name
	}
	if i := ThemeIndexOf(list, defaults[models.ItemScripture]); i >= 0 {
		st.ScriptureDefault = list[i].Name
	}
	if editorOpen {
		st.Editing = editing
	}
	return st
}

// FollowsDefault reports whether library deck d (not a schedule item) shows
// the default theme oldDefault unchanged, so it takes a new default theme of
// its kind. A deck restyled on its own (Theme.Custom) keeps its look.
func FollowsDefault(d *models.Deck, oldDefault int64) bool {
	return d != nil && d.ItemID == "" && d.Theme.ID == oldDefault && !d.Theme.Custom
}
