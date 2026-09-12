package service

import (
	"fmt"
	"strings"

	"task-planner/internal/domain"
)

// Suggestion is one recommended task plus why it is being recommended. The
// reason is the point: a list of ids answers "what", and the question people
// actually have in the morning is "why this one".
type Suggestion struct {
	Task   *domain.Task
	Reason string
}

// NextUp answers "지금 뭘 하지": open work that nothing is blocking, most
// urgent first. limit <= 0 returns everything actionable.
//
// 보류 is excluded even when its blockers are done - a released task returns to
// 대기중 on its own (see releaseDependents), so one that is still 보류 is
// waiting on something this tool cannot see.
func (s *Service) NextUp(limit int) []Suggestion {
	today := s.Today()
	var actionable []*domain.Task
	for _, t := range s.All() {
		if !t.IsOpen() || t.Status == domain.StatusBlocked {
			continue
		}
		if !s.allBlockersDone(t.BlockedBy) {
			continue
		}
		actionable = append(actionable, t)
	}
	domain.SortDefault(actionable, today)
	if limit > 0 && len(actionable) > limit {
		actionable = actionable[:limit]
	}
	out := make([]Suggestion, len(actionable))
	for i, t := range actionable {
		out[i] = Suggestion{Task: t, Reason: s.reasonFor(t, today)}
	}
	return out
}

// reasonFor spells out the signals that put a task at the top, in the order
// SortDefault weighs them.
func (s *Service) reasonFor(t *domain.Task, today domain.Date) string {
	var parts []string
	if t.Status == domain.StatusDoing {
		parts = append(parts, "이미 진행중")
	}
	switch {
	case t.Overdue(today):
		parts = append(parts, fmt.Sprintf("마감 %d일 초과", -t.Due.DaysUntil(today)))
	case !t.Due.IsZero() && t.Due.Equal(today):
		parts = append(parts, "오늘 마감")
	case t.DueSoon(today, s.Cfg.DueSoonDays):
		parts = append(parts, fmt.Sprintf("D-%d", t.Due.DaysUntil(today)))
	}
	if t.Priority != "" {
		parts = append(parts, string(t.Priority))
	}
	if !t.Scheduled.IsZero() && !t.Scheduled.After(today) {
		parts = append(parts, "착수 예정일 지남")
	}
	if t.RolloverCount >= s.Cfg.RolloverWarnAt {
		parts = append(parts, fmt.Sprintf("%d회 이월", t.RolloverCount))
	}
	if n := len(s.Blocking(t.ID)); n > 0 {
		parts = append(parts, fmt.Sprintf("%d건이 이걸 기다림", n))
	}
	if len(parts) == 0 {
		parts = append(parts, "막힌 것 없음")
	}
	return strings.Join(parts, " · ")
}
