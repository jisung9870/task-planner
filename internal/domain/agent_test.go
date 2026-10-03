package domain

import (
	"strings"
	"testing"
	"time"
)

func TestParseAgentAndTier(t *testing.T) {
	for _, ok := range []string{"", "claude", "Codex", "auto", "gemini-2"} {
		if _, err := ParseAgent(ok); err != nil {
			t.Errorf("ParseAgent(%q) = %v", ok, err)
		}
	}
	for _, bad := range []string{"2x", "claude code", "a:b", "-x"} {
		if _, err := ParseAgent(bad); err == nil {
			t.Errorf("ParseAgent(%q) accepted", bad)
		}
	}
	if _, err := ParseTier("heavy"); err == nil {
		t.Error("ParseTier(heavy) accepted")
	}
	if tr, _ := ParseTier(" Deep "); tr != TierDeep {
		t.Errorf("ParseTier = %q", tr)
	}
}

func TestClaimStartsAndRefusesSecondClaimant(t *testing.T) {
	at := time.Date(2026, 10, 3, 10, 0, 0, 0, time.Local)
	task := &Task{ID: "T-1", Title: "x", Status: StatusTodo, Agent: AgentAuto}
	if err := task.Claim("claude", "claude:s1", at, nil); err != nil {
		t.Fatal(err)
	}
	if task.Status != StatusDoing || task.ClaimedBy != "claude:s1" || task.Executor != ExecutorAgent {
		t.Fatalf("after claim: %s %q %s", task.Status, task.ClaimedBy, task.Executor)
	}
	if err := task.Claim("claude", "claude:s1", at, nil); err != nil {
		t.Fatalf("same claimant retry: %v", err)
	}
	if err := task.Claim("codex", "codex:s2", at, nil); err == nil || !strings.Contains(err.Error(), "이미") {
		t.Fatalf("second claimant: %v", err)
	}
	if err := task.Transition(StatusTodo, at, nil); err != nil {
		t.Fatal(err)
	}
	if task.ClaimedBy != "" {
		t.Fatalf("reopen kept claim %q", task.ClaimedBy)
	}
}

func TestClaimRespectsNamedAgentAndHold(t *testing.T) {
	at := time.Date(2026, 10, 3, 10, 0, 0, 0, time.Local)
	named := &Task{ID: "T-1", Title: "x", Status: StatusTodo, Agent: "codex"}
	if err := named.Claim("claude", "claude:s", at, nil); err == nil {
		t.Fatal("claude took a codex task")
	}
	held := &Task{ID: "T-2", Title: "y", Status: StatusBlocked, BlockedReason: "대기"}
	if err := held.Claim("claude", "claude:s", at, nil); err == nil {
		t.Fatal("claimed a blocked task")
	}
	done := &Task{ID: "T-3", Title: "z", Status: StatusDone, ClaimedBy: "codex:s"}
	if err := done.Claim("codex", "codex:s", at, nil); err == nil {
		t.Fatal("claimed a finished task")
	}
	if err := done.Transition(StatusCancelled, at, nil); err != nil || done.ClaimedBy != "codex:s" {
		t.Fatalf("terminal transition dropped claim: %v %q", err, done.ClaimedBy)
	}
}

func TestPickableAndRunAgent(t *testing.T) {
	cases := []struct {
		task Task
		want bool
	}{
		{Task{Status: StatusTodo, Agent: "claude"}, true},
		{Task{Status: StatusTodo, Agent: AgentAuto}, true},
		{Task{Status: StatusTodo}, false},
		{Task{Status: StatusTodo, Agent: "codex"}, false},
		{Task{Status: StatusTodo, Agent: "claude", ClaimedBy: "claude:x"}, false},
		{Task{Status: StatusBlocked, Agent: "claude"}, false},
		{Task{Status: StatusDone, Agent: "claude"}, false},
	}
	for i, c := range cases {
		if got := c.task.Pickable("claude"); got != c.want {
			t.Errorf("case %d: Pickable = %v", i, got)
		}
	}
	auto := Task{Agent: AgentAuto, ClaimedBy: "codex:s1"}
	if auto.RunAgent() != "codex" {
		t.Errorf("RunAgent = %q", auto.RunAgent())
	}
}
