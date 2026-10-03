package domain

import (
	"fmt"
	"time"
)

// BlockInfo carries the mandatory explanation for a hold.
type BlockInfo struct {
	Reason string
	By     []string
}

// TransitionOpts carries everything a state change needs beyond the target.
type TransitionOpts struct {
	Block *BlockInfo
	// SessionCap bounds a single 진행중 session. A task left running overnight
	// would otherwise record 14 hours and poison the estimate-vs-actual data
	// that makes tracking worth doing at all. Zero disables the cap.
	SessionCap Duration
	// Since is when the work really began, typed in at completion because the
	// timer was never started or started a moment ago. It replaces the start
	// of the session being closed. Only 완료 takes it.
	Since *time.Time
}

// Transition applies a status change together with every field the change
// implies, and records it in the body log. It is the only supported way to move
// a task between states: callers that set Status directly will drift.
func (t *Task) Transition(to Status, at time.Time, opts *TransitionOpts) error {
	if !to.Valid() {
		return fmt.Errorf("알 수 없는 상태: %q", to)
	}
	if opts == nil {
		opts = &TransitionOpts{}
	}
	block := opts.Block
	from := t.Status
	if from == to && to != StatusBlocked {
		return nil
	}
	today := DateOf(at)
	if opts.Since != nil {
		if err := t.checkSince(to, *opts.Since, at); err != nil {
			return err
		}
	}

	if to == StatusBlocked {
		if block != nil {
			if block.Reason != "" {
				t.BlockedReason = block.Reason
			}
			if len(block.By) > 0 {
				t.BlockedBy = block.By
			}
		}
		if t.BlockedReason == "" && len(t.BlockedBy) == 0 {
			return ErrBlockedNeedsReason
		}
		if t.BlockedSince.IsZero() {
			t.BlockedSince = today
		}
	} else {
		t.BlockedReason = ""
		t.BlockedBy = nil
		t.BlockedSince = Date{}
	}

	// Completed is the day the task stopped being work - cancelling ends it as
	// surely as finishing does. Recording only 완료 left every "오늘 취소 N"
	// and the weekly report's 취소 section permanently empty, because the
	// readers all filter on this date.
	if to.Terminal() {
		t.Completed = today
	} else {
		t.Completed = Date{}
	}

	// Back to 대기중 means the work is up for grabs again. 완료·취소 keep the
	// claim as the record of which run did it.
	if to == StatusTodo {
		t.ClaimedBy = ""
	}

	timerFrom, timerAt := from, at
	if opts.Since != nil {
		// The work ran from Since whatever the timer said: close it as one
		// session from there. Both ends sit on the minute, as the log prints
		// them, so 작업 시간 and the 걸린 기간 read back from the log agree.
		since := opts.Since.Truncate(time.Minute)
		t.StartedAt = &since
		timerFrom, timerAt = StatusDoing, at.Truncate(time.Minute)
	}
	capped := t.applyTimer(timerFrom, to, timerAt, opts.SessionCap)

	t.Status = to
	t.Updated = today

	detail := ""
	if to == StatusBlocked && t.BlockedReason != "" {
		detail = " (" + t.BlockedReason + ")"
	}
	if opts.Since != nil {
		detail += " (" + sinceMark + opts.Since.Format(sinceLayout) + ")"
	}
	if elapsed := t.lastSession; elapsed > 0 {
		detail += fmt.Sprintf(" [+%s]", elapsed)
		if capped {
			detail += " (상한 적용)"
		}
		t.lastSession = 0
	}
	t.AppendLog(at, "%s → %s%s", from, to, detail)
	return nil
}

// applyTimer starts or stops the work clock around a status change and reports
// whether the session hit the cap.
func (t *Task) applyTimer(from, to Status, at time.Time, cap Duration) bool {
	switch {
	case to == StatusDoing && from != StatusDoing:
		started := at
		t.StartedAt = &started
		return false
	case from == StatusDoing && to != StatusDoing && t.StartedAt != nil:
		elapsed, capped := t.RunningSession(at, cap)
		t.Actual += elapsed
		t.lastSession = elapsed
		t.StartedAt = nil
		return capped
	}
	return false
}

// checkSince refuses a backfilled start that would invent work: one in the
// future, one on anything but completion, or one reaching back into a session
// the log already counted.
func (t *Task) checkSince(to Status, since, at time.Time) error {
	if to != StatusDone {
		return fmt.Errorf("착수 시각은 완료할 때만 지정할 수 있음")
	}
	if since.After(at) {
		return fmt.Errorf("착수 시각(%s)이 완료 시각보다 나중임", since.Format(sinceLayout))
	}
	h := t.WorkHistory(at.Location())
	for _, s := range h.Sessions {
		if !s.End.IsZero() && since.Before(s.End) {
			return fmt.Errorf("착수 시각(%s)이 이미 기록된 진행 구간(%s~%s)과 겹침",
				since.Format(sinceLayout), s.Start.Format(sinceLayout), s.End.Format("15:04"))
		}
	}
	return nil
}

// BlockedDays reports how long a hold has been open, for the "왜 아직 보류지"
// question the Today view has to answer at a glance.
func (t *Task) BlockedDays(today Date) int {
	if t.Status != StatusBlocked || t.BlockedSince.IsZero() {
		return 0
	}
	return -t.BlockedSince.DaysUntil(today)
}
