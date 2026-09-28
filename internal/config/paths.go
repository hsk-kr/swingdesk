package config

import (
	"path/filepath"
	"strings"
)

const appName = "swingdesk"

// Getenv matches os.Getenv so tests can inject an environment.
type Getenv func(string) string

// Location is a config file path plus whether the user named it explicitly.
type Location struct {
	Path     string
	Explicit bool
}

// ConfigPath resolves the config file: flagValue, then $SWINGDESK_CONFIG, then
// $XDG_CONFIG_HOME/swingdesk/config.yaml, then ~/.config/swingdesk/config.yaml.
func ConfigPath(flagValue string, getenv Getenv, home string) Location {
	if flagValue != "" {
		return Location{Path: expandHome(flagValue, home), Explicit: true}
	}
	if p := getenv("SWINGDESK_CONFIG"); p != "" {
		return Location{Path: expandHome(p, home), Explicit: true}
	}
	return Location{
		Path: filepath.Join(xdgDir(getenv, "XDG_CONFIG_HOME", home, ".config"), appName, "config.yaml"),
	}
}

// DataDir resolves the data directory: cfg.DataDir, then
// $XDG_DATA_HOME/swingdesk, then ~/.local/share/swingdesk.
func DataDir(cfg Config, getenv Getenv, home string) string {
	if cfg.DataDir != "" {
		return expandHome(cfg.DataDir, home)
	}
	return filepath.Join(xdgDir(getenv, "XDG_DATA_HOME", home, filepath.Join(".local", "share")), appName)
}

// xdgDir honours an XDG variable only when it is absolute, per the spec.
func xdgDir(getenv Getenv, key, home, fallback string) string {
	if v := getenv(key); v != "" && filepath.IsAbs(v) {
		return v
	}
	return filepath.Join(home, fallback)
}

func isHomeRelative(p string) bool {
	return p == "~" || strings.HasPrefix(p, "~/")
}

func expandHome(p, home string) string {
	if p == "~" {
		return home
	}
	if strings.HasPrefix(p, "~/") {
		return filepath.Join(home, p[2:])
	}
	return p
}

// Paths bundles the resolved on-disk locations.
type Paths struct {
	ConfigFile string
	DataDir    string
	DBFile     string
	InboxDir   string
	RunsDir    string
}

// ResolvePaths derives all on-disk paths for cfg.
func ResolvePaths(cfg Config, configFile string, getenv Getenv, home string) Paths {
	data := DataDir(cfg, getenv, home)
	return Paths{
		ConfigFile: configFile,
		DataDir:    data,
		DBFile:     filepath.Join(data, "swingdesk.db"),
		InboxDir:   filepath.Join(data, "inbox"),
		RunsDir:    filepath.Join(data, "runs"),
	}
}
