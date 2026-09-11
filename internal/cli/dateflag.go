package cli

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"task-planner/internal/domain"
	"task-planner/internal/service"
)

// parseDateFlag accepts the shorthands worth typing at a prompt on top of the
// canonical YYYY-MM-DD: today/tomorrow, +Nd/+Nw, and weekday names (which mean
// "the next such day", today included).
func parseDateFlag(svc *service.Service, s string) (domain.Date, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return domain.Date{}, nil
	}
	today := svc.Today()
	switch s {
	case "today", "오늘", "t":
		return today, nil
	case "tomorrow", "내일", "tm":
		return today.AddDays(1), nil
	case "yesterday", "어제":
		return today.AddDays(-1), nil
	case "none", "clear", "-":
		return domain.Date{}, nil
	}
	if strings.HasPrefix(s, "+") || strings.HasPrefix(s, "-") {
		sign := 1
		if s[0] == '-' {
			sign = -1
		}
		body := s[1:]
		unit := byte('d')
		if len(body) > 0 {
			if last := body[len(body)-1]; last == 'd' || last == 'w' || last == 'm' {
				unit = last
				body = body[:len(body)-1]
			}
		}
		n, err := strconv.Atoi(body)
		if err != nil {
			return domain.Date{}, fmt.Errorf("상대 날짜 형식 오류: %q (예: +3d, +2w)", s)
		}
		switch unit {
		case 'w':
			return today.AddDays(sign * n * 7), nil
		case 'm':
			return today.AddMonths(sign * n), nil
		default:
			return today.AddDays(sign * n), nil
		}
	}
	if wd, ok := weekdayOf(s); ok {
		d := today
		for i := 0; i < 7; i++ {
			if d.Weekday() == wd {
				return d, nil
			}
			d = d.AddDays(1)
		}
	}
	return domain.ParseDate(s)
}

var weekdayNames = map[string]time.Weekday{
	"mon": time.Monday, "monday": time.Monday, "월": time.Monday,
	"tue": time.Tuesday, "tuesday": time.Tuesday, "화": time.Tuesday,
	"wed": time.Wednesday, "wednesday": time.Wednesday, "수": time.Wednesday,
	"thu": time.Thursday, "thursday": time.Thursday, "목": time.Thursday,
	"fri": time.Friday, "friday": time.Friday, "금": time.Friday,
	"sat": time.Saturday, "saturday": time.Saturday, "토": time.Saturday,
	"sun": time.Sunday, "sunday": time.Sunday, "일": time.Sunday,
}

func weekdayOf(s string) (time.Weekday, bool) {
	wd, ok := weekdayNames[s]
	return wd, ok
}
