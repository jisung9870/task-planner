package service

import (
	"fmt"

	"task-planner/internal/domain"
)

// Rolled describes one carried-over task.
type Rolled struct {
	Task *domain.Task
	From domain.Date
	// Count is the carry count after this rollover.
	Count int
}

// RolloverReport is the outcome of a daily rollover.
type RolloverReport struct {
	Rolled []Rolled
	// Stale lists tasks carried at least RolloverWarnAt times. A task that keeps
	// moving is not a scheduling problem, it is a scoping problem - the point of
	// counting is to make that visible instead of letting it slide daily.
	Stale []Rolled
}

func (r RolloverReport) Empty() bool { return len(r.Rolled) == 0 }

// Rollover moves unfinished work scheduled before today onto today.
//
// Only 대기중 and 진행중 tasks are carried. A 보류 task is waiting on someone
// else, so bumping its date every morning would inflate the carry count for a
// delay the owner does not control.
func (s *Service) Rollover() (RolloverReport, error) {
	today := s.Today()
	var rep RolloverReport
	for _, sum := range s.All() {
		if sum.Status != domain.StatusTodo && sum.Status != domain.StatusDoing {
			continue
		}
		if sum.Scheduled.IsZero() || !sum.Scheduled.Before(today) {
			continue
		}
		t, err := s.Load(sum.ID)
		if err != nil {
			return rep, err
		}
		from := t.Scheduled
		t.Scheduled = today
		t.RolloverCount++
		t.Updated = today
		t.AppendLog(s.now(), "rollover %s → %s (%d회째)", from, today, t.RolloverCount)
		if err := s.save(t); err != nil {
			return rep, err
		}
		s.recordChange(fmt.Sprintf("%s 이월", t.ShortID()),
			fmt.Sprintf("%s %s: %s → %s (%d회째)", t.ID, t.Title, from, today, t.RolloverCount))
		item := Rolled{Task: t, From: from, Count: t.RolloverCount}
		rep.Rolled = append(rep.Rolled, item)
		if t.RolloverCount >= s.Cfg.RolloverWarnAt {
			rep.Stale = append(rep.Stale, item)
		}
	}
	return rep, nil
}

// RolloverIfEnabled runs the rollover when config asks for it. Adapters call
// this at startup so the first thing shown in the morning is already correct.
func (s *Service) RolloverIfEnabled() (RolloverReport, error) {
	if !s.Cfg.AutoRollover {
		return RolloverReport{}, nil
	}
	return s.Rollover()
}

// StaleWarning renders the carry warning for a report, or "" when there is none.
func (r RolloverReport) StaleWarning() string {
	if len(r.Stale) == 0 {
		return ""
	}
	if len(r.Stale) == 1 {
		return fmt.Sprintf("%s 가 %d회 이월됨 — 쪼개거나 버릴 때",
			r.Stale[0].Task.ShortID(), r.Stale[0].Count)
	}
	return fmt.Sprintf("%d건이 반복 이월됨 — 쪼개거나 버릴 때", len(r.Stale))
}
