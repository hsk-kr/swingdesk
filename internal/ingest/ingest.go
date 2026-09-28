package ingest

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/hsk-kr/swingdesk/internal/db"
	"github.com/hsk-kr/swingdesk/internal/model"
)

// DefaultMaxItems is the contract's per-file cap.
const DefaultMaxItems = 40

// Options controls an ingest.
type Options struct {
	RunID    int64        // refresh_runs.id, 0 for none
	Now      time.Time    // created_at for new rows
	MaxItems int          // 0 = DefaultMaxItems
	Logger   *slog.Logger // nil = discard
}

// SkipKind says which list a skipped entry came from.
type SkipKind string

const (
	SkipItem        SkipKind = "item"
	SkipBias        SkipKind = "bias"
	SkipEvent       SkipKind = "event"        // item kept, event dropped
	SkipPublishedAt SkipKind = "published_at" // item kept, field ignored
)

// Skip is one item or bias that was not written.
type Skip struct {
	Kind   SkipKind
	Index  int
	Symbol string
	Reason string
}

// Result summarizes one file.
type Result struct {
	Job      model.Job
	Inserted int
	Updated  int
	Events   int
	Biases   int
	Skipped  []Skip // entries not written
	Warnings []Skip // items written with a field dropped
}

// ErrInvalidFile marks a job file that can never ingest (bad JSON, bad
// envelope, claude error result). Other errors (I/O, DB, ctx) are transient
// and the file should be retried.
var ErrInvalidFile = errors.New("invalid job file")

// IngestFile parses and ingests path.
func IngestFile(ctx context.Context, conn *sql.DB, path string, opts Options) (Result, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Result{}, fmt.Errorf("read %s: %w", path, err)
	}
	env, err := Parse(raw)
	if err != nil {
		return Result{}, fmt.Errorf("%s: %w: %w", path, ErrInvalidFile, err)
	}
	return Ingest(ctx, conn, env, opts)
}

// Ingest writes env in one transaction. Invalid or unknown-symbol entries are
// skipped and reported; a database error rolls back the whole file.
func Ingest(ctx context.Context, conn *sql.DB, env Envelope, opts Options) (Result, error) {
	opts = opts.withDefaults()
	instruments, err := db.ListInstruments(ctx, conn)
	if err != nil {
		return Result{}, err
	}
	known := make(map[string]model.Instrument, len(instruments))
	for _, in := range instruments {
		known[strings.ToUpper(in.Symbol)] = in
	}

	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return Result{}, fmt.Errorf("begin ingest: %w", err)
	}
	defer tx.Rollback()

	res := Result{Job: env.Job}
	if res, err = ingestItems(ctx, tx, env, known, opts, res); err != nil {
		return Result{}, err
	}
	if res, err = ingestBiases(ctx, tx, env, known, opts, res); err != nil {
		return Result{}, err
	}
	if err := tx.Commit(); err != nil {
		return Result{}, fmt.Errorf("commit ingest: %w", err)
	}
	for _, s := range res.Skipped {
		opts.Logger.Warn("ingest skipped", "job", env.Job, "kind", s.Kind, "index", s.Index, "symbol", s.Symbol, "reason", s.Reason)
	}
	for _, s := range res.Warnings {
		opts.Logger.Warn("ingest dropped field", "job", env.Job, "kind", s.Kind, "index", s.Index, "symbol", s.Symbol, "reason", s.Reason)
	}
	return res, nil
}

func (o Options) withDefaults() Options {
	if o.MaxItems <= 0 {
		o.MaxItems = DefaultMaxItems
	}
	if o.Now.IsZero() {
		o.Now = time.Now()
	}
	if o.Logger == nil {
		o.Logger = slog.New(slog.DiscardHandler)
	}
	return o
}

