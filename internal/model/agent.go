package model

import (
	"slices"
	"strings"
)

// Job is one Claude Code research job per refresh.
type Job string

const (
	JobMarket    Job = "market"
	JobTech      Job = "tech"
	JobNames     Job = "names"
	JobNamesRest Job = "names_rest" // second names batch when split_names is on
)

var jobs = [...]Job{JobMarket, JobTech, JobNames, JobNamesRest}

// Valid reports whether j is in the closed set.
func (j Job) Valid() bool { return slices.Contains(jobs[:], j) }

// Jobs returns every known job (including optional ones) in run order.
func Jobs() []Job { return slices.Clone(jobs[:]) }

// DefaultJobs is the v1 set run on every refresh: three jobs, not one per ticker.
func DefaultJobs() []Job { return []Job{JobMarket, JobTech, JobNames} }

// IsNames reports whether j is a per-instrument names batch (symbol required).
func (j Job) IsNames() bool { return j == JobNames || j == JobNamesRest }

// Template is the prompts/<name>.md file the job renders.
func (j Job) Template() string {
	if j.IsNames() {
		return string(JobNames)
	}
	return string(j)
}

// AgentState is the coarse state of the agent runner.
type AgentState string

const (
	AgentIdle    AgentState = "idle"
	AgentRunning AgentState = "running"
	AgentError   AgentState = "error"
)

var agentStates = [...]AgentState{AgentIdle, AgentRunning, AgentError}

// Valid reports whether s is in the closed set.
func (s AgentState) Valid() bool { return slices.Contains(agentStates[:], s) }

// AgentStates returns every agent state.
func AgentStates() []AgentState { return slices.Clone(agentStates[:]) }

// AgentStatus is what the header shows for the agent runner. The zero value
// is idle.
type AgentStatus struct {
	State   AgentState
	Running []Job  // jobs in flight when State is AgentRunning
	Err     string // human-readable cause when State is AgentError
}

// String renders e.g. "idle", "running market,tech", "error: tmux not found".
func (s AgentStatus) String() string {
	switch s.State {
	case AgentRunning:
		names := make([]string, len(s.Running))
		for i, j := range s.Running {
			names[i] = string(j)
		}
		if len(names) == 0 {
			return string(AgentRunning)
		}
		return string(AgentRunning) + " " + strings.Join(names, ",")
	case AgentError:
		if s.Err == "" {
			return string(AgentError)
		}
		return string(AgentError) + ": " + s.Err
	default:
		return string(AgentIdle)
	}
}

// RunStatus is refresh_runs.status.
type RunStatus string

const (
	RunRunning RunStatus = "running"
	RunOK      RunStatus = "ok"
	RunPartial RunStatus = "partial"
	RunError   RunStatus = "error"
)

var runStatuses = [...]RunStatus{RunRunning, RunOK, RunPartial, RunError}

// Valid reports whether s is in the closed set.
func (s RunStatus) Valid() bool { return slices.Contains(runStatuses[:], s) }

// RunStatusFor derives the final status from job counts out of total jobs.
func RunStatusFor(ok, total int) RunStatus {
	switch {
	case total > 0 && ok >= total:
		return RunOK
	case ok > 0:
		return RunPartial
	default:
		return RunError
	}
}
