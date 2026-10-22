package core

import (
	"strings"

	"freebeams/internal/models"
)

// Per-song themes (EasyWorship's "Theme" of a song): a song deck is drawn
// with the song's own theme when it has one that still exists, else with the
// configured default song theme. A theme id that names no theme (deleted, or
// carried in from elsewhere) is ignored, never an error.

// DefaultThemeLabel is the picker entry that means "no theme of its own: use
// the default song theme" (Song.ThemeID 0).
const DefaultThemeLabel = "(Default)"

// SongTheme resolves the theme a song deck uses: the theme with the song's
// ThemeID when list has it, else def (the default song theme).
func SongTheme(song models.Song, list []models.Theme, def models.Theme) models.Theme {
	if i := ThemeIndexOf(list, song.ThemeID); i >= 0 {
		return list[i]
	}
	return def
}

// SongThemeName is the name of the song's own theme, or "" when the song
// uses the default (ThemeID 0 or an id that names no theme in list).
func SongThemeName(themeID int64, list []models.Theme) string {
	if i := ThemeIndexOf(list, themeID); i >= 0 {
		return list[i].Name
	}
	return ""
}

// SongThemeArg resolves a theme name given to a control action to a
// Song.ThemeID: "" and "(Default)" are 0 (the default song theme), anything
// else must match a theme (FindTheme). ok is false when nothing matches.
func SongThemeArg(list []models.Theme, name string) (id int64, ok bool) {
	n := strings.TrimSpace(name)
	if n == "" || strings.EqualFold(n, DefaultThemeLabel) {
		return 0, true
	}
	i := FindTheme(list, n)
	if i < 0 {
		return 0, false
	}
	return list[i].ID, true
}

// RestyleLibraryDeck is a Presenter.RestyleDecks callback: a library deck
// (not a schedule item) that shows exactly old gets t. It re-themes a song in
// Preview after its theme changed, keeping the preview selection.
func RestyleLibraryDeck(old models.Deck, t models.Theme) func(*models.Deck) (models.Theme, bool) {
	return func(d *models.Deck) (models.Theme, bool) {
		if d != nil && d.ItemID == "" && old.ItemID == "" && SameDeck(d, &old) {
			return t, true
		}
		return models.Theme{}, false
	}
}

// ForgetSongTheme clears ThemeID on every row that uses theme id (it was
// deleted; the store clears the songs too) and reports how many changed.
func ForgetSongTheme(rows []models.SongSummary, id int64) int {
	if id == 0 {
		return 0
	}
	n := 0
	for i := range rows {
		if rows[i].ThemeID == id {
			rows[i].ThemeID = 0
			n++
		}
	}
	return n
}
