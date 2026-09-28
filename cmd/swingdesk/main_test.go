package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunPrintsPaths(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte("data_dir: "+dir+"/data\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := run([]string{"-config", cfgPath}, &out); err != nil {
		t.Fatalf("run: %v", err)
	}
	for _, want := range []string{cfgPath, dir + "/data/swingdesk.db", "every 30 min", "instruments: 14"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output missing %q:\n%s", want, out.String())
		}
	}
}

func TestRunBadConfig(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(cfgPath, []byte("refresh_minutes: 0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"-config", cfgPath}, &bytes.Buffer{}); err == nil {
		t.Fatal("expected error")
	}
}

func TestRunHelpExitsCleanly(t *testing.T) {
	if err := run([]string{"-h"}, &bytes.Buffer{}); err != nil {
		t.Fatalf("run -h: %v", err)
	}
}

func TestRunRejectsPositionalArgs(t *testing.T) {
	if err := run([]string{"extra"}, &bytes.Buffer{}); err == nil {
		t.Fatal("expected error")
	}
}

func TestRunMissingExplicitConfig(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.yaml")
	if err := run([]string{"-config", missing}, &bytes.Buffer{}); err == nil {
		t.Fatal("expected error")
	}
}
