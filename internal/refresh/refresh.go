package refresh

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/hsk-kr/swingdesk/internal/agent"
	"github.com/hsk-kr/swingdesk/internal/db"
	"github.com/hsk-kr/swingdesk/internal/ingest"
	"github.com/hsk-kr/swingdesk/internal/model"
)

// JobRunner is the agent runner (agent.Runner in production).
type JobRunner interface {
	Run(ctx context.Context, runID int64, now time.Time, instruments []model.Instrument, progress agent.Progress) ([]agent.Result, error)
}

// Deps wires a Refresher.
type Deps struct {
	Conn     *sql.DB
	Runner   JobRunner
	InboxDir string // data_dir/inbox
	RunsDir  string // data_dir/runs (ingested files are archived here)
	LockPath string // data_dir/refresh.lock
	MaxItems int
	Now      func() time.Time
	Logger   *slog.Logger // ingest skips and leftover failures; nil = discard
}

// Refresher performs whole refreshes.
type Refresher struct{ d Deps }

// New builds a Refresher.
func New(d Deps) Refresher {
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Logger == nil {
		d.Logger = slog.New(slog.DiscardHandler)
	}
	return Refresher{d: d}
}

// JobOutcome is one job's fate in a refresh.
type JobOutcome struct {
	Job      model.Job
	Err      error // runner or ingest failure
	Inserted int
	Updated  int
	Skipped  int
}

// Outcome summarizes a refresh.
type Outcome struct {
	RunID     int64
	Status    model.RunStatus
	Jobs      []JobOutcome
	Leftovers Leftovers
	Err       error // run-level failure (busy, tmux/claude missing, DB error)
	Finished  time.Time
}

// NewItems counts rows inserted by this refresh, leftovers included.
func (o Outcome) NewItems() int {
	n := o.Leftovers.Inserted
	for _, j := range o.Jobs {
		n += j.Inserted
	}
	return n
}

// Refresh records a refresh_runs row, imports leftover files from earlier
// sessions, runs the agents and ingests what they wrote. If ctx is cancelled
// (quit) the run is left 'running' for the next launch to reconcile.
// progress (optional) is told as each agent job finishes.
func (r Refresher) Refresh(ctx context.Context, progress agent.Progress) Outcome {
	var out Outcome
	lock, err := acquireLock(r.d.LockPath)
	if err != nil {
		out.Err, out.Status = err, model.RunError
		return r.done(out)
	}
	defer lock.release()

	started := r.d.Now()
	if out.RunID, err = db.StartRun(ctx, r.d.Conn, started); err != nil {
		out.Err, out.Status = err, model.RunError
		return r.done(out)
	}
	out.Leftovers = r.ImportLeftovers(ctx, out.RunID)
	instruments, err := enabledInstruments(ctx, r.d.Conn)
	if err != nil {
		return r.finish(ctx, out, err)
	}
	results, runErr := r.d.Runner.Run(ctx, out.RunID, started, instruments, progress)
	if ctx.Err() != nil {
		out.Err = ctx.Err()
		return r.done(out) // leave the row running; leftovers import finishes it
	}
	for _, res := range results {
		out.Jobs = append(out.Jobs, r.ingestResult(ctx, out.RunID, res))
	}
	return r.finish(ctx, out, runErr)
}

func (r Refresher) finish(ctx context.Context, out Outcome, runErr error) Outcome {
	ok, msgs := 0, []string{}
	if runErr != nil {
		msgs = append(msgs, runErr.Error())
	}
	for _, j := range out.Jobs {
		if j.Err == nil {
			ok++
			continue
		}
		msgs = append(msgs, fmt.Sprintf("%s: %v", j.Job, j.Err))
	}
	total := len(model.Jobs())
	out.Status = model.RunStatusFor(ok, total)
	out.Err = runErr
	run := db.Run{ID: out.RunID, FinishedAt: r.d.Now(), Status: out.Status, Error: strings.Join(msgs, "; "), JobsOK: ok, JobsFail: total - ok}
	if err := db.FinishRun(ctx, r.d.Conn, run); err != nil {
		out.Err = errors.Join(out.Err, err)
	}
	return r.done(out)
}

func (r Refresher) done(out Outcome) Outcome {
	out.Finished = r.d.Now()
	return out
}

// ingestResult ingests one successful job file. Invalid files are archived
// as rejected; on transient errors (DB busy, cancel) the file stays in the
// inbox so the next leftover import retries it.
func (r Refresher) ingestResult(ctx context.Context, runID int64, res agent.Result) JobOutcome {
	jo := JobOutcome{Job: res.Job, Err: res.Err}
	if res.Err != nil {
		return jo
	}
	ir, err := ingest.IngestFile(ctx, r.d.Conn, res.Path, r.ingestOptions(runID))
	jo.Err, jo.Inserted, jo.Updated, jo.Skipped = err, ir.Inserted, ir.Updated, len(ir.Skipped)
	if err != nil && !errors.Is(err, ingest.ErrInvalidFile) {
		return jo
	}
	if archErr := archive(res.Path, r.d.RunsDir, runID, res.Job, err != nil); archErr != nil {
		r.d.Logger.Error("archive job file", "path", res.Path, "err", archErr)
	}
	return jo
}

func (r Refresher) ingestOptions(runID int64) ingest.Options {
	return ingest.Options{RunID: runID, Now: r.d.Now(), MaxItems: r.d.MaxItems, Logger: r.d.Logger}
}

func enabledInstruments(ctx context.Context, conn *sql.DB) ([]model.Instrument, error) {
	all, err := db.ListInstruments(ctx, conn)
	if err != nil {
		return nil, err
	}
	out := make([]model.Instrument, 0, len(all))
	for _, in := range all {
		if in.Enabled {
			out = append(out, in)
		}
	}
	return out, nil
}
