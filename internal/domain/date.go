package domain

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// DateLayout is the only date format written to frontmatter.
const DateLayout = "2006-01-02"

// Date is a calendar day without a time zone component. Tasks are scheduled by
// day, never by instant, so carrying a full timestamp would invite off-by-one
// bugs across DST and machine time zone changes.
type Date struct{ t time.Time }

func NewDate(y int, m time.Month, d int) Date {
	return Date{time.Date(y, m, d, 0, 0, 0, 0, time.UTC)}
}

func DateOf(t time.Time) Date { return NewDate(t.Year(), t.Month(), t.Day()) }

func Today() Date { return DateOf(time.Now()) }

func ParseDate(s string) (Date, error) {
	t, err := time.Parse(DateLayout, strings.TrimSpace(s))
	if err != nil {
		return Date{}, fmt.Errorf("날짜 형식은 YYYY-MM-DD 여야 함: %q", s)
	}
	return DateOf(t), nil
}

func (d Date) IsZero() bool    { return d.t.IsZero() }
func (d Date) Time() time.Time { return d.t }
func (d Date) String() string {
	if d.IsZero() {
		return ""
	}
	return d.t.Format(DateLayout)
}
func (d Date) Before(o Date) bool    { return d.t.Before(o.t) }
func (d Date) After(o Date) bool     { return d.t.After(o.t) }
func (d Date) Equal(o Date) bool     { return d.t.Equal(o.t) }
func (d Date) AddDays(n int) Date    { return DateOf(d.t.AddDate(0, 0, n)) }
func (d Date) AddMonths(n int) Date  { return DateOf(d.t.AddDate(0, n, 0)) }
func (d Date) Weekday() time.Weekday { return d.t.Weekday() }

// DaysUntil is positive when d is in the future relative to from.
func (d Date) DaysUntil(from Date) int {
	return int(d.t.Sub(from.t).Hours() / 24)
}

// WeekStart returns the Monday of d's ISO week.
func (d Date) WeekStart() Date {
	off := (int(d.t.Weekday()) + 6) % 7 // Monday == 0
	return d.AddDays(-off)
}

func (d Date) ISOWeek() (int, int) { return d.t.ISOWeek() }

// WeekLabel renders the ISO week as used in report filenames: 2026-W37.
func (d Date) WeekLabel() string {
	y, w := d.ISOWeek()
	return fmt.Sprintf("%d-W%02d", y, w)
}

var weekdayKO = [...]string{"일", "월", "화", "수", "목", "금", "토"}

func (d Date) WeekdayKO() string { return weekdayKO[int(d.t.Weekday())] }

func (d Date) MarshalYAML() (any, error) {
	if d.IsZero() {
		return nil, nil
	}
	return d.String(), nil
}

func (d *Date) UnmarshalYAML(unmarshal func(any) error) error {
	var raw any
	if err := unmarshal(&raw); err != nil {
		return err
	}
	switch v := raw.(type) {
	case nil:
		*d = Date{}
	case string:
		if strings.TrimSpace(v) == "" {
			*d = Date{}
			return nil
		}
		parsed, err := ParseDate(v)
		if err != nil {
			return err
		}
		*d = parsed
	case time.Time:
		// goccy/go-yaml resolves bare YYYY-MM-DD scalars to time.Time.
		*d = DateOf(v)
	default:
		return fmt.Errorf("날짜로 해석할 수 없음: %v", raw)
	}
	return nil
}

func (d Date) MarshalJSON() ([]byte, error) {
	if d.IsZero() {
		return []byte(`""`), nil
	}
	return []byte(`"` + d.String() + `"`), nil
}

func (d *Date) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "" || s == "null" {
		*d = Date{}
		return nil
	}
	parsed, err := ParseDate(s)
	if err != nil {
		return err
	}
	*d = parsed
	return nil
}

// ParseDateRef reads the shorthands worth typing at a prompt on top of the
// canonical YYYY-MM-DD: MM-DD (올해), today/tomorrow, +Nd/+Nw/+Nm, and weekday
// names ("the next such day", today included). ref is the date relative forms
// count from - normally today, but a span's end counts from its start.
//
// It lives in domain rather than in the CLI because the TUI prompts accept the
// same forms, and an adapter may not import another adapter.
func ParseDateRef(s string, ref Date) (Date, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return Date{}, nil
	}
	switch s {
	case "today", "오늘", "t":
		return ref, nil
	case "tomorrow", "내일", "tm":
		return ref.AddDays(1), nil
	case "yesterday", "어제":
		return ref.AddDays(-1), nil
	case "none", "clear", "해제", "-":
		return Date{}, nil
	}
	if s[0] == '+' || s[0] == '-' {
		return parseRelative(s, ref)
	}
	if wd, ok := weekdayOf(s); ok {
		d := ref
		for i := 0; i < 7; i++ {
			if d.Weekday() == wd {
				return d, nil
			}
			d = d.AddDays(1)
		}
	}
	// MM-DD is the common case at a prompt: the year is almost always the
	// current one, and being explicit beats guessing a rollover the user did
	// not ask for.
	if len(s) == 5 && s[2] == '-' {
		return ParseDate(fmt.Sprintf("%04d-%s", ref.t.Year(), s))
	}
	return ParseDate(s)
}

func parseRelative(s string, ref Date) (Date, error) {
	sign := 1
	if s[0] == '-' {
		sign = -1
	}
	body := s[1:]
	unit := byte('d')
	if len(body) > 0 {
		if last := body[len(body)-1]; last == 'd' || last == 'w' || last == 'm' {
			unit, body = last, body[:len(body)-1]
		}
	}
	n, err := strconv.Atoi(body)
	if err != nil {
		return Date{}, fmt.Errorf("상대 날짜 형식 오류: %q (예: +3d, +2w)", s)
	}
	switch unit {
	case 'w':
		return ref.AddDays(sign * n * 7), nil
	case 'm':
		return ref.AddMonths(sign * n), nil
	default:
		return ref.AddDays(sign * n), nil
	}
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
