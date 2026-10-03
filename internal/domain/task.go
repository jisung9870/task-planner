package domain

import (
	"fmt"
	"regexp"
	"strconv"
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
	Executor Executor `yaml:"executor,omitempty"`
	Priority Priority `yaml:"priority,omitempty"`

	// Agent and Tier say who should carry the task out and how heavy it is;
	// ClaimedBy is the run that took it. See "실행 agent 지정과 가져가기" in
	// docs/product-task-planner-202609.md.
	Agent     Agent  `yaml:"agent,omitempty"`
	Tier      Tier   `yaml:"tier,omitempty"`
	ClaimedBy string `yaml:"claimed_by,omitempty"`

	Created Date `yaml:"created"`
	Updated Date `yaml:"updated"`

	// Scheduled is the day the task surfaces in Today (꺼낼 날). It is not a
	// deadline: passing it raises nothing but the stale signal below.
	Scheduled Date `yaml:"scheduled,omitempty"`
	// Due and Estimate are retired inputs (2026-10-03, 기획서 "시간: 계획이
	// 아니라 기록"). Older files keep them and they round-trip untouched; the
	// detail view shows them as 이전 값. Nothing plans or sorts by them.
	Due      Date     `yaml:"due,omitempty"`
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
	// DisplayRef is a derived, vault-aware reference for a sequence number shared
	// by older tasks. It never belongs in markdown or the index.
	DisplayRef string `yaml:"-" json:"-"`

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
	if _, err := ParseExecutor(string(t.Executor)); err != nil {
		return fmt.Errorf("%s: %w", t.ID, err)
	}
	if t.Status == StatusBlocked && t.BlockedReason == "" && len(t.BlockedBy) == 0 {
		return fmt.Errorf("%s: %w", t.ID, ErrBlockedNeedsReason)
	}
	if _, err := ParseAgent(string(t.Agent)); err != nil {
		return fmt.Errorf("%s: %w", t.ID, err)
	}
	if _, err := ParseTier(string(t.Tier)); err != nil {
		return fmt.Errorf("%s: %w", t.ID, err)
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

// RunningSession is the current 진행중 session bounded the way a transition
// records it: never negative, never past cap (zero cap means no bound). over
// reports that the cap cut it - a timer left running for days. Every reader of
// a live total goes through here so a screen never shows a number the next
// save will not keep.
func (t *Task) RunningSession(now time.Time, cap Duration) (d Duration, over bool) {
	if t.StartedAt == nil {
		return 0, false
	}
	d = Duration(now.Sub(*t.StartedAt))
	if d < 0 {
		d = 0
	}
	if cap > 0 && d > cap {
		return cap, true
	}
	return d, false
}

// ElapsedActual is Actual plus the running session, bounded by cap.
func (t *Task) ElapsedActual(now time.Time, cap Duration) Duration {
	d, _ := t.RunningSession(now, cap)
	return t.Actual + d
}

// ElapsedLabel is ElapsedActual for display, marked when the cap cut the
// running session so an abandoned timer stands out instead of hiding.
func (t *Task) ElapsedLabel(now time.Time, cap Duration) string {
	label := t.ElapsedActual(now, cap).String()
	if _, over := t.RunningSession(now, cap); over {
		label += " (상한)"
	}
	return label
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

// NewID builds T-YYYYMMDD-NNNN. The date records creation while seq is unique
// across the vault, including archived tasks.
func NewID(d Date, seq int) string {
	return fmt.Sprintf("T-%s-%04d", d.Time().Format("20060102"), seq)
}

// ShortID is the display form used in narrow list columns. Legacy collisions
// receive a date-qualified reference from the service layer.
func (t *Task) ShortID() string {
	if t.DisplayRef != "" {
		return t.DisplayRef
	}
	return ShortRef(t.ID)
}

// QualifiedRef is a compact unambiguous form of a canonical task id.
func QualifiedRef(id string) string {
	parts := strings.Split(id, "-")
	if len(parts) == 3 && parts[0] == "T" && len(parts[1]) == 8 {
		if n, err := strconv.Atoi(parts[2]); err == nil {
			return fmt.Sprintf("#%s-%d", parts[1], n)
		}
	}
	return id
}

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

// ShortRefs renders canonical references for dependency lists. A dependency
// label must remain actionable even when older tasks share a sequence number.
func ShortRefs(ids []string) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = QualifiedRef(id)
	}
	return out
}

// Preview renders the task the way a list row reads, without the column
// padding a terminal needs. Write adapters echo it back so the caller - an
// agent, usually - sees the sentence it just wrote in the shape a human will
// meet it in. Reading your own line back is what stops the next one from
// being a paragraph.
func (t *Task) Preview(today Date) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s %s", t.Status.Glyph(), t.ShortID(), t.Title)

	var meta []string
	if t.Project != "" {
		meta = append(meta, t.Project)
	}
	if t.Priority != "" {
		meta = append(meta, string(t.Priority))
	}
	for _, tag := range t.Tags {
		meta = append(meta, "#"+tag)
	}
	if len(meta) > 0 {
		fmt.Fprintf(&b, "  [%s]", strings.Join(meta, " · "))
	}
	if when := t.When(today); when != "" {
		b.WriteString("  " + when)
	}
	if t.Status == StatusBlocked {
		reason := t.BlockedReason
		if reason == "" && len(t.BlockedBy) > 0 {
			reason = "선행 " + strings.Join(ShortRefs(t.BlockedBy), ", ")
		}
		if d := t.BlockedDays(today); d > 0 {
			reason = fmt.Sprintf("%s (%d일 경과)", reason, d)
		}
		b.WriteString("  ← " + reason)
	}
	return b.String()
}

// When is the one time fact worth a row: how long it has been running, how
// long it has sat since it surfaced, or when it will surface.
func (t *Task) When(today Date) string {
	switch {
	case t.Status == StatusDoing:
		if d := t.DoingDays(today); d > 0 {
			return fmt.Sprintf("진행 %d일째", d+1)
		}
	case t.Status == StatusTodo && !t.Scheduled.IsZero() && t.Scheduled.After(today):
		return "꺼냄 " + t.Scheduled.Time().Format("01-02")
	case t.Status == StatusTodo:
		if d := t.WaitingDays(today); d > 0 {
			return fmt.Sprintf("꺼낸 지 %d일", d)
		}
	}
	return ""
}

// WaitingDays is how long a 대기중 task has sat in Today since the day it
// surfaced without being started - "쪼개거나 버릴 때" once it grows. Zero for
// anything not waiting, or not yet surfaced.
func (t *Task) WaitingDays(today Date) int {
	if t.Status != StatusTodo || t.Scheduled.IsZero() || t.Scheduled.After(today) {
		return 0
	}
	return today.DaysUntil(t.Scheduled)
}

// DoingDays is how many days the running session has crossed since it began -
// "이 일은 너무 크다" once it grows. It reads the current session only: a pause
// and resume restarts the count, which is the honest reading without the log.
func (t *Task) DoingDays(today Date) int {
	if t.Status != StatusDoing || t.StartedAt == nil {
		return 0
	}
	if d := today.DaysUntil(DateOf(*t.StartedAt)); d > 0 {
		return d
	}
	return 0
}

// Stale reports the signal that replaced the rollover count: surfaced and not
// started for n days, or running for n days.
func (t *Task) Stale(today Date, n int) bool {
	return n > 0 && (t.WaitingDays(today) >= n || t.DoingDays(today) >= n)
}
