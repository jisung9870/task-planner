package service

import (
	"strings"
	"testing"

	"task-planner/internal/domain"
)

func TestSetSpanWritesBothDates(t *testing.T) {
	svc := newTestService(t)
	task, _ := svc.Add(AddInput{Title: "API 설계"})
	sp, err := svc.ParseSpan("09-15~09-19")
	if err != nil {
		t.Fatal(err)
	}
	res, err := svc.SetSpan(task.ID, sp)
	if err != nil {
		t.Fatal(err)
	}
	if res.Task.Scheduled.String() != "2026-09-15" || res.Task.Due.String() != "2026-09-19" {
		t.Fatalf("%s~%s", res.Task.Scheduled, res.Task.Due)
	}
	if res.Task.SpanDays() != 5 {
		t.Fatalf("days = %d", res.Task.SpanDays())
	}
}

func TestSetSpanRejectsEndBeforeExistingStart(t *testing.T) {
	svc := newTestService(t)
	task, _ := svc.Add(AddInput{Title: "API 설계"})
	if _, err := svc.SetSpan(task.ID, mustSpan(t, svc, "09-15~09-19")); err != nil {
		t.Fatal(err)
	}
	// Only the end is given, and it lands before the start already on file.
	if _, err := svc.SetSpan(task.ID, mustSpan(t, svc, "~09-10")); err == nil {
		t.Fatal("거꾸로 된 기간이 저장됨")
	}
}

func TestShiftSpanKeepsLength(t *testing.T) {
	svc := newTestService(t)
	task, _ := svc.Add(AddInput{Title: "API 설계"})
	svc.SetSpan(task.ID, mustSpan(t, svc, "09-15~09-19"))

	res, err := svc.ShiftSpan(task.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if res.Task.Scheduled.String() != "2026-09-16" || res.Task.Due.String() != "2026-09-20" {
		t.Fatalf("%s~%s", res.Task.Scheduled, res.Task.Due)
	}
	if res.Task.SpanDays() != 5 {
		t.Fatalf("길이가 바뀜: %d일", res.Task.SpanDays())
	}
}

// The first press on an undated task pulls it onto today rather than jumping
// to an arbitrary date.
func TestShiftSpanSchedulesUndatedTaskToday(t *testing.T) {
	svc := newTestService(t)
	task, _ := svc.Add(AddInput{Title: "언젠가 할 일"})
	res, err := svc.ShiftSpan(task.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Task.Scheduled.Equal(svc.Today()) || !res.Task.Due.IsZero() {
		t.Fatalf("%s~%s", res.Task.Scheduled, res.Task.Due)
	}
}

func TestResizeSpanMovesOnlyTheEnd(t *testing.T) {
	svc := newTestService(t)
	task, _ := svc.Add(AddInput{Title: "API 설계"})
	svc.SetSpan(task.ID, mustSpan(t, svc, "09-15~09-19"))

	res, err := svc.ResizeSpan(task.ID, 2)
	if err != nil {
		t.Fatal(err)
	}
	if res.Task.Scheduled.String() != "2026-09-15" || res.Task.Due.String() != "2026-09-21" {
		t.Fatalf("%s~%s", res.Task.Scheduled, res.Task.Due)
	}
	// Shrinking past the start clamps to a one-day period instead of inverting.
	res, err = svc.ResizeSpan(task.ID, -30)
	if err != nil {
		t.Fatal(err)
	}
	if res.Task.Due.String() != "2026-09-15" || res.Task.SpanDays() != 1 {
		t.Fatalf("%s~%s (%d일)", res.Task.Scheduled, res.Task.Due, res.Task.SpanDays())
	}
}

func TestSetSpanClearsDates(t *testing.T) {
	svc := newTestService(t)
	task, _ := svc.Add(AddInput{Title: "API 설계"})
	svc.SetSpan(task.ID, mustSpan(t, svc, "09-15~09-19"))
	res, err := svc.SetSpan(task.ID, mustSpan(t, svc, "-"))
	if err != nil {
		t.Fatal(err)
	}
	if !res.Task.Scheduled.IsZero() || !res.Task.Due.IsZero() {
		t.Fatalf("%s~%s", res.Task.Scheduled, res.Task.Due)
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

// A span sent with other fields is one edit: every field lands, in one save
// and one log line.
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
