package query

import (
	"sort"
	"time"

	"task-planner/internal/domain"
)

// ProjectTime aggregates tracked effort for one project.
type ProjectTime struct {
	Slug     string
	Tasks    int
	Estimate domain.Duration
	Actual   domain.Duration
	// Estimated counts only the tasks that carried an estimate, so the ratio
	// below is not diluted by work nobody estimated.
	Estimated int
}

// Ratio is actual/estimate over the estimated tasks, or 0 when unknown.
// A number above 1 means the estimates were optimistic.
func (p ProjectTime) Ratio() float64 {
	if p.Estimate == 0 {
		return 0
	}
	return float64(p.Actual) / float64(p.Estimate)
}

// TimeSummary aggregates effort for tasks touched within [from, to].
//
// Membership is by completion date for finished work and by "currently running
// or already accumulated" for open work, which is what makes the numbers
// answer "이번 주에 어디에 시간을 썼나".
func TimeSummary(all []*domain.Task, from, to domain.Date, now time.Time) []ProjectTime {
	byslug := map[string]*ProjectTime{}
	for _, t := range all {
		if !inPeriod(t, from, to) {
			continue
		}
		actual := t.ElapsedActual(now)
		if actual == 0 && t.Estimate == 0 {
			continue
		}
		p, ok := byslug[t.Project]
		if !ok {
			p = &ProjectTime{Slug: t.Project}
			byslug[t.Project] = p
		}
		p.Tasks++
		p.Actual += actual
		if t.Estimate > 0 {
			p.Estimate += t.Estimate
			p.Estimated++
		}
	}
	out := make([]ProjectTime, 0, len(byslug))
	for _, p := range byslug {
		out = append(out, *p)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Actual != out[j].Actual {
			return out[i].Actual > out[j].Actual
		}
		if (out[i].Slug == "") != (out[j].Slug == "") {
			return out[j].Slug == ""
		}
		return out[i].Slug < out[j].Slug
	})
	return out
}

func inPeriod(t *domain.Task, from, to domain.Date) bool {
	if from.IsZero() && to.IsZero() {
		return true
	}
	d := t.Completed
	if d.IsZero() {
		d = t.Updated
	}
	if d.IsZero() {
		return false
	}
	if !from.IsZero() && d.Before(from) {
		return false
	}
	if !to.IsZero() && d.After(to) {
		return false
	}
	return true
}

// TotalTime sums a summary.
func TotalTime(ps []ProjectTime) ProjectTime {
	var total ProjectTime
	total.Slug = "합계"
	for _, p := range ps {
		total.Tasks += p.Tasks
		total.Actual += p.Actual
		total.Estimate += p.Estimate
		total.Estimated += p.Estimated
	}
	return total
}
