package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"freeworship/internal/models"
)

// Themes are stored as JSON (models.Theme) with their name duplicated in a
// column for listing. Two settings name the default theme per item type
// (DefaultThemeKey); EnsureDefaultThemes seeds the built-in themes and those
// settings on first launch.

// DefaultThemeKey is the setting that holds the default theme id for an item
// type: "theme.default.song" and "theme.default.scripture".
func DefaultThemeKey(kind models.ItemKind) string { return "theme.default." + string(kind) }

// DefaultThemeKinds are the item types that have a default theme.
var DefaultThemeKinds = []models.ItemKind{models.ItemSong, models.ItemScripture}

// BuiltinTheme is the built-in default theme for an item type.
func BuiltinTheme(kind models.ItemKind) models.Theme {
	if kind == models.ItemScripture {
		return models.DefaultScriptureTheme()
	}
	return models.DefaultSongTheme()
}

const themeColumns = `id, name, json`

// scanTheme reads one theme row (see themeColumns). The id and name columns
// win over the values inside the JSON.
func scanTheme(sc interface{ Scan(...any) error }) (models.Theme, error) {
	var (
		t    models.Theme
		id   int64
		name string
		js   string
	)
	if err := sc.Scan(&id, &name, &js); err != nil {
		return models.Theme{}, err
	}
	if err := json.Unmarshal([]byte(js), &t); err != nil {
		return models.Theme{}, fmt.Errorf("theme %d: decode: %w", id, err)
	}
	t.ID, t.Name = id, name
	return t, nil
}

// GetTheme loads one theme. A missing id returns ErrNotFound.
func (s *Store) GetTheme(ctx context.Context, id int64) (models.Theme, error) {
	t, err := scanTheme(s.db.QueryRowContext(ctx, `SELECT `+themeColumns+` FROM themes WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return models.Theme{}, fmt.Errorf("get theme %d: %w", id, ErrNotFound)
	}
	if err != nil {
		return models.Theme{}, fmt.Errorf("get theme %d: %w", id, err)
	}
	return t, nil
}

// ListThemes returns every theme ordered by name (case-insensitively), then id.
func (s *Store) ListThemes(ctx context.Context) ([]models.Theme, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+themeColumns+` FROM themes ORDER BY name COLLATE NOCASE, id`)
	if err != nil {
		return nil, fmt.Errorf("list themes: %w", err)
	}
	defer rows.Close()
	out := []models.Theme{}
	for rows.Next() {
		t, err := scanTheme(rows)
		if err != nil {
			return nil, fmt.Errorf("list themes: %w", err)
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list themes: %w", err)
	}
	return out, nil
}

// CountThemes returns the number of themes.
func (s *Store) CountThemes(ctx context.Context) (int, error) {
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM themes`).Scan(&n); err != nil {
		return 0, fmt.Errorf("count themes: %w", err)
	}
	return n, nil
}

// SaveTheme inserts (ID == 0) or updates a theme, assigning its ID. The name
// is trimmed and required. Updating an ID that does not exist returns
// ErrNotFound.
func (s *Store) SaveTheme(ctx context.Context, t *models.Theme) error {
	if t == nil {
		return errors.New("save theme: nil theme")
	}
	cp := *t
	cp.Name = strings.TrimSpace(cp.Name)
	if cp.Name == "" {
		return errors.New("save theme: name is required")
	}
	err := s.inTx(ctx, func(tx *sql.Tx) error {
		return saveThemeTx(ctx, tx, &cp, time.Now().UTC())
	})
	if err != nil {
		return err
	}
	*t = cp
	return nil
}

func saveThemeTx(ctx context.Context, tx *sql.Tx, t *models.Theme, now time.Time) error {
	stamp := now.Format(timeLayout)
	if t.ID == 0 {
		res, err := tx.ExecContext(ctx, `INSERT INTO themes (name, json, created, updated) VALUES (?, '{}', ?, ?)`,
			t.Name, stamp, stamp)
		if err != nil {
			return fmt.Errorf("save theme %q: %w", t.Name, err)
		}
		id, err := res.LastInsertId()
		if err != nil {
			return fmt.Errorf("save theme %q: %w", t.Name, err)
		}
		t.ID = id
	}
	// The stored JSON carries the id and name too, so a row is self-contained.
	js, err := json.Marshal(t)
	if err != nil {
		return fmt.Errorf("save theme %q: encode: %w", t.Name, err)
	}
	res, err := tx.ExecContext(ctx, `UPDATE themes SET name = ?, json = ?, updated = ? WHERE id = ?`,
		t.Name, string(js), stamp, t.ID)
	if err != nil {
		return fmt.Errorf("save theme %d: %w", t.ID, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("save theme %d: %w", t.ID, err)
	}
	if n == 0 {
		return fmt.Errorf("save theme %d: %w", t.ID, ErrNotFound)
	}
	return nil
}

// DeleteTheme removes a theme. A missing id returns ErrNotFound. Schedule
// items keep their snapshot of the theme; a default-theme setting that
// pointed at it is cleared, so GetDefaultTheme falls back to the built-in.
func (s *Store) DeleteTheme(ctx context.Context, id int64) error {
	return s.inTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `DELETE FROM themes WHERE id = ?`, id)
		if err != nil {
			return fmt.Errorf("delete theme %d: %w", id, err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("delete theme %d: %w", id, err)
		}
		if n == 0 {
			return fmt.Errorf("delete theme %d: %w", id, ErrNotFound)
		}
		for _, kind := range DefaultThemeKinds {
			if _, err := tx.ExecContext(ctx, `DELETE FROM settings WHERE key = ? AND value = ?`,
				DefaultThemeKey(kind), fmt.Sprint(id)); err != nil {
				return fmt.Errorf("delete theme %d: clear default: %w", id, err)
			}
		}
		return nil
	})
}

