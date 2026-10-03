package service

import (
	"fmt"

	"task-planner/internal/domain"
)

// ParseDate resolves a date expression against today (YYYY-MM-DD, MM-DD,
// today/tomorrow, +3d, 월요일 ...). Every adapter parses dates through here so
// the CLI flags and the TUI prompts accept the same things.
func (s *Service) ParseDate(expr string) (domain.Date, error) {
	return domain.ParseDateRef(expr, s.Today())
}

// ParseSpan resolves a 기간 expression ("09-15~09-19", "today~+4d", "-").
//
// Deprecated: 진행 기간 was retired with due (2026-10-03). Only the MCP span
// argument still reads it, for one release, so older agent skills do not fail.
func (s *Service) ParseSpan(expr string) (domain.Span, error) {
	return domain.ParseSpan(expr, s.Today())
}

// EditWithSpan applies field updates and, when sp is set, the 진행 기간 in one
// save and one log line. Deprecated with ParseSpan. Adapters that take both at once must come through
// here: a span applied on its own call either drops the other fields or, when
// a later field fails, leaves half the request written.
func (s *Service) EditWithSpan(ref string, in EditInput, sp *domain.Span) (*Result, error) {
	if sp == nil {
		return s.Edit(ref, in)
	}
	// The span writes scheduled and due; letting one of two sources win
	// silently is worse than refusing.
	if in.Scheduled != nil || in.Due != nil {
		return nil, fmt.Errorf("기간(span)은 scheduled/due 와 함께 지정할 수 없음 (같은 필드를 씀)")
	}
	t, err := s.Resolve(ref)
	if err != nil {
		return nil, err
	}
	start, end := t.Scheduled, t.Due
	if sp.SetStart {
		start = sp.Start
		in.Scheduled = &start
	}
	if sp.SetEnd {
		end = sp.End
		in.Due = &end
	}
	if !start.IsZero() && !end.IsZero() && end.Before(start) {
		return nil, fmt.Errorf("기간의 끝(%s)이 시작(%s)보다 빠름", end, start)
	}
	return s.Edit(t.ID, in)
}

// ShiftScheduled moves the day a task surfaces by n days - "이건 하루 밀자".
// A task with no date starts from today, so the first press pulls it out of
// the backlog instead of jumping somewhere arbitrary.
func (s *Service) ShiftScheduled(ref string, n int) (*Result, error) {
	t, err := s.Resolve(ref)
	if err != nil {
		return nil, err
	}
	d := s.Today()
	if !t.Scheduled.IsZero() {
		d = t.Scheduled.AddDays(n)
	}
	return s.Edit(t.ID, EditInput{Scheduled: &d})
}
