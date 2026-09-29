package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hsk-kr/swingdesk/internal/db"
	"github.com/hsk-kr/swingdesk/internal/model"
	"github.com/hsk-kr/swingdesk/internal/ui"
)

func noUI(t *testing.T) startUI {
	return func(ui.Model) error {
		t.Error("UI should not start")
		return nil
	}
}

func tempConfig(t *testing.T, body string) (string, string) {
	t.Helper()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte(body+"data_dir: "+dir+"/data\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir, cfgPath
}

func TestRunPrintsPaths(t *testing.T) {
	dir, cfgPath := tempConfig(t, "")
	var out bytes.Buffer
	if err := run([]string{"-config", cfgPath, "-paths"}, &out, noUI(t)); err != nil {
		t.Fatalf("run: %v", err)
	}
	for _, want := range []string{cfgPath, dir + "/data/swingdesk.db", "every 30 min", "instruments: 14"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output missing %q:\n%s", want, out.String())
		}
	}
}

func TestRunStartsUI(t *testing.T) {
	_, cfgPath := tempConfig(t, "")
	started := false
	err := run([]string{"-config", cfgPath}, &bytes.Buffer{}, func(ui.Model) error {
		started = true
		return nil
	})
	if err != nil || !started {
		t.Fatalf("started=%v err=%v", started, err)
	}
}

func TestRunPropagatesUIError(t *testing.T) {
	_, cfgPath := tempConfig(t, "")
	boom := errors.New("boom")
	if err := run([]string{"-config", cfgPath}, &bytes.Buffer{}, func(ui.Model) error { return boom }); !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
}

func TestRunBadConfig(t *testing.T) {
	_, cfgPath := tempConfig(t, "refresh_minutes: 0\n")
	if err := run([]string{"-config", cfgPath}, &bytes.Buffer{}, noUI(t)); err == nil {
		t.Fatal("expected error")
	}
}

func TestRunHelpExitsCleanly(t *testing.T) {
	if err := run([]string{"-h"}, &bytes.Buffer{}, noUI(t)); err != nil {
		t.Fatalf("run -h: %v", err)
	}
}

func TestRunRejectsPositionalArgs(t *testing.T) {
	if err := run([]string{"extra"}, &bytes.Buffer{}, noUI(t)); err == nil {
		t.Fatal("expected error")
	}
}

func TestRunMissingExplicitConfig(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.yaml")
	if err := run([]string{"-config", missing}, &bytes.Buffer{}, noUI(t)); err == nil {
		t.Fatal("expected error")
	}
}

func TestRunInsertSample(t *testing.T) {
	_, cfgPath := tempConfig(t, "")
	var out bytes.Buffer
	if err := run([]string{"-config", cfgPath, "-insert-sample"}, &out, noUI(t)); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(out.String(), "inserted 10 sample items") {
		t.Errorf("output = %q", out.String())
	}
	out.Reset()
	if err := run([]string{"-config", cfgPath, "-insert-sample"}, &out, noUI(t)); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "inserted 0 sample items") {
		t.Errorf("second run output = %q", out.String())
	}
}

func TestRunIngestFile(t *testing.T) {
	_, cfgPath := tempConfig(t, "")
	var out bytes.Buffer
	err := run([]string{"-config", cfgPath, "-ingest", "../../internal/ingest/testdata/names.json"}, &out, noUI(t))
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	for _, want := range []string{"names: 2 new", "skipped item #2 IBM: unknown symbol"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output missing %q:\n%s", want, out.String())
		}
	}
	if err := run([]string{"-config", cfgPath, "-ingest", "../../internal/ingest/testdata/invalid.json"}, &out, noUI(t)); err == nil {
		t.Error("invalid file should error")
	}
}

