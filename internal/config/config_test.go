package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDefaults(t *testing.T) {
	d := Defaults()
	if d.RefreshMinutes != 30 {
		t.Errorf("RefreshMinutes = %d, want 30", d.RefreshMinutes)
	}
	if d.Timezone != "Europe/London" {
		t.Errorf("Timezone = %q", d.Timezone)
	}
	if d.TmuxSession != "swingdesk" {
		t.Errorf("TmuxSession = %q", d.TmuxSession)
	}
	if d.ClaudeBin != "claude" {
		t.Errorf("ClaudeBin = %q", d.ClaudeBin)
	}
	if d.ClaudeModel != "" {
		t.Errorf("ClaudeModel = %q, want empty (CLI default)", d.ClaudeModel)
	}
	if d.ClaudePermissionMode != PermissionDontAsk {
		t.Errorf("ClaudePermissionMode = %q", d.ClaudePermissionMode)
	}
	if d.ClaudeSkipPermissions {
		t.Error("ClaudeSkipPermissions must default to false")
	}
	if d.JobTimeout() != 8*time.Minute || d.ClaudeMaxBudgetUSD != 0 {
		t.Errorf("JobTimeout = %v budget = %v", d.JobTimeout(), d.ClaudeMaxBudgetUSD)
	}
	if d.MaxItemsPerJob != 40 {
		t.Errorf("MaxItemsPerJob = %d", d.MaxItemsPerJob)
	}
	if d.RefreshInterval() != 30*time.Minute {
		t.Errorf("RefreshInterval = %v", d.RefreshInterval())
	}
}

func TestLoadMissingFileUsesDefaults(t *testing.T) {
	cfg, err := Load(Location{Path: filepath.Join(t.TempDir(), "nope.yaml")})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.RefreshMinutes != 30 || cfg.TmuxSession != "swingdesk" {
		t.Errorf("unexpected cfg: %+v", cfg)
	}
}

func TestLoadOverridesAndKeepsDefaults(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.yaml")
	writeFile(t, p, "refresh_minutes: 15\nclaude_model: sonnet\n")
	cfg, err := Load(Location{Path: p, Explicit: true})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.RefreshMinutes != 15 {
		t.Errorf("RefreshMinutes = %d", cfg.RefreshMinutes)
	}
	if cfg.ClaudeModel != "sonnet" {
		t.Errorf("ClaudeModel = %q", cfg.ClaudeModel)
	}
	if cfg.Timezone != "Europe/London" {
		t.Errorf("Timezone default lost: %q", cfg.Timezone)
	}
}

func TestLoadRejectsInvalid(t *testing.T) {
	cases := map[string]string{
		"zero refresh":    "refresh_minutes: 0\n",
		"bad timezone":    "timezone: Mars/Olympus\n",
		"bad perm mode":   "claude_permission_mode: yolo\n",
		"empty session":   "tmux_session: \"\"\n",
		"unknown field":   "refresh_minutez: 5\n",
		"bad yaml":        "refresh_minutes: [\n",
		"negative items":  "max_items_per_job: -1\n",
		"empty claudebin": "claude_bin: \"\"\n",
		"bypass mode":     "claude_permission_mode: bypassPermissions\n",
		"dotted session":  "tmux_session: sw.desk\n",
		"items over cap":  "max_items_per_job: 41\n",
		"stale default":   "claude_permission_mode: default\n",
		"zero timeout":    "job_timeout_minutes: 0\n",
		"negative budget": "claude_max_budget_usd: -1\n",
		"local timezone":  "timezone: Local\n",
		"relative data":   "data_dir: rel/data\n",
		"second document": "refresh_minutes: 5\n---\nrefresh_minutes: 0\n",
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "config.yaml")
			writeFile(t, p, body)
			if _, err := Load(Location{Path: p}); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestLoadExplicitMissingFileFails(t *testing.T) {
	loc := Location{Path: filepath.Join(t.TempDir(), "nope.yaml"), Explicit: true}
	if _, err := Load(loc); err == nil {
		t.Fatal("expected error for explicitly named missing file")
	}
}

func TestLoadEmptyAndCommentOnlyFiles(t *testing.T) {
	for name, body := range map[string]string{"empty": "", "comments": "# nothing here\n"} {
		t.Run(name, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "config.yaml")
			writeFile(t, p, body)
			cfg, err := Load(Location{Path: p, Explicit: true})
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if cfg != Defaults() {
				t.Errorf("cfg = %+v, want defaults", cfg)
			}
		})
	}
}

