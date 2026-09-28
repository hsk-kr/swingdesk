// Package config loads swingdesk settings from YAML and resolves XDG paths.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"slices"
	"time"

	"gopkg.in/yaml.v3"
)

// PermissionMode is a Claude Code --permission-mode value.
type PermissionMode string

const (
	PermissionDefault     PermissionMode = "default"
	PermissionAcceptEdits PermissionMode = "acceptEdits"
	PermissionPlan        PermissionMode = "plan"
	PermissionDontAsk     PermissionMode = "dontAsk"
)

// PermissionModes is the closed set of accepted permission modes.
// bypassPermissions is deliberately absent: use ClaudeSkipPermissions to opt in.
var PermissionModes = [...]PermissionMode{
	PermissionDefault, PermissionAcceptEdits, PermissionPlan, PermissionDontAsk,
}

// Config is the user-facing configuration file shape.
type Config struct {
	RefreshMinutes        int            `yaml:"refresh_minutes"`
	Timezone              string         `yaml:"timezone"`
	TmuxSession           string         `yaml:"tmux_session"`
	ClaudeBin             string         `yaml:"claude_bin"`
	ClaudeModel           string         `yaml:"claude_model"`
	ClaudePermissionMode  PermissionMode `yaml:"claude_permission_mode"`
	ClaudeSkipPermissions bool           `yaml:"claude_dangerously_skip_permissions"`
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
		MaxItemsPerJob:       40,
		DataDir:              "",
	}
}

// Load reads path over the defaults. A missing file yields defaults.
func Load(path string) (Config, error) {
	cfg := Defaults()
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("read config %s: %w", path, err)
	}
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil && !errors.Is(err, io.EOF) {
		return Config{}, fmt.Errorf("parse config %s: %w", path, err)
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, fmt.Errorf("config %s: %w", path, err)
	}
	return cfg, nil
}

// Validate checks field ranges and closed sets.
func (c Config) Validate() error {
	var errs []error
	if c.RefreshMinutes < 1 {
		errs = append(errs, fmt.Errorf("refresh_minutes must be >= 1, got %d", c.RefreshMinutes))
	}
	if _, err := time.LoadLocation(c.Timezone); err != nil || c.Timezone == "" {
		errs = append(errs, fmt.Errorf("timezone %q is not a valid IANA zone", c.Timezone))
	}
	if c.TmuxSession == "" {
		errs = append(errs, errors.New("tmux_session must not be empty"))
	}
	if c.ClaudeBin == "" {
		errs = append(errs, errors.New("claude_bin must not be empty"))
	}
	if !slices.Contains(PermissionModes[:], c.ClaudePermissionMode) {
		errs = append(errs, fmt.Errorf("claude_permission_mode %q must be one of %v", c.ClaudePermissionMode, PermissionModes))
	}
	if c.MaxItemsPerJob < 1 {
		errs = append(errs, fmt.Errorf("max_items_per_job must be >= 1, got %d", c.MaxItemsPerJob))
	}
	return errors.Join(errs...)
}

// RefreshInterval is RefreshMinutes as a duration.
func (c Config) RefreshInterval() time.Duration {
	return time.Duration(c.RefreshMinutes) * time.Minute
}

// Location returns the configured timezone.
func (c Config) Location() (*time.Location, error) {
	return time.LoadLocation(c.Timezone)
}
