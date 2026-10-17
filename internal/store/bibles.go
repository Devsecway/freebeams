package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"

	"freeworship/internal/models"
)

// verseKey encodes a verse as the verses_fts rowid:
// bible_id<<24 | book<<16 | chapter<<8 | verse. Books (1..66), chapters
// (<= 150) and verses (<= 176) each fit in 8 bits.
func verseKey(bibleID int64, book, chapter, verse int) int64 {
	return bibleID<<24 | int64(book&0xFF)<<16 | int64(chapter&0xFF)<<8 | int64(verse&0xFF)
}

func validVerseRef(book, chapter, verse int) bool {
	return book >= 1 && book <= 0xFF && chapter >= 1 && chapter <= 0xFF && verse >= 1 && verse <= 0xFF
}

// ListBibles returns installed translations ordered by abbrev.
func (s *Store) ListBibles(ctx context.Context) ([]models.Bible, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, abbrev, name, language, copyright FROM bibles ORDER BY abbrev COLLATE NOCASE`)
	if err != nil {
		return nil, fmt.Errorf("list bibles: %w", err)
	}
	defer rows.Close()
	out := []models.Bible{}
	for rows.Next() {
		var b models.Bible
		if err := rows.Scan(&b.ID, &b.Abbrev, &b.Name, &b.Language, &b.Copyright); err != nil {
			return nil, fmt.Errorf("list bibles: %w", err)
		}
		out = append(out, b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list bibles: %w", err)
	}
	return out, nil
}

// CreateBible inserts a translation and returns it with its ID. Abbrev is
// unique (case-insensitively); a duplicate returns ErrExists.
func (s *Store) CreateBible(ctx context.Context, b models.Bible) (models.Bible, error) {
	err := s.inTx(ctx, func(tx *sql.Tx) error {
		return createBibleTx(ctx, tx, &b)
	})
	if err != nil {
		return models.Bible{}, err
	}
	return b, nil
}

// CreateBibleWithVerses creates a translation and inserts its verses in one
// transaction, so an interrupted import never leaves a half-filled Bible.
func (s *Store) CreateBibleWithVerses(ctx context.Context, b models.Bible, verses []models.Verse) (models.Bible, error) {
	err := s.inTx(ctx, func(tx *sql.Tx) error {
		if err := createBibleTx(ctx, tx, &b); err != nil {
			return err
		}
		return insertVersesTx(ctx, tx, b.ID, verses)
	})
	if err != nil {
		return models.Bible{}, err
	}
	return b, nil
}

func createBibleTx(ctx context.Context, tx *sql.Tx, b *models.Bible) error {
	b.Abbrev = strings.TrimSpace(b.Abbrev)
	if b.Abbrev == "" {
		return errors.New("create bible: abbrev is required")
	}
	res, err := tx.ExecContext(ctx, `
		INSERT INTO bibles (abbrev, name, language, copyright) VALUES (?, ?, ?, ?)`,
		b.Abbrev, b.Name, b.Language, b.Copyright)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("create bible %s: %w", b.Abbrev, ErrExists)
		}
		return fmt.Errorf("create bible %s: %w", b.Abbrev, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("create bible %s: %w", b.Abbrev, err)
	}
	b.ID = id
	return nil
}

// InsertVerses bulk-inserts verses for a bible in one transaction. A verse
// that already exists is replaced.
func (s *Store) InsertVerses(ctx context.Context, bibleID int64, verses []models.Verse) error {
	return s.inTx(ctx, func(tx *sql.Tx) error {
		var exists int
		err := tx.QueryRowContext(ctx, `SELECT 1 FROM bibles WHERE id = ?`, bibleID).Scan(&exists)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("insert verses: bible %d: %w", bibleID, ErrNotFound)
		}
		if err != nil {
			return fmt.Errorf("insert verses: bible %d: %w", bibleID, err)
		}
		return insertVersesTx(ctx, tx, bibleID, verses)
	})
}

func insertVersesTx(ctx context.Context, tx *sql.Tx, bibleID int64, verses []models.Verse) error {
	ins, err := tx.PrepareContext(ctx, `
		INSERT INTO verses (bible_id, book, chapter, verse, text) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (bible_id, book, chapter, verse) DO UPDATE SET text = excluded.text`)
	if err != nil {
		return fmt.Errorf("insert verses: %w", err)
	}
	defer ins.Close()
	del, err := tx.PrepareContext(ctx, `DELETE FROM verses_fts WHERE rowid = ?`)
	if err != nil {
		return fmt.Errorf("insert verses: %w", err)
	}
	defer del.Close()
	idx, err := tx.PrepareContext(ctx, `INSERT INTO verses_fts (rowid, text) VALUES (?, ?)`)
	if err != nil {
		return fmt.Errorf("insert verses: %w", err)
	}
	defer idx.Close()
	// A fresh Bible has no index rows, so the per-verse delete is only needed
	// when verses already exist.
	var have int
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM verses WHERE bible_id = ?)`, bibleID).Scan(&have); err != nil {
		return fmt.Errorf("insert verses: %w", err)
	}
	seen := make(map[int64]bool, len(verses))
	for _, v := range verses {
		if !validVerseRef(v.Book, v.Chapter, v.Verse) {
			return fmt.Errorf("insert verses: invalid reference %d %d:%d", v.Book, v.Chapter, v.Verse)
		}
		key := verseKey(bibleID, v.Book, v.Chapter, v.Verse)
		if _, err := ins.ExecContext(ctx, bibleID, v.Book, v.Chapter, v.Verse, v.Text); err != nil {
			return fmt.Errorf("insert verse %d %d:%d: %w", v.Book, v.Chapter, v.Verse, err)
		}
		if have == 1 || seen[key] {
			if _, err := del.ExecContext(ctx, key); err != nil {
				return fmt.Errorf("index verse %d %d:%d: %w", v.Book, v.Chapter, v.Verse, err)
			}
		}
		seen[key] = true
		if _, err := idx.ExecContext(ctx, key, v.Text); err != nil {
			return fmt.Errorf("index verse %d %d:%d: %w", v.Book, v.Chapter, v.Verse, err)
		}
	}
	return nil
}

