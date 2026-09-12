// Package query turns the index into the lists the UI shows. It sits above the
// domain and below every adapter, so TUI, CLI and any future API answer the
// same question the same way.
package query

import (
	"sort"
	"strings"

	"task-planner/internal/domain"
)

// Today answers "오늘 뭘 해야 하지".
//
// A task qualifies when it is already in progress, when it was scheduled for
// today or earlier, or when its deadline has arrived. Tasks completed today are
// included so the view doubles as a record of the day.
func Today(all []*domain.Task, today domain.Date) []*domain.Task {
	var out []*domain.Task
	for _, t := range all {
		switch {
		case t.Status == domain.StatusDoing:
		case t.IsOpen() && !t.Scheduled.IsZero() && !t.Scheduled.After(today):
		case t.IsOpen() && !t.Due.IsZero() && !t.Due.After(today):
		case t.Status.Terminal() && t.Completed.Equal(today):
		default:
			continue
		}
		out = append(out, t)
	}
	domain.SortDefault(out, today)
	return out
}

// Week answers "이번 주에 뭐가 남았지" for the ISO week containing ref.
func Week(all []*domain.Task, ref domain.Date) []*domain.Task {
	start := ref.WeekStart()
	end := start.AddDays(6)
	var out []*domain.Task
	for _, t := range all {
		if t.Status == domain.StatusDoing {
			out = append(out, t)
			continue
		}
		// The 진행 기간 as a whole decides membership, not just its endpoints:
		// a task that started last week and is due next week is still work
		// this week has to make room for.
		if t.SpanOverlaps(start, end) {
			out = append(out, t)
			continue
		}
		// Anything overdue keeps showing up until it is dealt with.
		if t.Overdue(ref) {
			out = append(out, t)
		}
	}
	domain.SortDefault(out, ref)
	return out
}

// WeekDays buckets a week list onto the days it occupies. A task with a
// 진행 기간 (scheduled..due spanning several days) appears on every day of the
// period that falls inside the week, which is what makes the grid a calendar
// rather than a list of start dates. Tasks whose period misses the week
// entirely - and undated ones - land in the zero Date bucket, rendered as
// "미배정".
func WeekDays(ts []*domain.Task, ref domain.Date) (map[domain.Date][]*domain.Task, []domain.Date) {
	start := ref.WeekStart()
	days := make([]domain.Date, 7)
	for i := range days {
		days[i] = start.AddDays(i)
	}
	buckets := map[domain.Date][]*domain.Task{}
	for _, t := range ts {
		placed := false
		for _, d := range days {
			if t.InSpan(d) {
				buckets[d] = append(buckets[d], t)
				placed = true
			}
		}
		if !placed {
			buckets[domain.Date{}] = append(buckets[domain.Date{}], t)
		}
	}
	return buckets, days
}

// Open returns everything still needing attention.
func Open(all []*domain.Task, today domain.Date) []*domain.Task {
	var out []*domain.Task
	for _, t := range all {
		if t.IsOpen() {
			out = append(out, t)
		}
	}
	domain.SortDefault(out, today)
	return out
}

// ByProject returns the tasks of one project slug.
func ByProject(all []*domain.Task, slug string, today domain.Date, includeDone bool) []*domain.Task {
	var out []*domain.Task
	for _, t := range all {
		if !strings.EqualFold(t.Project, slug) {
			continue
		}
		if !includeDone && t.Status.Terminal() {
			continue
		}
		out = append(out, t)
	}
	domain.SortDefault(out, today)
	return out
}

// ProjectCount is a per-project rollup for the project list view. An empty
// Slug means "프로젝트 미지정".
type ProjectCount struct {
	Slug    string
	Open    int
	Doing   int
	Blocked int
	Done    int
	Overdue int
}

// ProjectCounts aggregates tasks per project slug, sorted by open work first.
func ProjectCounts(all []*domain.Task, today domain.Date) []ProjectCount {
	byslug := map[string]*ProjectCount{}
	for _, t := range all {
		// The empty slug is kept as-is so drilling into the row filters on
		// "no project" rather than on a display label.
		slug := t.Project
		c, ok := byslug[slug]
		if !ok {
			c = &ProjectCount{Slug: slug}
			byslug[slug] = c
		}
		switch {
		case t.Status == domain.StatusDoing:
			c.Doing++
			c.Open++
		case t.Status == domain.StatusBlocked:
			c.Blocked++
			c.Open++
		case t.Status == domain.StatusTodo:
			c.Open++
		case t.Status == domain.StatusDone:
			c.Done++
		}
		if t.Overdue(today) {
			c.Overdue++
		}
	}
	out := make([]ProjectCount, 0, len(byslug))
	for _, c := range byslug {
		out = append(out, *c)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Open != out[j].Open {
			return out[i].Open > out[j].Open
		}
		// Unassigned sorts last: it is a bucket, not a project.
		if (out[i].Slug == "") != (out[j].Slug == "") {
			return out[j].Slug == ""
		}
		return out[i].Slug < out[j].Slug
	})
	return out
}

// ProjectLabel renders a slug for display.
func ProjectLabel(slug string) string {
	if slug == "" {
		return "(미지정)"
	}
	return slug
}

// CountDoing is what the WIP guard checks.
func CountDoing(all []*domain.Task) int {
	n := 0
	for _, t := range all {
		if t.Status == domain.StatusDoing {
			n++
		}
	}
	return n
}
