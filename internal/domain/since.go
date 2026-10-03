package domain

import (
	"fmt"
	"strings"
	"time"
)

// sinceMark tags a completion whose start was typed in afterwards. The log keeps
// the line it would have written anyway and adds when the work really began,
// so the history stays append-only and WorkHistory can still draw the session.
const sinceMark = "착수 소급 "

const sinceLayout = "2006-01-02 15:04"

// ParseSince reads "when did this start" as typed at a prompt, relative to now.
//
//	10:30              오늘 10:30
//	2h, 1h30m, 45m     지금부터 그만큼 전
//	어제 14:00         날짜 표현(ParseDateRef) + 시각
//	2026-10-01 14:00   그대로
//
// A start in the future is refused: it would record negative work.
func ParseSince(s string, now time.Time) (time.Time, error) {
	s = strings.TrimSpace(s)
	if len(s) > 10 && s[10] == 'T' { // 2026-10-01T14:00
		s = s[:10] + " " + s[11:]
	}
	if s == "" {
		return time.Time{}, fmt.Errorf("착수 시각이 비어 있음")
	}
	// A duration needs its unit: a bare "1030" typed at this prompt means
	// 10:30, not 1030 minutes ago.
	if strings.ContainsAny(s, "hmd") && !strings.Contains(s, ":") {
		if d, err := ParseDuration(s); err == nil && d > 0 {
			return now.Add(-time.Duration(d)).Truncate(time.Minute), nil
		}
	}
	day, clock := DateOf(now), s
	if i := strings.LastIndexByte(s, ' '); i >= 0 {
		d, err := ParseDateRef(s[:i], DateOf(now))
		if err != nil {
			return time.Time{}, err
		}
		if d.IsZero() { // "- 14:00", "none 14:00": a cleared date is no date
			return time.Time{}, fmt.Errorf("착수 날짜를 읽을 수 없음: %q", s[:i])
		}
		day, clock = d, strings.TrimSpace(s[i+1:])
	}
	if n := len(clock); (n == 3 || n == 4) && strings.Trim(clock, "0123456789") == "" {
		clock = clock[:n-2] + ":" + clock[n-2:] // 1030 → 10:30, 930 → 9:30
	}
	hm, err := time.Parse("15:04", clock)
	if err != nil {
		return time.Time{}, fmt.Errorf("착수 시각을 읽을 수 없음: %q (예: 10:30, 2h, 어제 14:00)", s)
	}
	at := time.Date(day.Time().Year(), day.Time().Month(), day.Time().Day(),
		hm.Hour(), hm.Minute(), 0, 0, now.Location())
	if at.After(now) {
		return time.Time{}, fmt.Errorf("착수 시각(%s)이 지금보다 나중임", at.Format(sinceLayout))
	}
	return at, nil
}

// parseSinceMark finds a backfilled start in a log line.
func parseSinceMark(line string, loc *time.Location) (time.Time, bool) {
	i := strings.Index(line, "("+sinceMark)
	if i < 0 {
		return time.Time{}, false
	}
	rest := line[i+1+len(sinceMark):]
	if len(rest) < len(sinceLayout) {
		return time.Time{}, false
	}
	at, err := time.ParseInLocation(sinceLayout, rest[:len(sinceLayout)], loc)
	return at, err == nil
}
