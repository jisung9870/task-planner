// Package report renders markdown summaries from task data.
//
// The output is a draft to edit, not a finished report: it answers "지난주에
// 내가 뭘 했지" from records already written, so the Friday ritual starts from
// facts instead of from memory and git log archaeology.
package report

import (
	"fmt"
	"sort"
	"strings"

	"task-planner/internal/domain"
	"task-planner/internal/query"
)

// WeekInput is everything the weekly report needs.
type WeekInput struct {
	Tasks []*domain.Task
	// Ref is any date inside the target week.
	Ref domain.Date
	// Today is used for age calculations (blocked days, stale).
	Today domain.Date
	// Histories are the status logs by task id, read by the caller: 착수와
	// 걸린 기간 live in the log, not in the indexed fields.
	Histories map[string]domain.WorkHistory
	// StaleDays is config stale_days.
	StaleDays int
}

// Week renders the weekly report markdown.
func Week(in WeekInput) string {
	start := in.Ref.WeekStart()
	end := start.AddDays(6)

	var done, doing, blocked, stale, upcoming []*domain.Task
	for _, t := range in.Tasks {
		switch {
		case t.Status == domain.StatusDone && inWeek(t.Completed, start, end):
			done = append(done, t)
		case t.Status == domain.StatusCancelled && inWeek(t.Completed, start, end):
			// Cancellations are reported separately from completions further down.
		case t.Status == domain.StatusDoing:
			doing = append(doing, t)
		case t.Status == domain.StatusBlocked:
			blocked = append(blocked, t)
		}
		if t.Stale(in.Today, in.StaleDays) {
			stale = append(stale, t)
		}
		if t.Status == domain.StatusTodo && inWeek(t.Scheduled, end.AddDays(1), end.AddDays(7)) {
			upcoming = append(upcoming, t)
		}
	}
	var cancelled []*domain.Task
	for _, t := range in.Tasks {
		if t.Status == domain.StatusCancelled && inWeek(t.Completed, start, end) {
			cancelled = append(cancelled, t)
		}
	}

	for _, g := range [][]*domain.Task{done, doing, blocked, stale, upcoming, cancelled} {
		domain.SortDefault(g, in.Today)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "---\ntype: weekly-report\nweek: %s\nperiod: %s ~ %s\ngenerated: %s\n---\n\n",
		in.Ref.WeekLabel(), start, end, in.Today)
	fmt.Fprintf(&b, "# %s 주간 (%s ~ %s)\n\n", in.Ref.WeekLabel(), start, end)

	fmt.Fprintf(&b, "완료 %d · 진행중 %d · 보류 %d · 멈춤 %d\n\n",
		len(done), len(doing), len(blocked), len(stale))

	section(&b, "완료", done, in.timeNote)
	section(&b, "진행중", doing, in.timeNote)
	section(&b, "보류", blocked, func(t *domain.Task) string {
		reason := t.BlockedReason
		if reason == "" && len(t.BlockedBy) > 0 {
			reason = "선행 " + strings.Join(domain.ShortRefs(t.BlockedBy), ", ")
		}
		if d := t.BlockedDays(in.Today); d > 0 {
			return fmt.Sprintf("%s · %d일 경과", reason, d)
		}
		return reason
	})
	section(&b, "오래 멈춤", stale, func(t *domain.Task) string { return t.When(in.Today) })
	section(&b, "취소", cancelled, func(t *domain.Task) string { return "" })
	section(&b, "다음 주 꺼낼 일", upcoming, func(t *domain.Task) string {
		return "꺼냄 " + t.Scheduled.String()
	})

	if n := len(done) + len(doing) + len(blocked) + len(stale) + len(upcoming) + len(cancelled); n == 0 {
		b.WriteString("기록된 항목이 없습니다.\n")
	}
	return b.String()
}

// section writes one grouped list, skipping empty groups entirely - an empty
// heading is noise in a report meant to be pasted somewhere.
func section(b *strings.Builder, title string, ts []*domain.Task, note func(*domain.Task) string) {
	if len(ts) == 0 {
		return
	}
	fmt.Fprintf(b, "## %s (%d)\n\n", title, len(ts))
	byProject := map[string][]*domain.Task{}
	for _, t := range ts {
		byProject[t.Project] = append(byProject[t.Project], t)
	}
	slugs := make([]string, 0, len(byProject))
	for s := range byProject {
		slugs = append(slugs, s)
	}
	sort.Slice(slugs, func(i, j int) bool {
		// Unassigned last: it is a bucket, not a project.
		if (slugs[i] == "") != (slugs[j] == "") {
			return slugs[j] == ""
		}
		return slugs[i] < slugs[j]
	})
	for _, slug := range slugs {
		if len(byProject) > 1 || slug != "" {
			fmt.Fprintf(b, "**%s**\n\n", query.ProjectLabel(slug))
		}
		for _, t := range byProject[slug] {
			line := fmt.Sprintf("- %s %s", t.ShortID(), t.Title)
			if n := note(t); n != "" {
				line += " — " + n
			}
			b.WriteString(line + "\n")
		}
		b.WriteString("\n")
	}
}

func inWeek(d, start, end domain.Date) bool {
	return !d.IsZero() && !d.Before(start) && !d.After(end)
}

// timeNote is what a finished or running item reports: when it started, how
// long it took on the calendar, and the time on the clock. A task with no
// recorded start reports only its clock time, if any - never a guessed span.
func (in WeekInput) timeNote(t *domain.Task) string {
	var parts []string
	h := in.Histories[t.ID]
	if start, end, done := h.Lead(); !start.IsZero() {
		parts = append(parts, "착수 "+start.Format("01-02 15:04"))
		if done {
			parts = append(parts, "걸린 기간 "+domain.SpanText(end.Sub(start)))
		}
	}
	if !t.Actual.IsZero() {
		parts = append(parts, "작업 "+t.Actual.String())
	}
	return strings.Join(parts, " · ")
}
