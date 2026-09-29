package ui

import (
	"errors"
	"strings"

	"github.com/hsk-kr/swingdesk/internal/agent"
	"github.com/hsk-kr/swingdesk/internal/model"
	"github.com/hsk-kr/swingdesk/internal/refresh"
)

// emptyInbox explains why the inbox has no rows.
func (m Model) emptyInbox() []string {
	switch {
	case !m.loaded:
		return []string{styleMuted.Render(" loading…")}
	case m.loadErr != "":
		return []string{styleError.Render(" failed to load: " + m.loadErr), styleMuted.Render(" change filter to retry")}
	case m.query != "":
		return []string{styleMuted.Render(" no matches for /" + m.query + " · esc clears")}
	case m.authProblem:
		return []string{
			styleError.Render(" claude is not logged in"),
			styleMuted.Render(" run `claude` in a terminal, use /login, then press R"),
		}
	case m.counts.Total == 0 && m.status.LastRefresh.IsZero() && m.refresh != nil:
		return m.waitingLines()
	case m.showRead:
		return []string{styleMuted.Render(" no items")}
	}
	return []string{styleMuted.Render(" no unread items")}
}

func (m Model) waitingLines() []string {
	lines := []string{styleMuted.Render(" waiting on first refresh…")}
	if m.sched.Running() {
		lines = append(lines, styleMuted.Render(" agents are researching · watch: tmux attach -t "+m.sessionName()))
	}
	return lines
}

// authHints are phrases Claude Code itself prints when it cannot
// authenticate. They are matched only against claude's own output (exit
// errors), never against ingest errors that quote the model's prose.
var authHints = []string{"not logged in", "please run /login", "invalid api key", "authentication_error", "oauth token has expired"}

// isAuthProblem reports whether a refresh failed because claude is not
// logged in: at least one job failed and every failed job is a claude exit
// whose own message is an auth failure.
func isAuthProblem(out refresh.Outcome) bool {
	failed := 0
	for _, j := range out.Jobs {
		if j.Err == nil {
			continue
		}
		failed++
		var exitErr *agent.ExitError
		if !errors.As(j.Err, &exitErr) || !hasAuthHint(exitErr.Stderr) {
			return false
		}
	}
	return failed > 0
}

func hasAuthHint(s string) bool {
	s = strings.ToLower(s)
	for _, h := range authHints {
		if strings.Contains(s, h) {
			return true
		}
	}
	return false
}

// agentStatusFor maps a finished refresh onto the header's agent state.
func agentStatusFor(out refresh.Outcome) model.AgentStatus {
	switch {
	case isAuthProblem(out):
		return model.AgentStatus{State: model.AgentError, Err: "claude not logged in"}
	case out.Err != nil:
		return model.AgentStatus{State: model.AgentError, Err: out.Err.Error()}
	case out.Status == model.RunError:
		return model.AgentStatus{State: model.AgentError, Err: firstJobError(out)}
	}
	return model.AgentStatus{State: model.AgentIdle}
}

// firstJobError names the first failed job for the header.
func firstJobError(out refresh.Outcome) string {
	for _, j := range out.Jobs {
		if j.Err != nil {
			return string(j.Job) + ": " + j.Err.Error()
		}
	}
	return "all jobs failed"
}
