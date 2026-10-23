package core

import (
	"fmt"
	"strings"

	"freebeams/internal/models"
)

// Theme polish: EasyWorship's "Apply changes to items in schedule" choice,
// media as a theme background, and names for imported themes.

// ApplyToScheduleKey is the setting that remembers the theme editor's
// "Apply changes to items in schedule" checkbox (default on).
const ApplyToScheduleKey = "theme.applyToSchedule"

// RestyleLibraryByThemeID is RestyleByThemeID for library decks only (no
// ItemID): with "Apply changes to items in schedule" off, a Preview or Live
// deck that came from the schedule keeps its snapshot.
func RestyleLibraryByThemeID(t models.Theme) func(*models.Deck) (models.Theme, bool) {
	all := RestyleByThemeID(t)
	return func(d *models.Deck) (models.Theme, bool) {
		if d.ItemID != "" {
			return models.Theme{}, false
		}
		return all(d)
	}
}

// ThemeRestyler is the Presenter.RestyleDecks callback for a saved theme:
// every deck drawn with it, or only the library decks when the schedule
// keeps its snapshots.
func ThemeRestyler(t models.Theme, applyToSchedule bool) func(*models.Deck) (models.Theme, bool) {
	if applyToSchedule {
		return RestyleByThemeID(t)
	}
	return RestyleLibraryByThemeID(t)
}

// WithMediaBackground returns t with library item m as its background
// (MediaBackground: an image, or a looping video, letterboxed on black when
// fitted). Audio is refused.
func WithMediaBackground(t models.Theme, m models.Media) (models.Theme, error) {
	bg, err := MediaBackground(m)
	if err != nil {
		return t, err
	}
	t.Background = bg
	return t, nil
}

// MediaTheme is the new theme "Use As Theme Background" opens the editor
// on: base (the default song theme) with item m as the background, named
// after the item and unique among taken.
func MediaTheme(base models.Theme, m models.Media, taken []string) (models.Theme, error) {
	t, err := WithMediaBackground(base, m)
	if err != nil {
		return base, err
	}
	t.ID, t.Custom = 0, false
	t.Name = UniqueName(strings.TrimSpace(m.Title), taken)
	return t, nil
}

// UniqueName is name when no theme in taken has it (case-insensitively),
// else "name 2", "name 3", … An empty name becomes "Theme".
func UniqueName(name string, taken []string) string {
	used := make(map[string]bool, len(taken))
	for _, t := range taken {
		used[strings.ToLower(strings.TrimSpace(t))] = true
	}
	base := strings.TrimSpace(name)
	if base == "" {
		base = "Theme"
	}
	cand := base
	for n := 2; used[strings.ToLower(cand)]; n++ {
		cand = fmt.Sprintf("%s %d", base, n)
	}
	return cand
}
