package sample

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/hsk-kr/swingdesk"
	"github.com/hsk-kr/swingdesk/internal/db"
	"github.com/hsk-kr/swingdesk/internal/model"
	"github.com/hsk-kr/swingdesk/internal/watchlist"
)

func TestInsertIsIdempotentForItems(t *testing.T) {
	ctx := context.Background()
	conn, err := db.Open(ctx, filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	seed, _ := watchlist.Parse(swingdesk.WatchlistYAML)
	if _, err := db.SeedInstruments(ctx, conn, seed); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	n, err := Insert(ctx, conn, now)
	if err != nil || n != len(fixtureRows) {
		t.Fatalf("Insert = %d, %v", n, err)
	}
	if n, err := Insert(ctx, conn, now.Add(time.Minute)); err != nil || n != 0 {
		t.Errorf("second Insert = %d, %v", n, err)
	}
	items, err := db.UnreadItems(ctx, conn, model.ItemFilter{})
	if err != nil || len(items) != len(fixtureRows) {
		t.Fatalf("unread = %d, %v", len(items), err)
	}
	if items[0].Symbol != "NVDA" {
		t.Errorf("newest = %+v", items[0])
	}
	biases, err := db.LatestBiases(ctx, conn)
	if err != nil || len(biases) != 3 {
		t.Errorf("biases = %d, %v", len(biases), err)
	}
	var rows int
	if err := conn.QueryRow(`SELECT COUNT(*) FROM biases`).Scan(&rows); err != nil || rows != 3 {
		t.Errorf("bias rows = %d, %v; re-running must not duplicate", rows, err)
	}
}
