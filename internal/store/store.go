// Package store persists the FreeWorship library in SQLite (modernc.org/sqlite,
// pure Go). It must not import graphics.gd.
//
// The database runs in WAL mode with foreign keys on. The schema is versioned
// through PRAGMA user_version (see migrations.go). Songs and verses have FTS5
// indexes (songs_fts, verses_fts) that the store keeps in sync on writes.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite" // registers the "sqlite" database/sql driver
)

// ErrNotFound is returned (wrapped) when a row does not exist.
var ErrNotFound = errors.New("not found")

// ErrExists is returned (wrapped) when a unique key (e.g. a Bible abbrev) is taken.
var ErrExists = errors.New("already exists")

// Store is a handle to the library database. Safe for concurrent use.
type Store struct {
	db   *sql.DB
	path string
}

// maxOpenConns bounds the pool. WAL allows concurrent readers; writers are
// serialised by SQLite (write transactions BEGIN IMMEDIATE and wait up to the
// busy timeout), so a small pool is plenty for a desktop app.
const maxOpenConns = 4

// Open opens (creating if needed) the database at path and runs migrations.
func Open(path string) (*Store, error) {
	if path == "" {
		return nil, errors.New("open store: empty path")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("open store %q: %w", path, err)
	}
	q := url.Values{}
	q.Add("_pragma", "busy_timeout(10000)")
	q.Add("_pragma", "foreign_keys(1)")
	q.Add("_pragma", "journal_mode(WAL)")
	q.Add("_pragma", "synchronous(NORMAL)")
	q.Set("_txlock", "immediate")
	db, err := sql.Open("sqlite", fileURI(abs, q))
	if err != nil {
		return nil, fmt.Errorf("open store %q: %w", abs, err)
	}
	db.SetMaxOpenConns(maxOpenConns)
	db.SetMaxIdleConns(maxOpenConns)
	ctx := context.Background()
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("open store %q: %w", abs, err)
	}
	if err := migrate(ctx, db); err != nil {
		db.Close()
		return nil, fmt.Errorf("open store %q: %w", abs, err)
	}
	return &Store{db: db, path: abs}, nil
}

// fileURI builds a SQLite "file:" URI for an absolute path (works for
// Windows drive paths and paths containing '?', '#' or spaces).
func fileURI(abs string, q url.Values) string {
	p := filepath.ToSlash(abs)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p // C:/x -> /C:/x
	}
	u := url.URL{Scheme: "file", Path: p, RawQuery: q.Encode()}
	return u.String()
}

// DefaultPath returns the library path in the user's config dir:
// <os.UserConfigDir()>/FreeWorship/library.db (directory created).
func DefaultPath() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("default library path: %w", err)
	}
	dir := filepath.Join(base, "FreeWorship")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("default library path: %w", err)
	}
	return filepath.Join(dir, "library.db"), nil
}

// Close closes the database. The Store must not be used afterwards.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	// Best effort: refresh query planner statistics.
	_, _ = s.db.Exec(`PRAGMA optimize`)
	if err := s.db.Close(); err != nil {
		return fmt.Errorf("close store %q: %w", s.path, err)
	}
	return nil
}

// Path returns the absolute path of the database file.
func (s *Store) Path() string { return s.path }

// inTx runs fn in a write transaction, committing when fn returns nil.
func (s *Store) inTx(ctx context.Context, fn func(tx *sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// isUniqueViolation reports whether err is a SQLite UNIQUE/PRIMARY KEY
// constraint failure.
func isUniqueViolation(err error) bool {
	const (
		sqliteConstraintPrimaryKey = 1555 // SQLITE_CONSTRAINT_PRIMARYKEY
		sqliteConstraintUnique     = 2067 // SQLITE_CONSTRAINT_UNIQUE
	)
	var coded interface{ Code() int }
	if errors.As(err, &coded) {
		c := coded.Code()
		return c == sqliteConstraintUnique || c == sqliteConstraintPrimaryKey
	}
	return false
}
