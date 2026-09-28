package ui

import (
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/hsk-kr/swingdesk/internal/model"
	"github.com/hsk-kr/swingdesk/internal/refresh"
)

// RefreshFunc performs one blocking refresh. main binds it to the app
// context so quitting cancels it.
type RefreshFunc func() refresh.Outcome

// tickMsg drives the schedule and (later) the countdown.
type tickMsg time.Time

// refreshDoneMsg carries a finished refresh.
type refreshDoneMsg struct{ out refresh.Outcome }

func defaultTick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func runRefresh(fn RefreshFunc) tea.Cmd {
	return func() tea.Msg { return refreshDoneMsg{out: fn()} }
}

// onTick consults the schedule and re-arms the ticker.
func (m Model) onTick(now time.Time) (tea.Model, tea.Cmd) {
	s, d := m.sched.Tick(now)
	m.sched = s
	m, cmd := m.decide(d)
	return m, tea.Batch(cmd, m.tick())
}

// forceRefresh handles R: refresh now and reset the timer.
func (m Model) forceRefresh() (tea.Model, tea.Cmd) {
	if m.refresh == nil {
		m.notice = "refresh is not configured"
		return m, nil
	}
	s, d := m.sched.Force(m.now())
	m.sched = s
	return m.decide(d)
}

func (m Model) decide(d refresh.Decision) (Model, tea.Cmd) {
	m.status.NextRefresh = m.sched.Next()
	switch d {
	case refresh.Start:
		m.status.Agents = model.AgentStatus{State: model.AgentRunning, Running: model.Jobs()}
		return m, runRefresh(m.refresh)
	case refresh.Skipped:
		m.notice = "refresh skipped: previous run still running"
	}
	return m, nil
}

// onRefreshDone records the outcome and reloads the inbox.
func (m Model) onRefreshDone(out refresh.Outcome) (tea.Model, tea.Cmd) {
	m.sched = m.sched.Done()
	m.status.LastRefresh = out.Finished
	m.status.NextRefresh = m.sched.Next()
	m.status.Agents = agentStatusFor(out)
	m.notice, m.noticeErr = refreshNotice(out)
	if m.inFlight > 0 {
		return m, nil // applyMarked reloads once marks settle
	}
	return m.reload()
}

func agentStatusFor(out refresh.Outcome) model.AgentStatus {
	if out.Err != nil {
		return model.AgentStatus{State: model.AgentError, Err: out.Err.Error()}
	}
	return model.AgentStatus{State: model.AgentIdle}
}

func refreshNotice(out refresh.Outcome) (string, bool) {
	n := out.NewItems()
	switch {
	case out.Err != nil:
		return "refresh failed: " + out.Err.Error(), true
	case out.Status == model.RunPartial:
		for _, j := range out.Jobs {
			if j.Err != nil {
				return fmt.Sprintf("refresh partial (+%d new) · %s: %v", n, j.Job, j.Err), true
			}
		}
	}
	return fmt.Sprintf("refresh %s · +%d new", out.Status, n), false
}
