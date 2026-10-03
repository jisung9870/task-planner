package query

import (
	"sort"
	"time"

	"task-planner/internal/domain"
)

// ProjectTime aggregates recorded work time for one project. There is no
// estimate to compare against any more (기획서 "시간: 계획이 아니라 기록"):
// the number answers "어디에 시간을 썼나", nothing else.
type ProjectTime struct {
	Slug   string
	Tasks  int
	Actual domain.Duration
}

// TimeSummary aggregates effort for tasks touched within [from, to].
//
// Membership is by completion date for finished work and by "currently running
// or already accumulated" for open work, which is what makes the numbers
// answer "이번 주에 어디에 시간을 썼나".
//
// sessionCap bounds a running session as the next save will (config
// session_cap); a timer forgotten for days must not count as days of work.
func TimeSummary(all []*domain.Task, from, to domain.Date, now time.Time, sessionCap domain.Duration) []ProjectTime {
	byslug := map[string]*ProjectTime{}
	for _, t := range all {
		if !inPeriod(t, from, to) {
			continue
		}
		elapsed := t.ElapsedActual(now, sessionCap)
		if elapsed == 0 {
			continue
		}
		// Sum whole minutes, the unit every output prints. Summing the running
		// session's seconds made the ratio drift between calls while the
		// actual it was computed from still read "0m".
		actual := domain.Duration(time.Duration(elapsed).Round(time.Minute))
		p, ok := byslug[t.Project]
		if !ok {
			p = &ProjectTime{Slug: t.Project}
			byslug[t.Project] = p
		}
		p.Tasks++
		p.Actual += actual
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
	}
	return total
}
