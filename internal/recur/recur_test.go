package recur

import (
	"testing"
	"time"

	"task-planner/internal/domain"
)

var sat = domain.NewDate(2026, time.September, 12) // Saturday

func next(t *testing.T, expr string, from domain.Date) string {
	t.Helper()
	r, err := Parse(expr)
	if err != nil {
		t.Fatalf("%q: %v", expr, err)
	}
	return r.Next(from).String()
}

func TestParseAndNext(t *testing.T) {
	cases := map[string]string{
		"daily":          "2026-09-13",
		"매일":             "2026-09-13",
		"every 3 days":   "2026-09-15",
		"weekly":         "2026-09-19",
		"every 2 weeks":  "2026-09-26",
		"monthly":        "2026-10-12",
		"every 2 months": "2026-11-12",
		"weekdays":       "2026-09-14", // Sat -> Mon
		"평일":             "2026-09-14",
		"every monday":   "2026-09-14",
		"월":              "2026-09-14",
		"mon,thu":        "2026-09-14",
		"monthly on 15":  "2026-09-15",
		"매월 20일":         "2026-09-20",
	}
	for expr, want := range cases {
		if got := next(t, expr, sat); got != want {
			t.Errorf("%q -> %s, want %s", expr, got, want)
		}
	}
}

func TestParseRejectsGarbage(t *testing.T) {
	for _, expr := range []string{"", "가끔", "every banana", "monthly on 40", "every 0 days"} {
		if _, err := Parse(expr); err == nil {
			t.Errorf("%q 가 통과함", expr)
		}
	}
}

// Weekday lists must pick the nearest matching day, not the first listed.
func TestNextPicksNearestListedWeekday(t *testing.T) {
	wed := domain.NewDate(2026, time.September, 9)
	if got := next(t, "mon,thu", wed); got != "2026-09-10" {
		t.Fatalf("got %s", got)
	}
}

// Finishing a long-overdue chore must point forward, not replay the backlog
// one occurrence at a time.
func TestNextAfterSkipsPastOccurrences(t *testing.T) {
	r, err := Parse("weekly")
	if err != nil {
		t.Fatal(err)
	}
	longAgo := domain.NewDate(2026, time.June, 1)
	got := r.NextAfter(longAgo, sat)
	if !got.After(sat) {
		t.Fatalf("과거 회차가 선택됨: %s", got)
	}
	if got.String() != "2026-09-14" {
		t.Fatalf("got %s", got)
	}
}

// A month without the requested day must not skip a whole year.
func TestMonthDayFallsBackWhenDayMissing(t *testing.T) {
	r, err := Parse("monthly on 31")
	if err != nil {
		t.Fatal(err)
	}
	jan31 := domain.NewDate(2026, time.January, 31)
	got := r.Next(jan31)
	if got.DaysUntil(jan31) > 70 {
		t.Fatalf("다음 회차가 너무 멂: %s", got)
	}
}

func TestStringKeepsOriginalExpression(t *testing.T) {
	r, _ := Parse("Every 2 Weeks")
	if r.String() != "Every 2 Weeks" {
		t.Fatalf("String() = %q", r.String())
	}
}
