package ui

import (
	"fmt"
	"slices"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/hsk-kr/swingdesk/internal/model"
	"github.com/hsk-kr/swingdesk/internal/refresh"
)

// RefreshFunc performs one blocking refresh, calling progress (from any
// goroutine) as each agent job finishes. main binds it to the app context so
// quitting cancels it. Per-job failures arrive in the final Outcome.
type RefreshFunc func(progress func(model.Job)) refresh.Outcome

// flashFor is how long the "+N new" header notice stays up.
const flashFor = 10 * time.Second

// tickMsg drives the schedule and the countdown.
type tickMsg time.Time

// jobDoneMsg reports one finished agent job; ch streams the rest.
type jobDoneMsg struct {
	job model.Job
	ch  <-chan tea.Msg
}

// refreshDoneMsg carries a finished refresh.
type refreshDoneMsg struct{ out refresh.Outcome }

func defaultTick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// runRefresh starts the refresh in a goroutine and streams jobDoneMsgs then
// a refreshDoneMsg. The channel is buffered for every message so the
// goroutine never blocks on a slow UI.
func runRefresh(fn RefreshFunc) tea.Cmd {
	return func() tea.Msg {
		ch := make(chan tea.Msg, len(model.Jobs())*2+1)
		go func() {
			defer close(ch)
			out := fn(func(j model.Job) { ch <- jobDoneMsg{job: j, ch: ch} })
			ch <- refreshDoneMsg{out: out}
		}()
		return <-ch
	}
}

// next waits for the following message on ch.
func next(ch <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			return nil
		}
		return msg
	}
}

// onJobDone drops a finished job from the header's running list.
func (m Model) onJobDone(msg jobDoneMsg) (tea.Model, tea.Cmd) {
	if m.status.Agents.State == model.AgentRunning {
		running := slices.DeleteFunc(slices.Clone(m.status.Agents.Running), func(j model.Job) bool { return j == msg.job })
		m.status.Agents = model.AgentStatus{State: model.AgentRunning, Running: running}
	}
	return m, next(msg.ch)
}

// onTick consults the schedule and re-arms the ticker.
func (m Model) onTick(now time.Time) (tea.Model, tea.Cmd) {
	m.clock = now
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

// onRefreshDone records the outcome, flashes "+N new" and reloads the inbox.
func (m Model) onRefreshDone(out refresh.Outcome) (tea.Model, tea.Cmd) {
	m.sched = m.sched.Done()
	m.clock = m.now()
	if n := out.NewItems(); n > 0 {
		m.flashN, m.flashUntil = n, m.clock.Add(flashFor)
	}
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
