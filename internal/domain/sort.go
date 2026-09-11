package domain

import "sort"

// SortDefault orders a task list the way every view wants it: what is running
// first, then what is most urgent, with a stable id tiebreak so redraws do not
// shuffle rows.
func SortDefault(ts []*Task, today Date) {
	sort.SliceStable(ts, func(i, j int) bool {
		a, b := ts[i], ts[j]
		if r := statusRank(a.Status) - statusRank(b.Status); r != 0 {
			return r < 0
		}
		if a.Overdue(today) != b.Overdue(today) {
			return a.Overdue(today)
		}
		if !a.Due.Equal(b.Due) {
			if a.Due.IsZero() {
				return false
			}
			if b.Due.IsZero() {
				return true
			}
			return a.Due.Before(b.Due)
		}
		if r := a.Priority.Rank() - b.Priority.Rank(); r != 0 {
			return r < 0
		}
		if !a.Scheduled.Equal(b.Scheduled) {
			if a.Scheduled.IsZero() {
				return false
			}
			if b.Scheduled.IsZero() {
				return true
			}
			return a.Scheduled.Before(b.Scheduled)
		}
		return a.ID < b.ID
	})
}

func statusRank(s Status) int {
	for i, x := range AllStatuses {
		if x == s {
			return i
		}
	}
	return len(AllStatuses)
}

// GroupByStatus buckets tasks in display order.
func GroupByStatus(ts []*Task) map[Status][]*Task {
	g := make(map[Status][]*Task, len(AllStatuses))
	for _, t := range ts {
		g[t.Status] = append(g[t.Status], t)
	}
	return g
}
