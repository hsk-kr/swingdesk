package db

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/hsk-kr/swingdesk"
	"github.com/hsk-kr/swingdesk/internal/model"
	"github.com/hsk-kr/swingdesk/internal/watchlist"
)

func openTemp(t *testing.T) (*sql.DB, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "nested", "swingdesk.db")
	conn, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn, path
}

func seed(t *testing.T) []model.Instrument {
	t.Helper()
	in, err := watchlist.Parse(swingdesk.WatchlistYAML)
	if err != nil {
		t.Fatal(err)
	}
	return in
}

func TestOpenAppliesPragmasAndSchema(t *testing.T) {
	conn, _ := openTemp(t)
	ctx := context.Background()

	var mode string
	if err := conn.QueryRowContext(ctx, `PRAGMA journal_mode`).Scan(&mode); err != nil || mode != "wal" {
		t.Errorf("journal_mode = %q, %v", mode, err)
	}
	var fk int
	if err := conn.QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&fk); err != nil || fk != 1 {
		t.Errorf("foreign_keys = %d, %v", fk, err)
	}
	for _, table := range []string{"instruments", "refresh_runs", "items", "events", "biases", "schema_migrations", "app_meta"} {
		var name string
		err := conn.QueryRowContext(ctx, `SELECT name FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&name)
		if err != nil {
			t.Errorf("table %s missing: %v", table, err)
		}
	}
}

func TestForeignKeysEnforced(t *testing.T) {
	conn, _ := openTemp(t)
	_, err := conn.Exec(`INSERT INTO biases (instrument_id, stance, created_at) VALUES (999, 'long', 'x')`)
	if err == nil {
		t.Fatal("expected foreign key violation")
	}
}

func TestMigrateIsIdempotentAcrossReopen(t *testing.T) {
	conn, path := openTemp(t)
	conn.Close()
	conn2, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer conn2.Close()
	var n int
	if err := conn2.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&n); err != nil || n != 3 {
		t.Errorf("schema_migrations rows = %d, %v", n, err)
	}
}

func TestSeedAndQuerySymbols(t *testing.T) {
	conn, _ := openTemp(t)
	ctx := context.Background()

	inserted, err := SeedInstruments(ctx, conn, seed(t))
	if err != nil || !inserted {
		t.Fatalf("SeedInstruments = %v, %v", inserted, err)
	}
	got, err := ListInstruments(ctx, conn)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 14 {
		t.Fatalf("len = %d", len(got))
	}
	if got[0].Symbol != "GOOG" || got[len(got)-1].Symbol != "TECH100" {
		t.Errorf("order: first %s last %s", got[0].Symbol, got[len(got)-1].Symbol)
	}
	by := map[string]model.Instrument{}
	for _, in := range got {
		by[in.Symbol] = in
	}
	if by["SPCX"].Kind != model.KindCFD || !strings.Contains(by["SPCX"].Notes, "Private") {
		t.Errorf("SPCX = %+v", by["SPCX"])
	}
	if by["CRUDE"].Kind != model.KindCommodity || by["CRUDE"].Notes == "" {
		t.Errorf("CRUDE = %+v", by["CRUDE"])
	}
	if by["TECH100"].Kind != model.KindIndex || by["SKHY"].Notes == "" {
		t.Errorf("TECH100/SKHY lost fields")
	}
	if by["GOOG"].CompanyTag != "alphabet" || by["GOOGL"].CompanyTag != "alphabet" {
		t.Errorf("alphabet tag missing")
	}
	if by["NVDA"].CompanyTag != "" || !by["NVDA"].Enabled {
		t.Errorf("NVDA = %+v", by["NVDA"])
	}
}

func TestSeedDoesNotOverwriteUserEdits(t *testing.T) {
	conn, _ := openTemp(t)
	ctx := context.Background()
	if _, err := SeedInstruments(ctx, conn, seed(t)); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(`UPDATE instruments SET name = 'edited' WHERE symbol = 'NVDA'`); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(`DELETE FROM instruments WHERE symbol = 'DELL'`); err != nil {
		t.Fatal(err)
	}
	inserted, err := SeedInstruments(ctx, conn, seed(t))
	if err != nil || inserted {
		t.Fatalf("second seed = %v, %v", inserted, err)
	}
	var name string
	if err := conn.QueryRow(`SELECT name FROM instruments WHERE symbol='NVDA'`).Scan(&name); err != nil || name != "edited" {
		t.Errorf("name = %q, %v", name, err)
	}
	var n int
	if err := conn.QueryRow(`SELECT COUNT(*) FROM instruments WHERE symbol='DELL'`).Scan(&n); err != nil || n != 0 {
		t.Errorf("DELL resurrected")
	}
}

func TestLoadMigrationsValidation(t *testing.T) {
	bad := map[string]fstest.MapFS{
		"no number": {"migrations/init.sql": {Data: []byte("SELECT 1;")}},
		"zero":      {"migrations/0000_x.sql": {Data: []byte("SELECT 1;")}},
		"duplicate": {
			"migrations/0001_a.sql": {Data: []byte("SELECT 1;")},
			"migrations/1_b.sql":    {Data: []byte("SELECT 1;")},
		},
	}
	for name, fsys := range bad {
		t.Run(name, func(t *testing.T) {
			if _, err := loadMigrations(fsys); err == nil {
				t.Fatal("expected error")
			}
		})
	}
	good := fstest.MapFS{
		"migrations/0002_b.sql": {Data: []byte("B")},
		"migrations/0001_a.sql": {Data: []byte("A")},
		"migrations/README.txt": {Data: []byte("ignored")},
		"migrations/0010_c.sql": {Data: []byte("C")},
	}
	migs, err := loadMigrations(good)
	if err != nil {
		t.Fatal(err)
	}
	if len(migs) != 3 || migs[0].version != 1 || migs[2].version != 10 {
		t.Errorf("migs = %+v", migs)
	}
}

func TestFailedMigrationRollsBack(t *testing.T) {
	conn, _ := openTemp(t)
	ctx := context.Background()
	migs := []migration{{version: 4, name: "0004_bad.sql", sql: "CREATE TABLE t2 (x INT); NOT SQL;"}}
	if err := applyMigrations(ctx, conn, migs); err == nil {
		t.Fatal("expected error")
	}
	var n int
	if err := conn.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name='t2'`).Scan(&n); err != nil || n != 0 {
		t.Errorf("partial migration left table behind: n=%d err=%v", n, err)
	}
	var maxV int
	if err := conn.QueryRow(`SELECT MAX(version) FROM schema_migrations`).Scan(&maxV); err != nil || maxV != 3 {
		t.Errorf("version = %d, want 3 (%v)", maxV, err)
	}
}

