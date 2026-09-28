// Package ingest validates agent JSON (docs/AGENT_CONTRACT.md) and writes it
// to SQLite.
package ingest

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/hsk-kr/swingdesk/internal/model"
)

// Envelope is one job file.
type Envelope struct {
	Job         model.Job `json:"job"`
	GeneratedAt string    `json:"generated_at"`
	Items       []RawItem `json:"items"`
	Biases      []RawBias `json:"biases"`
}

// RawItem mirrors the contract; nullable fields are pointers.
type RawItem struct {
	Symbol      *string        `json:"symbol"`
	Category    model.Category `json:"category"`
	Title       string         `json:"title"`
	Summary     string         `json:"summary"`
	Body        string         `json:"body"`
	Source      string         `json:"source"`
	URL         string         `json:"url"`
	PublishedAt *string        `json:"published_at"`
	EventAt     *string        `json:"event_at"`
	EventKind   *string        `json:"event_kind"`
}

// RawBias mirrors the contract.
type RawBias struct {
	Symbol     string       `json:"symbol"`
	Stance     model.Stance `json:"stance"`
	Confidence *float64     `json:"confidence"`
	Rationale  string       `json:"rationale"`
}

// claudeResult is the subset of `claude -p --output-format json` we read.
type claudeResult struct {
	Type             string          `json:"type"`
	IsError          bool            `json:"is_error"`
	Result           *string         `json:"result"`
	StructuredOutput json.RawMessage `json:"structured_output"`
}

// Parse decodes a job file. It accepts the bare envelope or the Claude Code
// print-mode JSON wrapper (structured_output, or a JSON string in result).
// Envelope-level problems (bad JSON, unknown job, bad generated_at) fail the
// whole file; per-item problems are reported later by Ingest.
func Parse(raw []byte) (Envelope, error) {
	body, err := unwrap(raw)
	if err != nil {
		return Envelope{}, err
	}
	var env Envelope
	if err := json.Unmarshal(body, &env); err != nil {
		return Envelope{}, fmt.Errorf("decode envelope: %w", err)
	}
	if err := env.validate(); err != nil {
		return Envelope{}, err
	}
	return env, nil
}

func unwrap(raw []byte) ([]byte, error) {
	var probe claudeResult
	if err := json.Unmarshal(raw, &probe); err != nil {
		return nil, fmt.Errorf("decode job file: %w", err)
	}
	if probe.Type != "result" {
		return raw, nil
	}
	if probe.IsError {
		msg := ""
		if probe.Result != nil {
			msg = *probe.Result
		}
		return nil, fmt.Errorf("claude reported an error: %s", truncate(msg, 200))
	}
	if len(probe.StructuredOutput) > 0 && !bytes.Equal(probe.StructuredOutput, []byte("null")) {
		return probe.StructuredOutput, nil
	}
	if probe.Result != nil {
		return []byte(stripFence(*probe.Result)), nil
	}
	return nil, errors.New("claude result has neither structured_output nor result")
}

// stripFence removes a ```json ... ``` wrapper if the model added one.
func stripFence(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "```") {
		return s
	}
	s = strings.TrimPrefix(s, "```")
	if nl := strings.IndexByte(s, '\n'); nl >= 0 {
		s = s[nl+1:]
	}
	return strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), "```"))
}

func (e Envelope) validate() error {
	var errs []error
	if !e.Job.Valid() {
		errs = append(errs, fmt.Errorf("job %q must be one of %v", e.Job, model.Jobs()))
	}
	if _, err := time.Parse(time.RFC3339, e.GeneratedAt); err != nil {
		errs = append(errs, fmt.Errorf("generated_at %q is not RFC3339", e.GeneratedAt))
	}
	return errors.Join(errs...)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
