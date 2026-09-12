package domain

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Priority ranges P0 (highest) .. P3.
type Priority string

const (
	P0 Priority = "P0"
	P1 Priority = "P1"
	P2 Priority = "P2"
	P3 Priority = "P3"
)

var AllPriorities = []Priority{P0, P1, P2, P3}

func (p Priority) Rank() int {
	switch p {
	case P0:
		return 0
	case P1:
		return 1
	case P2:
		return 2
	case P3:
		return 3
	}
	return 4 // unset sorts last
}

func ParsePriority(s string) (Priority, error) {
	switch p := Priority(strings.ToUpper(strings.TrimSpace(s))); p {
	case "":
		return "", nil
	case P0, P1, P2, P3:
		return p, nil
	}
	return "", fmt.Errorf("우선순위는 P0~P3 이어야 함: %q", s)
}

// Task is the whole unit of work. One task == one markdown file, and this
// struct is the in-memory shape of that file: exported fields map 1:1 onto
// frontmatter keys, Body holds everything after the closing ---.
type Task struct {
	ID       string   `yaml:"id"`
	Title    string   `yaml:"title"`
	Status   Status   `yaml:"status"`
	Project  string   `yaml:"project,omitempty"`
	Priority Priority `yaml:"priority,omitempty"`

	Created Date `yaml:"created"`
	Updated Date `yaml:"updated"`

	// Scheduled is "when do I intend to work on this", Due is "when must it be
	// finished". Collapsing the two makes the Today view useless - see
	// docs/product-task-planner-202609.md.
	Scheduled Date `yaml:"scheduled,omitempty"`
	Due       Date `yaml:"due,omitempty"`

	Estimate Duration `yaml:"estimate,omitempty"`
	Actual   Duration `yaml:"actual,omitempty"`
	// StartedAt is set while Status == doing so elapsed time survives a restart.
	StartedAt *time.Time `yaml:"started_at,omitempty"`

	Tags  []string `yaml:"tags,omitempty"`
	Links []string `yaml:"links,omitempty"`

	// A blocked task must carry at least one of BlockedBy / BlockedReason.
	// Unexplained holds are how a personal tracker fills up with zombies.
	BlockedBy     []string `yaml:"blocked_by,omitempty"`
	BlockedReason string   `yaml:"blocked_reason,omitempty"`
	BlockedSince  Date     `yaml:"blocked_since,omitempty"`

	Recur         string `yaml:"recur,omitempty"`
	RecurOf       string `yaml:"recur_of,omitempty"`
	RolloverCount int    `yaml:"rollover_count,omitempty"`

	// Completed is the day work stopped, for 완료 and 취소 alike: every view
	// that reports "그 날 끝난 일" reads this one field.
	Completed Date `yaml:"completed,omitempty"`

	// Extra preserves frontmatter keys this version does not know about, so a
	// hand-added field is not silently dropped on the next save.
	Extra map[string]any `yaml:",inline"`

	Body string `yaml:"-"`
	Path string `yaml:"-"`

	// lastSession is the duration just accumulated, used to annotate the log
	// line. It is transient and never serialized.
	lastSession Duration
}

// ErrBlockedNeedsReason is returned when a hold has no explanation.
var ErrBlockedNeedsReason = fmt.Errorf("보류 상태는 사유(blocked_reason) 또는 선행 태스크(blocked_by)가 필요함")

// Validate enforces the invariants the whole tool relies on.
func (t *Task) Validate() error {
	if strings.TrimSpace(t.ID) == "" {
		return fmt.Errorf("id 가 비어 있음")
	}
	if strings.TrimSpace(t.Title) == "" {
		return fmt.Errorf("%s: title 이 비어 있음", t.ID)
	}
	if !t.Status.Valid() {
		return fmt.Errorf("%s: 알 수 없는 상태 %q", t.ID, t.Status)
	}
	if t.Status == StatusBlocked && t.BlockedReason == "" && len(t.BlockedBy) == 0 {
		return fmt.Errorf("%s: %w", t.ID, ErrBlockedNeedsReason)
	}
	if t.Priority != "" {
		if _, err := ParsePriority(string(t.Priority)); err != nil {
			return fmt.Errorf("%s: %w", t.ID, err)
		}
	}
	return nil
}

func (t *Task) IsOpen() bool { return t.Status.Open() }

// Overdue reports whether the deadline has passed for a task still open.
func (t *Task) Overdue(today Date) bool {
	return t.IsOpen() && !t.Due.IsZero() && t.Due.Before(today)
}

// DueSoon reports whether the deadline falls within the next n days.
func (t *Task) DueSoon(today Date, n int) bool {
	if !t.IsOpen() || t.Due.IsZero() {
		return false
	}
	d := t.Due.DaysUntil(today)
	return d >= 0 && d <= n
}

