package service

import (
	"testing"
	"time"

	"task-planner/internal/domain"
)

func TestRolloverMovesPastDueScheduledWork(t *testing.T) {
	svc := newTestService(t)
	past, _ := domain.ParseDate("2026-09-09")

	stale, _ := svc.Add(AddInput{Title: "어제 못 끝낸 것", Scheduled: past})
	future, _ := svc.Add(AddInput{Title: "다음 주", Scheduled: domain.NewDate(2026, time.September, 20)})
	backlog, _ := svc.Add(AddInput{Title: "날짜 없음"})

	rep, err := svc.Rollover()
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Rolled) != 1 || rep.Rolled[0].Task.ID != stale.ID {
		t.Fatalf("rolled = %+v", rep.Rolled)
	}
	got, _ := svc.Load(stale.ID)
	if got.Scheduled.String() != "2026-09-12" || got.RolloverCount != 1 {
		t.Fatalf("scheduled=%s count=%d", got.Scheduled, got.RolloverCount)
	}
	// Untouched tasks must not gain a carry count.
	for _, id := range []string{future.ID, backlog.ID} {
		u, _ := svc.Load(id)
		if u.RolloverCount != 0 {
			t.Fatalf("%s 가 이월됨", id)
		}
	}
}

// A hold is waiting on someone else; counting it as a carry would blame the
// owner for a delay they do not control.
func TestRolloverSkipsBlockedAndTerminal(t *testing.T) {
	svc := newTestService(t)
	past, _ := domain.ParseDate("2026-09-01")

	blocked, _ := svc.Add(AddInput{Title: "보류 항목", Scheduled: past})
	svc.Block(blocked.ID, "외부 회신 대기", nil)
	done, _ := svc.Add(AddInput{Title: "완료 항목", Scheduled: past})
	svc.Done(done.ID)

	rep, err := svc.Rollover()
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Rolled) != 0 {
		t.Fatalf("rolled = %+v", rep.Rolled)
	}
}

func TestRolloverFlagsRepeatedlyCarriedTasks(t *testing.T) {
	svc := newTestService(t)
	svc.Cfg.RolloverWarnAt = 3
	task, _ := svc.Add(AddInput{Title: "계속 미뤄지는 것", Scheduled: domain.NewDate(2026, time.September, 1)})

	var rep RolloverReport
	for i := 0; i < 3; i++ {
		// Each round pretends a new day arrived without the task being done.
		edited, _ := svc.Load(task.ID)
		edited.Scheduled = domain.NewDate(2026, time.September, 11)
		if err := svc.SaveTask(edited); err != nil {
			t.Fatal(err)
		}
		var err error
		if rep, err = svc.Rollover(); err != nil {
			t.Fatal(err)
		}
	}
	got, _ := svc.Load(task.ID)
	if got.RolloverCount != 3 {
		t.Fatalf("count = %d", got.RolloverCount)
	}
	if len(rep.Stale) != 1 {
		t.Fatalf("stale = %+v", rep.Stale)
	}
	if rep.StaleWarning() == "" {
		t.Fatal("경고 문구가 비어 있음")
	}
}

func TestRolloverIfEnabledRespectsConfig(t *testing.T) {
	svc := newTestService(t)
	svc.Cfg.AutoRollover = false
	svc.Add(AddInput{Title: "x", Scheduled: domain.NewDate(2026, time.September, 1)})
	rep, err := svc.RolloverIfEnabled()
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Empty() {
		t.Fatal("auto_rollover=false 인데 이월됨")
	}
}

// A multi-day task is not late just because it started yesterday. Carrying it
// would move its start and inflate the carry counter that flags real slippage.
func TestRolloverSkipsTaskStillInsideItsSpan(t *testing.T) {
	svc := newTestService(t)
	task, _ := svc.Add(AddInput{Title: "리팩터링 스프린트"})
	if _, err := svc.EditWithSpan(task.ID, EditInput{}, ptrSpan(mustSpan(t, svc, "09-09~09-20"))); err != nil {
		t.Fatal(err)
	}
	rep, err := svc.Rollover()
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Empty() {
		t.Fatalf("기간 중인 태스크가 이월됨: %d건", len(rep.Rolled))
	}
	after, _ := svc.Load(task.ID)
	if after.Scheduled.String() != "2026-09-09" || after.RolloverCount != 0 {
		t.Fatalf("scheduled=%s count=%d", after.Scheduled, after.RolloverCount)
	}
}

// A period that already ended still carries: that is exactly a missed plan.
func TestRolloverCarriesTaskWhoseSpanEnded(t *testing.T) {
	svc := newTestService(t)
	task, _ := svc.Add(AddInput{Title: "지난 주 작업"})
	if _, err := svc.EditWithSpan(task.ID, EditInput{}, ptrSpan(mustSpan(t, svc, "09-07~09-09"))); err != nil {
		t.Fatal(err)
	}
	rep, err := svc.Rollover()
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Rolled) != 1 {
		t.Fatalf("이월 %d건", len(rep.Rolled))
	}
	after, _ := svc.Load(task.ID)
	if !after.Scheduled.Equal(svc.Today()) {
		t.Fatalf("scheduled=%s", after.Scheduled)
	}
}
