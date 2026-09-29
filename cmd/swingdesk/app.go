package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/hsk-kr/swingdesk"
	"github.com/hsk-kr/swingdesk/internal/agent"
	"github.com/hsk-kr/swingdesk/internal/config"
	"github.com/hsk-kr/swingdesk/internal/db"
	"github.com/hsk-kr/swingdesk/internal/ingest"
	"github.com/hsk-kr/swingdesk/internal/model"
	"github.com/hsk-kr/swingdesk/internal/refresh"
	"github.com/hsk-kr/swingdesk/internal/ui"
	"github.com/hsk-kr/swingdesk/internal/watchlist"
)

// app holds everything opened for one process.
type app struct {
	cfg         config.Config
	paths       config.Paths
	tz          *time.Location
	conn        *sql.DB
	instruments []model.Instrument
	logFile     *os.File
	logger      *slog.Logger
}

func openApp(ctx context.Context, configFlag string) (*app, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve home dir: %w", err)
	}
	loc := config.ConfigPath(configFlag, os.Getenv, home)
	cfg, err := config.Load(loc)
	if err != nil {
		return nil, err
	}
	tz, err := cfg.Location()
	if err != nil {
		return nil, fmt.Errorf("timezone: %w", err)
	}
	a := &app{cfg: cfg, paths: config.ResolvePaths(cfg, loc.Path, os.Getenv, home), tz: tz}
	if a.conn, a.instruments, err = openAndSeed(ctx, a.paths.DBFile); err != nil {
		return nil, err
	}
	if err := a.openLog(); err != nil {
		a.Close()
		return nil, err
	}
	return a, nil
}

// openLog appends to data_dir/swingdesk.log; the TUI owns the terminal.
func (a *app) openLog() error {
	f, err := os.OpenFile(filepath.Join(a.paths.DataDir, "swingdesk.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open log: %w", err)
	}
	a.logFile, a.logger = f, slog.New(slog.NewTextHandler(f, nil))
	return nil
}

func (a *app) Close() {
	if a.conn != nil {
		a.conn.Close()
	}
	if a.logFile != nil {
		a.logFile.Close()
	}
}

func (a *app) runner() agent.Runner {
	models := make(map[model.Job]string, len(a.cfg.ClaudeJobModels))
	for job := range a.cfg.ClaudeJobModels {
		models[job] = a.cfg.ModelFor(job)
	}
	return agent.New(agent.Config{
		Session: a.cfg.TmuxSession,
		Claude: agent.ClaudeOptions{
			Bin:             a.cfg.ClaudeBin,
			Model:           a.cfg.ClaudeModel,
			PermissionMode:  a.cfg.ClaudePermissionMode,
			SkipPermissions: a.cfg.ClaudeSkipPermissions,
			MaxBudgetUSD:    a.cfg.ClaudeMaxBudgetUSD,
		},
		JobModels:  models,
		Jobs:       a.cfg.Jobs(),
		Megacaps:   a.cfg.MegacapSymbols,
		InboxDir:   a.paths.InboxDir,
		JobTimeout: a.cfg.JobTimeout(),
		MaxItems:   a.cfg.MaxItemsPerJob,
		Location:   a.tz,
	}, agent.ExecCommander{})
}

func (a *app) refresher() refresh.Refresher {
	return refresh.New(refresh.Deps{
		Conn: a.conn, Runner: a.runner(), InboxDir: a.paths.InboxDir, RunsDir: a.paths.RunsDir,
		LockPath: filepath.Join(a.paths.DataDir, "refresh.lock"), Jobs: a.cfg.Jobs(),
		MaxItems: a.cfg.MaxItemsPerJob, Logger: a.logger,
	})
}

// runUI starts the TUI. Quitting cancels an in-flight refresh; agents keep
// running in tmux and their files are imported on the next launch.
func (a *app) runUI(start startUI) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	status, err := a.initialStatus(ctx)
	if err != nil {
		return err
	}
	gate, err := refresh.NewHours(a.cfg.MarketHours)
	if err != nil {
		return err
	}
	r := a.refresher()
	uiErr := start(ui.New(ui.Options{
		Store:       db.NewStore(a.conn),
		Instruments: a.instruments,
		Location:    a.tz,
		Interval:    a.cfg.RefreshInterval(),
		TmuxSession: a.cfg.TmuxSession,
		Status:      status,
		Copy:        tmuxCopier(os.Getenv),
		Jobs:        a.cfg.Jobs(),
		Gate:        gate,
		Refresh: func(progress func(model.Job)) refresh.Outcome {
			return r.Refresh(ctx, func(res agent.Result) { progress(res.Job) })
		},
	}))
	if a.cfg.KillAgentsOnQuit {
		cancel()
		if err := a.runner().KillSession(context.Background()); err != nil {
			return errors.Join(uiErr, fmt.Errorf("kill agents: %w", err))
		}
	}
	return uiErr
}

