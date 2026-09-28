// Package config loads swingdesk settings from YAML and resolves XDG paths.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"time"

	"gopkg.in/yaml.v3"
)

// PermissionMode is a Claude Code --permission-mode value.
type PermissionMode string

// Values match `claude --help` (Claude Code 2.1).
const (
	PermissionManual      PermissionMode = "manual"
	PermissionAuto        PermissionMode = "auto"
	PermissionAcceptEdits PermissionMode = "acceptEdits"
	PermissionPlan        PermissionMode = "plan"
	PermissionDontAsk     PermissionMode = "dontAsk"
)

// permissionModes is the closed set of accepted permission modes.
// bypassPermissions is deliberately absent: use ClaudeSkipPermissions to opt in.
var permissionModes = [...]PermissionMode{
	PermissionManual, PermissionAuto, PermissionAcceptEdits, PermissionPlan, PermissionDontAsk,
}

// Valid reports whether m is an accepted permission mode.
func (m PermissionMode) Valid() bool {
	return slices.Contains(permissionModes[:], m)
}

// sessionName excludes '.' and ':' which tmux treats as target separators.
var sessionName = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// Config is the user-facing configuration file shape.
type Config struct {
	RefreshMinutes        int            `yaml:"refresh_minutes"`
	Timezone              string         `yaml:"timezone"`
	TmuxSession           string         `yaml:"tmux_session"`
	ClaudeBin             string         `yaml:"claude_bin"`
	ClaudeModel           string         `yaml:"claude_model"`
	ClaudePermissionMode  PermissionMode `yaml:"claude_permission_mode"`
	ClaudeSkipPermissions bool           `yaml:"claude_dangerously_skip_permissions"`
	ClaudeMaxBudgetUSD    float64        `yaml:"claude_max_budget_usd"`
	JobTimeoutMinutes     int            `yaml:"job_timeout_minutes"`
	MaxItemsPerJob        int            `yaml:"max_items_per_job"`
	DataDir               string         `yaml:"data_dir"`
}

// Defaults returns the built-in configuration from docs/PLAN.md.
func Defaults() Config {
	return Config{
		RefreshMinutes:       30,
		Timezone:             "Europe/London",
		TmuxSession:          "swingdesk",
		ClaudeBin:            "claude",
		ClaudeModel:          "",
		ClaudePermissionMode: PermissionDontAsk,
		JobTimeoutMinutes:    8,
		MaxItemsPerJob:       40,
		DataDir:              "",
	}
}

// Load reads loc.Path over the defaults. A missing file yields defaults
// only when the location is the implicit XDG fallback; an explicitly named
// file (flag or $SWINGDESK_CONFIG) must exist.
func Load(loc Location) (Config, error) {
	path := loc.Path
	cfg := Defaults()
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) && !loc.Explicit {
		return cfg, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("read config %s: %w", path, err)
	}
	if err := decodeStrict(raw, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config %s: %w", path, err)
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, fmt.Errorf("config %s: %w", path, err)
	}
	return cfg, nil
}

// decodeStrict decodes a single YAML document, rejecting unknown fields and
// any trailing documents. An empty file leaves cfg untouched.
func decodeStrict(raw []byte, cfg *Config) error {
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(cfg); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return err
	}
	var extra yaml.Node
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("config must contain exactly one YAML document")
	}
	return nil
}

// Validate checks field ranges and closed sets.
func (c Config) Validate() error {
	var errs []error
	if c.RefreshMinutes < 1 {
		errs = append(errs, fmt.Errorf("refresh_minutes must be >= 1, got %d", c.RefreshMinutes))
	}
	if _, err := time.LoadLocation(c.Timezone); err != nil || c.Timezone == "" || c.Timezone == "Local" {
		errs = append(errs, fmt.Errorf("timezone %q is not a valid IANA zone", c.Timezone))
	}
	if !sessionName.MatchString(c.TmuxSession) {
		errs = append(errs, fmt.Errorf("tmux_session %q must be non-empty letters, digits, _ or -", c.TmuxSession))
	}
	if c.ClaudeBin == "" {
		errs = append(errs, errors.New("claude_bin must not be empty"))
	}
	if !c.ClaudePermissionMode.Valid() {
		errs = append(errs, fmt.Errorf("claude_permission_mode %q must be one of %v", c.ClaudePermissionMode, permissionModes))
	}
	if c.JobTimeoutMinutes < 1 {
		errs = append(errs, fmt.Errorf("job_timeout_minutes must be >= 1, got %d", c.JobTimeoutMinutes))
	}
	if c.ClaudeMaxBudgetUSD < 0 {
		errs = append(errs, fmt.Errorf("claude_max_budget_usd must be >= 0, got %v", c.ClaudeMaxBudgetUSD))
	}
	if c.MaxItemsPerJob < 1 {
		errs = append(errs, fmt.Errorf("max_items_per_job must be >= 1, got %d", c.MaxItemsPerJob))
	}
	if c.DataDir != "" && !filepath.IsAbs(c.DataDir) && !isHomeRelative(c.DataDir) {
		errs = append(errs, fmt.Errorf("data_dir %q must be absolute or start with ~/", c.DataDir))
	}
	return errors.Join(errs...)
}

// RefreshInterval is RefreshMinutes as a duration.
func (c Config) RefreshInterval() time.Duration {
	return time.Duration(c.RefreshMinutes) * time.Minute
}

// JobTimeout is JobTimeoutMinutes as a duration.
func (c Config) JobTimeout() time.Duration {
	return time.Duration(c.JobTimeoutMinutes) * time.Minute
}

// Location returns the configured timezone.
func (c Config) Location() (*time.Location, error) {
	return time.LoadLocation(c.Timezone)
}
