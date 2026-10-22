package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"freebeams/internal/models"
)

// Saved message alerts (the Alert Manager's list) are stored as JSON
// (models.Alert) with the name duplicated in a column for listing, like
// themes. Migration 6 seeds three examples.

const alertColumns = `id, name, json`

func scanAlert(sc interface{ Scan(...any) error }) (models.Alert, error) {
	var (
		a    models.Alert
		id   int64
		name string
		js   string
	)
	if err := sc.Scan(&id, &name, &js); err != nil {
		return models.Alert{}, err
	}
	if err := json.Unmarshal([]byte(js), &a); err != nil {
		return models.Alert{}, fmt.Errorf("alert %d: decode: %w", id, err)
	}
	a.ID, a.Name = id, name
	return a.Normalized(), nil
}

// GetAlert loads one saved alert. A missing id returns ErrNotFound.
func (s *Store) GetAlert(ctx context.Context, id int64) (models.Alert, error) {
	a, err := scanAlert(s.db.QueryRowContext(ctx, `SELECT `+alertColumns+` FROM alerts WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return models.Alert{}, fmt.Errorf("get alert %d: %w", id, ErrNotFound)
	}
	if err != nil {
		return models.Alert{}, fmt.Errorf("get alert %d: %w", id, err)
	}
	return a, nil
}

// ListAlerts returns every saved alert ordered by name (case-insensitively),
// then id.
func (s *Store) ListAlerts(ctx context.Context) ([]models.Alert, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+alertColumns+` FROM alerts ORDER BY name COLLATE NOCASE, id`)
	if err != nil {
		return nil, fmt.Errorf("list alerts: %w", err)
	}
	defer rows.Close()
	out := []models.Alert{}
	for rows.Next() {
		a, err := scanAlert(rows)
		if err != nil {
			return nil, fmt.Errorf("list alerts: %w", err)
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list alerts: %w", err)
	}
	return out, nil
}

// SaveAlert inserts (ID == 0) or updates a saved alert, normalizing it and
// assigning its ID. A name and a message are required. Updating an ID that
// does not exist returns ErrNotFound.
func (s *Store) SaveAlert(ctx context.Context, a *models.Alert) error {
	if a == nil {
		return errors.New("save alert: nil alert")
	}
	cp := a.Normalized()
	if cp.Name == "" {
		return errors.New("save alert: name is required")
	}
	if cp.Message == "" {
		return fmt.Errorf("save alert %q: message is required", cp.Name)
	}
	err := s.inTx(ctx, func(tx *sql.Tx) error {
		stamp := time.Now().UTC().Format(timeLayout)
		if cp.ID == 0 {
			res, err := tx.ExecContext(ctx, `INSERT INTO alerts (name, json, created, updated) VALUES (?, '{}', ?, ?)`,
				cp.Name, stamp, stamp)
			if err != nil {
				return fmt.Errorf("save alert %q: %w", cp.Name, err)
			}
			if cp.ID, err = res.LastInsertId(); err != nil {
				return fmt.Errorf("save alert %q: %w", cp.Name, err)
			}
		}
		js, err := json.Marshal(cp)
		if err != nil {
			return fmt.Errorf("save alert %q: encode: %w", cp.Name, err)
		}
		res, err := tx.ExecContext(ctx, `UPDATE alerts SET name = ?, json = ?, updated = ? WHERE id = ?`,
			cp.Name, string(js), stamp, cp.ID)
		if err != nil {
			return fmt.Errorf("save alert %d: %w", cp.ID, err)
		}
		if n, err := res.RowsAffected(); err != nil {
			return fmt.Errorf("save alert %d: %w", cp.ID, err)
		} else if n == 0 {
			return fmt.Errorf("save alert %d: %w", cp.ID, ErrNotFound)
		}
		return nil
	})
	if err != nil {
		return err
	}
	*a = cp
	return nil
}

// DeleteAlert removes a saved alert. A missing id returns ErrNotFound.
func (s *Store) DeleteAlert(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM alerts WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete alert %d: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete alert %d: %w", id, err)
	}
	if n == 0 {
		return fmt.Errorf("delete alert %d: %w", id, ErrNotFound)
	}
	return nil
}
