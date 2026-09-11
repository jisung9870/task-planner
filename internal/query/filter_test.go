package query

import (
	"testing"
	"time"

	"task-planner/internal/domain"
)

func fixture() []*domain.Task {
	doing := task("doing-infra", domain.StatusDoing, "", "")
	doing.Project = "infra"
	doing.Priority = domain.P1
	doing.Tags = []string{"ops"}

	overdue := task("overdue", domain.StatusTodo, "", "2026-09-01")
	overdue.Project = "log-pipeline"
	overdue.RolloverCount = 4

	soon := task("due-soon", domain.StatusTodo, "", "2026-09-14")

	done := task("done", domain.StatusDone, "", "")
	done.Completed = today

	backlog := task("backlog-파이프라인", domain.StatusTodo, "", "")

	return []*domain.Task{doing, overdue, soon, done, backlog}
}

func run(t *testing.T, expr string) []string {
	t.Helper()
	f, err := ParseFilter(expr, today)
	if err != nil {
		t.Fatalf("%q: %v", expr, err)
	}
	return ids(f.Apply(fixture(), today, 3))
}

func eq(t *testing.T, got []string, want ...string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		found := false
		for _, g := range got {
			if g == want[i] {
				found = true
			}
		}
		if !found {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestFilterByField(t *testing.T) {
	eq(t, run(t, "status:doing"), "doing-infra")
	eq(t, run(t, "project:infra"), "doing-infra")
	eq(t, run(t, "tag:ops"), "doing-infra")
	eq(t, run(t, "#ops"), "doing-infra")
	eq(t, run(t, "priority:P1"), "doing-infra")
	eq(t, run(t, "id:done"), "done")
}

func TestFilterCombinesTermsWithAnd(t *testing.T) {
	eq(t, run(t, "status:doing project:infra"), "doing-infra")
	eq(t, run(t, "status:doing project:log-pipeline")) // 교집합 없음
}

func TestFilterNegation(t *testing.T) {
	got := run(t, "-status:done")
	for _, id := range got {
		if id == "done" {
			t.Fatalf("완료 항목이 남음: %v", got)
		}
	}
	if len(got) != 4 {
		t.Fatalf("got %v", got)
	}
}

func TestFilterDateComparison(t *testing.T) {
	eq(t, run(t, "due<today"), "overdue")
	eq(t, run(t, "due<=2026-09-14"), "overdue", "due-soon")
	eq(t, run(t, "due>today"), "due-soon")
	eq(t, run(t, "due<7d"), "overdue", "due-soon")
	// An absent date satisfies no comparison, otherwise every undated task
	// would flood a "due<7d" result.
	eq(t, run(t, "due:none"), "doing-infra", "done", "backlog-파이프라인")
	eq(t, run(t, "due:any"), "overdue", "due-soon")
}

func TestFilterIsShortcuts(t *testing.T) {
	eq(t, run(t, "is:overdue"), "overdue")
	eq(t, run(t, "is:duesoon"), "due-soon")
	eq(t, run(t, "is:carried"), "overdue")
	eq(t, run(t, "is:closed"), "done")
	eq(t, run(t, "is:unscheduled"), "doing-infra", "overdue", "due-soon", "done", "backlog-파이프라인")
}

func TestFilterNumericComparison(t *testing.T) {
	eq(t, run(t, "rollover>2"), "overdue")
	eq(t, run(t, "rollover:0"), "doing-infra", "due-soon", "done", "backlog-파이프라인")
}

func TestFilterFreeTextAndQuotes(t *testing.T) {
	eq(t, run(t, "파이프라인"), "backlog-파이프라인")
	eq(t, run(t, `"backlog-파이프라인"`), "backlog-파이프라인")
	// A bare word matches the project slug too.
	eq(t, run(t, "log-pipeline"), "overdue")
	// A bare word combined with a field term still ANDs.
	eq(t, run(t, "파이프라인 status:todo"), "backlog-파이프라인")
	eq(t, run(t, "파이프라인 status:doing"))
}

func TestFilterRejectsUnknownField(t *testing.T) {
	if _, err := ParseFilter("없는필드:값", today); err == nil {
		t.Fatal("알 수 없는 필드가 통과함")
	}
	if _, err := ParseFilter("status:이상함", today); err == nil {
		t.Fatal("알 수 없는 상태가 통과함")
	}
	if _, err := ParseFilter("is:이상함", today); err == nil {
		t.Fatal("알 수 없는 is 값이 통과함")
	}
}

func TestEmptyFilterMatchesAll(t *testing.T) {
	f, err := ParseFilter("   ", today)
	if err != nil {
		t.Fatal(err)
	}
	if !f.Empty() {
		t.Fatal("빈 질의가 비어 있지 않음")
	}
	if len(f.Apply(fixture(), today, 3)) != 5 {
		t.Fatal("빈 질의가 전부를 반환하지 않음")
	}
}

func TestRelativeDateUnits(t *testing.T) {
	for expr, want := range map[string]string{
		"2w": "2026-09-26", "1m": "2026-10-12", "+3d": "2026-09-15", "-3d": "2026-09-09",
		"today": "2026-09-12", "week": "2026-09-13",
	} {
		got, err := parseQueryDate(expr, today)
		if err != nil {
			t.Fatalf("%q: %v", expr, err)
		}
		if got.String() != want {
			t.Errorf("%q -> %s, want %s", expr, got, want)
		}
	}
	_ = time.Now
}
