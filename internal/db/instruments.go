package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/hsk-kr/swingdesk/internal/model"
)

// SeedInstruments inserts seed rows once per database, recorded as
// app_meta.seeded_at, so later user edits (including deleting every row) are
// never overwritten. It reports whether it inserted.
func SeedInstruments(ctx context.Context, conn *sql.DB, seed []model.Instrument) (bool, error) {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin seed: %w", err)
	}
	defer tx.Rollback()

	var seeded int
	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM app_meta WHERE key = 'seeded_at'`).Scan(&seeded); err != nil {
		return false, fmt.Errorf("check seed marker: %w", err)
	}
	if seeded > 0 {
		return false, nil
	}
	for _, in := range seed {
		if _, err := tx.ExecContext(ctx, `INSERT INTO instruments
			(symbol, name, kind, company_tag, notes, enabled, sort_order)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			in.Symbol, in.Name, string(in.Kind), nullIfEmpty(in.CompanyTag), nullIfEmpty(in.Notes),
			in.Enabled, in.SortOrder); err != nil {
			return false, fmt.Errorf("seed %s: %w", in.Symbol, err)
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO app_meta (key, value) VALUES ('seeded_at', ?)`,
		formatTime(time.Now())); err != nil {
		return false, fmt.Errorf("record seed marker: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit seed: %w", err)
	}
	return true, nil
}

// ListInstruments returns all instruments ordered by sort_order, symbol.
func ListInstruments(ctx context.Context, conn *sql.DB) ([]model.Instrument, error) {
	rows, err := conn.QueryContext(ctx, `SELECT id, symbol, name, kind,
		COALESCE(company_tag, ''), COALESCE(notes, ''), enabled, sort_order
		FROM instruments ORDER BY sort_order, symbol`)
	if err != nil {
		return nil, fmt.Errorf("list instruments: %w", err)
	}
	defer rows.Close()
	var out []model.Instrument
	for rows.Next() {
		var in model.Instrument
		var kind string
		if err := rows.Scan(&in.ID, &in.Symbol, &in.Name, &kind, &in.CompanyTag, &in.Notes, &in.Enabled, &in.SortOrder); err != nil {
			return nil, fmt.Errorf("scan instrument: %w", err)
		}
		in.Kind = model.Kind(kind)
		if !in.Kind.Valid() {
			return nil, fmt.Errorf("instrument %s has invalid kind %q", in.Symbol, kind)
		}
		out = append(out, in)
	}
	return out, rows.Err()
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
