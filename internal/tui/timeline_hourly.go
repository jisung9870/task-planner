package tui

import (
	"fmt"
	"strings"
	"time"

	"task-planner/internal/domain"
)

func (m *Model) hourlyTimeline() bool { return m.tlHourly }

func (m *Model) timelineDay() domain.Date {
	if m.tlDay.IsZero() {
		return m.svc.Today()
	}
	return m.tlDay
}

func (m *Model) timelineScaleLabel() string {
	if m.hourlyTimeline() {
		return "24시간"
	}
	return "일/주"
}

func (m *Model) toggleTimelineScale() {
	selected := m.current()
	if m.hourlyTimeline() {
		m.tlHourly = false
		m.tlStart = m.timelineDay().WeekStart()
	} else {
		// Zoom into today when it is visible; otherwise use the first visible
		// day with work on the selected task, or the window's first day.
		start, days := m.timelineWindow()
		day := m.svc.Today()
		if day.Before(start) || day.After(start.AddDays(days-1)) {
			day = start
		}
		if selected != nil {
			h := m.svc.WorkHistories([]*domain.Task{selected})[selected.ID]
			if !h.InDay(day, m.svc.Today()) {
				for d := start; !d.After(start.AddDays(days - 1)); d = d.AddDays(1) {
					if h.InDay(d, m.svc.Today()) {
						day = d
						break
					}
				}
			}
		}
		m.tlHourly, m.tlDay = true, day
	}
	m.cursor, m.listOffset = 0, 0
	m.reload()
	if selected != nil {
		m.selectID(selected.ID)
	}
}

// Keep all 24 hours visible. Wider terminals give each hour finer cells;
// even a 60-column split pane retains a readable label and hourly resolution.
func (m *Model) hourlyGeometry() (labelW, cellW int) {
	cellW = (m.contentWidth() - tlLabelMin - 3) / 24
	if cellW < 1 {
		cellW = 1
	}
	if cellW > 4 {
		cellW = 4
	}
	labelW = m.contentWidth() - 24*cellW - 3 // separator plus the final "24"
	if labelW < 8 {
		labelW = 8
	}
	return
}

func (m *Model) hourlyScale(labelW int) string {
	day := m.timelineDay()
	_, cellW := m.hourlyGeometry()
	cells := []rune(strings.Repeat(" ", 24*cellW+2))
	step := 3
	if cellW >= 2 {
		step = 2
	}
	if cellW >= 3 {
		step = 1
	}
	for hour := 0; hour <= 24; hour += step {
		copy(cells[hour*cellW:], []rune(fmt.Sprintf("%02d", hour)))
	}
	caption := fmt.Sprintf("  %s (%s) · 00:00~24:00", day, day.WeekdayKO())
	if day.Equal(m.svc.Today()) {
		caption += " · 오늘"
	}
	var rule strings.Builder
	now := m.svc.Now()
	nowCell := int(dayMinute(now, day) * float64(cellW) / 60)
	for i := 0; i < 24*cellW; i++ {
		switch {
		case day.Equal(domain.DateOf(now)) && i == nowCell:
			rule.WriteString(styDanger.Render("▼"))
		case i%cellW == 0:
			rule.WriteString(styRule.Render("┬"))
		default:
			rule.WriteString(styRule.Render("─"))
		}
	}
	return truncate(caption, m.contentWidth()) + "\n" +
		strings.Repeat(" ", labelW+1) + styMuted.Render(string(cells)) + "\n" +
		styRule.Render(strings.Repeat("─", labelW+1)) + rule.String() + "\n"
}

// dayMinute maps local wall-clock time to a 00..24 axis. Calendar comparisons
// avoid assuming every local day is exactly 24 elapsed hours across DST.
func dayMinute(at time.Time, day domain.Date) float64 {
	d := domain.DateOf(at)
	if d.Before(day) {
		return -1
	}
	if d.After(day) {
		return 1440
	}
	return float64(at.Hour()*60+at.Minute()) + float64(at.Second())/60
}

// hourlyBar draws intervals half-open, so a 10:00 stop does not occupy the
// next hour. Point events remain visible even for zero-minute sessions.
func hourlyBar(h domain.WorkHistory, day domain.Date, now time.Time, cellW int) string {
	cells := []rune(strings.Repeat(" ", 24*cellW))
	minuteW := 60 / float64(cellW)
	point := func(at time.Time, glyph rune) {
		minute := dayMinute(at, day)
		if minute >= 0 && minute < 1440 {
			i := int(minute / minuteW)
			if (glyph != '·' && glyph != '┊') || cells[i] == ' ' {
				cells[i] = glyph
			}
		}
	}
	for _, s := range h.Sessions {
		end := s.End
		if end.IsZero() {
			end = now
		}
		if end.Before(s.Start) {
			continue
		}
		a, b := dayMinute(s.Start, day), dayMinute(end, day)
		for i := range cells {
			left, right := float64(i)*minuteW, float64(i+1)*minuteW
			if b > a && a < right && b > left {
				cells[i] = '█'
			}
		}
		point(s.Start, '·')
		if !s.End.IsZero() {
			point(s.End, '·')
		}
		if a < 0 && b > 0 {
			cells[0] = '◀'
		}
		if domain.DateOf(end).After(day) && a < 1440 && b > 0 {
			cells[len(cells)-1] = '▶'
		}
	}
	for _, f := range h.Finishes {
		glyph := '✓'
		if f.Status == domain.StatusCancelled {
			glyph = '×'
		}
		point(f.At, glyph)
	}
	if domain.DateOf(now).Equal(day) {
		point(now, '┊')
	}
	return string(cells)
}

func (m *Model) hourlyRow(i int, t *domain.Task, labelW int) string {
	_, cellW := m.hourlyGeometry()
	h, day := m.tlHistory[t.ID], m.timelineDay()
	bar := hourlyBar(h, day, m.svc.Now(), cellW)
	// A date-only completion cannot be placed at an invented hour.
	if h.Completed.Equal(day) && strings.Trim(bar, " ┊") == "" {
		bar = "시각 미기록"
	}
	sty := styDoing
	if t.Status.Terminal() {
		sty = styMuted
	}
	if t.Status == domain.StatusBlocked {
		sty = styBlocked
	}
	label := m.timelineLabel(i, t, labelW, m.svc.Today())
	return truncate(label+" "+sty.Render(bar), m.contentWidth())
}
