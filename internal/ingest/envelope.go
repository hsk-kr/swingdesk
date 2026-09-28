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
	if err := errors.Join(requireKeys(body, "items", "biases"), env.validate()); err != nil {
		return Envelope{}, err
	}
	return env, nil
}

// requireKeys rejects envelopes missing a list key (an explicit [] is fine),
// so a truncated or mis-shaped payload is not mistaken for an empty run.
func requireKeys(body []byte, keys ...string) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		return fmt.Errorf("decode envelope: %w", err)
	}
	var errs []error
	for _, k := range keys {
		if v, ok := fields[k]; !ok || bytes.Equal(v, []byte("null")) {
			errs = append(errs, fmt.Errorf("envelope is missing %q", k))
		}
	}
	return errors.Join(errs...)
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
		return firstJSONObject(*probe.Result)
	}
	return nil, errors.New("claude result has neither structured_output nor result")
}

// firstJSONObject extracts the first complete JSON object from free text, so
// prose or ``` fences around the envelope do not cost a whole job run.
func firstJSONObject(s string) ([]byte, error) {
	for start := strings.IndexByte(s, '{'); start >= 0; {
		var obj json.RawMessage
		if err := json.NewDecoder(strings.NewReader(s[start:])).Decode(&obj); err == nil {
			return obj, nil
		}
		next := strings.IndexByte(s[start+1:], '{')
		if next < 0 {
			break
		}
		start += 1 + next
	}
	return nil, fmt.Errorf("claude result contains no JSON object: %s", truncate(s, 200))
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

// truncate shortens s to at most n runes.
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
