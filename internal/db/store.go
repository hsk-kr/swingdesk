package db

import (
	"context"
	"database/sql"
	"time"

	"github.com/hsk-kr/swingdesk/internal/model"
)

// Store adapts a connection to the inbox operations the UI needs.
type Store struct{ conn *sql.DB }

// NewStore wraps conn.
func NewStore(conn *sql.DB) Store { return Store{conn: conn} }

// Unread returns unread items for f, newest first.
func (s Store) Unread(ctx context.Context, f model.ItemFilter) ([]model.Item, error) {
	return UnreadItems(ctx, s.conn, f)
}

// Counts returns unread badge counts.
func (s Store) Counts(ctx context.Context) (model.UnreadCounts, error) {
	return CountUnread(ctx, s.conn)
}

// Biases returns the latest stance per instrument.
func (s Store) Biases(ctx context.Context) (map[int64]model.Bias, error) {
	return LatestBiases(ctx, s.conn)
}

// MarkRead marks ids read at `at` and returns the ids changed.
func (s Store) MarkRead(ctx context.Context, ids []int64, at time.Time) ([]int64, error) {
	return MarkRead(ctx, s.conn, ids, at)
}

// MarkUnread clears read_at on ids and returns the ids changed.
func (s Store) MarkUnread(ctx context.Context, ids []int64) ([]int64, error) {
	return MarkUnread(ctx, s.conn, ids)
}
