package domain

import (
	"fmt"
	"strings"
	"time"
)

// WorkSession is an observed doing interval. A zero End means still running.
type WorkSession struct{ Start, End time.Time }

type WorkFinish struct {
	At     time.Time
	Status Status
}

// WorkHistory is derived from markdown, never written over the source log.
type WorkHistory struct {
	Sessions  []WorkSession
	Finishes  []WorkFinish
	Completed Date // date-only fallback for tasks without a terminal log entry
}

func (t *Task) WorkHistory(loc *time.Location) WorkHistory {
	var h WorkHistory
	var running time.Time
	for _, line := range t.LogLines() {
		if len(line) < 17 {
			continue
		}
		at, err := time.ParseInLocation("2006-01-02 15:04", line[:16], loc)
		if err != nil {
			continue
		}
		fields := strings.Fields(line[16:])
		if len(fields) < 3 || fields[1] != "→" {
			continue
		}
		from, to := Status(fields[0]), Status(fields[2])
		if !from.Valid() || !to.Valid() || from == to {
			continue
		}
		closed := false
		if from == StatusDoing && !running.IsZero() {
			if !at.Before(running) {
				h.Sessions = append(h.Sessions, WorkSession{running, at})
				closed = true
			}
			running = time.Time{}
		}
		if to == StatusDoing {
			running = at
		}
		// A backfilled completion closes a session the timer never saw, or
		// moves the start of the one it did.
		if since, ok := parseSinceMark(line, loc); ok && to == StatusDone && !at.Before(since) {
			if closed {
				h.Sessions = h.Sessions[:len(h.Sessions)-1]
			}
			h.Sessions = append(h.Sessions, WorkSession{since, at})
		}
		if to.Terminal() {
			h.Finishes = append(h.Finishes, WorkFinish{at, to})
		}
	}
	if t.Status == StatusDoing {
		if t.StartedAt != nil {
			running = t.StartedAt.In(loc)
		}
		if !running.IsZero() {
			h.Sessions = append(h.Sessions, WorkSession{Start: running})
		}
	}
	if t.Status.Terminal() && !t.Completed.IsZero() {
		if len(h.Finishes) == 0 || !DateOf(h.Finishes[len(h.Finishes)-1].At).Equal(t.Completed) {
			h.Completed = t.Completed
		}
	}
	return h
}

// Lead is the 걸린 기간: from the first time work started to the last
// completion. ok is false until the task has both; a running task reports
// the start with a zero end.
func (h WorkHistory) Lead() (start, end time.Time, ok bool) {
	if len(h.Sessions) == 0 {
		return time.Time{}, time.Time{}, false
	}
	start = h.Sessions[0].Start
	if h.Running() {
		return start, time.Time{}, false // reopened and running again
	}
	for i := len(h.Finishes) - 1; i >= 0; i-- {
		if h.Finishes[i].Status == StatusDone {
			end = h.Finishes[i].At
			break
		}
	}
	if !end.IsZero() && end.Before(start) {
		end = time.Time{}
	}
	return start, end, !end.IsZero()
}

// InDay leaves paused days blank and includes completion without a doing step.
func (h WorkHistory) InDay(d, today Date) bool {
	for _, s := range h.Sessions {
		end := today
		if !s.End.IsZero() {
			end = DateOf(s.End)
		}
		if !d.Before(DateOf(s.Start)) && !d.After(end) {
			return true
		}
	}
	for _, f := range h.Finishes {
		if DateOf(f.At).Equal(d) {
			return true
		}
	}
	return !h.Completed.IsZero() && h.Completed.Equal(d)
}

// ContinuesAt describes the session crossing an edge, not an older session
// separated from it by a pause.
func (h WorkHistory) ContinuesAt(d, today Date) (before, after bool) {
	for _, s := range h.Sessions {
		start, end := DateOf(s.Start), today
		if !s.End.IsZero() {
			end = DateOf(s.End)
		}
		if !d.Before(start) && !d.After(end) {
			before = before || start.Before(d)
			after = after || end.After(d)
		}
	}
	return
}

func (h WorkHistory) Bounds(today Date) (Date, Date) {
	var start, end Date
	add := func(d Date) {
		if d.IsZero() {
			return
		}
		if start.IsZero() || d.Before(start) {
			start = d
		}
		if end.IsZero() || d.After(end) {
			end = d
		}
	}
	for _, s := range h.Sessions {
		add(DateOf(s.Start))
		if s.End.IsZero() {
			add(today)
		} else {
			add(DateOf(s.End))
		}
	}
	for _, f := range h.Finishes {
		add(DateOf(f.At))
	}
	add(h.Completed)
	return start, end
}

// Running reports whether the last recorded session is still open.
func (h WorkHistory) Running() bool {
	return len(h.Sessions) > 0 && h.Sessions[len(h.Sessions)-1].End.IsZero()
}

// LeadLabel spells out 걸린 기간 for a detail view: when work started, when it
// finished, and the span between in calendar terms. A running task reads as
// "since"; a task with no recorded start says nothing rather than guess.
func (h WorkHistory) LeadLabel(now time.Time) string {
	if len(h.Sessions) == 0 {
		return ""
	}
	start, end, done := h.Lead()
	at := func(t time.Time) string { return t.Format("01-02 15:04") }
	switch {
	case h.Running():
		return fmt.Sprintf("착수 %s · %s째", at(start), SpanText(now.Sub(start)))
	case !done:
		return fmt.Sprintf("착수 %s · 멈춤 (완료 전)", at(start))
	}
	return fmt.Sprintf("착수 %s → 완료 %s (%s)", at(start), at(end), SpanText(end.Sub(start)))
}

// SpanText renders an elapsed span the way it is said: minutes and hours under
// a day, days and hours past it. "50h" hides that it was three days.
func SpanText(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	d = d.Round(time.Minute)
	if d == 0 {
		return "0m"
	}
	if d < 24*time.Hour {
		return Duration(d).String()
	}
	days, hours := int(d/(24*time.Hour)), int(d%(24*time.Hour)/time.Hour)
	if hours == 0 {
		return fmt.Sprintf("%d일", days)
	}
	return fmt.Sprintf("%d일 %d시간", days, hours)
}
