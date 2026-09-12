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

func TestTodayIncludesDoingScheduledAndDue(t *testing.T) {
	all := []*domain.Task{
		task("doing-unscheduled", domain.StatusDoing, "", ""),
		task("scheduled-today", domain.StatusTodo, "2026-09-12", ""),
		task("scheduled-past", domain.StatusTodo, "2026-09-10", ""),
		task("due-today", domain.StatusTodo, "", "2026-09-12"),
		task("scheduled-future", domain.StatusTodo, "2026-09-20", ""),
		task("backlog", domain.StatusTodo, "", ""),
	}
	got := Today(all, today)
	for _, want := range []string{"doing-unscheduled", "scheduled-today", "scheduled-past", "due-today"} {
		if !has(got, want) {
			t.Errorf("%s 가 Today 에 없음: %v", want, ids(got))
		}
	}
	for _, no := range []string{"scheduled-future", "backlog"} {
		if has(got, no) {
			t.Errorf("%s 가 Today 에 잘못 포함됨: %v", no, ids(got))
		}
	}
}

// Separating scheduled from due is the reason the Today view is usable; a task
// due later but scheduled for today must show up.
func TestTodaySeparatesScheduledFromDue(t *testing.T) {
	all := []*domain.Task{task("t", domain.StatusTodo, "2026-09-12", "2026-09-30")}
	if got := Today(all, today); len(got) != 1 {
		t.Fatalf("got %v", ids(got))
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

func TestWeekCoversISOWeekAndOverdue(t *testing.T) {
	all := []*domain.Task{
		task("in-week", domain.StatusTodo, "2026-09-09", ""),
		task("week-edge-mon", domain.StatusTodo, "2026-09-07", ""),
		task("week-edge-sun", domain.StatusTodo, "2026-09-13", ""),
		task("next-week", domain.StatusTodo, "2026-09-14", ""),
		task("overdue", domain.StatusTodo, "", "2026-08-30"),
	}
	got := Week(all, today, today)
	for _, want := range []string{"in-week", "week-edge-mon", "week-edge-sun", "overdue"} {
		if !has(got, want) {
			t.Errorf("%s 가 Week 에 없음: %v", want, ids(got))
		}
	}
	if has(got, "next-week") {
		t.Errorf("다음 주 항목이 포함됨: %v", ids(got))
	}
}

func TestWeekDaysBucketsUnassigned(t *testing.T) {
	all := []*domain.Task{
		task("mon", domain.StatusTodo, "2026-09-07", ""),
		task("floating", domain.StatusDoing, "", ""),
	}
	buckets, days := WeekDays(all, today)
	if len(days) != 7 || days[0].String() != "2026-09-07" {
		t.Fatalf("days = %v", days)
	}
	if len(buckets[domain.NewDate(2026, time.September, 7)]) != 1 {
		t.Fatalf("월요일 버킷 = %v", buckets)
	}
	if len(buckets[domain.Date{}]) != 1 {
		t.Fatalf("미배정 버킷 = %v", buckets[domain.Date{}])
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
	counts := ProjectCounts(all, today)
	if counts[0].Slug != "infra" || counts[0].Open != 2 || counts[0].Blocked != 1 {
		t.Fatalf("counts[0] = %+v", counts[0])
	}
	if counts[1].Slug != "log" || counts[1].Open != 1 || counts[1].Done != 1 {
		t.Fatalf("counts[1] = %+v", counts[1])
	}
}

func TestSortPutsDoingAndOverdueFirst(t *testing.T) {
	all := []*domain.Task{
		task("todo-later", domain.StatusTodo, "", "2026-09-30"),
		task("todo-overdue", domain.StatusTodo, "", "2026-09-01"),
		task("doing", domain.StatusDoing, "", ""),
	}
	domain.SortDefault(all, today)
	if all[0].ID != "doing" || all[1].ID != "todo-overdue" {
		t.Fatalf("order = %v", ids(all))
	}
}

// A 진행 기간 has to land on every day it covers, or the Week grid shows a
// multi-day task as a single start-day card and the rest of the week looks free.
func TestWeekDaysSpreadsSpanAcrossDays(t *testing.T) {
	span := task("T-span", domain.StatusTodo, "2026-09-08", "2026-09-10") // 화~목
	single := task("T-one", domain.StatusTodo, "2026-09-09", "")
	buckets, days := WeekDays([]*domain.Task{span, single}, today)

	for i, d := range days {
		want := i >= 1 && i <= 3 // 월=0 이므로 화·수·목
		if got := has(buckets[d], "T-span"); got != want {
			t.Errorf("%s: 기간 태스크 포함 %v, want %v", d, got, want)
		}
	}
	// 화요일에는 기간 태스크만, 수요일에는 둘 다.
	if len(buckets[days[1]]) != 1 || len(buckets[days[2]]) != 2 || !has(buckets[days[2]], "T-one") {
		t.Errorf("화 %v 수 %v", ids(buckets[days[1]]), ids(buckets[days[2]]))
	}
	if len(buckets[domain.Date{}]) != 0 {
		t.Errorf("미배정 = %v", ids(buckets[domain.Date{}]))
	}
}

// A period that runs into the week from outside still occupies the days it
// covers inside it - the old code dropped such tasks into 미배정.
func TestWeekDaysClipsSpanToTheWeek(t *testing.T) {
	long := task("T-long", domain.StatusTodo, "2026-08-31", "2026-09-09")
	buckets, days := WeekDays([]*domain.Task{long}, today)
	for i, d := range days {
		if got, want := has(buckets[d], "T-long"), i <= 2; got != want {
			t.Errorf("%s: %v, want %v", d, got, want)
		}
	}
}

func TestWeekDaysKeepsUndatedInUnassigned(t *testing.T) {
	doing := task("T-doing", domain.StatusDoing, "", "")
	past := task("T-past", domain.StatusTodo, "2026-08-01", "2026-08-05")
	buckets, _ := WeekDays([]*domain.Task{doing, past}, today)
	lane := buckets[domain.Date{}]
	if !has(lane, "T-doing") || !has(lane, "T-past") {
		t.Fatalf("미배정 = %v", ids(lane))
	}
}

// Paging to another week must not drag today's leftovers along: 다음 주 화면은
// 다음 주의 계획이지 오늘의 잔업 목록이 아니다.
func TestWeekDropsTodaysLeftoversOnOtherWeeks(t *testing.T) {
	all := []*domain.Task{
		task("overdue", domain.StatusTodo, "", "2026-08-30"),
		task("floating-doing", domain.StatusDoing, "", ""),
		task("next-week", domain.StatusTodo, "2026-09-14", ""),
	}
	next := domain.NewDate(2026, time.September, 14)
	got := Week(all, next, today)
	if !has(got, "next-week") {
		t.Errorf("다음 주 항목이 빠짐: %v", ids(got))
	}
	for _, gone := range []string{"overdue", "floating-doing"} {
		if has(got, gone) {
			t.Errorf("%s 가 다음 주까지 따라옴: %v", gone, ids(got))
		}
	}
	// 오늘이 든 주에서는 그대로 따라와야 한다.
	if now := Week(all, today, today); !has(now, "overdue") || !has(now, "floating-doing") {
		t.Errorf("이번 주에서 빠짐: %v", ids(now))
	}
}

// A period that straddles the whole week has neither endpoint inside it; the
// week still has to show it.
func TestWeekIncludesSpanStraddlingTheWeek(t *testing.T) {
	long := task("T-long", domain.StatusTodo, "2026-09-01", "2026-09-30")
	got := Week([]*domain.Task{long}, today, today)
	if !has(got, "T-long") {
		t.Fatalf("week = %v", ids(got))
	}
	buckets, days := WeekDays(got, today)
	for _, d := range days {
		if !has(buckets[d], "T-long") {
			t.Errorf("%s 에 없음", d)
		}
	}
}
