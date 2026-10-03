package service

import (
	"fmt"

	"task-planner/internal/domain"
	"task-planner/internal/recur"
)

// spawnNextOccurrence creates the follow-up for a recurring task.
//
// The new task is a fresh file rather than a reset of the old one: keeping each
// occurrence separate is what lets the weekly report say what actually happened
// in a given week, and what preserves the per-occurrence notes.
func (s *Service) spawnNextOccurrence(finished *domain.Task) (*domain.Task, error) {
	if finished.Recur == "" {
		return nil, nil
	}
	rule, err := recur.Parse(finished.Recur)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", finished.ID, err)
	}
	base := finished.Scheduled
	if base.IsZero() {
		base = finished.Due // a series started before due was retired
	}
	if base.IsZero() {
		base = s.Today()
	}
	// Never schedule the next occurrence in the past: finishing a long-overdue
	// chore should point forward, not replay the backlog.
	next := rule.NextAfter(base, s.Today().AddDays(-1))

	in := AddInput{
		Title:    finished.Title,
		Project:  finished.Project,
		Executor: finished.Executor,
		Priority: finished.Priority,
		Agent:    finished.Agent,
		Tier:     finished.Tier,
		Tags:     finished.Tags,
		Links:    finished.Links,
		Recur:    finished.Recur,
		RecurOf:  seriesRoot(finished),
	}
	in.Scheduled = next

	t, err := s.Add(in)
	if err != nil {
		return nil, err
	}
	t.AppendLog(s.now(), "반복 생성 (이전 %s)", finished.ID)
	if err := s.save(t); err != nil {
		return nil, err
	}
	return t, nil
}

// seriesRoot returns the id that identifies a recurring series.
func seriesRoot(t *domain.Task) string {
	if t.RecurOf != "" {
		return t.RecurOf
	}
	return t.ID
}

// Skip closes the current occurrence without doing it and schedules the next
// one. Cancelling would end the series, which is rarely what "이번 주는 건너뛴다"
// means.
func (s *Service) Skip(ref string) (*Result, error) {
	t, err := s.Load(ref)
	if err != nil {
		return nil, err
	}
	if t.Recur == "" {
		return nil, fmt.Errorf("%s 는 반복 태스크가 아님 (건너뛸 회차가 없음)", t.ShortID())
	}
	if err := t.Transition(domain.StatusCancelled, s.now(), &domain.TransitionOpts{SessionCap: s.Cfg.SessionCap}); err != nil {
		return nil, err
	}
	t.AppendLog(s.now(), "이번 회차 건너뜀")
	if err := s.save(t); err != nil {
		return nil, err
	}
	s.recordChange(t.ShortID()+" 건너뜀", fmt.Sprintf("%s %s: 이번 회차 건너뜀", t.ID, t.Title))

	res := &Result{Task: t}
	next, err := s.spawnNextOccurrence(t)
	if err != nil {
		return res, err
	}
	if next != nil {
		res.Next = next
	}
	return res, nil
}

// SeriesOccurrences returns every task belonging to the same recurring series,
// oldest first.
func (s *Service) SeriesOccurrences(t *domain.Task) []*domain.Task {
	root := seriesRoot(t)
	var out []*domain.Task
	for _, c := range s.All() {
		if c.ID == root || c.RecurOf == root {
			out = append(out, c)
		}
	}
	return out
}
