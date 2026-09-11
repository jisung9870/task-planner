package service

import (
	"fmt"
	"strings"

	"task-planner/internal/domain"
	"task-planner/internal/query"
)

// WIPStatus describes how much work is in progress right now.
type WIPStatus struct {
	Count   int
	Limit   int
	Running []*domain.Task
}

// Exceeded reports whether the limit is set and passed.
func (w WIPStatus) Exceeded() bool { return w.Limit > 0 && w.Count > w.Limit }

// AtLimit reports whether starting one more task would exceed the limit.
func (w WIPStatus) AtLimit() bool { return w.Limit > 0 && w.Count >= w.Limit }

// WIP returns the current in-progress state.
func (s *Service) WIP() WIPStatus {
	var running []*domain.Task
	for _, t := range s.All() {
		if t.Status == domain.StatusDoing {
			running = append(running, t)
		}
	}
	domain.SortDefault(running, s.Today())
	return WIPStatus{Count: len(running), Limit: s.Cfg.WIPLimit, Running: running}
}

// wipWarning is produced when a task is started past the limit.
//
// It warns rather than blocks on purpose: the limit is a signal about attention,
// not a permission system, and a tool that refuses to record what someone is
// actually doing just gets bypassed. Naming the running tasks makes the warning
// actionable - the useful next move is to finish or park one of them.
func (s *Service) wipWarning(startedID string) string {
	w := s.WIP()
	if w.Limit <= 0 || w.Count <= w.Limit {
		return ""
	}
	var others []string
	for _, t := range w.Running {
		if t.ID == startedID {
			continue
		}
		others = append(others, t.ShortID())
	}
	return fmt.Sprintf("진행중 %d개로 WIP 한도(%d) 초과 — 먼저 끝내거나 보류할 것: %s",
		w.Count, w.Limit, strings.Join(others, ", "))
}

// countDoing is shared with the query layer for the header display.
func (s *Service) countDoing() int { return query.CountDoing(s.All()) }
