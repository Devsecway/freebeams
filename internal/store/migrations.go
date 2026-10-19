package store

import (
	"context"
	"database/sql"
	"fmt"
)

// migrations are applied in order; migrations[i] brings the schema from
// version i to version i+1 (PRAGMA user_version). Never edit a released
// migration: append a new one instead.
var migrations = []string{
	// 1: songs, bibles, settings and their full-text indexes.
	`
CREATE TABLE songs (
	id            INTEGER PRIMARY KEY,
	uuid          TEXT    NOT NULL UNIQUE,
	title         TEXT    NOT NULL,
	author        TEXT    NOT NULL DEFAULT '',
	copyright     TEXT    NOT NULL DEFAULT '',
	administrator TEXT    NOT NULL DEFAULT '',
	ccli          TEXT    NOT NULL DEFAULT '',
	tags          TEXT    NOT NULL DEFAULT '',
	theme_id      INTEGER NOT NULL DEFAULT 0,
	created       TEXT    NOT NULL,
	updated       TEXT    NOT NULL
);
CREATE INDEX songs_title ON songs (title COLLATE NOCASE);

-- lines is a JSON array of strings.
CREATE TABLE song_sections (
	song_id INTEGER NOT NULL REFERENCES songs (id) ON DELETE CASCADE,
	ord     INTEGER NOT NULL,
	kind    TEXT    NOT NULL,
	number  INTEGER NOT NULL DEFAULT 0,
	label   TEXT    NOT NULL DEFAULT '',
	lines   TEXT    NOT NULL DEFAULT '[]',
	PRIMARY KEY (song_id, ord)
) WITHOUT ROWID;

-- rowid = songs.id. Maintained by SaveSong; the trigger covers deletes.
CREATE VIRTUAL TABLE songs_fts USING fts5 (
	title, author, lyrics,
	tokenize = 'unicode61 remove_diacritics 2'
);
CREATE TRIGGER songs_fts_delete AFTER DELETE ON songs BEGIN
	DELETE FROM songs_fts WHERE rowid = old.id;
END;

CREATE TABLE bibles (
	id        INTEGER PRIMARY KEY,
	abbrev    TEXT NOT NULL UNIQUE COLLATE NOCASE,
	name      TEXT NOT NULL DEFAULT '',
	language  TEXT NOT NULL DEFAULT '',
	copyright TEXT NOT NULL DEFAULT ''
);

CREATE TABLE verses (
	bible_id INTEGER NOT NULL REFERENCES bibles (id) ON DELETE CASCADE,
	book     INTEGER NOT NULL,
	chapter  INTEGER NOT NULL,
	verse    INTEGER NOT NULL,
	text     TEXT    NOT NULL,
	PRIMARY KEY (bible_id, book, chapter, verse)
) WITHOUT ROWID;

-- Contentless index; rowid encodes the verse (see verseKey).
CREATE VIRTUAL TABLE verses_fts USING fts5 (
	text,
	content = '',
	contentless_delete = 1,
	tokenize = 'unicode61 remove_diacritics 2'
);

CREATE TABLE settings (
	key   TEXT PRIMARY KEY,
	value TEXT NOT NULL
) WITHOUT ROWID;
`,
	// 2: the media library (images and motion backgrounds).
	`
-- duration is in milliseconds (0 for stills). thumb_path and transcoded_path
-- are derived files in the app's cache; "" until they have been made.
CREATE TABLE media (
	id              INTEGER PRIMARY KEY,
	path            TEXT    NOT NULL UNIQUE,
	kind            TEXT    NOT NULL,
	title           TEXT    NOT NULL DEFAULT '',
	duration        INTEGER NOT NULL DEFAULT 0,
	width           INTEGER NOT NULL DEFAULT 0,
	height          INTEGER NOT NULL DEFAULT 0,
	thumb_path      TEXT    NOT NULL DEFAULT '',
	transcoded_path TEXT    NOT NULL DEFAULT '',
	added           TEXT    NOT NULL
);
CREATE INDEX media_kind_title ON media (kind, title COLLATE NOCASE);
`,
	// 3: the presentation library (PDF / PowerPoint decks rendered to page images).
	`
-- pages, cache_key, renderer and partial describe the rendered pages in the
-- app's cache (internal/presentations); pages is 0 until they are made.
CREATE TABLE presentations (
	id        INTEGER PRIMARY KEY,
	path      TEXT    NOT NULL UNIQUE,
	title     TEXT    NOT NULL DEFAULT '',
	pages     INTEGER NOT NULL DEFAULT 0,
	cache_key TEXT    NOT NULL DEFAULT '',
	renderer  TEXT    NOT NULL DEFAULT '',
	partial   INTEGER NOT NULL DEFAULT 0,
	added     TEXT    NOT NULL
);
CREATE INDEX presentations_title ON presentations (title COLLATE NOCASE);
`,
}

// migrate brings the schema up to date. Each migration runs in its own
// transaction together with the user_version bump, so a failed migration
// leaves the database at the previous version.
func migrate(ctx context.Context, db *sql.DB) error {
	var version int
	if err := db.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&version); err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}
	if version > len(migrations) {
		return fmt.Errorf("database schema version %d is newer than this app supports (%d)", version, len(migrations))
	}
	for v := version; v < len(migrations); v++ {
		if err := runMigration(ctx, db, v+1, migrations[v]); err != nil {
			return err
		}
	}
	return nil
}

func runMigration(ctx context.Context, db *sql.DB, to int, script string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("migration %d: begin: %w", to, err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op after commit
	// Re-check under the write lock (_txlock=immediate): another process may
	// have migrated the file since we read the version.
	var current int
	if err := tx.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&current); err != nil {
		return fmt.Errorf("migration %d: read version: %w", to, err)
	}
	if current >= to {
		return nil
	}
	if _, err := tx.ExecContext(ctx, script); err != nil {
		return fmt.Errorf("migration %d: %w", to, err)
	}
	// PRAGMA does not accept bound parameters; to is an int we control.
	if _, err := tx.ExecContext(ctx, fmt.Sprintf(`PRAGMA user_version = %d`, to)); err != nil {
		return fmt.Errorf("migration %d: set version: %w", to, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("migration %d: commit: %w", to, err)
	}
	return nil
}
