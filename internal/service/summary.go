package service

import "task-planner/internal/domain"

// Summary is the one-line morning briefing the TUI header shows. Each field
// maps to a question from the planning doc: what is due, what slipped, how
// much is in flight, and what has been stuck the longest.
type Summary struct {
	DueToday      int
	Overdue       int
	Doing         int
	WIPLimit      int
	Blocked       int
	BlockedMaxDay int // longest-running hold, in days
	Carried       int // open tasks with at least one rollover
}

// Summarize computes the briefing from the index.
func (s *Service) Summarize() Summary {
	today := s.Today()
	var sum Summary
	sum.WIPLimit = s.Cfg.WIPLimit
	for _, t := range s.All() {
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
		if !t.Due.IsZero() {
			switch {
			case t.Due.Before(today):
				sum.Overdue++
			case t.Due.Equal(today):
				sum.DueToday++
			}
		}
		if t.RolloverCount > 0 {
			sum.Carried++
		}
	}
	return sum
}
