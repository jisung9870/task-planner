package service

import (
	"strings"
	"testing"
	"time"

	"task-planner/internal/domain"
)

func TestNeedsBackfillOnlyForBarelyTimedHumanWork(t *testing.T) {
	svc := newTestService(t)
	human, _ := svc.Add(AddInput{Title: "사람 작업"})
	agent, _ := svc.Add(AddInput{Title: "agent 작업", Executor: domain.ExecutorAgent})
	if !svc.NeedsBackfill(human) {
		t.Error("todo human task with nothing on the clock should ask")
	}
	if svc.NeedsBackfill(agent) {
		t.Error("agent work must not ask")
	}
	timed, _ := svc.Add(AddInput{Title: "오래 한 작업"})
	timed.Actual = domain.Duration(time.Hour)
	if svc.NeedsBackfill(timed) {
		t.Error("an hour on the clock should not ask")
	}
	svc.Cfg.BackfillUnder = 0
	if svc.NeedsBackfill(human) {
		t.Error("backfill_under: 0 must disable the question")
	}
}

func TestDoneSinceRecordsStartInLog(t *testing.T) {
	svc := newTestService(t) // now = 2026-09-12 09:14
	task, _ := svc.Add(AddInput{Title: "보고서"})
	since, err := svc.ParseSince("어제 15:00")
	if err != nil {
		t.Fatal(err)
	}
	res, err := svc.DoneSince(task.ID, &since)
	if err != nil {
		t.Fatal(err)
	}
	if res.Task.Status != domain.StatusDone || res.Task.Actual != svc.Cfg.SessionCap {
		t.Fatalf("status=%s actual=%s (18h14m capped to session_cap)", res.Task.Status, res.Task.Actual)
	}
	full, _ := svc.Load(task.ID)
	logs := full.LogLines()
	if !strings.Contains(logs[len(logs)-1], "(착수 소급 2026-09-11 15:00)") {
		t.Errorf("log = %q", logs[len(logs)-1])
	}
	start, end, ok := full.WorkHistory(time.Local).Lead()
	if !ok || end.Sub(start) != 18*time.Hour+14*time.Minute {
		t.Errorf("lead = %s ~ %s", start, end)
	}
	if res, err := svc.DoneSince(task.ID, nil); err != nil || res.Task.Status != domain.StatusDone {
		t.Errorf("plain done on a done task: %v", err)
	}
}
