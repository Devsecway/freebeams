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

const presentationColumns = `id, path, title, pages, cache_key, renderer, partial, added`

// scanPresentation reads one presentations row (see presentationColumns).
func scanPresentation(sc interface{ Scan(...any) error }) (models.Presentation, error) {
	var (
		p       models.Presentation
		partial int
		added   string
	)
	if err := sc.Scan(&p.ID, &p.Path, &p.Title, &p.Pages, &p.CacheKey, &p.Renderer, &partial, &added); err != nil {
		return models.Presentation{}, err
	}
	p.Partial = partial != 0
	p.Added, _ = time.Parse(timeLayout, added)
	return p, nil
}

// GetPresentation loads one presentation. A missing id returns ErrNotFound.
func (s *Store) GetPresentation(ctx context.Context, id int64) (models.Presentation, error) {
	p, err := scanPresentation(s.db.QueryRowContext(ctx, `SELECT `+presentationColumns+` FROM presentations WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return models.Presentation{}, fmt.Errorf("get presentation %d: %w", id, ErrNotFound)
	}
	if err != nil {
		return models.Presentation{}, fmt.Errorf("get presentation %d: %w", id, err)
	}
	return p, nil
}

// PresentationByPath loads the presentation registered for a document. A
// path that is not in the library returns ErrNotFound.
func (s *Store) PresentationByPath(ctx context.Context, path string) (models.Presentation, error) {
	p, err := scanPresentation(s.db.QueryRowContext(ctx, `SELECT `+presentationColumns+` FROM presentations WHERE path = ?`, path))
	if errors.Is(err, sql.ErrNoRows) {
		return models.Presentation{}, fmt.Errorf("presentation %q: %w", path, ErrNotFound)
	}
	if err != nil {
		return models.Presentation{}, fmt.Errorf("presentation %q: %w", path, err)
	}
	return p, nil
}

// SavePresentation inserts (ID == 0) or updates a presentation, assigning ID
// and Added. The path is unique: inserting a path that is already registered
// returns ErrExists, and updating an ID that does not exist returns
// ErrNotFound. An empty Title is derived from the path.
func (s *Store) SavePresentation(ctx context.Context, p *models.Presentation) error {
	if p == nil {
		return errors.New("save presentation: nil item")
	}
	return s.SavePresentations(ctx, []*models.Presentation{p})
}

// SavePresentations saves several items (insert or update, as
// SavePresentation) in one transaction: either all are saved or none.
func (s *Store) SavePresentations(ctx context.Context, items []*models.Presentation) error {
	now := time.Now().UTC()
	copies := make([]models.Presentation, len(items))
	err := s.inTx(ctx, func(tx *sql.Tx) error {
		for i, p := range items {
			if p == nil {
				return fmt.Errorf("save presentation: item %d is nil", i)
			}
			copies[i] = *p
			if err := savePresentationTx(ctx, tx, &copies[i], now); err != nil {
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

func savePresentationTx(ctx context.Context, tx *sql.Tx, p *models.Presentation, now time.Time) error {
	path := strings.TrimSpace(p.Path)
	if path == "" {
		return errors.New("save presentation: path is required")
	}
	if !models.IsPresentationFile(path) {
		return fmt.Errorf("save presentation %q: not a presentation file", path)
	}
	title := strings.TrimSpace(p.Title)
	if title == "" {
		title = models.MediaTitle(path)
	}
	pages := max(p.Pages, 0)
	partial := 0
	if p.Partial {
		partial = 1
	}
	if p.ID == 0 {
		added := p.Added
		if added.IsZero() {
			added = now
		}
		res, err := tx.ExecContext(ctx, `
			INSERT INTO presentations (path, title, pages, cache_key, renderer, partial, added)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			path, title, pages, p.CacheKey, p.Renderer, partial, added.UTC().Format(timeLayout))
		if err != nil {
			if isUniqueViolation(err) {
				return fmt.Errorf("save presentation %q: %w", path, ErrExists)
			}
			return fmt.Errorf("save presentation %q: %w", path, err)
		}
		id, err := res.LastInsertId()
		if err != nil {
			return fmt.Errorf("save presentation %q: %w", path, err)
		}
		p.ID, p.Added = id, added.UTC()
	} else {
		var added string
		err := tx.QueryRowContext(ctx, `SELECT added FROM presentations WHERE id = ?`, p.ID).Scan(&added)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("save presentation %d: %w", p.ID, ErrNotFound)
		}
		if err != nil {
			return fmt.Errorf("save presentation %d: %w", p.ID, err)
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE presentations SET path = ?, title = ?, pages = ?, cache_key = ?, renderer = ?, partial = ?
			WHERE id = ?`,
			path, title, pages, p.CacheKey, p.Renderer, partial, p.ID); err != nil {
			if isUniqueViolation(err) {
				return fmt.Errorf("save presentation %d: path %q: %w", p.ID, path, ErrExists)
			}
			return fmt.Errorf("save presentation %d: %w", p.ID, err)
		}
		if t, err := time.Parse(timeLayout, added); err == nil {
			p.Added = t
		}
	}
	p.Path, p.Title, p.Pages = path, title, pages
	return nil
}

// DeletePresentation removes a presentation from the library (the document
// and its cached pages are left alone). A missing id returns ErrNotFound.
func (s *Store) DeletePresentation(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM presentations WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete presentation %d: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete presentation %d: %w", id, err)
	}
	if n == 0 {
		return fmt.Errorf("delete presentation %d: %w", id, ErrNotFound)
	}
	return nil
}

// SearchPresentations lists presentations whose title or path contains every
// word of query (case- and diacritic-insensitive, like the song search),
// ordered by title. An empty query lists all. limit <= 0 means no limit.
// The words are matched in Go (fold); the limit applies to the matches.
func (s *Store) SearchPresentations(ctx context.Context, query string, limit int) ([]models.Presentation, error) {
	terms := textTerms(query)
	sqlLimit := limit
	if sqlLimit <= 0 || len(terms) > 0 {
		sqlLimit = -1
	}
	rows, err := s.db.QueryContext(ctx, `SELECT `+presentationColumns+` FROM presentations
		ORDER BY title COLLATE NOCASE, id LIMIT ?`, sqlLimit)
	if err != nil {
		return nil, fmt.Errorf("search presentations %q: %w", query, err)
	}
	defer rows.Close()
	out := []models.Presentation{}
	for rows.Next() {
		p, err := scanPresentation(rows)
		if err != nil {
			return nil, fmt.Errorf("search presentations %q: %w", query, err)
		}
		if !matchesAll(terms, p.Title, p.Path) {
			continue
		}
		out = append(out, p)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("search presentations %q: %w", query, err)
	}
	return out, nil
}

// CountPresentations returns the number of presentations in the library.
func (s *Store) CountPresentations(ctx context.Context) (int, error) {
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM presentations`).Scan(&n); err != nil {
		return 0, fmt.Errorf("count presentations: %w", err)
	}
	return n, nil
}
