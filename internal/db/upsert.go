package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/hsk-kr/swingdesk/internal/model"
)

// UpsertOutcome says what UpsertItem did.
type UpsertOutcome int

const (
	UpsertInserted UpsertOutcome = iota + 1
	UpsertUpdated
)

// UpsertItem inserts it keyed by DedupeKey(it.Symbol, it.Title, it.URL). On
// a key match it refreshes summary, body, source and published_at but keeps
// read_at, created_at and the original run, so a re-reported story neither
// resurfaces as unread nor jumps the queue.
func UpsertItem(ctx context.Context, conn DBTX, runID int64, it model.Item) (int64, UpsertOutcome, error) {
	if !it.Category.Valid() {
		return 0, 0, fmt.Errorf("invalid category %q", it.Category)
	}
	key := DedupeKey(it.Symbol, it.Title, it.URL)
	var id int64
	err := conn.QueryRowContext(ctx, `SELECT id FROM items WHERE dedupe_key = ?`, key).Scan(&id)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return insertNewItem(ctx, conn, runID, key, it)
	case err != nil:
		return 0, 0, fmt.Errorf("lookup item %q: %w", it.Title, err)
	}
	if _, err := conn.ExecContext(ctx, `UPDATE items
		SET summary = ?, body = ?, source = ?, published_at = COALESCE(?, published_at)
		WHERE id = ?`,
		it.Summary, it.Body, nullIfEmpty(it.Source), nullTime(it.PublishedAt), id); err != nil {
		return 0, 0, fmt.Errorf("update item %d: %w", id, err)
	}
	return id, UpsertUpdated, nil
}

func insertNewItem(ctx context.Context, conn DBTX, runID int64, key string, it model.Item) (int64, UpsertOutcome, error) {
	res, err := conn.ExecContext(ctx, `INSERT INTO items
		(dedupe_key, run_id, instrument_id, category, title, summary, body, source, url, published_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		key, nullID(runID), nullID(it.InstrumentID), string(it.Category), it.Title, it.Summary, it.Body,
		nullIfEmpty(it.Source), nullIfEmpty(it.URL), nullTime(it.PublishedAt), formatTime(it.CreatedAt))
	if err != nil {
		return 0, 0, fmt.Errorf("insert item %q: %w", it.Title, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, 0, fmt.Errorf("insert item id: %w", err)
	}
	return id, UpsertInserted, nil
}

// UpsertEvent records the dated event for an item, replacing title, date and
// kind if the item already has one (re-ingest must not duplicate events).
func UpsertEvent(ctx context.Context, conn DBTX, ev model.Event, createdAt time.Time) error {
	if !ev.Kind.Valid() {
		return fmt.Errorf("invalid event kind %q", ev.Kind)
	}
	res, err := conn.ExecContext(ctx, `UPDATE events SET title = ?, event_at = ?, kind = ? WHERE item_id = ?`,
		ev.Title, nullTime(ev.At), string(ev.Kind), ev.ItemID)
	if err != nil {
		return fmt.Errorf("update event for item %d: %w", ev.ItemID, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("event rows affected: %w", err)
	}
	if n > 0 {
		return nil
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO events (instrument_id, item_id, title, event_at, kind, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		nullID(ev.InstrumentID), ev.ItemID, ev.Title, nullTime(ev.At), string(ev.Kind), formatTime(createdAt)); err != nil {
		return fmt.Errorf("insert event for item %d: %w", ev.ItemID, err)
	}
	return nil
}
