package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"freeworship/internal/models"
)

const mediaColumns = `id, path, kind, title, duration, width, height, thumb_path, transcoded_path, added`

// scanMedia reads one media row (see mediaColumns).
func scanMedia(sc interface{ Scan(...any) error }) (models.Media, error) {
	var (
		m          models.Media
		durationMS int64
		added      string
	)
	if err := sc.Scan(&m.ID, &m.Path, &m.Kind, &m.Title, &durationMS, &m.Width, &m.Height,
		&m.ThumbPath, &m.TranscodedPath, &added); err != nil {
		return models.Media{}, err
	}
	m.Duration = time.Duration(durationMS) * time.Millisecond
	m.Added, _ = time.Parse(timeLayout, added)
	return m, nil
}

// GetMedia loads one media item. A missing id returns ErrNotFound.
func (s *Store) GetMedia(ctx context.Context, id int64) (models.Media, error) {
	m, err := scanMedia(s.db.QueryRowContext(ctx, `SELECT `+mediaColumns+` FROM media WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return models.Media{}, fmt.Errorf("get media %d: %w", id, ErrNotFound)
	}
	if err != nil {
		return models.Media{}, fmt.Errorf("get media %d: %w", id, err)
	}
	return m, nil
}

// MediaByPath loads the media item registered for a file. A path that is not
// in the library returns ErrNotFound.
func (s *Store) MediaByPath(ctx context.Context, path string) (models.Media, error) {
	m, err := scanMedia(s.db.QueryRowContext(ctx, `SELECT `+mediaColumns+` FROM media WHERE path = ?`, path))
	if errors.Is(err, sql.ErrNoRows) {
		return models.Media{}, fmt.Errorf("media %q: %w", path, ErrNotFound)
	}
	if err != nil {
		return models.Media{}, fmt.Errorf("media %q: %w", path, err)
	}
	return m, nil
}

// SaveMedia inserts (ID == 0) or updates a media item, assigning ID and
// Added. The path is unique: inserting a path that is already registered
// returns ErrExists, and updating an ID that does not exist returns
// ErrNotFound. An empty Title is derived from the path.
func (s *Store) SaveMedia(ctx context.Context, m *models.Media) error {
	if m == nil {
		return errors.New("save media: nil item")
	}
	return s.SaveMediaItems(ctx, []*models.Media{m})
}

// SaveMediaItems saves several items (insert or update, as SaveMedia) in one
// transaction: either all are saved or none. Used by importers and folder scans.
func (s *Store) SaveMediaItems(ctx context.Context, items []*models.Media) error {
	now := time.Now().UTC()
	copies := make([]models.Media, len(items))
	err := s.inTx(ctx, func(tx *sql.Tx) error {
		for i, m := range items {
			if m == nil {
				return fmt.Errorf("save media: item %d is nil", i)
			}
			copies[i] = *m
			if err := saveMediaTx(ctx, tx, &copies[i], now); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	for i := range items {
		*items[i] = copies[i]
	}
	return nil
}

func saveMediaTx(ctx context.Context, tx *sql.Tx, m *models.Media, now time.Time) error {
	path := strings.TrimSpace(m.Path)
	if path == "" {
		return errors.New("save media: path is required")
	}
	if m.Kind == "" {
		kind, ok := models.MediaKindOf(path)
		if !ok {
			return fmt.Errorf("save media %q: not an image or video file", path)
		}
		m.Kind = kind
	}
	if m.Kind != models.MediaImage && m.Kind != models.MediaVideo {
		return fmt.Errorf("save media %q: unknown kind %q", path, m.Kind)
	}
	title := strings.TrimSpace(m.Title)
	if title == "" {
		title = models.MediaTitle(path)
	}
	durationMS := m.Duration.Milliseconds()
	if durationMS < 0 {
		durationMS = 0
	}
	if m.ID == 0 {
		added := m.Added
		if added.IsZero() {
			added = now
		}
		res, err := tx.ExecContext(ctx, `
			INSERT INTO media (path, kind, title, duration, width, height, thumb_path, transcoded_path, added)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			path, string(m.Kind), title, durationMS, m.Width, m.Height, m.ThumbPath, m.TranscodedPath,
			added.UTC().Format(timeLayout))
		if err != nil {
			if isUniqueViolation(err) {
				return fmt.Errorf("save media %q: %w", path, ErrExists)
			}
			return fmt.Errorf("save media %q: %w", path, err)
		}
		id, err := res.LastInsertId()
		if err != nil {
			return fmt.Errorf("save media %q: %w", path, err)
		}
		m.ID, m.Added = id, added.UTC()
	} else {
		var added string
		err := tx.QueryRowContext(ctx, `SELECT added FROM media WHERE id = ?`, m.ID).Scan(&added)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("save media %d: %w", m.ID, ErrNotFound)
		}
		if err != nil {
			return fmt.Errorf("save media %d: %w", m.ID, err)
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE media SET path = ?, kind = ?, title = ?, duration = ?, width = ?, height = ?,
				thumb_path = ?, transcoded_path = ?
			WHERE id = ?`,
			path, string(m.Kind), title, durationMS, m.Width, m.Height, m.ThumbPath, m.TranscodedPath, m.ID); err != nil {
			if isUniqueViolation(err) {
				return fmt.Errorf("save media %d: path %q: %w", m.ID, path, ErrExists)
			}
			return fmt.Errorf("save media %d: %w", m.ID, err)
		}
		if t, err := time.Parse(timeLayout, added); err == nil {
			m.Added = t
		}
	}
	m.Path, m.Title = path, title
	m.Duration = time.Duration(durationMS) * time.Millisecond
	return nil
}

// DeleteMedia removes a media item from the library (the files on disk are
// left alone). A missing id returns ErrNotFound.
func (s *Store) DeleteMedia(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM media WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete media %d: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete media %d: %w", id, err)
	}
	if n == 0 {
		return fmt.Errorf("delete media %d: %w", id, ErrNotFound)
	}
	return nil
}

// SearchMedia lists media items whose title or path contains every word of
// query (case-insensitive), of the given kind ("" for both), ordered by
// title. An empty query lists all. limit <= 0 means no limit.
func (s *Store) SearchMedia(ctx context.Context, query string, kind models.MediaKind, limit int) ([]models.Media, error) {
	if limit <= 0 {
		limit = -1
	}
	var (
		where []string
		args  []any
	)
	if kind != "" {
		where = append(where, `kind = ?`)
		args = append(args, string(kind))
	}
	for _, w := range strings.Fields(strings.ToLower(query)) {
		pat := "%" + likeEscape(w) + "%"
		where = append(where, `(lower(title) LIKE ? ESCAPE '\' OR lower(path) LIKE ? ESCAPE '\')`)
		args = append(args, pat, pat)
	}
	q := `SELECT ` + mediaColumns + ` FROM media`
	if len(where) > 0 {
		q += ` WHERE ` + strings.Join(where, ` AND `)
	}
	q += ` ORDER BY title COLLATE NOCASE, id LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("search media %q: %w", query, err)
	}
	defer rows.Close()
	out := []models.Media{}
	for rows.Next() {
		m, err := scanMedia(rows)
		if err != nil {
			return nil, fmt.Errorf("search media %q: %w", query, err)
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("search media %q: %w", query, err)
	}
	return out, nil
}

// likeEscape escapes the LIKE wildcards in a literal word (ESCAPE '\').
func likeEscape(w string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(w)
}

// CountMedia returns the number of media items of a kind ("" for all).
func (s *Store) CountMedia(ctx context.Context, kind models.MediaKind) (int, error) {
	var (
		n   int
		err error
	)
	if kind == "" {
		err = s.db.QueryRowContext(ctx, `SELECT count(*) FROM media`).Scan(&n)
	} else {
		err = s.db.QueryRowContext(ctx, `SELECT count(*) FROM media WHERE kind = ?`, string(kind)).Scan(&n)
	}
	if err != nil {
		return 0, fmt.Errorf("count media: %w", err)
	}
	return n, nil
}
