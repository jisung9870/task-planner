package service

import (
	"strings"
	"sync"
	"testing"
	"time"

	"task-planner/internal/config"
	"task-planner/internal/domain"
)

func TestAddWithAgentForcesAgentExecutor(t *testing.T) {
	svc := newTestService(t)
	task, err := svc.Add(AddInput{Title: "로그 정리", Agent: "codex", Tier: domain.TierFast})
	if err != nil {
		t.Fatal(err)
	}
	if task.Executor != domain.ExecutorAgent || svc.ModelFor(task) != "gpt-6-luna" {
		t.Fatalf("executor=%s model=%q", task.Executor, svc.ModelFor(task))
	}
	if _, err := svc.Add(AddInput{Title: "x", Agent: "gemini"}); err == nil {
		t.Fatal("accepted an agent outside agents.allowed")
	}
	if _, err := svc.Add(AddInput{Title: "y", Agent: "claude", Executor: domain.ExecutorHuman}); err == nil {
		t.Fatal("accepted human executor with an agent")
	}
}

func TestEditAgentSwitchesExecutor(t *testing.T) {
	svc := newTestService(t)
	task, _ := svc.Add(AddInput{Title: "사람 일"})
	a, tier := domain.Agent("claude"), domain.TierDeep
	res, err := svc.Edit(task.ID, EditInput{Agent: &a, Tier: &tier})
	if err != nil {
		t.Fatal(err)
	}
	if res.Task.Executor != domain.ExecutorAgent || res.Task.Agent != "claude" || res.Task.Tier != tier {
		t.Fatalf("got %s %s %s", res.Task.Executor, res.Task.Agent, res.Task.Tier)
	}
	human := domain.ExecutorHuman
	if _, err := svc.Edit(task.ID, EditInput{Executor: &human}); err == nil {
		t.Fatal("made an agent-named task human")
	}
	none := domain.Agent("")
	if res, err := svc.Edit(task.ID, EditInput{Executor: &human, Agent: &none}); err != nil || res.Task.Executor != human {
		t.Fatalf("clearing agent with human: %v", err)
	}
	bad := domain.Agent("gemini")
	if _, err := svc.Edit(task.ID, EditInput{Agent: &bad}); err == nil {
		t.Fatal("accepted disallowed agent on edit")
	}
}

func TestClaimPicksMostUrgentPickable(t *testing.T) {
	svc := newTestService(t)
	svc.Add(AddInput{Title: "사람 일", Priority: domain.P0})
	svc.Add(AddInput{Title: "codex 일", Agent: "codex", Priority: domain.P0})
	low, _ := svc.Add(AddInput{Title: "아무나 낮음", Agent: domain.AgentAuto, Priority: domain.P3})
	high, _ := svc.Add(AddInput{Title: "claude 높음", Agent: "claude", Priority: domain.P1})

	res, err := svc.Claim(ClaimInput{Agent: "claude", Session: "s1"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Task.ID != high.ID || res.Task.Status != domain.StatusDoing || res.Task.ClaimedBy != "claude:s1" {
		t.Fatalf("picked %s %s %q", res.Task.ID, res.Task.Status, res.Task.ClaimedBy)
	}
	res, err = svc.Claim(ClaimInput{Agent: "claude", Session: "s2"})
	if err != nil || res.Task.ID != low.ID {
		t.Fatalf("second pick: %v %v", res, err)
	}
	if _, err := svc.Claim(ClaimInput{Agent: "claude", Session: "s3"}); err == nil {
		t.Fatal("picked when nothing was left")
	}
	if got := svc.ModelFor(res.Task); got != "" {
		t.Fatalf("model without tier = %q", got)
	}
}

func TestReleaseOnlyByClaimantUnlessForced(t *testing.T) {
	svc := newTestService(t)
	task, _ := svc.Add(AddInput{Title: "일", Agent: "claude"})
	if _, err := svc.Claim(ClaimInput{Ref: task.ID, Agent: "claude", Session: "s1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Release(task.ID, "claude:other", false); err == nil {
		t.Fatal("released someone else's claim")
	}
	res, err := svc.Release(task.ID, "", true)
	if err != nil {
		t.Fatal(err)
	}
	if res.Task.ClaimedBy != "" || res.Task.Status != domain.StatusTodo {
		t.Fatalf("after release: %q %s", res.Task.ClaimedBy, res.Task.Status)
	}
	if !strings.Contains(res.Task.Body, "release: claude:s1") {
		t.Fatalf("release not logged:\n%s", res.Task.Body)
	}
}

func TestRecurCarriesAgentNotClaim(t *testing.T) {
	svc := newTestService(t)
	task, _ := svc.Add(AddInput{Title: "주간 점검", Agent: "codex", Tier: domain.TierStandard, Recur: "weekly", Scheduled: svc.Today()})
	if _, err := svc.Claim(ClaimInput{Ref: task.ID, Agent: "codex", Session: "s"}); err != nil {
		t.Fatal(err)
	}
	res, err := svc.Done(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if res.Next == nil || res.Next.Agent != "codex" || res.Next.Tier != domain.TierStandard || res.Next.ClaimedBy != "" {
		t.Fatalf("next = %+v", res.Next)
	}
}

// Two services on one vault stand in for two agent sessions. Exactly one may
// win each task; the rest must see the claim and fail.
func TestConcurrentClaimsHaveOneWinner(t *testing.T) {
	svc := newTestService(t)
	task, _ := svc.Add(AddInput{Title: "경합", Agent: domain.AgentAuto})
	svc.Close()

	const n = 8
	var wg sync.WaitGroup
	var mu sync.Mutex
	wins := 0
	for i := 0; i < n; i++ {
		other, err := Open(mustLoad(t, svc.Cfg.Vault))
		if err != nil {
			t.Fatal(err)
		}
		other.SetClock(func() time.Time { return time.Date(2026, 9, 12, 10, 0, 0, 0, time.Local) })
		agent := domain.Agent("claude")
		if i%2 == 1 {
			agent = "codex"
		}
		wg.Add(1)
		go func(s *Service, i int) {
			defer wg.Done()
			if _, err := s.Claim(ClaimInput{Ref: task.ID, Agent: agent, Session: "s" + string(rune('a'+i))}); err == nil {
				mu.Lock()
				wins++
				mu.Unlock()
			}
		}(other, i)
	}
	wg.Wait()
	if wins != 1 {
		t.Fatalf("wins = %d, want 1", wins)
	}
}

func mustLoad(t *testing.T, vault string) *config.Config {
	t.Helper()
	cfg, err := config.Load(vault)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}
