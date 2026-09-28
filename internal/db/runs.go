package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/hsk-kr/swingdesk/internal/model"
)

// Run is a refresh_runs row.
type Run struct {
	ID         int64
	StartedAt  time.Time
	FinishedAt time.Time // zero while running
	Status     model.RunStatus
	Error      string
	JobsOK     int
	JobsFail   int
}

// StartRun inserts a running row and returns its id.
func StartRun(ctx context.Context, conn DBTX, at time.Time) (int64, error) {
	res, err := conn.ExecContext(ctx, `INSERT INTO refresh_runs (started_at, status) VALUES (?, ?)`,
		formatTime(at), string(model.RunRunning))
	if err != nil {
		return 0, fmt.Errorf("start run: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("start run id: %w", err)
	}
	return id, nil
}

// FinishRun closes a run with its final status and job counts.
func FinishRun(ctx context.Context, conn DBTX, run Run) error {
	if !run.Status.Valid() || run.Status == model.RunRunning {
		return fmt.Errorf("invalid final run status %q", run.Status)
	}
	res, err := conn.ExecContext(ctx, `UPDATE refresh_runs
		SET finished_at = ?, status = ?, error = ?, jobs_ok = ?, jobs_fail = ? WHERE id = ?`,
		formatTime(run.FinishedAt), string(run.Status), nullIfEmpty(run.Error), run.JobsOK, run.JobsFail, run.ID)
	if err != nil {
		return fmt.Errorf("finish run %d: %w", run.ID, err)
	}
	if n, err := res.RowsAffected(); err != nil || n == 0 {
		return fmt.Errorf("finish run %d: no such run (%v)", run.ID, err)
	}
	return nil
}

// RecordLateJob credits a job file ingested after its run was closed (or
// while it is still marked running) and recomputes a closed run's status.
func RecordLateJob(ctx context.Context, conn DBTX, runID int64, totalJobs int) error {
	_, err := conn.ExecContext(ctx, `UPDATE refresh_runs SET
		jobs_ok   = jobs_ok + 1,
		jobs_fail = MAX(jobs_fail - 1, 0),
		status    = CASE WHEN status = 'running' THEN status
		                 WHEN jobs_ok + 1 >= ? THEN 'ok' ELSE 'partial' END
		WHERE id = ?`, totalJobs, runID)
	if err != nil {
		return fmt.Errorf("record late job for run %d: %w", runID, err)
	}
	return nil
}

// StaleRuns returns runs still marked running other than exceptID.
func StaleRuns(ctx context.Context, conn DBTX, exceptID int64) ([]Run, error) {
	rows, err := conn.QueryContext(ctx, `SELECT id, started_at, jobs_ok, jobs_fail FROM refresh_runs
		WHERE status = 'running' AND id != ? ORDER BY id`, exceptID)
	if err != nil {
		return nil, fmt.Errorf("query stale runs: %w", err)
	}
	defer rows.Close()
	var out []Run
	for rows.Next() {
		var r Run
		var started string
		if err := rows.Scan(&r.ID, &started, &r.JobsOK, &r.JobsFail); err != nil {
			return nil, fmt.Errorf("scan stale run: %w", err)
		}
		if r.StartedAt, err = parseTime(started); err != nil {
			return nil, err
		}
		r.Status = model.RunRunning
		out = append(out, r)
	}
	return out, rows.Err()
}

// LastFinishedRun returns the most recently finished run, if any.
func LastFinishedRun(ctx context.Context, conn DBTX) (Run, bool, error) {
	var r Run
	var started, finished, status string
	var errText sql.NullString
	err := conn.QueryRowContext(ctx, `SELECT id, started_at, finished_at, status, error, jobs_ok, jobs_fail
		FROM refresh_runs WHERE finished_at IS NOT NULL ORDER BY finished_at DESC, id DESC LIMIT 1`).
		Scan(&r.ID, &started, &finished, &status, &errText, &r.JobsOK, &r.JobsFail)
	if errors.Is(err, sql.ErrNoRows) {
		return Run{}, false, nil
	}
	if err != nil {
		return Run{}, false, fmt.Errorf("last finished run: %w", err)
	}
	r.Status, r.Error = model.RunStatus(status), errText.String
	if !r.Status.Valid() {
		return Run{}, false, fmt.Errorf("run %d has invalid status %q", r.ID, status)
	}
	if r.StartedAt, err = parseTime(started); err != nil {
		return Run{}, false, err
	}
	if r.FinishedAt, err = parseTime(finished); err != nil {
		return Run{}, false, err
	}
	return r, true, nil
}

// RunExists reports whether a refresh_runs row with id exists.
func RunExists(ctx context.Context, conn DBTX, id int64) (bool, error) {
	var n int
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM refresh_runs WHERE id = ?`, id).Scan(&n); err != nil {
		return false, fmt.Errorf("check run %d: %w", id, err)
	}
	return n > 0, nil
}