// TestInitMigrationMirrorsDocsSchema keeps docs/SCHEMA.sql from drifting.
func TestInitMigrationMirrorsDocsSchema(t *testing.T) {
	docs, err := os.ReadFile("../../docs/SCHEMA.sql")
	if err != nil {
		t.Fatal(err)
	}
	mig, err := migrationFS.ReadFile("migrations/0001_init.sql")
	if err != nil {
		t.Fatal(err)
	}
	if normalize(string(docs)) != normalize(string(mig)) {
		t.Error("migrations/0001_init.sql and docs/SCHEMA.sql differ (ignoring comments, PRAGMAs and blank lines)")
	}
}

func normalize(s string) string {
	var keep []string
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "--") || strings.HasPrefix(line, "PRAGMA") {
			continue
		}
		keep = append(keep, line)
	}
	return strings.Join(keep, "\n")
}

func TestSeedNotResurrectedAfterDeletingAll(t *testing.T) {
	conn, _ := openTemp(t)
	ctx := context.Background()
	if _, err := SeedInstruments(ctx, conn, seed(t)); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(`DELETE FROM instruments`); err != nil {
		t.Fatal(err)
	}
	inserted, err := SeedInstruments(ctx, conn, seed(t))
	if err != nil || inserted {
		t.Fatalf("reseed = %v, %v", inserted, err)
	}
}

func TestOpenEscapesSpecialPathChars(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "a#b%c?d")
	path := filepath.Join(dir, "swingdesk.db")
	conn, err := Open(context.Background(), path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer conn.Close()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("db not at exact path: %v", err)
	}
}

func TestPendingMigrations(t *testing.T) {
	migs := []migration{{version: 1, name: "0001_a.sql"}, {version: 2, name: "0002_b.sql"}, {version: 3, name: "0003_c.sql"}}
	got, err := pendingMigrations(migs, map[int]bool{1: true})
	if err != nil || len(got) != 2 || got[0].version != 2 {
		t.Errorf("pending = %+v, %v", got, err)
	}
	if _, err := pendingMigrations(migs, map[int]bool{1: true, 3: true}); err == nil {
		t.Error("expected gap error for unapplied 0002 below applied 3")
	}
	if _, err := pendingMigrations(migs, map[int]bool{1: true, 9: true}); err == nil {
		t.Error("expected error for db ahead of binary")
	}
}

func TestListInstrumentsRejectsInvalidKind(t *testing.T) {
	conn, _ := openTemp(t)
	// Bypass the CHECK constraint to simulate a corrupted row. One connection
	// so the per-connection pragma applies to the insert.
	conn.SetMaxOpenConns(1)
	if _, err := conn.Exec(`PRAGMA ignore_check_constraints = ON`); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(`INSERT INTO instruments (symbol, name, kind) VALUES ('X', 'X', 'bogus')`); err != nil {
		t.Fatal(err)
	}
	if _, err := ListInstruments(context.Background(), conn); err == nil {
		t.Fatal("expected invalid kind error")
	}
}
