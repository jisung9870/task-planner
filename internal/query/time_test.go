package query

import (
	"testing"
	"time"

	"task-planner/internal/domain"
)

func timed(id, project string, est, act string, completed string) *domain.Task {
	t := &domain.Task{ID: id, Title: id, Status: domain.StatusDone, Project: project}
	t.Estimate, _ = domain.ParseDuration(est)
	t.Actual, _ = domain.ParseDuration(act)
	if completed != "" {
		t.Completed, _ = domain.ParseDate(completed)
	}
	return t
}

var noon = time.Date(2026, time.September, 12, 12, 0, 0, 0, time.Local)

func TestTimeSummaryAggregatesPerProject(t *testing.T) {
	all := []*domain.Task{
		timed("a", "infra", "2h", "3h", "2026-09-10"),
		timed("b", "infra", "1h", "30m", "2026-09-11"),
		timed("c", "", "", "45m", "2026-09-11"),
	}
	rows := TimeSummary(all, domain.Date{}, domain.Date{}, noon)
	if len(rows) != 2 {
		t.Fatalf("rows = %+v", rows)
	}
	if rows[0].Slug != "infra" || rows[0].Tasks != 2 {
		t.Fatalf("rows[0] = %+v", rows[0])
	}
	if got := rows[0].Actual.String(); got != "3h30m" {
		t.Fatalf("actual = %s", got)
	}
	if got := rows[0].Estimate.String(); got != "3h" {
		t.Fatalf("estimate = %s", got)
	}
	// 3h30m / 3h
	if r := rows[0].Ratio(); r < 1.16 || r > 1.17 {
		t.Fatalf("ratio = %f", r)
	}
	// Unassigned sorts last even though it has time.
	if rows[1].Slug != "" {
		t.Fatalf("rows[1] = %+v", rows[1])
	}
}

func TestTimeSummaryRespectsPeriod(t *testing.T) {
	all := []*domain.Task{
		timed("in", "infra", "1h", "1h", "2026-09-10"),
		timed("out", "infra", "1h", "1h", "2026-08-10"),
	}
	from, _ := domain.ParseDate("2026-09-07")
	to, _ := domain.ParseDate("2026-09-13")
	rows := TimeSummary(all, from, to, noon)
	if len(rows) != 1 || rows[0].Tasks != 1 {
		t.Fatalf("rows = %+v", rows)
	}
}

// A running task must contribute its live elapsed time, otherwise the current
// day always reads as empty.
func TestTimeSummaryIncludesRunningSession(t *testing.T) {
	running := &domain.Task{
		ID: "r", Title: "r", Status: domain.StatusDoing, Project: "infra",
		Updated: domain.NewDate(2026, time.September, 12),
	}
	started := noon.Add(-90 * time.Minute)
	running.StartedAt = &started

	rows := TimeSummary([]*domain.Task{running}, domain.Date{}, domain.Date{}, noon)
	if len(rows) != 1 || rows[0].Actual.String() != "1h30m" {
		t.Fatalf("rows = %+v", rows)
	}
}

// Tasks with neither estimate nor tracked time are noise in an effort report.
func TestTimeSummarySkipsUntrackedTasks(t *testing.T) {
	all := []*domain.Task{timed("none", "infra", "", "", "2026-09-10")}
	if rows := TimeSummary(all, domain.Date{}, domain.Date{}, noon); len(rows) != 0 {
		t.Fatalf("rows = %+v", rows)
	}
}

// Unestimated work must not dilute the accuracy ratio.
func TestRatioUsesEstimatedTasksOnly(t *testing.T) {
	all := []*domain.Task{
		timed("a", "infra", "2h", "2h", "2026-09-10"),
		timed("b", "infra", "", "5h", "2026-09-10"),
	}
	rows := TimeSummary(all, domain.Date{}, domain.Date{}, noon)
	if rows[0].Estimated != 1 {
		t.Fatalf("estimated = %d", rows[0].Estimated)
	}
	if rows[0].Estimate.String() != "2h" {
		t.Fatalf("estimate = %s", rows[0].Estimate)
	}
}

func TestTotalTimeSums(t *testing.T) {
	rows := []ProjectTime{
		{Slug: "a", Tasks: 1, Actual: domain.Duration(time.Hour), Estimate: domain.Duration(time.Hour)},
		{Slug: "b", Tasks: 2, Actual: domain.Duration(2 * time.Hour)},
	}
	total := TotalTime(rows)
	if total.Tasks != 3 || total.Actual.String() != "3h" {
		t.Fatalf("total = %+v", total)
	}
}
