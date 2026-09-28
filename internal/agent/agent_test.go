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
	"testing"
	"time"

	"github.com/hsk-kr/swingdesk"
	"github.com/hsk-kr/swingdesk/internal/config"
	"github.com/hsk-kr/swingdesk/internal/ingest"
	"github.com/hsk-kr/swingdesk/internal/model"
	"github.com/hsk-kr/swingdesk/internal/watchlist"
)

var testNow = time.Date(2026, 9, 27, 10, 41, 0, 0, time.UTC)

func instruments(t *testing.T) []model.Instrument {
	t.Helper()
	in, err := watchlist.Parse(swingdesk.WatchlistYAML)
	if err != nil {
		t.Fatal(err)
	}
	return in
}

func TestRenderPromptInjectsWatchlistAndNow(t *testing.T) {
	london, _ := time.LoadLocation("Europe/London")
	for _, job := range model.Jobs() {
		out, err := RenderPrompt(PromptInput{Job: job, Instruments: instruments(t), Now: testNow, Location: london, MaxItems: 40})
		if err != nil {
			t.Fatalf("%s: %v", job, err)
		}
		for _, want := range []string{
			"2026-09-27T11:41:00+01:00", "Europe/London",
			"| SPCX | SpaceX | cfd | Private company.", "| CRUDE |", "company: alphabet",
			"At most 40 items", "`job` set to `" + string(job) + "`",
			"3 to 10 day", "not financial advice", "Output only the JSON",
		} {
			if !strings.Contains(out, want) {
				t.Errorf("%s prompt missing %q", job, want)
			}
		}
	}
	if _, err := RenderPrompt(PromptInput{Job: "macro"}); err == nil {
		t.Error("unknown job should fail")
	}
}

func TestWatchlistTableSkipsDisabledAndEscapes(t *testing.T) {
	got := WatchlistTable([]model.Instrument{
		{Symbol: "A", Name: "x|y", Kind: model.KindEquity, Enabled: true, Notes: "line1\nline2"},
		{Symbol: "OFF", Name: "off", Kind: model.KindEquity},
	})
	if strings.Contains(got, "OFF") || !strings.Contains(got, "| A | x/y | equity | line1 line2 |") {
		t.Errorf("table = %q", got)
	}
}

func TestSchemaMatchesIngestClosedSets(t *testing.T) {
	raw, err := Schema()
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	for _, j := range model.Jobs() {
		if !strings.Contains(s, `"`+string(j)+`"`) {
			t.Errorf("schema missing job %s", j)
		}
	}
	for _, c := range model.Categories() {
		if !strings.Contains(s, `"`+string(c)+`"`) {
			t.Errorf("schema missing category %s", c)
		}
	}
	for _, st := range model.Stances() {
		if !strings.Contains(s, `"`+string(st)+`"`) {
			t.Errorf("schema missing stance %s", st)
		}
	}
	for _, k := range model.EventKinds() {
		if !strings.Contains(s, `"`+string(k)+`"`) {
			t.Errorf("schema missing event kind %s", k)
		}
	}
}

func TestClaudeArgs(t *testing.T) {
	def := strings.Join(claudeArgs(ClaudeOptions{Bin: "claude", PermissionMode: config.PermissionDontAsk}), " ")
	for _, want := range []string{"--output-format json", "--tools WebSearch,WebFetch", "--allowedTools WebSearch,WebFetch", "--permission-mode dontAsk", "--strict-mcp-config"} {
		if !strings.Contains(def, want) {
			t.Errorf("args missing %q: %s", want, def)
		}
	}
	for _, bad := range []string{"--model", "--dangerously-skip-permissions", "--max-budget-usd"} {
		if strings.Contains(def, bad) {
			t.Errorf("default args must not contain %s", bad)
		}
	}
	opt := strings.Join(claudeArgs(ClaudeOptions{Model: "haiku", SkipPermissions: true, MaxBudgetUSD: 1.5}), " ")
	for _, want := range []string{"--model haiku", "--dangerously-skip-permissions", "--max-budget-usd 1.5"} {
		if !strings.Contains(opt, want) {
			t.Errorf("args missing %q: %s", want, opt)
		}
	}
	if strings.Contains(opt, "--permission-mode") {
		t.Error("skip-permissions replaces --permission-mode")
	}
}

func TestShellQuote(t *testing.T) {
	if got := shellQuote("it's"); got != `'it'\''s'` {
		t.Errorf("quote = %s", got)
	}
}

