package agent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hsk-kr/swingdesk/internal/model"
)

// Typed errors the TUI can show.
var (
	ErrTmuxNotFound   = errors.New("tmux not found")
	ErrClaudeNotFound = errors.New("claude not found")
	ErrJobTimeout     = errors.New("job timed out")
	ErrJobBusy        = errors.New("previous run of this job is still running")
)

// ExitError is a job whose claude process exited non-zero.
type ExitError struct {
	Job    model.Job
	Code   int
	Stderr string // last lines
}

func (e *ExitError) Error() string {
	msg := fmt.Sprintf("%s job exited %d", e.Job, e.Code)
	if e.Stderr != "" {
		msg += ": " + e.Stderr
	}
	return msg
}

// Config configures a Runner.
type Config struct {
	TmuxBin      string // default "tmux"
	TmuxSocket   string // tmux -L socket; empty = user's default server
	Session      string
	Claude       ClaudeOptions
	InboxDir     string // runs write to InboxDir/<run id>/
	JobTimeout   time.Duration
	MaxItems     int
	Location     *time.Location
	PollInterval time.Duration // default 1s
}

// Result is the outcome of one job.
type Result struct {
	Job      model.Job
	Path     string // <run dir>/<job>.json on success
	Err      error
	Duration time.Duration
}

// Runner starts jobs in tmux windows and waits for their JSON files.
type Runner struct {
	cfg      Config
	tmux     tmux
	lookPath func(string) (string, error)
}

// New builds a Runner. cmd is usually ExecCommander{}.
func New(cfg Config, cmd Commander) Runner {
	if cfg.TmuxBin == "" {
		cfg.TmuxBin = "tmux"
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = time.Second
	}
	return Runner{
		cfg:      cfg,
		tmux:     tmux{bin: cfg.TmuxBin, socket: cfg.TmuxSocket, cmd: cmd},
		lookPath: exec.LookPath,
	}
}

// RunDir is where run runID writes its files.
func (r Runner) RunDir(runID int64) string {
	return filepath.Join(r.cfg.InboxDir, strconv.FormatInt(runID, 10))
}

// Preflight checks that tmux and claude are installed.
func (r Runner) Preflight() error {
	var errs []error
	if _, err := r.lookPath(r.cfg.TmuxBin); err != nil {
		errs = append(errs, ErrTmuxNotFound)
	}
	if _, err := r.lookPath(r.cfg.Claude.Bin); err != nil {
		errs = append(errs, ErrClaudeNotFound)
	}
	return errors.Join(errs...)
}

// Run executes every job for runID concurrently (one tmux window each) and
// blocks until all finish, time out, or ctx is cancelled. Cancelling ctx
// leaves running agents alone so a late file can still be imported later.
func (r Runner) Run(ctx context.Context, runID int64, now time.Time, instruments []model.Instrument) ([]Result, error) {
	if err := r.Preflight(); err != nil {
		return nil, err
	}
	dir := r.RunDir(runID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create run dir: %w", err)
	}
	schema, err := Schema()
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(dir, "schema.json"), schema, 0o600); err != nil {
		return nil, fmt.Errorf("write schema: %w", err)
	}
	if err := r.tmux.ensureSession(ctx, r.cfg.Session); err != nil {
		return nil, err
	}

	jobs := model.Jobs()
	results := make([]Result, len(jobs))
	var wg sync.WaitGroup
	for i, job := range jobs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i] = r.runJob(ctx, runID, dir, job, now, instruments)
		}()
	}
	wg.Wait()
	return results, nil
}

func (r Runner) runJob(ctx context.Context, runID int64, dir string, job model.Job, now time.Time, instruments []model.Instrument) Result {
	start := time.Now()
	res := Result{Job: job}
	if err := r.startJob(ctx, runID, dir, job, now, instruments); err != nil {
		res.Err = err
		return res
	}
	res.Path, res.Err = r.waitJob(ctx, dir, job)
	res.Duration = time.Since(start)
	return res
}

// startJob writes the prompt and script and opens the job's tmux window.
func (r Runner) startJob(ctx context.Context, runID int64, dir string, job model.Job, now time.Time, instruments []model.Instrument) error {
	prompt, err := RenderPrompt(PromptInput{Job: job, Instruments: instruments, Now: now, Location: r.cfg.Location, MaxItems: r.cfg.MaxItems})
	if err != nil {
		return err
	}
	f := filesFor(job)
	if err := os.WriteFile(filepath.Join(dir, f.prompt), []byte(prompt), 0o600); err != nil {
		return fmt.Errorf("write %s prompt: %w", job, err)
	}
	script := filepath.Join(dir, f.script)
	if err := os.WriteFile(script, []byte(buildScript(job, runID, dir, r.cfg.Claude)), 0o700); err != nil {
		return fmt.Errorf("write %s script: %w", job, err)
	}
	exists, alive, err := r.tmux.windowState(ctx, r.cfg.Session, string(job))
	if err != nil {
		return err
	}
	if alive {
		return ErrJobBusy
	}
	if exists {
		if err := r.tmux.killWindow(ctx, r.cfg.Session, string(job)); err != nil {
			return err
		}
	}
	return r.tmux.newWindow(ctx, r.cfg.Session, string(job), "sh "+shellQuote(script))
}

// waitJob polls for <job>.exit. On timeout it kills the job's window.
func (r Runner) waitJob(ctx context.Context, dir string, job model.Job) (string, error) {
	f := filesFor(job)
	deadline := time.NewTimer(r.cfg.JobTimeout)
	defer deadline.Stop()
	tick := time.NewTicker(r.cfg.PollInterval)
	defer tick.Stop()
	for {
		if code, ok := readExit(filepath.Join(dir, f.exit)); ok {
			return finished(dir, job, code)
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-deadline.C:
			// Background context: the kill must happen even if ctx is ending.
			if err := r.tmux.killWindow(context.Background(), r.cfg.Session, string(job)); err != nil {
				return "", fmt.Errorf("%w; kill failed: %v", ErrJobTimeout, err)
			}
			return "", ErrJobTimeout
		case <-tick.C:
		}
	}
}

func readExit(path string) (int, bool) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	code, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil {
		return 99, true
	}
	return code, true
}

func finished(dir string, job model.Job, code int) (string, error) {
	f := filesFor(job)
	if code != 0 {
		return "", &ExitError{Job: job, Code: code, Stderr: tail(filepath.Join(dir, f.stderr), 3)}
	}
	out := filepath.Join(dir, f.out)
	if _, err := os.Stat(out); err != nil {
		return "", fmt.Errorf("%s job exited 0 but %s is missing: %w", job, f.out, err)
	}
	return out, nil
}

// tail returns the last n non-empty lines of a file, joined by " | ".
func tail(path string, n int) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var lines []string
	for _, l := range strings.Split(string(raw), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			lines = append(lines, l)
		}
	}
	return strings.Join(lines[max(len(lines)-n, 0):], " | ")
}
