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

// Transition applies a status change together with every field the change
// implies, and records it in the body log. It is the only supported way to move
// a task between states: callers that set Status directly will drift.
func (t *Task) Transition(to Status, at time.Time, block *BlockInfo) error {
	if !to.Valid() {
		return fmt.Errorf("알 수 없는 상태: %q", to)
	}
	from := t.Status
	if from == to && to != StatusBlocked {
		return nil
	}
	today := DateOf(at)

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

	if to == StatusDone {
		t.Completed = today
	} else {
		t.Completed = Date{}
	}

	t.Status = to
	t.Updated = today

	detail := ""
	if to == StatusBlocked && t.BlockedReason != "" {
		detail = " (" + t.BlockedReason + ")"
	}
	t.AppendLog(at, "%s → %s%s", from, to, detail)
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
