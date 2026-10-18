package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

// GetSetting unmarshals the JSON value for key into dst; returns false if unset.
func (s *Store) GetSetting(ctx context.Context, key string, dst any) (bool, error) {
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("get setting %q: %w", key, err)
	}
	if dst != nil {
		if err := json.Unmarshal([]byte(raw), dst); err != nil {
			return true, fmt.Errorf("get setting %q: decode: %w", key, err)
		}
	}
	return true, nil
}

// SetSetting stores v (JSON-marshalled) under key.
func (s *Store) SetSetting(ctx context.Context, key string, v any) error {
	js, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("set setting %q: encode: %w", key, err)
	}
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO settings (key, value) VALUES (?, ?)
		ON CONFLICT (key) DO UPDATE SET value = excluded.value`, key, string(js)); err != nil {
		return fmt.Errorf("set setting %q: %w", key, err)
	}
	return nil
}
