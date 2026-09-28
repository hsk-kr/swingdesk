package model

import "testing"

func TestKindValid(t *testing.T) {
	for _, k := range kinds {
		if !k.Valid() {
			t.Errorf("%q should be valid", k)
		}
	}
	for _, k := range []Kind{"", "stock", "EQUITY"} {
		if k.Valid() {
			t.Errorf("%q should be invalid", k)
		}
	}
}

func TestCategoryAndStanceValid(t *testing.T) {
	for _, c := range categories {
		if !c.Valid() {
			t.Errorf("%q should be valid", c)
		}
	}
	if Category("sports").Valid() || Category("").Valid() {
		t.Error("unexpected valid category")
	}
	for _, s := range stances {
		if !s.Valid() {
			t.Errorf("%q should be valid", s)
		}
	}
	if Stance("flat").Valid() {
		t.Error("unexpected valid stance")
	}
}

func TestAgentStatusString(t *testing.T) {
	cases := map[string]AgentStatus{
		"idle":                  {},
		"running":               {State: AgentRunning},
		"running market,tech":   {State: AgentRunning, Running: []Job{JobMarket, JobTech}},
		"error":                 {State: AgentError},
		"error: tmux not found": {State: AgentError, Err: "tmux not found"},
	}
	for want, s := range cases {
		if got := s.String(); got != want {
			t.Errorf("%+v = %q, want %q", s, got, want)
		}
	}
}

func TestClosedSetAccessorsReturnCopies(t *testing.T) {
	c := Categories()
	c[0] = "mutated"
	if Categories()[0] == "mutated" {
		t.Error("Categories must return a copy")
	}
	for _, j := range Jobs() {
		if !j.Valid() {
			t.Errorf("job %q invalid", j)
		}
	}
	for _, s := range AgentStates() {
		if !s.Valid() {
			t.Errorf("agent state %q invalid", s)
		}
	}
	if Job("macro").Valid() || AgentState("busy").Valid() {
		t.Error("unexpected valid value")
	}
}
