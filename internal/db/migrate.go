package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
	"time"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

type migration struct {
	version int
	name    string
	sql     string
}

// loadMigrations reads NNNN_name.sql files sorted by version.
func loadMigrations(fsys fs.FS) ([]migration, error) {
	entries, err := fs.ReadDir(fsys, "migrations")
	if err != nil {
		return nil, fmt.Errorf("read migrations: %w", err)
	}
	out := make([]migration, 0, len(entries))
	seen := map[int]string{}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".sql") {
			continue
		}
		prefix, _, ok := strings.Cut(name, "_")
		v, err := strconv.Atoi(prefix)
		if !ok || err != nil || v < 1 {
			return nil, fmt.Errorf("migration %q: name must start with a positive number and underscore", name)
		}
		if prev, dup := seen[v]; dup {
			return nil, fmt.Errorf("migrations %q and %q share version %d", prev, name, v)
		}
		seen[v] = name
		body, err := fs.ReadFile(fsys, "migrations/"+name)
		if err != nil {
			return nil, fmt.Errorf("read migration %s: %w", name, err)
		}
		out = append(out, migration{version: v, name: name, sql: string(body)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].version < out[j].version })
	return out, nil
}

// Migrate applies every embedded migration newer than the recorded version,
// each in its own transaction.
func Migrate(ctx context.Context, conn *sql.DB) error {
	migs, err := loadMigrations(migrationFS)
	if err != nil {
		return err
	}
	return applyMigrations(ctx, conn, migs)
}

func applyMigrations(ctx context.Context, conn *sql.DB, migs []migration) error {
	if _, err := conn.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version    INTEGER PRIMARY KEY,
		name       TEXT NOT NULL,
		applied_at TEXT NOT NULL
	)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}
	applied, err := appliedVersions(ctx, conn)
	if err != nil {
		return err
	}
	pending, err := pendingMigrations(migs, applied)
	if err != nil {
		return err
	}
	for _, m := range pending {
		if err := applyOne(ctx, conn, m); err != nil {
			return err
		}
	}
	return nil
}

func appliedVersions(ctx context.Context, conn *sql.DB) (map[int]bool, error) {
	rows, err := conn.QueryContext(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, fmt.Errorf("read schema versions: %w", err)
	}
	defer rows.Close()
	applied := map[int]bool{}
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return nil, fmt.Errorf("scan schema version: %w", err)
		}
		applied[v] = true
	}
	return applied, rows.Err()
}

// pendingMigrations returns the unapplied migrations in order. It fails when
// the database has a version this binary does not know (written by a newer
// build) or when an unapplied migration sits below an applied one (out of
// order merge), since either would silently diverge the schema.
func pendingMigrations(migs []migration, applied map[int]bool) ([]migration, error) {
	known := make(map[int]bool, len(migs))
	for _, m := range migs {
		known[m.version] = true
	}
	maxApplied := 0
	for v := range applied {
		if !known[v] {
			return nil, fmt.Errorf("database has schema version %d unknown to this binary; upgrade swingdesk", v)
		}
		maxApplied = max(maxApplied, v)
	}
	var pending []migration
	for _, m := range migs {
		if applied[m.version] {
			continue
		}
		if m.version < maxApplied {
			return nil, fmt.Errorf("migration %s is below applied version %d; renumber it", m.name, maxApplied)
		}
		pending = append(pending, m)
	}
	return pending, nil
}

func applyOne(ctx context.Context, conn *sql.DB, m migration) error {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration %s: %w", m.name, err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, m.sql); err != nil {
		return fmt.Errorf("apply migration %s: %w", m.name, err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO schema_migrations (version, name, applied_at) VALUES (?, ?, ?)`,
		m.version, m.name, time.Now().UTC().Format(time.RFC3339)); err != nil {
		return fmt.Errorf("record migration %s: %w", m.name, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration %s: %w", m.name, err)
	}
	return nil
}
