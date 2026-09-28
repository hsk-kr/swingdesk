// Package agent runs the Claude Code research jobs inside tmux.
package agent

import (
	"fmt"
	"io/fs"
	"strconv"
	"strings"
	"time"

	"github.com/hsk-kr/swingdesk"
	"github.com/hsk-kr/swingdesk/internal/model"
)

// PromptInput is what gets injected into a job template at run time.
type PromptInput struct {
	Job         model.Job
	Instruments []model.Instrument
	Now         time.Time
	Location    *time.Location
	MaxItems    int
}

// RenderPrompt fills prompts/<job>.md (plus the shared rules) for in.
func RenderPrompt(in PromptInput) (string, error) {
	if !in.Job.Valid() {
		return "", fmt.Errorf("unknown job %q", in.Job)
	}
	tmpl, err := fs.ReadFile(swingdesk.Prompts, "prompts/"+string(in.Job)+".md")
	if err != nil {
		return "", fmt.Errorf("read %s prompt: %w", in.Job, err)
	}
	rules, err := fs.ReadFile(swingdesk.Prompts, "prompts/_rules.md")
	if err != nil {
		return "", fmt.Errorf("read shared rules: %w", err)
	}
	loc := in.Location
	if loc == nil {
		loc = time.UTC
	}
	common := []string{
		"{{job}}", string(in.Job),
		"{{max_items}}", strconv.Itoa(in.MaxItems),
	}
	filledRules := strings.NewReplacer(common...).Replace(string(rules))
	out := strings.NewReplacer(append(common,
		"{{rules}}", strings.TrimSpace(filledRules),
		"{{timezone}}", loc.String(),
		"{{now_rfc3339}}", in.Now.In(loc).Format(time.RFC3339),
		"{{watchlist_table}}", WatchlistTable(in.Instruments),
	)...).Replace(string(tmpl))
	if i := strings.Index(out, "{{"); i >= 0 {
		return "", fmt.Errorf("%s prompt has an unfilled placeholder near %q", in.Job, out[i:min(i+30, len(out))])
	}
	return out, nil
}

// WatchlistTable renders enabled instruments as a markdown table.
func WatchlistTable(instruments []model.Instrument) string {
	var b strings.Builder
	b.WriteString("| symbol | name | kind | notes |\n|---|---|---|---|\n")
	for _, in := range instruments {
		if !in.Enabled {
			continue
		}
		notes := in.Notes
		if in.CompanyTag != "" {
			notes = strings.TrimSpace(notes + " (company: " + in.CompanyTag + ")")
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s |\n", cell(in.Symbol), cell(in.Name), in.Kind, cell(notes))
	}
	return strings.TrimRight(b.String(), "\n")
}

func cell(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "|", "/"), "\n", " ")
}

// Schema returns prompts/schema.json.
func Schema() ([]byte, error) {
	return fs.ReadFile(swingdesk.Prompts, "prompts/schema.json")
}