// Verses returns the verses of a passage in canonical order. Whole-chapter and
// to-end-of-chapter ranges are expanded. Missing verses are skipped. Ranges
// are sorted and overlapping ranges do not produce duplicates.
func (s *Store) Verses(ctx context.Context, bibleID int64, p models.Passage) ([]models.Verse, error) {
	out := []models.Verse{}
	seen := map[[2]int]bool{}
	for _, r := range p.Ranges {
		lo, hi := r.VerseStart, r.VerseEnd
		switch {
		case lo <= 0: // whole chapter
			lo, hi = 1, 0xFF
		case hi == 0: // to the end of the chapter
			hi = 0xFF
		case hi < lo:
			lo, hi = hi, lo
		}
		rows, err := s.db.QueryContext(ctx, `
			SELECT verse, text FROM verses
			WHERE bible_id = ? AND book = ? AND chapter = ? AND verse BETWEEN ? AND ?
			ORDER BY verse`, bibleID, p.Book, r.Chapter, lo, hi)
		if err != nil {
			return nil, fmt.Errorf("verses %d %d:%d-%d: %w", p.Book, r.Chapter, lo, hi, err)
		}
		for rows.Next() {
			v := models.Verse{Book: p.Book, Chapter: r.Chapter}
			if err := rows.Scan(&v.Verse, &v.Text); err != nil {
				rows.Close()
				return nil, fmt.Errorf("verses %d %d: %w", p.Book, r.Chapter, err)
			}
			k := [2]int{v.Chapter, v.Verse}
			if !seen[k] {
				seen[k] = true
				out = append(out, v)
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, fmt.Errorf("verses %d %d: %w", p.Book, r.Chapter, err)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Chapter != out[j].Chapter {
			return out[i].Chapter < out[j].Chapter
		}
		return out[i].Verse < out[j].Verse
	})
	return out, nil
}

// ChapterCount returns the number of chapters of book (0 if the book is absent).
func (s *Store) ChapterCount(ctx context.Context, bibleID int64, book int) (int, error) {
	var n int
	if err := s.db.QueryRowContext(ctx, `
		SELECT coalesce(max(chapter), 0) FROM verses WHERE bible_id = ? AND book = ?`,
		bibleID, book).Scan(&n); err != nil {
		return 0, fmt.Errorf("chapter count %d: %w", book, err)
	}
	return n, nil
}

// VerseCount returns the number of verses of a chapter (0 if absent).
func (s *Store) VerseCount(ctx context.Context, bibleID int64, book, chapter int) (int, error) {
	var n int
	if err := s.db.QueryRowContext(ctx, `
		SELECT coalesce(max(verse), 0) FROM verses WHERE bible_id = ? AND book = ? AND chapter = ?`,
		bibleID, book, chapter).Scan(&n); err != nil {
		return 0, fmt.Errorf("verse count %d %d: %w", book, chapter, err)
	}
	return n, nil
}

// SearchVerses does a full-text search within a bible (limit <= 0: 100).
// Every word is a prefix term and all words must match; results are in
// canonical order. Input with no searchable words returns no verses.
func (s *Store) SearchVerses(ctx context.Context, bibleID int64, text string, limit int) ([]models.Verse, error) {
	if limit <= 0 {
		limit = 100
	}
	out := []models.Verse{}
	match := ftsQuery(text)
	if match == "" {
		return out, nil
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT v.book, v.chapter, v.verse, v.text
		FROM verses_fts f
		JOIN verses v ON v.bible_id = ?1
			AND v.book = (f.rowid >> 16) & 255
			AND v.chapter = (f.rowid >> 8) & 255
			AND v.verse = f.rowid & 255
		WHERE verses_fts MATCH ?2 AND f.rowid BETWEEN ?3 AND ?4
		ORDER BY f.rowid
		LIMIT ?5`, bibleID, match, bibleID<<24, bibleID<<24|0xFFFFFF, limit)
	if err != nil {
		return nil, fmt.Errorf("search verses %q: %w", text, err)
	}
	defer rows.Close()
	for rows.Next() {
		var v models.Verse
		if err := rows.Scan(&v.Book, &v.Chapter, &v.Verse, &v.Text); err != nil {
			return nil, fmt.Errorf("search verses %q: %w", text, err)
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("search verses %q: %w", text, err)
	}
	return out, nil
}
