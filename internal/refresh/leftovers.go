package refresh

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"

	"github.com/hsk-kr/swingdesk/internal/db"
	"github.com/hsk-kr/swingdesk/internal/ingest"
	"github.com/hsk-kr/swingdesk/internal/model"
)

// Leftovers summarizes files found in the inbox from earlier sessions.
type Leftovers struct {
	Files    int
	Inserted int
	Rejected int
	Failed   int // transient failures; the file stays for the next attempt
	Closed   int // stale 'running' runs finalized
}

// ImportLeftovers ingests inbox/<run>/<job>.json files that were never
// ingested (a late agent, or the app quit mid-run), archives them, credits
// their runs, and closes runs other than currentRun left 'running' by a
// previous process. Failures are logged and counted, never fatal: one bad
// file must not block every future refresh.
func (r Refresher) ImportLeftovers(ctx context.Context, currentRun int64) Leftovers {
	var out Leftovers
	files, err := leftoverFiles(r.d.InboxDir, currentRun)
	if err != nil {
		r.d.Logger.Error("scan inbox", "err", err)
	}
	for _, f := range files {
		out.Files++
		ins, rejected, err := r.importOne(ctx, f)
		switch {
		case err != nil:
			out.Failed++
			r.d.Logger.Error("import leftover", "path", f.path, "err", err)
		case rejected:
			out.Rejected++
		default:
			out.Inserted += ins
		}
	}
	closed, err := r.closeStaleRuns(ctx, currentRun)
	if err != nil {
		r.d.Logger.Error("close stale runs", "err", err)
	}
	out.Closed = closed
	return out
}

type leftover struct {
	path    string
	runID   int64
	job     model.Job
	modTime time.Time
}

// leftoverFiles lists inbox/<numeric run>/<job>.json in run order, skipping
// the run currently in progress.
func leftoverFiles(inbox string, currentRun int64) ([]leftover, error) {
	dirs, err := os.ReadDir(inbox)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read inbox: %w", err)
	}
	var out []leftover
	for _, d := range dirs {
		id, err := strconv.ParseInt(d.Name(), 10, 64)
		if !d.IsDir() || err != nil || id < 1 || id == currentRun {
			continue
		}
		for _, job := range model.Jobs() {
			p := filepath.Join(inbox, d.Name(), string(job)+".json")
			if info, err := os.Stat(p); err == nil {
				out = append(out, leftover{path: p, runID: id, job: job, modTime: info.ModTime()})
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].runID < out[j].runID })
	return out, nil
}

// importOne ingests and archives one leftover. Invalid files are archived as
// rejected; transient errors leave the file in place and are returned.
func (r Refresher) importOne(ctx context.Context, f leftover) (int, bool, error) {
	runID, err := r.owningRun(ctx, f)
	if err != nil {
		return 0, false, err
	}
	res, ingErr := ingest.IngestFile(ctx, r.d.Conn, f.path, r.ingestOptions(runID))
	if ingErr != nil && !errors.Is(ingErr, ingest.ErrInvalidFile) {
		return 0, false, ingErr
	}
	if err := archive(f.path, r.d.RunsDir, f.runID, f.job, ingErr != nil); err != nil {
		return 0, false, err
	}
	if ingErr != nil {
		return 0, true, nil
	}
	if runID != 0 {
		if err := db.RecordLateJob(ctx, r.d.Conn, runID); err != nil {
			return 0, false, err
		}
	}
	return res.Inserted, false, nil
}

// owningRun returns f.runID if that run exists and started before the file
// was written; otherwise 0 (e.g. the DB was reset and ids restarted).
func (r Refresher) owningRun(ctx context.Context, f leftover) (int64, error) {
	started, ok, err := db.RunStartedAt(ctx, r.d.Conn, f.runID)
	if err != nil {
		return 0, err
	}
	if !ok || f.modTime.Before(started) {
		return 0, nil
	}
	return f.runID, nil
}

// closeStaleRuns finalizes runs other than currentRun that a previous
// process left 'running'.
func (r Refresher) closeStaleRuns(ctx context.Context, currentRun int64) (int, error) {
	stale, err := db.StaleRuns(ctx, r.d.Conn, currentRun)
	if err != nil {
		return 0, err
	}
	for _, run := range stale {
		total := run.JobsTotal // the run's own job count, not today's config
		run.FinishedAt = r.d.Now()
		run.JobsOK = min(run.JobsOK, total)
		run.Status = model.RunStatusFor(run.JobsOK, total)
		run.JobsFail = total - run.JobsOK
		run.Error = "interrupted: swingdesk exited before the run finished"
		if err := db.FinishRun(ctx, r.d.Conn, run); err != nil {
			return 0, err
		}
	}
	return len(stale), nil
}

// archive moves an inbox file to runs/<run>/<job>.json (or .rejected.json),
// never overwriting an existing archive file.
func archive(path, runsDir string, runID int64, job model.Job, rejected bool) error {
	dir := filepath.Join(runsDir, strconv.FormatInt(runID, 10))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create archive dir: %w", err)
	}
	base := string(job)
	if rejected {
		base += ".rejected"
	}
	dst := filepath.Join(dir, base+".json")
	for i := 2; fileExists(dst); i++ {
		dst = filepath.Join(dir, fmt.Sprintf("%s.%d.json", base, i))
	}
	if err := os.Rename(path, dst); err != nil {
		return fmt.Errorf("archive %s: %w", path, err)
	}
	return nil
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