// ElapsedActual is Actual plus the currently running session, if any.
func (t *Task) ElapsedActual(now time.Time) Duration {
	total := t.Actual
	if t.StartedAt != nil {
		total += Duration(now.Sub(*t.StartedAt))
	}
	return total
}

// SpanStart and SpanEnd bound the 진행 기간: scheduled 는 착수일, due 는 마감일
// 이고 그 사이가 작업 기간이다. 한쪽만 있으면 그 날 하루짜리 일로 본다.
func (t *Task) SpanStart() Date {
	if !t.Scheduled.IsZero() {
		return t.Scheduled
	}
	return t.Due
}

func (t *Task) SpanEnd() Date {
	if !t.Due.IsZero() {
		return t.Due
	}
	return t.Scheduled
}

// Span is the pair as a value, for prompts and edits.
func (t *Task) Span() Span {
	return Span{Start: t.Scheduled, End: t.Due, SetStart: true, SetEnd: true}
}

// HasSpan reports whether the task occupies more than a single day - the case
// a calendar view has to draw across columns.
func (t *Task) HasSpan() bool {
	s, e := t.SpanStart(), t.SpanEnd()
	return !s.IsZero() && !e.IsZero() && e.After(s)
}

// SpanDays counts the period inclusively; 0 when the task has no date at all.
func (t *Task) SpanDays() int {
	s, e := t.SpanStart(), t.SpanEnd()
	if s.IsZero() || e.IsZero() {
		return 0
	}
	if e.Before(s) {
		return 1 // a due date before the start is a typo, not a negative period
	}
	return e.DaysUntil(s) + 1
}

// DayIndex is d's 1-based position inside the period, 0 when outside it.
func (t *Task) DayIndex(d Date) int {
	if !t.InSpan(d) {
		return 0
	}
	return d.DaysUntil(t.SpanStart()) + 1
}

// InSpan reports whether d falls inside the working period.
func (t *Task) InSpan(d Date) bool {
	s, e := t.SpanStart(), t.SpanEnd()
	if s.IsZero() || e.IsZero() || d.IsZero() {
		return false
	}
	if e.Before(s) {
		e = s
	}
	return !d.Before(s) && !d.After(e)
}

// SpanOverlaps reports whether the working period intersects [start, end].
// A week view asks this: a task that started before the week and ends after it
// is still this week's work even though neither of its dates falls inside.
func (t *Task) SpanOverlaps(start, end Date) bool {
	s, e := t.SpanStart(), t.SpanEnd()
	if s.IsZero() || e.IsZero() {
		return false
	}
	if e.Before(s) {
		e = s
	}
	return !s.After(end) && !e.Before(start)
}

// HasTag is case-insensitive; tags are typed by hand.
func (t *Task) HasTag(tag string) bool {
	for _, x := range t.Tags {
		if strings.EqualFold(x, tag) {
			return true
		}
	}
	return false
}

// Summary strips the body, which is what the index stores. Tasks are small but
// bodies are unbounded, and no list view needs them.
func (t *Task) Summary() *Task {
	c := *t
	c.Body = ""
	return &c
}

var slugStrip = regexp.MustCompile(`[^\p{L}\p{N}]+`)

// Slug builds the filename tail from the title. Non-ASCII is kept: the vault is
// browsed by humans, and a transliterated Korean title is unreadable.
func Slug(title string) string {
	s := slugStrip.ReplaceAllString(strings.ToLower(title), "-")
	s = strings.Trim(s, "-")
	if r := []rune(s); len(r) > 40 {
		s = strings.Trim(string(r[:40]), "-")
	}
	if s == "" {
		s = "task"
	}
	return s
}

// NewID builds T-YYYYMMDD-NNNN. seq is the count of tasks already created that
// day, so ids stay sortable and collision-free without a central counter.
func NewID(d Date, seq int) string {
	return fmt.Sprintf("T-%s-%04d", d.Time().Format("20060102"), seq)
}

// ShortID is the display form used in narrow list columns.
func (t *Task) ShortID() string { return ShortRef(t.ID) }

// ShortRef renders any task id as "#12". Blocker lists store canonical ids but
// showing them raw makes a one-line row unreadable.
func ShortRef(id string) string {
	if i := strings.LastIndexByte(id, '-'); i >= 0 && len(id) > i+1 {
		if short := strings.TrimLeft(id[i+1:], "0"); short != "" {
			return "#" + short
		}
	}
	return id
}

// ShortRefs renders a list of ids.
func ShortRefs(ids []string) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = ShortRef(id)
	}
	return out
}
