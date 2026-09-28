package main

import (
	"bytes"
	"errors"
	"os"
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
