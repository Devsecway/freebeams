package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"freeworship/internal/models"
)

const timeLayout = time.RFC3339Nano

// SearchSongs returns songs matching query (FTS over title, author, lyrics;
// prefix matching per word), ordered by title. Empty query lists all songs.
// A query with no searchable words (only punctuation) also lists all songs.
// limit <= 0 means no limit.
func (s *Store) SearchSongs(ctx context.Context, query string, limit int) ([]models.SongSummary, error) {
	if limit <= 0 {
		limit = -1 // SQLite: no limit
	}
	var (
		rows *sql.Rows
		err  error
	)
	if match := ftsQuery(query); match == "" {
		rows, err = s.db.QueryContext(ctx, `
			SELECT id, title, author, copyright, ccli FROM songs
			ORDER BY title COLLATE NOCASE, id
			LIMIT ?`, limit)
	} else {
		rows, err = s.db.QueryContext(ctx, `
			SELECT s.id, s.title, s.author, s.copyright, s.ccli
			FROM songs_fts f JOIN songs s ON s.id = f.rowid
			WHERE songs_fts MATCH ?
			ORDER BY s.title COLLATE NOCASE, s.id
			LIMIT ?`, match, limit)
	}
	if err != nil {
		return nil, fmt.Errorf("search songs %q: %w", query, err)
	}
	defer rows.Close()
	out := []models.SongSummary{}
	for rows.Next() {
		var r models.SongSummary
		if err := rows.Scan(&r.ID, &r.Title, &r.Author, &r.Copyright, &r.CCLI); err != nil {
			return nil, fmt.Errorf("search songs %q: %w", query, err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("search songs %q: %w", query, err)
	}
	return out, nil
}

// GetSong loads a song with its sections.
func (s *Store) GetSong(ctx context.Context, id int64) (models.Song, error) {
	var (
		song             models.Song
		created, updated string
	)
	err := s.db.QueryRowContext(ctx, `
		SELECT id, uuid, title, author, copyright, administrator, ccli, tags, theme_id, created, updated
		FROM songs WHERE id = ?`, id).Scan(
		&song.ID, &song.UUID, &song.Title, &song.Author, &song.Copyright, &song.Administrator,
		&song.CCLI, &song.Tags, &song.ThemeID, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return models.Song{}, fmt.Errorf("get song %d: %w", id, ErrNotFound)
	}
	if err != nil {
		return models.Song{}, fmt.Errorf("get song %d: %w", id, err)
	}
	song.Created, _ = time.Parse(timeLayout, created)
	song.Updated, _ = time.Parse(timeLayout, updated)

	rows, err := s.db.QueryContext(ctx, `
		SELECT kind, number, label, lines FROM song_sections
		WHERE song_id = ? ORDER BY ord`, id)
	if err != nil {
		return models.Song{}, fmt.Errorf("get song %d sections: %w", id, err)
	}
	defer rows.Close()
	for rows.Next() {
		var (
			sec   models.Section
			kind  string
			lines string
		)
		if err := rows.Scan(&kind, &sec.Number, &sec.Label, &lines); err != nil {
			return models.Song{}, fmt.Errorf("get song %d sections: %w", id, err)
		}
		sec.Kind = models.SectionKind(kind)
		if err := json.Unmarshal([]byte(lines), &sec.Lines); err != nil {
			return models.Song{}, fmt.Errorf("get song %d sections: decode lines: %w", id, err)
		}
		song.Sections = append(song.Sections, sec)
	}
	if err := rows.Err(); err != nil {
		return models.Song{}, fmt.Errorf("get song %d sections: %w", id, err)
	}
	return song, nil
}

// SaveSong inserts (ID == 0) or updates a song, assigning ID/UUID/Created/Updated.
// Updating an ID that does not exist returns ErrNotFound.
func (s *Store) SaveSong(ctx context.Context, song *models.Song) error {
	if song == nil {
		return errors.New("save song: nil song")
	}
	return s.SaveSongs(ctx, []*models.Song{song})
}

// SaveSongs saves several songs (insert or update, as SaveSong) in a single
// transaction: either all are saved or none. Used by bulk importers.
func (s *Store) SaveSongs(ctx context.Context, songs []*models.Song) error {
	now := time.Now()
	// Work on copies so a rollback does not leave IDs assigned on the caller's songs.
	copies := make([]models.Song, len(songs))
	err := s.inTx(ctx, func(tx *sql.Tx) error {
		for i, song := range songs {
			if song == nil {
				return fmt.Errorf("save songs: song %d is nil", i)
			}
			copies[i] = *song
			if err := saveSongTx(ctx, tx, &copies[i], now); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	for i := range songs {
		*songs[i] = copies[i]
	}
	return nil
}

func saveSongTx(ctx context.Context, tx *sql.Tx, song *models.Song, now time.Time) error {
	now = now.UTC()
	title := strings.TrimSpace(song.Title)
	if title == "" {
		return errors.New("save song: title is required")
	}
	updated := now
	if song.ID == 0 {
		uuid := song.UUID
		if uuid == "" {
			uuid = newUUID()
		}
		created := song.Created
		if created.IsZero() {
			created = now
		}
		res, err := tx.ExecContext(ctx, `
			INSERT INTO songs (uuid, title, author, copyright, administrator, ccli, tags, theme_id, created, updated)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			uuid, title, song.Author, song.Copyright, song.Administrator, song.CCLI, song.Tags,
			song.ThemeID, created.UTC().Format(timeLayout), updated.Format(timeLayout))
		if err != nil {
			if isUniqueViolation(err) {
				return fmt.Errorf("save song %q: uuid %s: %w", title, uuid, ErrExists)
			}
			return fmt.Errorf("save song %q: %w", title, err)
		}
		id, err := res.LastInsertId()
		if err != nil {
			return fmt.Errorf("save song %q: %w", title, err)
		}
		song.ID, song.UUID, song.Created = id, uuid, created.UTC()
	} else {
		var uuid, created string
		err := tx.QueryRowContext(ctx, `SELECT uuid, created FROM songs WHERE id = ?`, song.ID).Scan(&uuid, &created)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("save song %d: %w", song.ID, ErrNotFound)
		}
		if err != nil {
			return fmt.Errorf("save song %d: %w", song.ID, err)
		}
		if song.UUID == "" {
			song.UUID = uuid
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE songs SET uuid = ?, title = ?, author = ?, copyright = ?, administrator = ?, ccli = ?,
				tags = ?, theme_id = ?, updated = ?
			WHERE id = ?`,
			song.UUID, title, song.Author, song.Copyright, song.Administrator, song.CCLI, song.Tags,
			song.ThemeID, updated.Format(timeLayout), song.ID); err != nil {
			if isUniqueViolation(err) {
				return fmt.Errorf("save song %d: uuid %s: %w", song.ID, song.UUID, ErrExists)
			}
			return fmt.Errorf("save song %d: %w", song.ID, err)
		}
		if t, err := time.Parse(timeLayout, created); err == nil {
			song.Created = t
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM song_sections WHERE song_id = ?`, song.ID); err != nil {
			return fmt.Errorf("save song %d: clear sections: %w", song.ID, err)
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM songs_fts WHERE rowid = ?`, song.ID); err != nil {
			return fmt.Errorf("save song %d: clear index: %w", song.ID, err)
		}
	}
	song.Title = title
	song.Updated = updated

	var lyrics strings.Builder
	for i, sec := range song.Sections {
		lines := sec.Lines
		if lines == nil {
			lines = []string{}
		}
		js, err := json.Marshal(lines)
		if err != nil {
			return fmt.Errorf("save song %d: encode section %d: %w", song.ID, i, err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO song_sections (song_id, ord, kind, number, label, lines) VALUES (?, ?, ?, ?, ?, ?)`,
			song.ID, i, string(sec.Kind), sec.Number, sec.Label, string(js)); err != nil {
			return fmt.Errorf("save song %d: section %d: %w", song.ID, i, err)
		}
		for _, l := range sec.Lines {
			lyrics.WriteString(l)
			lyrics.WriteByte('\n')
		}
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO songs_fts (rowid, title, author, lyrics) VALUES (?, ?, ?, ?)`,
		song.ID, title, song.Author, lyrics.String()); err != nil {
		return fmt.Errorf("save song %d: index: %w", song.ID, err)
	}
	return nil
}

// DeleteSong deletes a song (its sections and index entry go with it).
// Deleting a missing song returns ErrNotFound.
func (s *Store) DeleteSong(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM songs WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete song %d: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete song %d: %w", id, err)
	}
	if n == 0 {
		return fmt.Errorf("delete song %d: %w", id, ErrNotFound)
	}
	return nil
}

// CountSongs returns the number of songs.
func (s *Store) CountSongs(ctx context.Context) (int, error) {
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM songs`).Scan(&n); err != nil {
		return 0, fmt.Errorf("count songs: %w", err)
	}
	return n, nil
}

// newUUID returns a random (version 4) UUID string.
func newUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:]) // crypto/rand.Read never fails on supported platforms
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
