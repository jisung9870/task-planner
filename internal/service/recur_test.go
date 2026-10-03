package service

import (
	"strings"
	"testing"

	"task-planner/internal/domain"
)

func TestCompletingRecurringTaskCreatesNextOccurrence(t *testing.T) {
	svc := newTestService(t)
	sched, _ := domain.ParseDate("2026-09-12")
	task, err := svc.Add(AddInput{
		Title: "주간보고 작성", Project: "ops", Recur: "weekly",
		Scheduled: sched, Estimate: mustDur(t, "30m"), Tags: []string{"routine"},
		// Estimate is a retired field an older task may still carry.
	})
	if err != nil {
		t.Fatal(err)
	}

	res, err := svc.Done(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if res.Next == nil {
		t.Fatal("다음 회차가 생성되지 않음")
	}
	n := res.Next
	if n.Scheduled.String() != "2026-09-19" {
		t.Fatalf("다음 예정일 = %s", n.Scheduled)
	}
	if n.Title != task.Title || n.Project != "ops" || n.Recur != "weekly" {
		t.Fatalf("필드가 승계되지 않음: %+v", n)
	}
	if len(n.Tags) != 1 {
		t.Fatalf("태그 승계 실패: %+v", n)
	}
	if !n.Estimate.IsZero() {
		t.Fatalf("폐지된 estimate 가 다음 회차로 넘어감: %s", n.Estimate)
	}
	if n.RecurOf != task.ID {
		t.Fatalf("recur_of = %q, want %q", n.RecurOf, task.ID)
	}
	if n.Status != domain.StatusTodo {
		t.Fatalf("다음 회차 상태 = %s", n.Status)
	}
	// The completed occurrence stays as its own record.
	old, _ := svc.Load(task.ID)
	if old.Status != domain.StatusDone {
		t.Fatalf("이전 회차 상태 = %s", old.Status)
	}
}

// Cancelling must end the series; otherwise there is no way to stop a chore
// without editing frontmatter by hand.
func TestCancellingRecurringTaskEndsSeries(t *testing.T) {
	svc := newTestService(t)
	task, _ := svc.Add(AddInput{Title: "그만둘 반복", Recur: "daily"})
	res, err := svc.Cancel(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if res.Next != nil {
		t.Fatalf("취소했는데 다음 회차가 생김: %s", res.Next.ID)
	}
}

// Skipping is the "이번 주는 건너뛴다" path: this occurrence ends, the series
// continues.
func TestSkipClosesOccurrenceAndContinuesSeries(t *testing.T) {
	svc := newTestService(t)
	sched, _ := domain.ParseDate("2026-09-12")
	task, _ := svc.Add(AddInput{Title: "격주 점검", Recur: "every 2 weeks", Scheduled: sched})

	res, err := svc.Skip(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if res.Task.Status != domain.StatusCancelled {
		t.Fatalf("상태 = %s", res.Task.Status)
	}
	if res.Next == nil || res.Next.Scheduled.String() != "2026-09-26" {
		t.Fatalf("다음 회차 = %+v", res.Next)
	}
	if !strings.Contains(strings.Join(res.Task.LogLines(), "\n"), "건너뜀") {
		t.Fatalf("건너뜀 로그 없음: %v", res.Task.LogLines())
	}
}

func TestSkipRejectsNonRecurringTask(t *testing.T) {
	svc := newTestService(t)
	task, _ := svc.Add(AddInput{Title: "일반 태스크"})
	if _, err := svc.Skip(task.ID); err == nil {
		t.Fatal("반복이 아닌 태스크가 건너뛰기 됨")
	}
}

// A series started before due was retired rolls forward on its 꺼낼 날 and
// leaves the deadline behind.
func TestNextOccurrenceDropsRetiredDue(t *testing.T) {
	svc := newTestService(t)
	sched, _ := domain.ParseDate("2026-09-12")
	due, _ := domain.ParseDate("2026-09-15")
	task, _ := svc.Add(AddInput{Title: "월간 정산", Recur: "monthly", Scheduled: sched, Due: due})

	res, _ := svc.Done(task.ID)
	if res.Next.Scheduled.String() != "2026-10-12" {
		t.Fatalf("scheduled = %s", res.Next.Scheduled)
	}
	if !res.Next.Due.IsZero() {
		t.Fatalf("due = %s, want none", res.Next.Due)
	}
}

// A chore completed long after it was due should schedule forward, not into
// the past.
func TestNextOccurrenceNeverLandsInThePast(t *testing.T) {
	svc := newTestService(t)
	old, _ := domain.ParseDate("2026-06-01")
	task, _ := svc.Add(AddInput{Title: "밀린 주간 점검", Recur: "weekly", Scheduled: old})

	res, _ := svc.Done(task.ID)
	if !res.Next.Scheduled.After(svc.Today()) {
		t.Fatalf("다음 예정일이 과거임: %s", res.Next.Scheduled)
	}
}

func TestAddRejectsInvalidRecurRule(t *testing.T) {
	svc := newTestService(t)
	if _, err := svc.Add(AddInput{Title: "x", Recur: "가끔"}); err == nil {
		t.Fatal("잘못된 반복 규칙이 통과함")
	}
}

func TestSeriesOccurrencesLinksGenerations(t *testing.T) {
	svc := newTestService(t)
	sched, _ := domain.ParseDate("2026-09-12")
	first, _ := svc.Add(AddInput{Title: "반복", Recur: "daily", Scheduled: sched})
	res, _ := svc.Done(first.ID)
	res2, _ := svc.Done(res.Next.ID)

	all := svc.SeriesOccurrences(res2.Next)
	if len(all) != 3 {
		t.Fatalf("회차 %d개: %+v", len(all), all)
	}
}

func mustDur(t *testing.T, s string) domain.Duration {
	t.Helper()
	d, err := domain.ParseDuration(s)
	if err != nil {
		t.Fatal(err)
	}
	return d
}
