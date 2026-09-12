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
func (s *Service) ParseSpan(expr string) (domain.Span, error) {
	return domain.ParseSpan(expr, s.Today())
}

// SetSpan writes a task's 진행 기간 - scheduled is the start, due is the end.
func (s *Service) SetSpan(ref string, sp domain.Span) (*Result, error) {
	t, err := s.Resolve(ref)
	if err != nil {
		return nil, err
	}
	start, end := t.Scheduled, t.Due
	in := EditInput{}
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

// ShiftSpan moves the whole period by n days, keeping its length. An undated
// task starts at today, so the first press pulls it onto the calendar instead
// of jumping somewhere arbitrary.
func (s *Service) ShiftSpan(ref string, n int) (*Result, error) {
	t, err := s.Resolve(ref)
	if err != nil {
		return nil, err
	}
	sp := domain.Span{SetStart: true, SetEnd: true}
	switch {
	case t.Scheduled.IsZero() && t.Due.IsZero():
		sp.Start, sp.SetEnd = s.Today(), false
	case t.Scheduled.IsZero():
		sp.SetStart, sp.End = false, t.Due.AddDays(n)
	case t.Due.IsZero():
		sp.Start, sp.SetEnd = t.Scheduled.AddDays(n), false
	default:
		sp.Start, sp.End = t.Scheduled.AddDays(n), t.Due.AddDays(n)
	}
	return s.SetSpan(t.ID, sp)
}

// ResizeSpan moves only the end of the period by n days, which is how a task
// grows or shrinks without changing when it starts. The end never crosses the
// start: a negative period is a typo, not an intent.
func (s *Service) ResizeSpan(ref string, n int) (*Result, error) {
	t, err := s.Resolve(ref)
	if err != nil {
		return nil, err
	}
	end := t.SpanEnd()
	if end.IsZero() {
		end = s.Today()
	}
	end = end.AddDays(n)
	start := t.SpanStart()
	if !start.IsZero() && end.Before(start) {
		end = start
	}
	sp := domain.Span{End: end, SetEnd: true}
	// Shrinking an undated task would leave a due date with no start; give it
	// one so the period stays a period.
	if t.Scheduled.IsZero() {
		sp.Start, sp.SetStart = start, !start.IsZero()
		if start.IsZero() {
			sp.Start, sp.SetStart = s.Today(), true
		}
	}
	return s.SetSpan(t.ID, sp)
}
