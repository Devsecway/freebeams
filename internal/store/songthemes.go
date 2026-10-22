package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// SetSongTheme gives song songID its own theme (themeID 0 = the default song
// theme) with a targeted update: the lyrics and the search index are left
// alone. The song must exist, and so must a non-zero theme (ErrNotFound).
func (s *Store) SetSongTheme(ctx context.Context, songID, themeID int64) error {
	return s.inTx(ctx, func(tx *sql.Tx) error {
		if themeID != 0 {
			var one int
			err := tx.QueryRowContext(ctx, `SELECT 1 FROM themes WHERE id = ?`, themeID).Scan(&one)
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("set song %d theme: theme %d: %w", songID, themeID, ErrNotFound)
			}
			if err != nil {
				return fmt.Errorf("set song %d theme: %w", songID, err)
			}
		}
		res, err := tx.ExecContext(ctx, `UPDATE songs SET theme_id = ?, updated = ? WHERE id = ?`,
			themeID, time.Now().UTC().Format(timeLayout), songID)
		if err != nil {
			return fmt.Errorf("set song %d theme: %w", songID, err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("set song %d theme: %w", songID, err)
		}
		if n == 0 {
			return fmt.Errorf("set song %d theme: %w", songID, ErrNotFound)
		}
		return nil
	})
}
