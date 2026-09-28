package main

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

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
