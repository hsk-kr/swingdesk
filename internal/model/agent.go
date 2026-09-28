package model

import (
	"slices"
	"strings"
)

// Job is one Claude Code research job per refresh.
type Job string

const (
	JobMarket Job = "market"
	JobTech   Job = "tech"
	JobNames  Job = "names"
)

var jobs = [...]Job{JobMarket, JobTech, JobNames}

// Valid reports whether j is in the closed set.
func (j Job) Valid() bool { return slices.Contains(jobs[:], j) }

// Jobs returns every job in run order.
func Jobs() []Job { return slices.Clone(jobs[:]) }

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
