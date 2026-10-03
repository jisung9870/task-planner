package service

import "task-planner/internal/domain"

// Summary is the one-line morning briefing the TUI header shows: how much is
// in flight, what has been stuck the longest, and what has sat or run past
// stale_days.
type Summary struct {
	Doing         int
	WIPLimit      int
	Blocked       int
	BlockedMaxDay int // longest-running hold, in days
	Stale         int // open tasks past stale_days, waiting or running
}

// Summarize computes the briefing from the index.
func (s *Service) Summarize() Summary {
	return s.SummarizeTasks(s.All())
}

func (s *Service) SummarizeTasks(tasks []*domain.Task) Summary {
	today := s.Today()
	var sum Summary
	sum.WIPLimit = s.Cfg.WIPLimit
	for _, t := range tasks {
		if !t.IsOpen() {
			continue
		}
		switch t.Status {
		case domain.StatusDoing:
			sum.Doing++
		case domain.StatusBlocked:
			sum.Blocked++
			if d := t.BlockedDays(today); d > sum.BlockedMaxDay {
				sum.BlockedMaxDay = d
			}
		}
		if t.Stale(today, s.Cfg.StaleDays) {
			sum.Stale++
		}
	}
	return sum
}
