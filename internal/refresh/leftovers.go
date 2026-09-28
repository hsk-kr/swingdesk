package refresh

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"

	"github.com/hsk-kr/swingdesk/internal/db"
	"github.com/hsk-kr/swingdesk/internal/ingest"
	"github.com/hsk-kr/swingdesk/internal/model"
)

// Leftovers summarizes files found in the inbox from earlier sessions.
type Leftovers struct {
	Files    int
	Inserted int
	Rejected int
	Closed   int // stale 'running' runs finalized
}

// ImportLeftovers ingests inbox/<run>/<job>.json files that were never
// ingested (a late agent, or the app quit mid-run), archives them, credits
// their runs, and closes runs left 'running' by a previous process.
func (r Refresher) ImportLeftovers(ctx context.Context) (Leftovers, error) {
	var out Leftovers
	files, err := leftoverFiles(r.d.InboxDir)
	if err != nil {
		return out, err
	}
	for _, f := range files {
		ins, rejected, err := r.importOne(ctx, f)
		if err != nil {
			return out, err
		}
		out.Files++
		out.Inserted += ins
		if rejected {
			out.Rejected++
		}
	}
	closed, err := r.closeStaleRuns(ctx)
	out.Closed = closed
	return out, err
}

type leftover struct {
	path  string
	runID int64 // 0 when the directory is not a known run
	job   model.Job
}

// leftoverFiles lists inbox/<numeric run>/<job>.json in run order.
func leftoverFiles(inbox string) ([]leftover, error) {
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
		if !d.IsDir() || err != nil || id < 1 {
			continue
		}
		for _, job := range model.Jobs() {
			p := filepath.Join(inbox, d.Name(), string(job)+".json")
			if _, err := os.Stat(p); err == nil {
				out = append(out, leftover{path: p, runID: id, job: job})
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].runID < out[j].runID })
	return out, nil
}

// importOne ingests and archives one leftover. An invalid file is archived as
// rejected (so it is not retried forever); only DB errors are returned.
func (r Refresher) importOne(ctx context.Context, f leftover) (int, bool, error) {
	known, err := db.RunExists(ctx, r.d.Conn, f.runID)
	if err != nil {
		return 0, false, err
	}
	runID := f.runID
	if !known {
		runID = 0
	}
	res, ingErr := ingest.IngestFile(ctx, r.d.Conn, f.path, r.ingestOptions(runID))
	if err := archive(f.path, r.d.RunsDir, f.runID, f.job, ingErr != nil); err != nil {
		return 0, false, err
	}
	if ingErr != nil {
		return 0, true, nil
	}
	if known {
		if err := db.RecordLateJob(ctx, r.d.Conn, runID, len(model.Jobs())); err != nil {
			return 0, false, err
		}
	}
	return res.Inserted, false, nil
}

// closeStaleRuns finalizes runs a previous process left 'running'.
func (r Refresher) closeStaleRuns(ctx context.Context) (int, error) {
	stale, err := db.StaleRuns(ctx, r.d.Conn, 0)
	if err != nil {
		return 0, err
	}
	total := len(model.Jobs())
	for _, run := range stale {
		run.FinishedAt = r.d.Now()
		run.Status = model.RunStatusFor(run.JobsOK, total)
		run.JobsFail = total - run.JobsOK
		run.Error = "interrupted: swingdesk exited before the run finished"
		if err := db.FinishRun(ctx, r.d.Conn, run); err != nil {
			return 0, err
		}
	}
	return len(stale), nil
}

// archive moves an inbox file to runs/<run>/<job>.json (or .rejected.json).
func archive(path, runsDir string, runID int64, job model.Job, rejected bool) error {
	dir := filepath.Join(runsDir, strconv.FormatInt(runID, 10))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create archive dir: %w", err)
	}
	name := string(job) + ".json"
	if rejected {
		name = string(job) + ".rejected.json"
	}
	if err := os.Rename(path, filepath.Join(dir, name)); err != nil {
		return fmt.Errorf("archive %s: %w", path, err)
	}
	return nil
}
