package domain

import "fmt"

// Status is the task lifecycle state. The set is intentionally fixed and not
// configurable: making it dynamic would force every view, transition rule and
// report aggregation to be generalized for no real benefit in a personal tool.
type Status string

const (
	StatusTodo      Status = "todo"      // 대기중
	StatusDoing     Status = "doing"     // 진행중
	StatusBlocked   Status = "blocked"   // 보류
	StatusDone      Status = "done"      // 완료
	StatusCancelled Status = "cancelled" // 취소 - done 과 분리해야 완료 통계가 왜곡되지 않는다
)

// AllStatuses is the display order used by grouped views.
var AllStatuses = []Status{StatusDoing, StatusTodo, StatusBlocked, StatusDone, StatusCancelled}

var statusLabels = map[Status]string{
	StatusTodo:      "대기중",
	StatusDoing:     "진행중",
	StatusBlocked:   "보류",
	StatusDone:      "완료",
	StatusCancelled: "취소",
}

var statusGlyphs = map[Status]string{
	StatusTodo:      "○",
	StatusDoing:     "●",
	StatusBlocked:   "⊘",
	StatusDone:      "✓",
	StatusCancelled: "✕",
}

func (s Status) Valid() bool { _, ok := statusLabels[s]; return ok }

func (s Status) Label() string {
	if l, ok := statusLabels[s]; ok {
		return l
	}
	return string(s)
}

func (s Status) Glyph() string {
	if g, ok := statusGlyphs[s]; ok {
		return g
	}
	return "?"
}

// Open reports whether the task still needs attention.
func (s Status) Open() bool { return s == StatusTodo || s == StatusDoing || s == StatusBlocked }

// Terminal reports whether the task has left the workflow.
func (s Status) Terminal() bool { return s == StatusDone || s == StatusCancelled }

// ParseStatus accepts both the canonical id and the Korean label so that
// `tp done`, TUI keys and hand-edited markdown all converge on one value.
func ParseStatus(s string) (Status, error) {
	switch s {
	case "todo", "t", "대기", "대기중":
		return StatusTodo, nil
	case "doing", "d", "wip", "진행", "진행중":
		return StatusDoing, nil
	case "blocked", "b", "hold", "보류":
		return StatusBlocked, nil
	case "done", "complete", "완료":
		return StatusDone, nil
	case "cancelled", "canceled", "x", "취소":
		return StatusCancelled, nil
	}
	return "", fmt.Errorf("알 수 없는 상태: %q (todo|doing|blocked|done|cancelled)", s)
}

// NextStatus is the cycle used by the TUI space key. Blocked is skipped because
// entering it requires a reason, which needs a prompt rather than a keystroke.
func NextStatus(s Status) Status {
	switch s {
	case StatusTodo:
		return StatusDoing
	case StatusDoing:
		return StatusDone
	case StatusDone, StatusCancelled:
		return StatusTodo
	case StatusBlocked:
		return StatusDoing
	}
	return StatusTodo
}