func TestPreflightTypedErrors(t *testing.T) {
	r := New(Config{Claude: ClaudeOptions{Bin: "claude"}}, ExecCommander{})
	r.lookPath = func(string) (string, error) { return "", exec.ErrNotFound }
	err := r.Preflight()
	if !errors.Is(err, ErrTmuxNotFound) || !errors.Is(err, ErrClaudeNotFound) {
		t.Errorf("err = %v", err)
	}
	if _, err := r.Run(context.Background(), 1, testNow, nil, nil); !errors.Is(err, ErrTmuxNotFound) {
		t.Errorf("Run should surface preflight error, got %v", err)
	}
}

// --- integration against a real tmux server on a private socket ---

// fakeClaude writes a claude stand-in whose behaviour depends on mode:
// "ok" emits a valid wrapped envelope, "fail" exits 3, "hang" sleeps.
func fakeClaude(t *testing.T, mode string) string {
	t.Helper()
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args")
	script := fmt.Sprintf(`#!/bin/sh
printf '%%s\n' "$@" > %q.$$
case "$2" in
  *"macro/market desk"*) job=market ;;
  *"tech sector desk"*) job=tech ;;
  *) job=names ;;
esac
case %q in
  fail) echo "Not logged in · Please run /login" >&2; exit 3 ;;
  jsonerr) echo '{"type":"result","subtype":"error","is_error":true,"result":"Credit balance is too low"}'; exit 1 ;;
  hang) echo $$ > "hang.$job.pid"; exec sleep 30 ;;
esac
echo "researching $job" >&2
cat <<JSON
{"type":"result","is_error":false,"result":"","structured_output":{"job":"$job","generated_at":"2026-09-27T11:41:00Z","items":[{"symbol":"NVDA","category":"news","title":"$job headline","summary":"s","body":"","source":"Reuters","url":"https://example.com/$job","published_at":null,"event_at":null,"event_kind":null}],"biases":[]}}
JSON
`, argsFile, mode)
	path := filepath.Join(dir, "claude")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func tmuxRunner(t *testing.T, claudeBin string, timeout time.Duration) Runner {
	t.Helper()
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not installed")
	}
	// Short private socket dir (unix socket paths are length-limited) so no
	// test touches the user's tmux server or leaves socket files behind.
	sockDir, err := os.MkdirTemp("", "sdt")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMUX_TMPDIR", sockDir)
	t.Setenv("TMUX", "")
	socket := "swingdesk-test"
	t.Cleanup(func() {
		_ = exec.Command("tmux", "-L", socket, "kill-server").Run()
		_ = os.RemoveAll(sockDir)
	})
	return New(Config{
		TmuxSocket:   socket,
		Session:      "sdtest",
		Claude:       ClaudeOptions{Bin: claudeBin, PermissionMode: config.PermissionDontAsk},
		InboxDir:     t.TempDir(),
		JobTimeout:   timeout,
		MaxItems:     40,
		PollInterval: 50 * time.Millisecond,
	}, ExecCommander{})
}

func TestRunInTmuxWritesIngestableFiles(t *testing.T) {
	r := tmuxRunner(t, fakeClaude(t, "ok"), 20*time.Second)
	progress := make(chan model.Job, 3)
	results, err := r.Run(context.Background(), 7, testNow, instruments(t), func(res Result) { progress <- res.Job })
	if len(progress) != 3 {
		t.Errorf("progress calls = %d", len(progress))
	}
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 3 {
		t.Fatalf("results = %d", len(results))
	}
	for _, res := range results {
		if res.Err != nil {
			t.Fatalf("%s: %v", res.Job, res.Err)
		}
		if res.Path != filepath.Join(r.RunDir(7), string(res.Job)+".json") {
			t.Errorf("%s path = %s", res.Job, res.Path)
		}
		raw, err := os.ReadFile(res.Path)
		if err != nil {
			t.Fatal(err)
		}
		env, err := ingest.Parse(raw)
		if err != nil || env.Job != res.Job {
			t.Errorf("%s: parse = %+v, %v", res.Job, env.Job, err)
		}
		if _, err := os.Stat(res.Path + ".tmp"); !os.IsNotExist(err) {
			t.Errorf("%s: tmp file left behind", res.Job)
		}
	}
	stderr, _ := os.ReadFile(filepath.Join(r.RunDir(7), "names.stderr"))
	if !strings.Contains(string(stderr), "researching names") {
		t.Errorf("stderr not captured: %q", stderr)
	}
	// Windows stay (remain-on-exit) so the user can read them; a second run
	// replaces dead windows instead of failing.
	if _, err := r.Run(context.Background(), 8, testNow, instruments(t), nil); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("tmux", "-L", r.cfg.TmuxSocket, "list-windows", "-t", "=sdtest", "-F", "#{window_name}").Output()
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Fields(string(out)); len(got) != 4 {
		t.Errorf("windows = %v, want desk + 3 jobs", got)
	}
}

