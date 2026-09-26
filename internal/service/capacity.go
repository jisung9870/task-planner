package service

import (
	"task-planner/internal/domain"
)

// DayLoad is how much work a single day is carrying: the estimates of every
// open task whose 진행 기간 covers that day.
//
// A task that spans several days contributes its estimate divided by the
// length of the period. That is a crude model, but the alternative - counting
// the whole estimate on every day it touches - makes a two-week task look like
// two weeks of work every single day, which is worse than no number at all.
type DayLoad struct {
	Date domain.Date
	// Planned is the summed daily share.
	Planned domain.Duration
	// Tasks is how many tasks land on the day, Estimated how many of them carry
	// an estimate. Planned is only as good as that ratio.
	Tasks     int
	Estimated int
	Limit     domain.Duration
}

// Over reports whether the day is committed past its capacity.
func (l DayLoad) Over() bool { return l.Limit > 0 && l.Planned > l.Limit }

// Empty reports whether there is nothing to say about this day.
func (l DayLoad) Empty() bool { return l.Tasks == 0 }

// DayLoad computes the commitment for one day.
func (s *Service) DayLoad(d domain.Date) DayLoad {
	return s.DayLoadFor(d, s.All())
}

func (s *Service) DayLoadFor(d domain.Date, tasks []*domain.Task) DayLoad {
	load := DayLoad{Date: d, Limit: s.Cfg.DailyCapacity}
	if d.IsZero() {
		return load
	}
	for _, t := range tasks {
		if !t.IsOpen() || !t.InSpan(d) {
			continue
		}
		load.Tasks++
		if t.Estimate.IsZero() {
			continue
		}
		load.Estimated++
		load.Planned += dailyShare(t)
	}
	return load
}

// WeekLoad computes the commitment for each day of ref's week.
func (s *Service) WeekLoad(ref domain.Date) []DayLoad {
	return s.WeekLoadFor(ref, s.All())
}

func (s *Service) WeekLoadFor(ref domain.Date, tasks []*domain.Task) []DayLoad {
	start := ref.WeekStart()
	out := make([]DayLoad, 7)
	for i := range out {
		out[i] = s.DayLoadFor(start.AddDays(i), tasks)
	}
	return out
}

// dailyShare spreads an estimate over the days of the period.
func dailyShare(t *domain.Task) domain.Duration {
	days := t.SpanDays()
	if days <= 1 {
		return t.Estimate
	}
	return domain.Duration(int64(t.Estimate) / int64(days))
}
