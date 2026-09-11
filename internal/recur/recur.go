// Package recur parses repeat rules and computes the next occurrence.
//
// The grammar is deliberately small. Full RRULE would cover cases a personal
// planner never has (BYSETPOS, EXDATE) at the cost of a rule nobody can read in
// frontmatter six months later.
package recur

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"task-planner/internal/domain"
)

type kind int

const (
	kindDay kind = iota
	kindWeek
	kindMonth
	kindWeekdays // Mon-Fri
	kindOnDays   // specific weekdays
	kindMonthDay // a day-of-month
)

// Rule is a parsed repeat specification.
type Rule struct {
	kind     kind
	interval int
	days     []time.Weekday
	monthDay int
	src      string
}

func (r Rule) String() string { return r.src }

// Parse compiles a repeat rule.
//
//	daily | 매일          weekly | 매주        monthly | 매월
//	weekdays | 평일       every 3 days         every 2 weeks
//	every monday         every mon,thu        monthly on 15 | 매월 15일
func Parse(expr string) (Rule, error) {
	raw := strings.TrimSpace(expr)
	s := strings.ToLower(raw)
	s = strings.TrimPrefix(s, "every ")
	s = strings.TrimPrefix(s, "매 ")
	r := Rule{interval: 1, src: raw}
	if s == "" {
		return r, fmt.Errorf("반복 규칙이 비어 있음")
	}

	switch s {
	case "day", "daily", "매일":
		r.kind = kindDay
		return r, nil
	case "week", "weekly", "매주":
		r.kind = kindWeek
		return r, nil
	case "month", "monthly", "매월", "매달":
		r.kind = kindMonth
		return r, nil
	case "weekday", "weekdays", "평일":
		r.kind = kindWeekdays
		return r, nil
	}

	// "monthly on 15" / "매월 15일"
	if rest, ok := cutPrefix(s, "monthly on ", "month on ", "매월 ", "매달 "); ok {
		n, err := strconv.Atoi(strings.TrimSuffix(strings.TrimSpace(rest), "일"))
		if err != nil || n < 1 || n > 31 {
			return r, fmt.Errorf("월중 일자는 1~31 이어야 함: %q", rest)
		}
		r.kind, r.monthDay = kindMonthDay, n
		return r, nil
	}

	// "3 days" / "2 weeks" / "2 months"
	if fields := strings.Fields(s); len(fields) == 2 {
		if n, err := strconv.Atoi(fields[0]); err == nil && n > 0 {
			switch strings.TrimSuffix(fields[1], "s") {
			case "day", "일":
				r.kind, r.interval = kindDay, n
				return r, nil
			case "week", "주":
				r.kind, r.interval = kindWeek, n
				return r, nil
			case "month", "달", "개월":
				r.kind, r.interval = kindMonth, n
				return r, nil
			}
		}
	}

	// "monday" / "mon,thu" / "월,목"
	var days []time.Weekday
	for _, part := range strings.Split(s, ",") {
		wd, ok := weekday(strings.TrimSpace(part))
		if !ok {
			days = nil
			break
		}
		days = append(days, wd)
	}
	if len(days) > 0 {
		r.kind, r.days = kindOnDays, days
		return r, nil
	}

	return r, fmt.Errorf("알 수 없는 반복 규칙: %q (daily|weekly|monthly|weekdays|every N days|every monday|monthly on 15)", raw)
}

// Next returns the first occurrence strictly after from.
func (r Rule) Next(from domain.Date) domain.Date {
	switch r.kind {
	case kindDay:
		return from.AddDays(r.interval)
	case kindWeek:
		return from.AddDays(7 * r.interval)
	case kindMonth:
		return from.AddMonths(r.interval)
	case kindWeekdays:
		d := from.AddDays(1)
		for d.Weekday() == time.Saturday || d.Weekday() == time.Sunday {
			d = d.AddDays(1)
		}
		return d
	case kindOnDays:
		for i := 1; i <= 7; i++ {
			d := from.AddDays(i)
			for _, wd := range r.days {
				if d.Weekday() == wd {
					return d
				}
			}
		}
		return from.AddDays(7)
	case kindMonthDay:
		d := from
		for i := 0; i < 62; i++ {
			d = d.AddDays(1)
			if d.Time().Day() == r.monthDay {
				return d
			}
		}
		// Months without the requested day (e.g. the 31st of February) fall
		// through to a month interval rather than silently skipping a year.
		return from.AddMonths(1)
	}
	return from.AddDays(1)
}

// NextAfter returns the first occurrence strictly after both from and floor.
// Completing a long-overdue chore should schedule the next one in the future,
// not replay the backlog one occurrence at a time.
func (r Rule) NextAfter(from, floor domain.Date) domain.Date {
	d := r.Next(from)
	for i := 0; i < 400 && !d.After(floor); i++ {
		d = r.Next(d)
	}
	return d
}

func cutPrefix(s string, prefixes ...string) (string, bool) {
	for _, p := range prefixes {
		if rest, ok := strings.CutPrefix(s, p); ok {
			return rest, true
		}
	}
	return "", false
}

var weekdays = map[string]time.Weekday{
	"mon": time.Monday, "monday": time.Monday, "월": time.Monday, "월요일": time.Monday,
	"tue": time.Tuesday, "tuesday": time.Tuesday, "화": time.Tuesday, "화요일": time.Tuesday,
	"wed": time.Wednesday, "wednesday": time.Wednesday, "수": time.Wednesday, "수요일": time.Wednesday,
	"thu": time.Thursday, "thursday": time.Thursday, "목": time.Thursday, "목요일": time.Thursday,
	"fri": time.Friday, "friday": time.Friday, "금": time.Friday, "금요일": time.Friday,
	"sat": time.Saturday, "saturday": time.Saturday, "토": time.Saturday, "토요일": time.Saturday,
	"sun": time.Sunday, "sunday": time.Sunday, "일": time.Sunday, "일요일": time.Sunday,
}

func weekday(s string) (time.Weekday, bool) {
	wd, ok := weekdays[s]
	return wd, ok
}
