package service

import (
	"strings"
	"testing"

	"task-planner/internal/domain"
)

func TestShiftScheduledMovesTheDay(t *testing.T) {
	svc := newTestService(t) // today = 2026-09-12
	task, _ := svc.Add(AddInput{Title: "API 설계", Scheduled: domain.NewDate(2026, 9, 15)})
	res, err := svc.ShiftScheduled(task.ID, 2)
	if err != nil {
		t.Fatal(err)
	}
	if res.Task.Scheduled.String() != "2026-09-17" {
		t.Fatalf("scheduled = %s", res.Task.Scheduled)
	}
}

func TestShiftScheduledPullsUndatedTaskToToday(t *testing.T) {
	svc := newTestService(t)
	task, _ := svc.Add(AddInput{Title: "백로그"})
	res, err := svc.ShiftScheduled(task.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if res.Task.Scheduled.String() != "2026-09-12" {
		t.Fatalf("scheduled = %s", res.Task.Scheduled)
	}
}

func TestEditWithSpanAppliesEveryField(t *testing.T) {
	svc := newTestService(t)
	task, _ := svc.Add(AddInput{Title: "API 설계"})
	sp, _ := svc.ParseSpan("09-15~09-19")
	p1 := domain.P1
	res, err := svc.EditWithSpan(task.ID, EditInput{Priority: &p1}, &sp)
	if err != nil {
		t.Fatal(err)
	}
	if res.Task.Priority != domain.P1 || res.Task.Scheduled.String() != "2026-09-15" || res.Task.Due.String() != "2026-09-19" {
		t.Fatalf("got %s %s~%s", res.Task.Priority, res.Task.Scheduled, res.Task.Due)
	}
	edits := 0
	for _, line := range res.Task.LogLines() {
		if strings.Contains(line, "edit:") {
			edits++
		}
	}
	if edits != 1 {
		t.Fatalf("edit log lines = %d, want 1:\n%s", edits, res.Task.Body)
	}
}

// A field that fails leaves the span unwritten too: nothing is half applied.
func TestEditWithSpanWritesNothingWhenAFieldFails(t *testing.T) {
	svc := newTestService(t)
	task, _ := svc.Add(AddInput{Title: "API 설계"})
	sp, _ := svc.ParseSpan("09-15~09-19")
	bad := domain.Agent("gemini")
	if _, err := svc.EditWithSpan(task.ID, EditInput{Agent: &bad}, &sp); err == nil {
		t.Fatal("accepted a disallowed agent")
	}
	got, _ := svc.Load(task.ID)
	if !got.Scheduled.IsZero() || !got.Due.IsZero() {
		t.Fatalf("span written despite the failure: %s~%s", got.Scheduled, got.Due)
	}
}

func TestEditWithSpanRefusesScheduledOrDue(t *testing.T) {
	svc := newTestService(t)
	task, _ := svc.Add(AddInput{Title: "API 설계"})
	sp, _ := svc.ParseSpan("09-15~09-19")
	d := svc.Today()
	if _, err := svc.EditWithSpan(task.ID, EditInput{Due: &d}, &sp); err == nil || !strings.Contains(err.Error(), "함께 지정할 수 없음") {
		t.Fatalf("err = %v", err)
	}
}

func mustSpan(t *testing.T, svc *Service, expr string) domain.Span {
	t.Helper()
	sp, err := svc.ParseSpan(expr)
	if err != nil {
		t.Fatal(err)
	}
	return sp
}

func ptrSpan(sp domain.Span) *domain.Span { return &sp }
