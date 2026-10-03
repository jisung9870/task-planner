package query

import (
	"testing"
	"time"

	"task-planner/internal/domain"
)

var today = domain.NewDate(2026, time.September, 12) // Saturday, ISO week 37

func task(id string, st domain.Status, sched, due string) *domain.Task {
	t := &domain.Task{ID: id, Title: id, Status: st}
	if sched != "" {
		t.Scheduled, _ = domain.ParseDate(sched)
	}
	if due != "" {
		t.Due, _ = domain.ParseDate(due)
	}
	return t
}

func ids(ts []*domain.Task) []string {
	out := make([]string, len(ts))
	for i, t := range ts {
		out[i] = t.ID
	}
	return out
}

func has(ts []*domain.Task, id string) bool {
	for _, t := range ts {
		if t.ID == id {
			return true
		}
	}
	return false
}

func TestTodayIncludesDoingAndSurfaced(t *testing.T) {
	all := []*domain.Task{
		task("doing-unscheduled", domain.StatusDoing, "", ""),
		task("scheduled-today", domain.StatusTodo, "2026-09-12", ""),
		task("scheduled-past", domain.StatusTodo, "2026-09-10", ""),
		task("due-today", domain.StatusTodo, "", "2026-09-12"),
		task("scheduled-future", domain.StatusTodo, "2026-09-20", ""),
		task("backlog", domain.StatusTodo, "", ""),
	}
	got := Today(all, today)
	for _, want := range []string{"doing-unscheduled", "scheduled-today", "scheduled-past"} {
		if !has(got, want) {
			t.Errorf("%s 가 Today 에 없음: %v", want, ids(got))
		}
	}
	// due is retired: a deadline no longer pulls a task into Today.
	for _, no := range []string{"scheduled-future", "backlog", "due-today"} {
		if has(got, no) {
			t.Errorf("%s 가 Today 에 잘못 포함됨: %v", no, ids(got))
		}
	}
}

func TestTodayIncludesTasksCompletedToday(t *testing.T) {
	done := task("done-today", domain.StatusDone, "", "")
	done.Completed = today
	older := task("done-earlier", domain.StatusDone, "", "")
	older.Completed = today.AddDays(-1)
	got := Today([]*domain.Task{done, older}, today)
	if !has(got, "done-today") || has(got, "done-earlier") {
		t.Fatalf("got %v", ids(got))
	}
}

// at builds a local time on a September 2026 day, for work histories.
func at(day, hour int) time.Time {
	return time.Date(2026, time.September, day, hour, 0, 0, 0, time.Local)
}

func worked(start, end time.Time) domain.WorkHistory {
	return domain.WorkHistory{Sessions: []domain.WorkSession{{Start: start, End: end}}}
}

func TestWeekCoversWorkAndSurfacingInTheISOWeek(t *testing.T) {
	all := []*domain.Task{
		task("surfaces-wed", domain.StatusTodo, "2026-09-09", ""),
		task("surfaces-mon", domain.StatusTodo, "2026-09-07", ""),
		task("surfaces-sun", domain.StatusTodo, "2026-09-13", ""),
		task("next-week", domain.StatusTodo, "2026-09-14", ""),
		task("worked-tue", domain.StatusDone, "", ""),
		task("worked-last-week", domain.StatusDone, "", ""),
		task("backlog", domain.StatusTodo, "", ""),
		task("due-only", domain.StatusTodo, "", "2026-09-10"),
	}
	hs := map[string]domain.WorkHistory{
		"worked-tue":       worked(at(8, 9), at(8, 11)),
		"worked-last-week": worked(at(1, 9), at(1, 11)),
	}
	got := Week(all, hs, today, today)
	for _, want := range []string{"surfaces-wed", "surfaces-mon", "surfaces-sun", "worked-tue", "backlog"} {
		if !has(got, want) {
			t.Errorf("%s 가 Week 에 없음: %v", want, ids(got))
		}
	}
	for _, no := range []string{"next-week", "worked-last-week"} {
		if has(got, no) {
			t.Errorf("%s 가 포함됨: %v", no, ids(got))
		}
	}
	// A due date no longer puts a task on a day: undated, it is backlog.
	buckets, _ := WeekDays(got, hs, today, today)
	if !has(buckets[domain.Date{}], "due-only") {
		t.Errorf("due-only 가 백로그에 없음: %v", ids(buckets[domain.Date{}]))
	}
}

