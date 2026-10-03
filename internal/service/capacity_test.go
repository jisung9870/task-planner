package service

import (
	"testing"
	"time"

	"task-planner/internal/config"
	"task-planner/internal/domain"
)

// A multi-day task contributes its estimate spread over the period. Counting
// the whole estimate on every day it touches would report a two-week task as
// two weeks of work every single day.
func TestDayLoadSpreadsSpanEstimate(t *testing.T) {
	svc := newTestService(t)
	long, _ := svc.Add(AddInput{Title: "리팩터링", Estimate: dur(t, "8h")})
	if _, err := svc.EditWithSpan(long.ID, EditInput{}, ptrSpan(mustSpan(t, svc, "09-12~09-15"))); err != nil {
		t.Fatal(err)
	}
	single, _ := svc.Add(AddInput{Title: "회고", Estimate: dur(t, "1h"), Scheduled: svc.Today()})

	load := svc.DayLoad(svc.Today())
	if load.Tasks != 2 || load.Estimated != 2 {
		t.Fatalf("%+v", load)
	}
	// 8h/4일 = 2h, 더하기 1h.
	if load.Planned != dur(t, "3h") {
		t.Fatalf("planned = %s", load.Planned)
	}
	_ = single

	// A day inside the span but outside the single task only carries the share.
	next := svc.DayLoad(svc.Today().AddDays(1))
	if next.Planned != dur(t, "2h") || next.Tasks != 1 {
		t.Fatalf("%+v", next)
	}
	// And a day past the period carries nothing.
	after := svc.DayLoad(svc.Today().AddDays(10))
	if !after.Empty() {
		t.Fatalf("%+v", after)
	}
}

func TestDayLoadFlagsOverCommitment(t *testing.T) {
	svc := newTestService(t)
	svc.Cfg.DailyCapacity = dur(t, "6h")
	for _, title := range []string{"A", "B", "C", "D"} {
		if _, err := svc.Add(AddInput{Title: title, Estimate: dur(t, "2h"), Scheduled: svc.Today()}); err != nil {
			t.Fatal(err)
		}
	}
	load := svc.DayLoad(svc.Today())
	if !load.Over() || load.Planned != dur(t, "8h") {
		t.Fatalf("%+v", load)
	}
}

// Tasks without an estimate are counted but not summed: an unestimated day is
// unknown, not free, and the ratio is what says so.
func TestDayLoadReportsHowMuchIsEstimated(t *testing.T) {
	svc := newTestService(t)
	svc.Add(AddInput{Title: "추정 있음", Estimate: dur(t, "2h"), Scheduled: svc.Today()})
	svc.Add(AddInput{Title: "추정 없음", Scheduled: svc.Today()})

	load := svc.DayLoad(svc.Today())
	if load.Tasks != 2 || load.Estimated != 1 || load.Planned != dur(t, "2h") {
		t.Fatalf("%+v", load)
	}
}

func TestWeekLoadCoversSevenDays(t *testing.T) {
	svc := newTestService(t)
	loads := svc.WeekLoad(svc.Today())
	if len(loads) != 7 {
		t.Fatalf("%d일", len(loads))
	}
	start := svc.Today().WeekStart()
	for i, l := range loads {
		if !l.Date.Equal(start.AddDays(i)) {
			t.Fatalf("%d번째 = %s", i, l.Date)
		}
	}
}

func TestDefaultConfigHasDailyCapacity(t *testing.T) {
	if got := config.Default("/tmp/x").DailyCapacity; got != domain.Duration(6*time.Hour) {
		t.Fatalf("daily_capacity = %s", got)
	}
}

func dur(t *testing.T, s string) domain.Duration {
	t.Helper()
	d, err := domain.ParseDuration(s)
	if err != nil {
		t.Fatal(err)
	}
	return d
}
