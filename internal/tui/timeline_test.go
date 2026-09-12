package tui

import (
	"testing"
	"time"

	"task-planner/internal/domain"
)

// The window is whole weeks and never eats the label gutter: a chart wide
// enough to draw two months and too narrow to read a title is not a plan.
func TestTimelineWindowFitsWidth(t *testing.T) {
	start := domain.NewDate(2026, time.September, 7)
	cases := []struct {
		width, days int
	}{
		{200, 28}, // capped at tlMaxWeeks
		{120, 28},
		{100, 28},
		{80, 21},
		{64, 14},
		{46, 7},
		{20, 7}, // never below one week, even when nothing fits
	}
	for _, c := range cases {
		m := &Model{tlStart: start, width: c.width}
		got, days := m.timelineWindow()
		if days != c.days {
			t.Errorf("width %d: %d일 창, want %d", c.width, days, c.days)
		}
		if days%7 != 0 {
			t.Errorf("width %d: 창이 주 단위가 아님 (%d일)", c.width, days)
		}
		if !got.Equal(start) {
			t.Errorf("width %d: 시작이 %s 로 밀림", c.width, got)
		}
		if label := c.width - days*tlCell - 1; label < 8 && c.width > 40 {
			t.Errorf("width %d: 라벨 폭이 %d 로 눌림", c.width, label)
		}
	}
}
