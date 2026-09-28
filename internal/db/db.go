// Package db opens the swingdesk SQLite store and applies migrations.
package db

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite" // registers the "sqlite" driver
)

// Open opens (creating if needed) the database at path with WAL, foreign keys
// and a busy timeout on every connection, then applies pending migrations.
func Open(ctx context.Context, path string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	q := url.Values{}
	q.Add("_pragma", "foreign_keys(1)")
	q.Add("_pragma", "journal_mode(WAL)")
	q.Add("_pragma", "busy_timeout(5000)")
	// BEGIN IMMEDIATE: take the write lock up front so busy_timeout covers
	// contention (a deferred read→write upgrade fails with SQLITE_BUSY at once).
	q.Add("_txlock", "immediate")
	// Build a proper URI so '#', '?' and '%' in the path are escaped.
	dsn := (&url.URL{Scheme: "file", Path: path, RawQuery: q.Encode()}).String()

	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	if err := conn.PingContext(ctx); err != nil {
		conn.Close()
		return nil, fmt.Errorf("ping %s: %w", path, err)
	}
	if err := Migrate(ctx, conn); err != nil {
		conn.Close()
		return nil, err
	}
	return conn, nil
}
