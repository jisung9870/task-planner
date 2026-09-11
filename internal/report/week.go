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
	// Today is used for age calculations (blocked days, overdue).
	Today domain.Date
}

// Week renders the weekly report markdown.
func Week(in WeekInput) string {
	start := in.Ref.WeekStart()
	end := start.AddDays(6)

	var done, doing, blocked, carried, upcoming []*domain.Task
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
		if t.IsOpen() && t.RolloverCount > 0 {
			carried = append(carried, t)
		}
		if t.IsOpen() && (inWeek(t.Scheduled, end.AddDays(1), end.AddDays(7)) ||
			inWeek(t.Due, end.AddDays(1), end.AddDays(7))) {
			upcoming = append(upcoming, t)
		}
	}
	var cancelled []*domain.Task
	for _, t := range in.Tasks {
		if t.Status == domain.StatusCancelled && inWeek(t.Completed, start, end) {
			cancelled = append(cancelled, t)
		}
	}

	for _, g := range [][]*domain.Task{done, doing, blocked, carried, upcoming, cancelled} {
		domain.SortDefault(g, in.Today)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "---\ntype: weekly-report\nweek: %s\nperiod: %s ~ %s\ngenerated: %s\n---\n\n",
		in.Ref.WeekLabel(), start, end, in.Today)
	fmt.Fprintf(&b, "# %s 주간 (%s ~ %s)\n\n", in.Ref.WeekLabel(), start, end)

	fmt.Fprintf(&b, "완료 %d · 진행중 %d · 보류 %d · 이월 %d\n\n",
		len(done), len(doing), len(blocked), len(carried))

	section(&b, "완료", done, func(t *domain.Task) string { return "" })
	section(&b, "진행중", doing, func(t *domain.Task) string {
		if t.Estimate.IsZero() {
			return ""
		}
		return "예상 " + t.Estimate.String()
	})
	section(&b, "보류", blocked, func(t *domain.Task) string {
		reason := t.BlockedReason
		if reason == "" && len(t.BlockedBy) > 0 {
			reason = "선행 " + strings.Join(t.BlockedBy, ", ")
		}
		if d := t.BlockedDays(in.Today); d > 0 {
			return fmt.Sprintf("%s · %d일 경과", reason, d)
		}
		return reason
	})
	section(&b, "이월", carried, func(t *domain.Task) string {
		return fmt.Sprintf("%d회", t.RolloverCount)
	})
	section(&b, "취소", cancelled, func(t *domain.Task) string { return "" })
	section(&b, "다음 주 예정", upcoming, func(t *domain.Task) string {
		if t.Due.IsZero() {
			return ""
		}
		return "마감 " + t.Due.String()
	})

	if n := len(done) + len(doing) + len(blocked) + len(carried) + len(upcoming) + len(cancelled); n == 0 {
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
