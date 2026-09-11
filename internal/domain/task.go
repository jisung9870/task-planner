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

	Completed Date `yaml:"completed,omitempty"`

	// Extra preserves frontmatter keys this version does not know about, so a
	// hand-added field is not silently dropped on the next save.
	Extra map[string]any `yaml:",inline"`

	Body string `yaml:"-"`
	Path string `yaml:"-"`
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