// TestRefreshOnceEndToEnd runs the real runner against a private tmux server
// (TMUX_TMPDIR) and a fake claude, then checks rows landed and files archived.
func TestRefreshOnceEndToEnd(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not installed")
	}
	sockDir, err := os.MkdirTemp("", "sdm")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMUX_TMPDIR", sockDir)
	t.Setenv("TMUX", "")
	t.Cleanup(func() {
		_ = exec.Command("tmux", "kill-server").Run()
		_ = os.RemoveAll(sockDir)
	})
	fake := filepath.Join(t.TempDir(), "claude")
	fixture, err := filepath.Abs("../../internal/ingest/testdata")
	if err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\ncase \"$2\" in *'macro/market desk'*) j=market;; *'tech sector desk'*) j=tech;; *) j=names;; esac\ncat '" + fixture + "'/$j.json\n"
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	dir, cfgPath := tempConfig(t, "claude_bin: "+fake+"\ntmux_session: sdmain\n")
	var out bytes.Buffer
	if err := run([]string{"-config", cfgPath, "-refresh-once"}, &out, noUI(t)); err != nil {
		t.Fatalf("run: %v\n%s", err, out.String())
	}
	for _, want := range []string{"run 1: ok, 7 new items", "market: 3 new", "names: 2 new"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output missing %q:\n%s", want, out.String())
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "data", "runs", "1", "names.json")); err != nil {
		t.Errorf("names.json not archived: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "data", "swingdesk.log")); err != nil {
		t.Errorf("log file missing: %v", err)
	}
}

func TestInitialStatusRestoresLastRefresh(t *testing.T) {
	_, cfgPath := tempConfig(t, "")
	a, err := openApp(context.Background(), cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	st, err := a.initialStatus(context.Background())
	if err != nil || !st.LastRefresh.IsZero() {
		t.Fatalf("fresh db: %+v %v", st, err)
	}
	ctx := context.Background()
	finished := time.Date(2026, 9, 27, 11, 0, 0, 0, time.UTC)
	id, err := db.StartRun(ctx, a.conn, finished.Add(-time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.FinishRun(ctx, a.conn, db.Run{ID: id, FinishedAt: finished, Status: model.RunOK, JobsOK: 3}); err != nil {
		t.Fatal(err)
	}
	st, err = a.initialStatus(ctx)
	if err != nil || !st.LastRefresh.Equal(finished) {
		t.Errorf("status = %+v %v", st, err)
	}
}

func TestTmuxCopier(t *testing.T) {
	if tmuxCopier(func(string) string { return "" }) != nil {
		t.Error("no copier outside tmux")
	}
	if tmuxCopier(func(k string) string { return map[string]string{"TMUX": "/tmp/x,1,0"}[k] }) == nil {
		t.Error("copier expected inside tmux")
	}
}

func TestKillAgentsFlagWithoutSession(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not installed")
	}
	sockDir, err := os.MkdirTemp("", "sdk")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMUX_TMPDIR", sockDir)
	t.Setenv("TMUX", "")
	t.Cleanup(func() { _ = os.RemoveAll(sockDir) })
	_, cfgPath := tempConfig(t, "tmux_session: sdkill\n")
	var out bytes.Buffer
	if err := run([]string{"-config", cfgPath, "-kill-agents"}, &out, noUI(t)); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(out.String(), "killed tmux session sdkill") {
		t.Errorf("out = %q", out.String())
	}
}

func TestSplitNamesRefreshOnce(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not installed")
	}
	sockDir, err := os.MkdirTemp("", "sds")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMUX_TMPDIR", sockDir)
	t.Setenv("TMUX", "")
	t.Cleanup(func() {
		_ = exec.Command("tmux", "kill-server").Run()
		_ = os.RemoveAll(sockDir)
	})
	fixture, err := filepath.Abs("../../internal/ingest/testdata")
	if err != nil {
		t.Fatal(err)
	}
	fake := filepath.Join(t.TempDir(), "claude")
	// names_rest reuses the names fixture with its job field rewritten.
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" >> \"$(dirname \"$0\")/argv\"\n" +
		"case \"$2\" in *'macro/market desk'*) cat '" + fixture + "'/market.json;; *'tech sector desk'*) cat '" + fixture + "'/tech.json;;\n" +
		" *'`names_rest`'*) sed 's/\"job\": \"names\"/\"job\": \"names_rest\"/' '" + fixture + "'/names.json;; *) cat '" + fixture + "'/names.json;; esac\n"
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	_, cfgPath := tempConfig(t, "claude_bin: "+fake+"\ntmux_session: sdsplit\nsplit_names: true\nclaude_job_models: {tech: haiku}\n")
	var out bytes.Buffer
	if err := run([]string{"-config", cfgPath, "-refresh-once"}, &out, noUI(t)); err != nil {
		t.Fatalf("run: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "run 1: ok") || !strings.Contains(out.String(), "names_rest: 0 new, 2 updated") {
		t.Errorf("output:\n%s", out.String())
	}
	argv, err := os.ReadFile(filepath.Join(filepath.Dir(fake), "argv"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(argv), "haiku") != 1 {
		t.Errorf("exactly the tech job should get --model haiku:\n%s", argv)
	}
}