func TestWeekDaysPlacesWorkAndSurfacing(t *testing.T) {
	all := []*domain.Task{
		task("mon", domain.StatusTodo, "2026-09-07", ""),
		task("two-days", domain.StatusDone, "", ""),
		task("backlog", domain.StatusTodo, "", ""),
	}
	hs := map[string]domain.WorkHistory{"two-days": worked(at(9, 15), at(10, 11))}
	buckets, days := WeekDays(all, hs, today, today)
	if len(days) != 7 || days[0].String() != "2026-09-07" {
		t.Fatalf("days = %v", days)
	}
	if !has(buckets[days[0]], "mon") {
		t.Fatalf("월요일 버킷 = %v", ids(buckets[days[0]]))
	}
	for i, d := range days {
		want := i == 2 || i == 3 // 수·목
		if got := has(buckets[d], "two-days"); got != want {
			t.Errorf("%s: 작업한 날 포함 %v, want %v", d, got, want)
		}
	}
	if lane := buckets[domain.Date{}]; len(lane) != 1 || !has(lane, "backlog") {
		t.Fatalf("백로그 버킷 = %v", ids(lane))
	}
}

// A pause leaves the day blank: the week shows when work happened, not a bar
// from first start to last finish.
func TestWeekDaysLeavesPausedDaysBlank(t *testing.T) {
	paused := task("paused", domain.StatusDone, "", "")
	hs := map[string]domain.WorkHistory{"paused": {Sessions: []domain.WorkSession{
		{Start: at(7, 9), End: at(7, 10)}, {Start: at(10, 9), End: at(10, 10)},
	}}}
	buckets, days := WeekDays([]*domain.Task{paused}, hs, today, today)
	for i, d := range days {
		if got, want := has(buckets[d], "paused"), i == 0 || i == 3; got != want {
			t.Errorf("%s: %v, want %v", d, got, want)
		}
	}
}

func TestProjectCountsOrdersByOpenWork(t *testing.T) {
	mk := func(project string, st domain.Status) *domain.Task {
		t := task(project+"-"+string(st), st, "", "")
		t.Project = project
		return t
	}
	all := []*domain.Task{
		mk("infra", domain.StatusDoing), mk("infra", domain.StatusBlocked),
		mk("log", domain.StatusTodo), mk("log", domain.StatusDone),
	}
	counts := ProjectCounts(all, today, 5)
	if counts[0].Slug != "infra" || counts[0].Open != 2 || counts[0].Blocked != 1 {
		t.Fatalf("counts[0] = %+v", counts[0])
	}
	if counts[1].Slug != "log" || counts[1].Open != 1 || counts[1].Done != 1 {
		t.Fatalf("counts[1] = %+v", counts[1])
	}
}

// Priority decides the order, not a deadline: due is retired.
func TestSortPutsDoingThenPriority(t *testing.T) {
	all := []*domain.Task{
		task("p2-due-soon", domain.StatusTodo, "", "2026-09-01"),
		task("p1", domain.StatusTodo, "", ""),
		task("doing", domain.StatusDoing, "", ""),
	}
	all[0].Priority, all[1].Priority = domain.P2, domain.P1
	domain.SortDefault(all, today)
	if got := ids(all); got[0] != "doing" || got[1] != "p1" || got[2] != "p2-due-soon" {
		t.Fatalf("order = %v", got)
	}
}

// Paging to another week must not drag today's situation along: 다음 주 화면은
// 다음 주에 꺼낼 일이지 오늘의 진행중·백로그가 아니다.
func TestWeekDropsTodaysSituationOnOtherWeeks(t *testing.T) {
	all := []*domain.Task{
		task("backlog", domain.StatusTodo, "", ""),
		task("floating-doing", domain.StatusDoing, "", ""),
		task("next-week", domain.StatusTodo, "2026-09-14", ""),
	}
	next := domain.NewDate(2026, time.September, 14)
	got := Week(all, nil, next, today)
	if !has(got, "next-week") {
		t.Errorf("다음 주 항목이 빠짐: %v", ids(got))
	}
	for _, gone := range []string{"backlog", "floating-doing"} {
		if has(got, gone) {
			t.Errorf("%s 가 다음 주까지 따라옴: %v", gone, ids(got))
		}
	}
	if now := Week(all, nil, today, today); !has(now, "backlog") || !has(now, "floating-doing") {
		t.Errorf("이번 주에서 빠짐: %v", ids(now))
	}
}
