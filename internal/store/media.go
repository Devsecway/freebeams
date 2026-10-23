package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"freebeams/internal/models"
)

const mediaColumns = `id, path, kind, title, duration, width, height, thumb_path, transcoded_path, added, aspect, repeat, tags`

// scanMedia reads one media row (see mediaColumns).
func scanMedia(sc interface{ Scan(...any) error }) (models.Media, error) {
	var (
		m          models.Media
		durationMS int64
		added      string
		aspect     string
		repeat     string
		tags       string
	)
	if err := sc.Scan(&m.ID, &m.Path, &m.Kind, &m.Title, &durationMS, &m.Width, &m.Height,
		&m.ThumbPath, &m.TranscodedPath, &added, &aspect, &repeat, &tags); err != nil {
		return models.Media{}, err
	}
	m.Duration = time.Duration(durationMS) * time.Millisecond
	m.Added, _ = time.Parse(timeLayout, added)
	m.Aspect, m.Repeat = models.MediaAspect(aspect).Normalized(), models.MediaRepeat(repeat).Normalized()
	m.Tags = tags
	return m, nil
}

// SetMediaProperties saves the Properties dialog: the title ("" keeps the
// current one), the tags, Aspect Ratio and Repeating, without touching
// the derived columns the pipeline writes. A missing id returns ErrNotFound.
func (s *Store) SetMediaProperties(ctx context.Context, id int64, p models.MediaProperties) error {
	if p.Aspect != "" && !p.Aspect.Valid() {
		return fmt.Errorf("set media %d properties: unknown aspect ratio %q", id, p.Aspect)
	}
	if p.Repeat != "" && !p.Repeat.Valid() {
		return fmt.Errorf("set media %d properties: unknown repeat mode %q", id, p.Repeat)
	}
	res, err := s.db.ExecContext(ctx, `UPDATE media SET
			title = CASE WHEN ? = '' THEN title ELSE ? END, tags = ?, aspect = ?, repeat = ? WHERE id = ?`,
		strings.TrimSpace(p.Title), strings.TrimSpace(p.Title), models.NormalizeTags(p.Tags),
		string(p.Aspect.Normalized()), string(p.Repeat.Normalized()), id)
	if err != nil {
		return fmt.Errorf("set media %d properties: %w", id, err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return fmt.Errorf("set media %d properties: %w", id, ErrNotFound)
	}
	return nil
}

// SetMediaOptions sets an item's Aspect Ratio and Repeating choices without
// touching anything else (the pipeline rewrites whole rows: see SaveMedia).
// "" keeps the default. A missing id returns ErrNotFound.
func (s *Store) SetMediaOptions(ctx context.Context, id int64, aspect models.MediaAspect, repeat models.MediaRepeat) error {
	if aspect != "" && !aspect.Valid() {
		return fmt.Errorf("set media %d options: unknown aspect ratio %q", id, aspect)
	}
	if repeat != "" && !repeat.Valid() {
		return fmt.Errorf("set media %d options: unknown repeat mode %q", id, repeat)
	}
	res, err := s.db.ExecContext(ctx, `UPDATE media SET aspect = ?, repeat = ? WHERE id = ?`,
		string(aspect.Normalized()), string(repeat.Normalized()), id)
	if err != nil {
		return fmt.Errorf("set media %d options: %w", id, err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return fmt.Errorf("set media %d options: %w", id, ErrNotFound)
	}
	return nil
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
// ErrNotFound. An empty Title is derived from the path; an empty Aspect or
// Repeat is the default (fill, loop), and unknown values are rejected.
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
			return fmt.Errorf("save media %q: not an image, video or audio file", path)
		}
		m.Kind = kind
	}
	if m.Kind != models.MediaImage && m.Kind != models.MediaVideo && m.Kind != models.MediaAudio {
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
	if m.Aspect != "" && !m.Aspect.Valid() {
		return fmt.Errorf("save media %q: unknown aspect ratio %q", path, m.Aspect)
	}
	if m.Repeat != "" && !m.Repeat.Valid() {
		return fmt.Errorf("save media %q: unknown repeat mode %q", path, m.Repeat)
	}
	aspect, repeat := m.Aspect.Normalized(), m.RepeatMode() // audio defaults to once
	if m.ID == 0 {
		added := m.Added
		if added.IsZero() {
			added = now
		}
		res, err := tx.ExecContext(ctx, `
			INSERT INTO media (path, kind, title, duration, width, height, thumb_path, transcoded_path, added, aspect, repeat)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			path, string(m.Kind), title, durationMS, m.Width, m.Height, m.ThumbPath, m.TranscodedPath,
			added.UTC().Format(timeLayout), string(aspect), string(repeat))
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
				thumb_path = ?, transcoded_path = ?, aspect = ?, repeat = ?
			WHERE id = ?`,
			path, string(m.Kind), title, durationMS, m.Width, m.Height, m.ThumbPath, m.TranscodedPath,
			string(aspect), string(repeat), m.ID); err != nil {
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
	m.Aspect, m.Repeat = aspect, repeat
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
// query (case- and diacritic-insensitive, like the song search), of the
// given kind ("" for both), ordered by title. An empty query lists all.
// limit <= 0 means no limit.
//
// The words are matched in Go (fold), so the rows are read in title order
// and the limit applies to the matches.
func (s *Store) SearchMedia(ctx context.Context, query string, kind models.MediaKind, limit int) ([]models.Media, error) {
	terms := textTerms(query)
	sqlLimit := limit
	if sqlLimit <= 0 || len(terms) > 0 {
		sqlLimit = -1
	}
	q := `SELECT ` + mediaColumns + ` FROM media`
	var args []any
	if kind != "" {
		q += ` WHERE kind = ?`
		args = append(args, string(kind))
	}
	q += ` ORDER BY title COLLATE NOCASE, id LIMIT ?`
	args = append(args, sqlLimit)
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
		if !matchesAll(terms, m.Title, m.Path, m.Tags) {
			continue
		}
		out = append(out, m)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("search media %q: %w", query, err)
	}
	return out, nil
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
