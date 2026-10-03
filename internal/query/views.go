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
// A task qualifies when it is already in progress or when its 꺼낼 날 is today
// or earlier. Tasks completed today are
// included so the view doubles as a record of the day.
func Today(all []*domain.Task, today domain.Date) []*domain.Task {
	var out []*domain.Task
	for _, t := range all {
		switch {
		case t.Status == domain.StatusDoing:
		case t.IsOpen() && !t.Scheduled.IsZero() && !t.Scheduled.After(today):
		case t.Status.Terminal() && t.Completed.Equal(today):
		default:
			continue
		}
		out = append(out, t)
	}
	domain.SortDefault(out, today)
	return out
}

// Week answers "이번 주에 뭘 했고 뭐가 꺼내질 예정이지" for the ISO week
// containing ref: work that ran or finished inside the week (from the status
// log, hs), 대기중 work whose 꺼낼 날 falls inside it, and - while the week
// holds today - what is running now and the unscheduled backlog. Paging to
// another week drops the last two: they are today's situation, not that
// week's record.
func Week(all []*domain.Task, hs map[string]domain.WorkHistory, ref, today domain.Date) []*domain.Task {
	start := ref.WeekStart()
	end := start.AddDays(6)
	current := !today.IsZero() && !today.Before(start) && !today.After(end)
	var out []*domain.Task
	for _, t := range all {
		switch {
		case current && t.Status == domain.StatusDoing:
		case current && Backlog(t):
		case t.Status == domain.StatusTodo && !t.Scheduled.IsZero() &&
			!t.Scheduled.Before(start) && !t.Scheduled.After(end):
		case workedWithin(hs[t.ID], start, end, today):
		default:
			continue
		}
		out = append(out, t)
	}
	domain.SortDefault(out, ref)
	return out
}

// Backlog is 대기중 work with no 꺼낼 날: decided on, not yet surfaced.
func Backlog(t *domain.Task) bool {
	return t.Status == domain.StatusTodo && t.Scheduled.IsZero()
}

func workedWithin(h domain.WorkHistory, start, end, today domain.Date) bool {
	for d := start; !d.After(end); d = d.AddDays(1) {
		if h.InDay(d, today) {
			return true
		}
	}
	return false
}

// WeekDays buckets a week list onto days. A task sits on every day it was
// worked or finished (a pause leaves the day blank), and a 대기중 task on its
// 꺼낼 날. The rest - the backlog - lands in the zero Date bucket.
func WeekDays(ts []*domain.Task, hs map[string]domain.WorkHistory, ref, today domain.Date) (map[domain.Date][]*domain.Task, []domain.Date) {
	start := ref.WeekStart()
	days := make([]domain.Date, 7)
	for i := range days {
		days[i] = start.AddDays(i)
	}
	buckets := map[domain.Date][]*domain.Task{}
	for _, t := range ts {
		placed := false
		h := hs[t.ID]
		for _, d := range days {
			if h.InDay(d, today) || (t.Status == domain.StatusTodo && t.Scheduled.Equal(d)) {
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
	Slug      string
	Open      int
	Doing     int
	Blocked   int
	Done      int
	Cancelled int
	// Stale counts open work that surfaced and sat, or has run, past
	// stale_days - the project's "쪼개거나 버릴 것".
	Stale int
}

// Progress is the share of decided work that is done. Cancelled work is
// excluded from both sides: dropping a task is not progress, and counting it
// as such would let a project hit 100% by abandonment.
func (c ProjectCount) Progress() float64 {
	total := c.Open + c.Done
	if total == 0 {
		return 0
	}
	return float64(c.Done) / float64(total)
}

// ProjectCounts aggregates tasks per project slug, sorted by open work first.
func ProjectCounts(all []*domain.Task, today domain.Date, staleDays int) []ProjectCount {
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
		case t.Status == domain.StatusCancelled:
			c.Cancelled++
		}
		if t.Stale(today, staleDays) {
			c.Stale++
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
