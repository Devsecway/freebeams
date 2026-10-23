package store

import (
	"context"
	"fmt"
	"strings"

	"freebeams/internal/models"
)

// BuiltinThemesKey records the models.BuiltinThemesVersion a library was
// given, so the built-in themes are added once: a theme the operator
// deletes or renames afterwards is not brought back.
const BuiltinThemesKey = "theme.builtins"

// EnsureBuiltinThemes adds the built-in themes (models.BuiltinThemes) that
// the library has not been given yet, skipping any whose name is taken. It
// returns the names it inserted.
func (s *Store) EnsureBuiltinThemes(ctx context.Context) ([]string, error) {
	var have int
	if _, err := s.GetSetting(ctx, BuiltinThemesKey, &have); err != nil {
		return nil, fmt.Errorf("ensure built-in themes: %w", err)
	}
	if have >= models.BuiltinThemesVersion {
		return nil, nil
	}
	list, err := s.ListThemes(ctx)
	if err != nil {
		return nil, fmt.Errorf("ensure built-in themes: %w", err)
	}
	taken := map[string]bool{}
	for _, t := range list {
		taken[strings.ToLower(t.Name)] = true
	}
	var added []string
	for _, t := range models.BuiltinThemes() {
		if taken[strings.ToLower(t.Name)] {
			continue
		}
		if err := s.SaveTheme(ctx, &t); err != nil {
			return added, fmt.Errorf("ensure built-in themes: %w", err)
		}
		added = append(added, t.Name)
	}
	if err := s.SetSetting(ctx, BuiltinThemesKey, models.BuiltinThemesVersion); err != nil {
		return added, fmt.Errorf("ensure built-in themes: %w", err)
	}
	return added, nil
}