func ingestItems(ctx context.Context, tx *sql.Tx, env Envelope, known map[string]model.Instrument, opts Options, res Result) (Result, error) {
	for i, raw := range env.Items {
		if i >= opts.MaxItems {
			res.Skipped = append(res.Skipped, Skip{Kind: SkipItem, Index: i, Reason: fmt.Sprintf("over cap of %d items", opts.MaxItems)})
			continue
		}
		sym, err := checkItem(env.Job, raw, known)
		if err != nil {
			res.Skipped = append(res.Skipped, Skip{Kind: SkipItem, Index: i, Symbol: symbolOf(raw), Reason: err.Error()})
			continue
		}
		w, err := writeItem(ctx, tx, raw, known[sym], opts)
		if err != nil {
			return Result{}, fmt.Errorf("item %d: %w", i, err)
		}
		res = res.add(i, symbolOf(raw), w)
	}
	return res, nil
}

// written describes what writeItem did for one item.
type written struct {
	outcome  db.UpsertOutcome
	event    bool
	warnings []Skip // Index/Symbol filled in by Result.add
}

func (r Result) add(index int, symbol string, w written) Result {
	switch w.outcome {
	case db.UpsertInserted:
		r.Inserted++
	case db.UpsertUpdated:
		r.Updated++
	}
	if w.event {
		r.Events++
	}
	for _, warn := range w.warnings {
		warn.Index, warn.Symbol = index, symbol
		r.Warnings = append(r.Warnings, warn)
	}
	return r
}

// writeItem upserts one validated item and its event (if event_at parses).
// Unparseable optional timestamps are dropped and reported, not fatal.
func writeItem(ctx context.Context, tx *sql.Tx, raw RawItem, in model.Instrument, opts Options) (written, error) {
	var w written
	published, err := parseWhen(raw.PublishedAt)
	if err != nil {
		w.warnings = append(w.warnings, Skip{Kind: SkipPublishedAt, Reason: err.Error()})
	}
	it := model.Item{
		InstrumentID: in.ID,
		Symbol:       in.Symbol,
		Category:     raw.Category,
		Title:        strings.TrimSpace(raw.Title),
		Summary:      strings.TrimSpace(raw.Summary),
		Body:         strings.TrimSpace(raw.Body),
		Source:       strings.TrimSpace(raw.Source),
		URL:          strings.TrimSpace(raw.URL),
		PublishedAt:  published,
		CreatedAt:    opts.Now,
	}
	id, outcome, err := db.UpsertItem(ctx, tx, opts.RunID, it)
	if err != nil {
		return written{}, err
	}
	w.outcome = outcome
	eventAt, err := parseWhen(raw.EventAt)
	switch {
	case err != nil:
		w.warnings = append(w.warnings, Skip{Kind: SkipEvent, Reason: err.Error()})
		return w, nil
	case eventAt.IsZero():
		return w, nil
	}
	ev := model.Event{InstrumentID: in.ID, ItemID: id, Title: it.Title, At: eventAt, Kind: eventKind(raw.EventKind)}
	if err := db.UpsertEvent(ctx, tx, ev, opts.Now); err != nil {
		return written{}, err
	}
	w.event = true
	return w, nil
}

func ingestBiases(ctx context.Context, tx *sql.Tx, env Envelope, known map[string]model.Instrument, opts Options, res Result) (Result, error) {
	seen := map[int64]bool{}
	for i, raw := range env.Biases {
		in, err := checkBias(raw, known)
		if err == nil && seen[in.ID] {
			err = fmt.Errorf("duplicate bias for %s in this file", in.Symbol)
		}
		if err != nil {
			res.Skipped = append(res.Skipped, Skip{Kind: SkipBias, Index: i, Symbol: raw.Symbol, Reason: err.Error()})
			continue
		}
		seen[in.ID] = true
		b := model.Bias{
			InstrumentID: in.ID,
			Stance:       raw.Stance,
			Confidence:   *raw.Confidence,
			Rationale:    strings.TrimSpace(raw.Rationale),
			CreatedAt:    opts.Now,
		}
		if err := db.InsertBias(ctx, tx, opts.RunID, b); err != nil {
			return Result{}, fmt.Errorf("bias %d: %w", i, err)
		}
		res.Biases++
	}
	return res, nil
}

func symbolOf(it RawItem) string {
	if it.Symbol == nil {
		return ""
	}
	return *it.Symbol
}