func TestLoadAcceptsHomeRelativeDataDir(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.yaml")
	writeFile(t, p, "data_dir: ~/sd\n")
	if _, err := Load(Location{Path: p}); err != nil {
		t.Fatalf("Load: %v", err)
	}
}

func TestLocation(t *testing.T) {
	cfg := Defaults()
	loc, err := cfg.Location()
	if err != nil {
		t.Fatal(err)
	}
	if loc.String() != "Europe/London" {
		t.Errorf("loc = %s", loc)
	}
}

func TestResolvePaths(t *testing.T) {
	home := "/home/u"
	t.Run("defaults", func(t *testing.T) {
		env := mapEnv(map[string]string{})
		want := Location{Path: "/home/u/.config/swingdesk/config.yaml"}
		if got := ConfigPath("", env, home); got != want {
			t.Errorf("ConfigPath = %+v", got)
		}
		if got := DataDir(Defaults(), env, home); got != "/home/u/.local/share/swingdesk" {
			t.Errorf("DataDir = %s", got)
		}
	})
	t.Run("xdg", func(t *testing.T) {
		env := mapEnv(map[string]string{"XDG_CONFIG_HOME": "/x/cfg", "XDG_DATA_HOME": "/x/data"})
		if got := ConfigPath("", env, home); got.Path != "/x/cfg/swingdesk/config.yaml" || got.Explicit {
			t.Errorf("ConfigPath = %+v", got)
		}
		if got := DataDir(Defaults(), env, home); got != "/x/data/swingdesk" {
			t.Errorf("DataDir = %s", got)
		}
	})
	t.Run("relative xdg ignored", func(t *testing.T) {
		env := mapEnv(map[string]string{"XDG_CONFIG_HOME": "rel", "XDG_DATA_HOME": "rel"})
		if got := ConfigPath("", env, home); got.Path != "/home/u/.config/swingdesk/config.yaml" {
			t.Errorf("ConfigPath = %+v", got)
		}
		if got := DataDir(Defaults(), env, home); got != "/home/u/.local/share/swingdesk" {
			t.Errorf("DataDir = %s", got)
		}
	})
	t.Run("explicit overrides", func(t *testing.T) {
		env := mapEnv(map[string]string{"SWINGDESK_CONFIG": "/etc/sd.yaml", "XDG_DATA_HOME": "/x/data"})
		if got := ConfigPath("", env, home); got != (Location{Path: "/etc/sd.yaml", Explicit: true}) {
			t.Errorf("ConfigPath = %+v", got)
		}
		if got := ConfigPath("~/flag.yaml", env, home); got != (Location{Path: "/home/u/flag.yaml", Explicit: true}) {
			t.Errorf("flag ConfigPath = %+v", got)
		}
		tilde := mapEnv(map[string]string{"SWINGDESK_CONFIG": "~/x.yaml"})
		if got := ConfigPath("", tilde, home); got.Path != "/home/u/x.yaml" {
			t.Errorf("env tilde ConfigPath = %+v", got)
		}
		cfg := Defaults()
		cfg.DataDir = "/srv/sd"
		if got := DataDir(cfg, env, home); got != "/srv/sd" {
			t.Errorf("DataDir = %s", got)
		}
	})
	t.Run("tilde data dir", func(t *testing.T) {
		cfg := Defaults()
		cfg.DataDir = "~/sd"
		if got := DataDir(cfg, mapEnv(nil), home); got != "/home/u/sd" {
			t.Errorf("DataDir = %s", got)
		}
	})
}

func TestResolvePathsAllFields(t *testing.T) {
	cfg := Defaults()
	cfg.DataDir = "/d"
	got := ResolvePaths(cfg, "/c.yaml", mapEnv(nil), "/home/u")
	want := Paths{
		ConfigFile: "/c.yaml",
		DataDir:    "/d",
		DBFile:     "/d/swingdesk.db",
		InboxDir:   "/d/inbox",
		RunsDir:    "/d/runs",
	}
	if got != want {
		t.Errorf("ResolvePaths = %+v, want %+v", got, want)
	}
}

func mapEnv(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}
