package db

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/hsk-kr/swingdesk/internal/model"
)

// DedupeKey is sha256(lower(symbol) + '|' + lower(title) + '|' + url).
func DedupeKey(symbol, title, url string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(symbol) + "|" + strings.ToLower(title) + "|" + url))
	return hex.EncodeToString(sum[:])
}

const itemColumns = `i.id, COALESCE(i.instrument_id, 0), COALESCE(ins.symbol, ''), i.category,
	i.title, i.summary, i.body, COALESCE(i.source, ''), COALESCE(i.url, ''),
	COALESCE(i.published_at, ''), i.created_at, COALESCE(i.read_at, '')`

// UnreadItems returns unread items matching f, newest created_at first.
func UnreadItems(ctx context.Context, conn *sql.DB, f model.ItemFilter) ([]model.Item, error) {
	q := `SELECT ` + itemColumns + ` FROM items i
		LEFT JOIN instruments ins ON ins.id = i.instrument_id
		WHERE i.read_at IS NULL`
	var args []any
	if f.InstrumentID != 0 {
		q += ` AND i.instrument_id = ?`
		args = append(args, f.InstrumentID)
	}
	if f.Category != "" {
		q += ` AND i.category = ?`
		args = append(args, string(f.Category))
	}
	q += ` ORDER BY i.created_at DESC, i.id DESC`

	rows, err := conn.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("query unread items: %w", err)
	}
	defer rows.Close()
	var out []model.Item
	for rows.Next() {
		it, err := scanItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

func scanItem(rows *sql.Rows) (model.Item, error) {
	var it model.Item
	var cat, published, created, read string
	if err := rows.Scan(&it.ID, &it.InstrumentID, &it.Symbol, &cat, &it.Title, &it.Summary, &it.Body,
		&it.Source, &it.URL, &published, &created, &read); err != nil {
		return model.Item{}, fmt.Errorf("scan item: %w", err)
	}
	it.Category = model.Category(cat)
	if !it.Category.Valid() {
		return model.Item{}, fmt.Errorf("item %d has invalid category %q", it.ID, cat)
	}
	var err error
	if it.PublishedAt, err = parseTime(published); err != nil {
		return model.Item{}, fmt.Errorf("item %d published_at: %w", it.ID, err)
	}
	if it.CreatedAt, err = parseTime(created); err != nil {
		return model.Item{}, fmt.Errorf("item %d created_at: %w", it.ID, err)
	}
	if it.ReadAt, err = parseTime(read); err != nil {
		return model.Item{}, fmt.Errorf("item %d read_at: %w", it.ID, err)
	}
	return it, nil
}

// CountUnread returns unread totals by category and by instrument.
func CountUnread(ctx context.Context, conn *sql.DB) (model.UnreadCounts, error) {
	counts := model.UnreadCounts{ByCategory: map[model.Category]int{}, ByInstrument: map[int64]int{}}
	rows, err := conn.QueryContext(ctx, `SELECT category, COALESCE(instrument_id, 0), COUNT(*)
		FROM items WHERE read_at IS NULL GROUP BY category, instrument_id`)
	if err != nil {
		return model.UnreadCounts{}, fmt.Errorf("count unread: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var cat string
		var instID int64
		var n int
		if err := rows.Scan(&cat, &instID, &n); err != nil {
			return model.UnreadCounts{}, fmt.Errorf("scan unread count: %w", err)
		}
		counts.Total += n
		counts.ByCategory[model.Category(cat)] += n
		if instID != 0 {
			counts.ByInstrument[instID] += n
		}
	}
	return counts, rows.Err()
}

// MarkRead sets read_at on the given unread items and returns the ids that
// actually changed (already-read or missing ids are skipped).
func MarkRead(ctx context.Context, conn *sql.DB, ids []int64, at time.Time) ([]int64, error) {
	return updateReadAt(ctx, conn, ids, formatTime(at), "read_at IS NULL")
}

// MarkUnread clears read_at on the given items and returns the ids changed.
func MarkUnread(ctx context.Context, conn *sql.DB, ids []int64) ([]int64, error) {
	return updateReadAt(ctx, conn, ids, nil, "read_at IS NOT NULL")
}

func updateReadAt(ctx context.Context, conn *sql.DB, ids []int64, value any, guard string) ([]int64, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin read update: %w", err)
	}
	defer tx.Rollback()
	changed := make([]int64, 0, len(ids))
	for _, id := range ids {
		res, err := tx.ExecContext(ctx, `UPDATE items SET read_at = ? WHERE id = ? AND `+guard, value, id)
		if err != nil {
			return nil, fmt.Errorf("update read_at for item %d: %w", id, err)
		}
		if n, err := res.RowsAffected(); err != nil {
			return nil, fmt.Errorf("rows affected: %w", err)
		} else if n > 0 {
			changed = append(changed, id)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit read update: %w", err)
	}
	return changed, nil
}

// InsertItem inserts it (keyed by DedupeKey) unless the key already exists.
// It returns the row id and whether a new row was created. Used by the
// sample-data helper; the agent ingest path upserts instead.
func InsertItem(ctx context.Context, conn *sql.DB, it model.Item) (int64, bool, error) {
	if !it.Category.Valid() {
		return 0, false, fmt.Errorf("invalid category %q", it.Category)
	}
	key := DedupeKey(it.Symbol, it.Title, it.URL)
	res, err := conn.ExecContext(ctx, `INSERT INTO items
		(dedupe_key, instrument_id, category, title, summary, body, source, url, published_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(dedupe_key) DO NOTHING`,
		key, nullID(it.InstrumentID), string(it.Category), it.Title, it.Summary, it.Body,
		nullIfEmpty(it.Source), nullIfEmpty(it.URL), nullTime(it.PublishedAt), formatTime(it.CreatedAt))
	if err != nil {
		return 0, false, fmt.Errorf("insert item %q: %w", it.Title, err)
	}
	if n, err := res.RowsAffected(); err != nil || n == 0 {
		return 0, false, err
	}
	id, err := res.LastInsertId()
	return id, err == nil, err
}

func nullID(id int64) any {
	if id == 0 {
		return nil
	}
	return id
}