// killAgents ends the agents' tmux session (-kill-agents).
func (a *app) killAgents(ctx context.Context, out io.Writer) error {
	if err := a.runner().KillSession(ctx); err != nil {
		return fmt.Errorf("kill agents: %w", err)
	}
	fmt.Fprintf(out, "killed tmux session %s (if it was running)\n", a.cfg.TmuxSession)
	return nil
}

// tmuxCopier returns a clipboard path for use inside tmux: `load-buffer -w`
// makes tmux itself emit OSC 52 to the outer terminal (tmux ignores OSC 52
// from applications unless set-clipboard is on). Nil outside tmux.
func tmuxCopier(getenv func(string) string) func(string) error {
	if getenv("TMUX") == "" {
		return nil
	}
	return func(s string) error {
		cmd := exec.Command("tmux", "load-buffer", "-w", "-")
		cmd.Stdin = strings.NewReader(s)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
		}
		return nil
	}
}

// initialStatus shows the last finished run so a restart does not say "never".
func (a *app) initialStatus(ctx context.Context) (ui.Status, error) {
	last, ok, err := db.LastFinishedRun(ctx, a.conn)
	if err != nil || !ok {
		return ui.Status{}, err
	}
	return ui.Status{LastRefresh: last.FinishedAt}, nil
}

func (a *app) refreshOnce(ctx context.Context, out io.Writer) error {
	o := a.refresher().Refresh(ctx, nil)
	if o.Leftovers.Files > 0 {
		fmt.Fprintf(out, "leftovers: %d files, %d new items\n", o.Leftovers.Files, o.Leftovers.Inserted)
	}
	fmt.Fprintf(out, "run %d: %s, %d new items\n", o.RunID, o.Status, o.NewItems())
	for _, j := range o.Jobs {
		if j.Err != nil {
			fmt.Fprintf(out, "  %s: %v\n", j.Job, j.Err)
			continue
		}
		fmt.Fprintf(out, "  %s: %d new, %d updated, %d skipped\n", j.Job, j.Inserted, j.Updated, j.Skipped)
	}
	if o.Err != nil {
		return o.Err
	}
	if o.Status == model.RunError {
		return errors.New("refresh failed")
	}
	return nil
}

func (a *app) ingestOne(ctx context.Context, out io.Writer, path string) error {
	res, err := ingest.IngestFile(ctx, a.conn, path, ingest.Options{MaxItems: a.cfg.MaxItemsPerJob})
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "%s: %d new, %d updated, %d events, %d biases, %d skipped\n",
		res.Job, res.Inserted, res.Updated, res.Events, res.Biases, len(res.Skipped))
	for _, s := range res.Skipped {
		fmt.Fprintf(out, "  skipped %s #%d %s: %s\n", s.Kind, s.Index, s.Symbol, s.Reason)
	}
	for _, w := range res.Warnings {
		fmt.Fprintf(out, "  dropped %s on item #%d %s: %s\n", w.Kind, w.Index, w.Symbol, w.Reason)
	}
	return nil
}

func (a *app) printPaths(out io.Writer) {
	fmt.Fprintf(out, "config:  %s\n", a.paths.ConfigFile)
	fmt.Fprintf(out, "data:    %s\n", a.paths.DataDir)
	fmt.Fprintf(out, "db:      %s\n", a.paths.DBFile)
	fmt.Fprintf(out, "inbox:   %s\n", a.paths.InboxDir)
	fmt.Fprintf(out, "runs:    %s\n", a.paths.RunsDir)
	fmt.Fprintf(out, "refresh: every %d min (%s)\n", a.cfg.RefreshMinutes, a.cfg.Timezone)
	fmt.Fprintf(out, "instruments: %d\n", len(a.instruments))
}

// openAndSeed opens the DB, applies migrations, seeds the watchlist on first
// launch, and returns the open connection plus the stored instruments.
func openAndSeed(ctx context.Context, dbFile string) (*sql.DB, []model.Instrument, error) {
	seed, err := watchlist.Parse(swingdesk.WatchlistYAML)
	if err != nil {
		return nil, nil, fmt.Errorf("embedded watchlist: %w", err)
	}
	conn, err := db.Open(ctx, dbFile)
	if err != nil {
		return nil, nil, err
	}
	if _, err := db.SeedInstruments(ctx, conn, seed); err != nil {
		conn.Close()
		return nil, nil, err
	}
	instruments, err := db.ListInstruments(ctx, conn)
	if err != nil {
		conn.Close()
		return nil, nil, err
	}
	return conn, instruments, nil
}
