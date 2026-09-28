package refresh

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/hsk-kr/swingdesk"
	"github.com/hsk-kr/swingdesk/internal/agent"
	"github.com/hsk-kr/swingdesk/internal/db"
	"github.com/hsk-kr/swingdesk/internal/model"
	"github.com/hsk-kr/swingdesk/internal/watchlist"
)

var now = time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)

// fakeRunner copies ingest fixtures into the run dir like the real runner.
type fakeRunner struct {
	inbox   string
	fail    map[model.Job]error
	runErr  error
	gotRuns *[]int64
	cancel  context.CancelFunc // if set, cancel ctx during Run
}

func (f fakeRunner) Run(ctx context.Context, runID int64, _ time.Time, instruments []model.Instrument) ([]agent.Result, error) {
	if f.gotRuns != nil {
		*f.gotRuns = append(*f.gotRuns, runID)
	}
	if f.cancel != nil {
		f.cancel()
		return nil, ctx.Err()
	}
	if f.runErr != nil {
		return nil, f.runErr
	}
	if len(instruments) == 0 {
		return nil, errors.New("no instruments passed")
	}
	dir := filepath.Join(f.inbox, strconv.FormatInt(runID, 10))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	var out []agent.Result
	for _, job := range model.Jobs() {
		if err := f.fail[job]; err != nil {
			out = append(out, agent.Result{Job: job, Err: err})
			continue
		}
		path := filepath.Join(dir, string(job)+".json")
		if err := copyFixture(string(job), path); err != nil {
			return nil, err
		}
		out = append(out, agent.Result{Job: job, Path: path})
	}
	return out, nil
}

func copyFixture(name, dst string) error {
	raw, err := os.ReadFile(filepath.Join("..", "ingest", "testdata", name+".json"))
	if err != nil {
		return err
	}
	return os.WriteFile(dst, raw, 0o600)
}

type env struct {
	conn  *sql.DB
	inbox string
	runs  string
}