func TestRunInTmuxReportsExitErrors(t *testing.T) {
	r := tmuxRunner(t, fakeClaude(t, "fail"), 20*time.Second)
	results, err := r.Run(context.Background(), 1, testNow, instruments(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, res := range results {
		var exitErr *ExitError
		if !errors.As(res.Err, &exitErr) || exitErr.Code != 3 || !strings.Contains(exitErr.Stderr, "Not logged in") {
			t.Errorf("%s: err = %v", res.Job, res.Err)
		}
		if _, err := os.Stat(filepath.Join(r.RunDir(1), string(res.Job)+".json")); !os.IsNotExist(err) {
			t.Errorf("%s: failed job must not publish json", res.Job)
		}
	}
}

func TestRunInTmuxTimesOutAndKills(t *testing.T) {
	r := tmuxRunner(t, fakeClaude(t, "hang"), 700*time.Millisecond)
	results, err := r.Run(context.Background(), 1, testNow, instruments(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, res := range results {
		if !errors.Is(res.Err, ErrJobTimeout) {
			t.Errorf("%s: err = %v", res.Job, res.Err)
		}
	}
	w, err := r.tmux.windowState(context.Background(), "sdtest", "names")
	if err != nil || w.exists {
		t.Errorf("timed-out window should be killed: exists=%v err=%v", w.exists, err)
	}
	for _, job := range model.Jobs() {
		raw, err := os.ReadFile(filepath.Join(r.RunDir(1), "hang."+string(job)+".pid"))
		if err != nil {
			t.Fatalf("%s pid: %v", job, err)
		}
		pid := strings.TrimSpace(string(raw))
		if !eventually(2*time.Second, func() bool { return exec.Command("kill", "-0", pid).Run() != nil }) {
			_ = exec.Command("kill", pid).Run()
			t.Errorf("%s: claude process %s survived the timeout kill", job, pid)
		}
	}
}

func TestBusyJobIsNotReplaced(t *testing.T) {
	r := tmuxRunner(t, fakeClaude(t, "hang"), 20*time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = r.Run(ctx, 1, testNow, instruments(t), nil)
	}()
	if !eventually(5*time.Second, func() bool {
		for _, job := range model.Jobs() {
			if _, err := os.Stat(filepath.Join(r.RunDir(1), "hang."+string(job)+".pid")); err != nil {
				return false
			}
		}
		return true
	}) {
		t.Fatal("hanging jobs never started")
	}
	cancel() // quitting does not kill agents
	<-done
	if w, _ := r.tmux.windowState(context.Background(), "sdtest", "names"); !w.alive {
		t.Fatal("cancel must leave the running agent alone")
	}
	results, err := r.Run(context.Background(), 2, testNow, instruments(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, res := range results {
		if !errors.Is(res.Err, ErrJobBusy) {
			t.Errorf("%s: err = %v, want busy", res.Job, res.Err)
		}
	}
}

// eventually polls cond until it holds or d elapses.
func eventually(d time.Duration, cond func() bool) bool {
	deadline := time.Now().Add(d)
	for !cond() {
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(20 * time.Millisecond)
	}
	return true
}

func TestExitErrorPrefersClaudeJSONError(t *testing.T) {
	r := tmuxRunner(t, fakeClaude(t, "jsonerr"), 20*time.Second)
	results, err := r.Run(context.Background(), 1, testNow, instruments(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, res := range results {
		var exitErr *ExitError
		if !errors.As(res.Err, &exitErr) || exitErr.Stderr != "Credit balance is too low" {
			t.Errorf("%s: err = %v", res.Job, res.Err)
		}
	}
}

// runHanging starts a hanging run in the background and waits until every
// job's fake claude is running (so each script has installed its trap).
func runHanging(t *testing.T, r Runner) <-chan []Result {
	t.Helper()
	done := make(chan []Result, 1)
	go func() {
		res, _ := r.Run(context.Background(), 1, testNow, instruments(t), nil)
		done <- res
	}()
	if !eventually(5*time.Second, func() bool {
		for _, job := range model.Jobs() {
			if _, err := os.Stat(filepath.Join(r.RunDir(1), "hang."+string(job)+".pid")); err != nil {
				return false
			}
		}
		return true
	}) {
		t.Fatal("hanging jobs never started")
	}
	return done
}

func resultFor(results []Result, job model.Job) Result {
	for _, r := range results {
		if r.Job == job {
			return r
		}
	}
	return Result{}
}

func TestCtrlCInPanePublishesExit130(t *testing.T) {
	r := tmuxRunner(t, fakeClaude(t, "hang"), 20*time.Second)
	done := runHanging(t, r)
	start := time.Now()
	if out, err := exec.Command("tmux", "-L", r.cfg.TmuxSocket, "send-keys", "-t", "=sdtest:names", "C-c").CombinedOutput(); err != nil {
		t.Fatalf("send-keys: %v %s", err, out)
	}
	for _, j := range []string{"market", "tech"} {
		_ = exec.Command("tmux", "-L", r.cfg.TmuxSocket, "send-keys", "-t", "=sdtest:"+j, "C-c").Run()
	}
	results := <-done
	var exitErr *ExitError
	if res := resultFor(results, model.JobNames); !errors.As(res.Err, &exitErr) || exitErr.Code != 130 {
		t.Errorf("names err = %v", res.Err)
	}
	if time.Since(start) > 5*time.Second {
		t.Error("interrupted job should be reported promptly, not at the timeout")
	}
}

func TestPaneKilledWithoutExitFileFailsFast(t *testing.T) {
	r := tmuxRunner(t, fakeClaude(t, "hang"), 20*time.Second)
	done := runHanging(t, r)
	start := time.Now()
	for _, job := range model.Jobs() {
		pid, err := r.tmux.panePID(context.Background(), "sdtest", string(job))
		if err != nil {
			t.Fatal(err)
		}
		if err := exec.Command("kill", "-9", strconv.Itoa(pid)).Run(); err != nil {
			t.Fatal(err)
		}
	}
	results := <-done
	for _, res := range results {
		if !errors.Is(res.Err, ErrJobDied) {
			t.Errorf("%s: err = %v, want ErrJobDied", res.Job, res.Err)
		}
	}
	if time.Since(start) > 5*time.Second {
		t.Error("dead pane should be detected promptly")
	}
}

func TestClearStaleRemovesOldResults(t *testing.T) {
	dir := t.TempDir()
	f := filesFor(model.JobNames)
	for _, name := range []string{f.exit, f.code, f.out, f.tmp, f.stderr} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("0"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := clearStale(dir, f); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Errorf("left behind: %v", entries)
	}
	if err := clearStale(dir, f); err != nil {
		t.Errorf("clearing an empty dir should succeed: %v", err)
	}
}

func TestNewDefaultsJobTimeout(t *testing.T) {
	if r := New(Config{}, ExecCommander{}); r.cfg.JobTimeout != DefaultJobTimeout {
		t.Errorf("JobTimeout = %v", r.cfg.JobTimeout)
	}
}

func TestScriptUsesSafeModeAndTrapsInterruptOnly(t *testing.T) {
	s := buildScript(model.JobNames, 3, "/tmp/run dir's", ClaudeOptions{Bin: "claude", PermissionMode: config.PermissionDontAsk})
	for _, want := range []string{"--safe-mode", `'/tmp/run dir'\''s'`, "trap 'publish 130; exit 130' INT\n", "publish \"$code\""} {
		if !strings.Contains(s, want) {
			t.Errorf("script missing %q:\n%s", want, s)
		}
	}
	if strings.Contains(s, "HUP") {
		t.Error("HUP must not be trapped (claude would outlive a killed pane)")
	}
}

func TestOrphanedAgentOlderThanTimeoutIsReplaced(t *testing.T) {
	r := tmuxRunner(t, fakeClaude(t, "hang"), 20*time.Second)
	done := runHanging(t, r) // run 1 hangs
	w, err := r.tmux.windowState(context.Background(), "sdtest", "names")
	if err != nil || w.started.IsZero() {
		t.Fatalf("started option not recorded: %+v %v", w, err)
	}
	// A later process with a short timeout sees run 1's windows as orphaned.
	r2 := r
	r2.cfg.JobTimeout = 1 * time.Second
	time.Sleep(1100 * time.Millisecond)
	results, err := r2.Run(context.Background(), 2, testNow, instruments(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, res := range results {
		if errors.Is(res.Err, ErrJobBusy) {
			t.Errorf("%s: stale orphan should have been replaced", res.Job)
		}
	}
	<-done // run 1 observes its panes dying
}