// DefaultThemeID returns the configured default theme id for an item type
// (0 when unset).
func (s *Store) DefaultThemeID(ctx context.Context, kind models.ItemKind) (int64, error) {
	var id int64
	if _, err := s.GetSetting(ctx, DefaultThemeKey(kind), &id); err != nil {
		return 0, fmt.Errorf("default %s theme: %w", kind, err)
	}
	return id, nil
}

// SetDefaultTheme makes theme id the default for an item type. The theme
// must exist.
func (s *Store) SetDefaultTheme(ctx context.Context, kind models.ItemKind, id int64) error {
	if kind != models.ItemSong && kind != models.ItemScripture {
		return fmt.Errorf("set default theme: no default for %q items", kind)
	}
	if _, err := s.GetTheme(ctx, id); err != nil {
		return fmt.Errorf("set default %s theme: %w", kind, err)
	}
	return s.SetSetting(ctx, DefaultThemeKey(kind), id)
}

// GetDefaultTheme returns the default theme for an item type: the configured
// one when it exists, else the built-in (with ID 0).
func (s *Store) GetDefaultTheme(ctx context.Context, kind models.ItemKind) (models.Theme, error) {
	id, err := s.DefaultThemeID(ctx, kind)
	if err != nil {
		return models.Theme{}, err
	}
	if id != 0 {
		t, err := s.GetTheme(ctx, id)
		if err == nil {
			return t, nil
		}
		if !errors.Is(err, ErrNotFound) {
			return models.Theme{}, fmt.Errorf("default %s theme: %w", kind, err)
		}
	}
	return BuiltinTheme(kind), nil
}

// EnsureDefaultThemes seeds the library on first launch: when there are no
// themes, the built-in song and scripture themes are inserted; an unset
// default-theme setting is then pointed at the theme named like the matching
// built-in (inserting it when there is none). It reports whether anything was
// written.
func (s *Store) EnsureDefaultThemes(ctx context.Context) (bool, error) {
	list, err := s.ListThemes(ctx)
	if err != nil {
		return false, fmt.Errorf("ensure default themes: %w", err)
	}
	wrote := false
	byName := map[string]int64{}
	for _, t := range list {
		if _, dup := byName[t.Name]; !dup {
			byName[t.Name] = t.ID
		}
	}
	if len(list) == 0 {
		for _, kind := range DefaultThemeKinds {
			t := BuiltinTheme(kind)
			if err := s.SaveTheme(ctx, &t); err != nil {
				return wrote, fmt.Errorf("ensure default themes: %w", err)
			}
			byName[t.Name] = t.ID
			wrote = true
		}
	}
	for _, kind := range DefaultThemeKinds {
		id, err := s.DefaultThemeID(ctx, kind)
		if err != nil {
			return wrote, fmt.Errorf("ensure default themes: %w", err)
		}
		if id != 0 {
			continue
		}
		t := BuiltinTheme(kind)
		if existing, ok := byName[t.Name]; ok {
			id = existing
		} else {
			if err := s.SaveTheme(ctx, &t); err != nil {
				return wrote, fmt.Errorf("ensure default themes: %w", err)
			}
			id = t.ID
			byName[t.Name] = id
		}
		if err := s.SetSetting(ctx, DefaultThemeKey(kind), id); err != nil {
			return wrote, fmt.Errorf("ensure default themes: %w", err)
		}
		wrote = true
	}
	return wrote, nil
}
