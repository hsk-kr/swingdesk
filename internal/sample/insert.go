package sample

import (
	"context"
	"database/sql"
	"time"

	"github.com/hsk-kr/swingdesk/internal/db"
)

// Insert writes the sample items and biases into conn. Items already present
// (same dedupe key) are skipped. It returns the number of new items.
func Insert(ctx context.Context, conn *sql.DB, now time.Time) (int, error) {
	instruments, err := db.ListInstruments(ctx, conn)
	if err != nil {
		return 0, err
	}
	added := 0
	for _, it := range Items(instruments, now) {
		_, inserted, err := db.InsertItem(ctx, conn, it)
		if err != nil {
			return added, err
		}
		if inserted {
			added++
		}
	}
	for _, b := range Biases(instruments, now) {
		if err := db.InsertBias(ctx, conn, 0, b); err != nil {
			return added, err
		}
	}
	return added, nil
}
