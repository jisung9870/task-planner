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

// HasSpan reports whether the task carries a 진행 기간: 착수일과 마감일이 둘 다
// 정해진 경우다. 같은 날이어도 기간이다 - "14일에 시작해서 14일에 끝낸다" 는
// 약속은 하루여도 약속이고, 그 약속을 화면에서 지우면 날짜를 한쪽만 넣은
// 태스크와 구별되지 않는다.
// 마감이 착수보다 빠르면 기간이 아니다 - 이월이 착수일만 오늘로 당기면 이런
// 짝이 남는데, 그것을 "하루" 라고 부르면 늦은 마감이 화면에서 사라진다.
func (t *Task) HasSpan() bool {
	return !t.Scheduled.IsZero() && !t.Due.IsZero() && !t.Due.Before(t.Scheduled)
}

// MultiDay reports whether the period occupies more than a single day - the
// case a calendar has to draw across columns. HasSpan asks "기간이 있는가",
// this asks "여러 칸에 걸치는가"; 하루짜리는 앞은 참, 뒤는 거짓이다.
func (t *Task) MultiDay() bool {
	s, e := t.SpanStart(), t.SpanEnd()
	return !s.IsZero() && !e.IsZero() && e.After(s)
}

// SpanLabel renders the period the way it is read aloud - 하루짜리는 날짜 하나에
// "하루", 여러 날은 양끝과 일수. SpanLabelShort drops the year and the
// parentheses for grid rows, which have columns only for month and day.
func (t *Task) SpanLabel() string      { return t.spanLabel(false) }
func (t *Task) SpanLabelShort() string { return t.spanLabel(true) }

func (t *Task) spanLabel(short bool) string {
	s, e := t.SpanStart(), t.SpanEnd()
	if s.IsZero() || e.IsZero() {
		return ""
	}
	if e.Before(s) {
		e = s
	}
	day := func(d Date) string {
		if short {
			return d.Time().Format("01-02")
		}
		return d.String()
	}
	if !e.After(s) {
		return day(s) + " 하루"
	}
	if short {
		return fmt.Sprintf("%s~%s %d일", day(s), day(e), t.SpanDays())
	}
	return fmt.Sprintf("%s~%s (%d일)", day(s), day(e), t.SpanDays())
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
	if !t.Estimate.IsZero() {
		meta = append(meta, "~"+t.Estimate.String())
	}
	for _, tag := range t.Tags {
		meta = append(meta, "#"+tag)
	}
	if len(meta) > 0 {
		fmt.Fprintf(&b, "  [%s]", strings.Join(meta, " · "))
	}
	if when := t.previewWhen(today); when != "" {
		b.WriteString("  " + when)
	}
	if t.RolloverCount > 0 && t.IsOpen() {
		fmt.Fprintf(&b, "  ↻%d", t.RolloverCount)
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

// previewWhen picks the one date fact worth a row: a missed deadline first,
// then the working period, then whichever single date exists.
func (t *Task) previewWhen(today Date) string {
	short := func(d Date) string { return d.Time().Format("01-02") }
	if t.Overdue(today) {
		return fmt.Sprintf("!! 마감 %d일 초과", -t.Due.DaysUntil(today))
	}
	if t.HasSpan() {
		return t.SpanLabelShort()
	}
	if !t.Due.IsZero() {
		if t.IsOpen() && t.Due.DaysUntil(today) == 0 {
			return "! 오늘 마감"
		}
		return "~" + short(t.Due)
	}
	if !t.Scheduled.IsZero() {
		return "착수 " + short(t.Scheduled)
	}
	return ""
}
