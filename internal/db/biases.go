package db

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/hsk-kr/swingdesk/internal/model"
)

// LatestBiases returns the newest bias row per instrument.
func LatestBiases(ctx context.Context, conn *sql.DB) (map[int64]model.Bias, error) {
	rows, err := conn.QueryContext(ctx, `SELECT b.instrument_id, b.stance, b.confidence, b.rationale, b.created_at
		FROM biases b
		WHERE b.id = (SELECT b2.id FROM biases b2 WHERE b2.instrument_id = b.instrument_id
		              ORDER BY b2.created_at DESC, b2.id DESC LIMIT 1)`)
	if err != nil {
		return nil, fmt.Errorf("query latest biases: %w", err)
	}
	defer rows.Close()
	out := map[int64]model.Bias{}
	for rows.Next() {
		var b model.Bias
		var stance, created string
		if err := rows.Scan(&b.InstrumentID, &stance, &b.Confidence, &b.Rationale, &created); err != nil {
			return nil, fmt.Errorf("scan bias: %w", err)
		}
		b.Stance = model.Stance(stance)
		if !b.Stance.Valid() {
			return nil, fmt.Errorf("bias for instrument %d has invalid stance %q", b.InstrumentID, stance)
		}
		if b.CreatedAt, err = parseTime(created); err != nil {
			return nil, fmt.Errorf("bias created_at: %w", err)
		}
		out[b.InstrumentID] = b
	}
	return out, rows.Err()
}

// InsertBias records a stance for an instrument (runID 0 = no run).
func InsertBias(ctx context.Context, conn DBTX, runID int64, b model.Bias) error {
	if !b.Stance.Valid() {
		return fmt.Errorf("invalid stance %q", b.Stance)
	}
	_, err := conn.ExecContext(ctx, `INSERT INTO biases (run_id, instrument_id, stance, confidence, rationale, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		nullID(runID), b.InstrumentID, string(b.Stance), b.Confidence, b.Rationale, formatTime(b.CreatedAt))
	if err != nil {
		return fmt.Errorf("insert bias for instrument %d: %w", b.InstrumentID, err)
	}
	return nil
}
