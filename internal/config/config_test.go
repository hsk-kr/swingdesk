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
	if d.MaxItemsPerJob != 40 {
		t.Errorf("MaxItemsPerJob = %d", d.MaxItemsPerJob)
	}
	if d.RefreshInterval() != 30*time.Minute {
		t.Errorf("RefreshInterval = %v", d.RefreshInterval())
	}
}

func TestLoadMissingFileUsesDefaults(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "nope.yaml"))
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
	cfg, err := Load(p)
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
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "config.yaml")
			writeFile(t, p, body)
			if _, err := Load(p); err == nil {
				t.Fatal("expected error")
			}
		})
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
		if got := ConfigPath(env, home); got != "/home/u/.config/swingdesk/config.yaml" {
			t.Errorf("ConfigPath = %s", got)
		}
		if got := DataDir(Defaults(), env, home); got != "/home/u/.local/share/swingdesk" {
			t.Errorf("DataDir = %s", got)
		}
	})
	t.Run("xdg", func(t *testing.T) {
		env := mapEnv(map[string]string{"XDG_CONFIG_HOME": "/x/cfg", "XDG_DATA_HOME": "/x/data"})
		if got := ConfigPath(env, home); got != "/x/cfg/swingdesk/config.yaml" {
			t.Errorf("ConfigPath = %s", got)
		}
		if got := DataDir(Defaults(), env, home); got != "/x/data/swingdesk" {
			t.Errorf("DataDir = %s", got)
		}
	})
	t.Run("relative xdg ignored", func(t *testing.T) {
		env := mapEnv(map[string]string{"XDG_CONFIG_HOME": "rel", "XDG_DATA_HOME": "rel"})
		if got := ConfigPath(env, home); got != "/home/u/.config/swingdesk/config.yaml" {
			t.Errorf("ConfigPath = %s", got)
		}
		if got := DataDir(Defaults(), env, home); got != "/home/u/.local/share/swingdesk" {
			t.Errorf("DataDir = %s", got)
		}
	})
	t.Run("explicit overrides", func(t *testing.T) {
		env := mapEnv(map[string]string{"SWINGDESK_CONFIG": "/etc/sd.yaml", "XDG_DATA_HOME": "/x/data"})
		if got := ConfigPath(env, home); got != "/etc/sd.yaml" {
			t.Errorf("ConfigPath = %s", got)
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

func mapEnv(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}