func setup(t *testing.T) env {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	conn, err := db.Open(ctx, filepath.Join(dir, "r.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	seed, _ := watchlist.Parse(swingdesk.WatchlistYAML)
	if _, err := db.SeedInstruments(ctx, conn, seed); err != nil {
		t.Fatal(err)
	}
	return env{conn: conn, inbox: filepath.Join(dir, "inbox"), runs: filepath.Join(dir, "runs")}
}

func (e env) refresher(r JobRunner) Refresher {
	return New(Deps{Conn: e.conn, Runner: r, InboxDir: e.inbox, RunsDir: e.runs, MaxItems: 40, Now: func() time.Time { return now }})
}

func runRow(t *testing.T, conn *sql.DB, id int64) (string, int, int, string) {
	t.Helper()
	var status, errText string
	var ok, fail int
	if err := conn.QueryRow(`SELECT status, jobs_ok, jobs_fail, COALESCE(error,'') FROM refresh_runs WHERE id = ?`, id).
		Scan(&status, &ok, &fail, &errText); err != nil {
		t.Fatal(err)
	}
	return status, ok, fail, errText
}

func TestRefreshOK(t *testing.T) {
	e := setup(t)
	out := e.refresher(fakeRunner{inbox: e.inbox}).Refresh(context.Background())
	if out.Err != nil || out.Status != model.RunOK || out.RunID == 0 {
		t.Fatalf("outcome = %+v", out)
	}
	if out.NewItems() != 7 { // market 3 + tech 2 + names 2
		t.Errorf("new items = %d", out.NewItems())
	}
	status, ok, fail, _ := runRow(t, e.conn, out.RunID)
	if status != "ok" || ok != 3 || fail != 0 {
		t.Errorf("row = %s %d %d", status, ok, fail)
	}
	for _, job := range model.Jobs() {
		if _, err := os.Stat(filepath.Join(e.runs, strconv.FormatInt(out.RunID, 10), string(job)+".json")); err != nil {
			t.Errorf("%s not archived: %v", job, err)
		}
		if _, err := os.Stat(filepath.Join(e.inbox, strconv.FormatInt(out.RunID, 10), string(job)+".json")); !os.IsNotExist(err) {
			t.Errorf("%s still in inbox", job)
		}
	}
	// A second refresh has no leftovers and dedupes the same stories.
	out2 := e.refresher(fakeRunner{inbox: e.inbox}).Refresh(context.Background())
	if out2.Leftovers.Files != 0 || out2.NewItems() != 0 || out2.RunID == out.RunID {
		t.Errorf("second outcome = %+v", out2)
	}
}

func TestRefreshPartialAndError(t *testing.T) {
	e := setup(t)
	out := e.refresher(fakeRunner{inbox: e.inbox, fail: map[model.Job]error{model.JobTech: agent.ErrJobTimeout}}).Refresh(context.Background())
	if out.Status != model.RunPartial {
		t.Errorf("status = %s", out.Status)
	}
	status, ok, fail, errText := runRow(t, e.conn, out.RunID)
	if status != "partial" || ok != 2 || fail != 1 || errText != "tech: job timed out" {
		t.Errorf("row = %s %d %d %q", status, ok, fail, errText)
	}

	out = e.refresher(fakeRunner{inbox: e.inbox, runErr: agent.ErrTmuxNotFound}).Refresh(context.Background())
	if !errors.Is(out.Err, agent.ErrTmuxNotFound) || out.Status != model.RunError {
		t.Errorf("outcome = %+v", out)
	}
	status, _, fail, errText = runRow(t, e.conn, out.RunID)
	if status != "error" || fail != 3 || errText != "tmux not found" {
		t.Errorf("row = %s fail=%d %q", status, fail, errText)
	}
}

func TestInvalidJobFileIsRejectedAndArchived(t *testing.T) {
	e := setup(t)
	r := badNamesRunner{fakeRunner{inbox: e.inbox}}
	out := e.refresher(r).Refresh(context.Background())
	if out.Status != model.RunPartial {
		t.Errorf("status = %s", out.Status)
	}
	rej := filepath.Join(e.runs, strconv.FormatInt(out.RunID, 10), "names.rejected.json")
	if _, err := os.Stat(rej); err != nil {
		t.Errorf("rejected file not archived: %v", err)
	}
}

type badNamesRunner struct{ fakeRunner }

func (b badNamesRunner) Run(ctx context.Context, runID int64, n time.Time, ins []model.Instrument) ([]agent.Result, error) {
	res, err := b.fakeRunner.Run(ctx, runID, n, ins)
	for _, r := range res {
		if r.Job == model.JobNames {
			if werr := copyFixture("invalid", r.Path); werr != nil {
				return nil, werr
			}
		}
	}
	return res, err
}

func TestCancelledRefreshLeavesRunForLeftoverImport(t *testing.T) {
	e := setup(t)
	ctx, cancel := context.WithCancel(context.Background())
	out := e.refresher(fakeRunner{inbox: e.inbox, cancel: cancel}).Refresh(ctx)
	if !errors.Is(out.Err, context.Canceled) {
		t.Fatalf("err = %v", out.Err)
	}
	if status, _, _, _ := runRow(t, e.conn, out.RunID); status != "running" {
		t.Fatalf("status = %s", status)
	}
	// A late agent finishes two jobs after the quit.
	dir := filepath.Join(e.inbox, strconv.FormatInt(out.RunID, 10))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, job := range []string{"market", "names"} {
		if err := copyFixture(job, filepath.Join(dir, job+".json")); err != nil {
			t.Fatal(err)
		}
	}
	// Next launch: leftovers are ingested before the new run starts.
	var runs []int64
	next := e.refresher(fakeRunner{inbox: e.inbox, gotRuns: &runs}).Refresh(context.Background())
	if next.Leftovers.Files != 2 || next.Leftovers.Closed != 1 || next.Leftovers.Inserted != 5 {
		t.Errorf("leftovers = %+v", next.Leftovers)
	}
	status, ok, fail, errText := runRow(t, e.conn, out.RunID)
	if status != "partial" || ok != 2 || fail != 1 || errText == "" {
		t.Errorf("old run = %s %d %d %q", status, ok, fail, errText)
	}
	var n int
	if err := e.conn.QueryRow(`SELECT COUNT(*) FROM items WHERE run_id = ?`, out.RunID).Scan(&n); err != nil || n != 5 {
		t.Errorf("leftover items credited to old run = %d, %v", n, err)
	}
	if len(runs) != 1 || runs[0] == out.RunID {
		t.Errorf("new run ids = %v", runs)
	}
}

func TestLeftoverFromUnknownRunDirIngestsWithoutRun(t *testing.T) {
	e := setup(t)
	dir := filepath.Join(e.inbox, "424242")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := copyFixture("tech", filepath.Join(dir, "tech.json")); err != nil {
		t.Fatal(err)
	}
	// Junk that must be ignored.
	if err := os.MkdirAll(filepath.Join(e.inbox, "notarun"), 0o700); err != nil {
		t.Fatal(err)
	}
	left, err := e.refresher(fakeRunner{inbox: e.inbox}).ImportLeftovers(context.Background())
	if err != nil || left.Files != 1 || left.Inserted != 2 {
		t.Fatalf("leftovers = %+v, %v", left, err)
	}
}

func TestSchedule(t *testing.T) {
	t0 := now
	s := NewSchedule(30 * time.Minute)
	s, d := s.Tick(t0)
	if d != Start || !s.Running() || !s.Next().Equal(t0.Add(30*time.Minute)) {
		t.Fatalf("first tick = %v next %v", d, s.Next())
	}
	if _, d := s.Tick(t0.Add(time.Minute)); d != Wait {
		t.Errorf("tick before due = %v", d)
	}
	s, d = s.Tick(t0.Add(30 * time.Minute))
	if d != Skipped || !s.Next().Equal(t0.Add(60*time.Minute)) {
		t.Errorf("overlap tick = %v next %v", d, s.Next())
	}
	s, d = s.Force(t0.Add(31 * time.Minute))
	if d != Skipped {
		t.Errorf("force while running = %v", d)
	}
	s = s.Done()
	s, d = s.Force(t0.Add(40 * time.Minute))
	if d != Start || !s.Next().Equal(t0.Add(70*time.Minute)) {
		t.Errorf("force resets timer: %v next %v", d, s.Next())
	}
	s = s.Done()
	if _, d := s.Tick(t0.Add(69 * time.Minute)); d != Wait {
		t.Errorf("tick before reset deadline = %v", d)
	}
	if _, d := s.Tick(t0.Add(70 * time.Minute)); d != Start {
		t.Errorf("tick at deadline = %v", d)
	}
}
