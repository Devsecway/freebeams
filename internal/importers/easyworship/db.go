package easyworship

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"modernc.org/sqlite"
)

var registerCollations sync.Once

// ensureCollations registers EasyWorship's custom collation with the sqlite
// driver (once per process). EW schemas declare text columns COLLATE
// UTF8_U_CI. SQLite only needs it when a statement compares those columns
// (ORDER BY, =, index lookups); our queries avoid that, but registering a
// compatible case-insensitive collation means such a statement still works
// instead of failing with "no such collation sequence".
func ensureCollations() {
	registerCollations.Do(func() {
		_ = sqlite.RegisterCollationUtf8("UTF8_U_CI", func(a, b string) int {
			return strings.Compare(strings.ToLower(a), strings.ToLower(b))
		})
	})
}

// openReadOnly opens an EasyWorship SQLite file strictly read-only
// (mode=ro&immutable=1): no journal, no WAL, no locks, never a write to the
// user's EasyWorship data.
func openReadOnly(path string) (*sql.DB, error) {
	ensureCollations()
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	if _, err := os.Stat(abs); err != nil {
		return nil, fmt.Errorf("open %s: %w", abs, err)
	}
	p := filepath.ToSlash(abs)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p // C:/x -> /C:/x
	}
	u := url.URL{Scheme: "file", Path: p, RawQuery: "mode=ro&immutable=1"}
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", abs, err)
	}
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("open %s: %w", abs, err)
	}
	return db, nil
}
