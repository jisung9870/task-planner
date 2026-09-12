package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"task-planner/internal/domain"
)

// The Timeline tab is the calendar turned sideways: 한 줄에 태스크 하나, 가로축은
// 날짜다. Week 는 "이 요일에 뭐가 있나" 를 묻고 Timeline 은 "이 일이 언제부터
// 언제까지인가" 를 묻는다 — 같은 데이터의 다른 질문이라 뷰를 나눈다.
//
// 커서 모델은 목록 탭과 같다(m.rows/m.cursor). 그래서 상태 변경·선택·기간 이동
// 키가 여기서도 그대로 듣는다: 새 뷰가 새 조작법을 요구하면 뷰가 아니라 앱이
// 하나 더 늘어난다.
const (
	// tlCell is one day's width. 요일 이름이 한글 한 글자(2셀)이라 2 보다 좁으면
	// 눈금과 막대가 어긋난다.
	tlCell = 2
	// tlLabelMin is the narrowest label gutter worth keeping: 커서·상태·번호·제목.
	// 창 길이는 이 폭을 지키는 선에서만 늘어난다 — 제목이 안 보이는 간트는
	// 그림일 뿐 계획이 아니다.
	tlLabelMin = 26
	// tlMaxWeeks caps the window. 넓은 터미널에서 두 달을 그리면 막대는 길어지고
	// 오늘 근처의 하루 차이는 안 보인다.
	tlMaxWeeks = 4
	// tlHeadLines is what timelineScale occupies: 날짜·요일·눈금.
	tlHeadLines = 3
)

// timelineWindow resolves the visible range. The length is whole weeks so the
// weekday row repeats cleanly under the date row.
func (m *Model) timelineWindow() (domain.Date, int) {
	start := m.tlStart
	if start.IsZero() {
		start = m.svc.Today().WeekStart()
	}
	weeks := (m.innerWidth() - tlLabelMin) / (7 * tlCell)
	if weeks > tlMaxWeeks {
		weeks = tlMaxWeeks
	}
	if weeks < 1 {
		weeks = 1
	}
	return start, weeks * 7
}

// reloadTimeline picks the tasks whose 진행 기간 touches the window. 날짜가 없는
// 태스크는 그릴 자리가 없다 - 건수만 세어 알린다.
func (m *Model) reloadTimeline(today domain.Date) {
	start, days := m.timelineWindow()
	m.tlStart = start
	end := start.AddDays(days - 1)

	ts := m.svc.All()
	if m.filter != nil && !m.filter.Empty() {
		ts = m.svc.ApplyFilter(m.filter, ts)
	}
	m.tlUndated = 0
	rows := make([]row, 0, len(ts))
	for _, t := range ts {
		if t.SpanStart().IsZero() {
			if t.IsOpen() {
				m.tlUndated++
			}
			continue
		}
		if t.SpanOverlaps(start, end) {
			rows = append(rows, row{task: t})
		}
	}
	// 간트는 시작 순으로 읽는다. 상태·우선순위 순으로 뿌리면 막대가 위아래로
	// 튀어서 무엇이 무엇 뒤에 오는지가 사라진다.
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i].task, rows[j].task
		if !a.SpanStart().Equal(b.SpanStart()) {
			return a.SpanStart().Before(b.SpanStart())
		}
		if !a.SpanEnd().Equal(b.SpanEnd()) {
			return a.SpanEnd().Before(b.SpanEnd())
		}
		return a.ID < b.ID
	})
	m.rows = rows
	m.clampCursor()
}

// shiftTimeline moves the window by whole weeks; 0 returns to 이번 주.
func (m *Model) shiftTimeline(weeks int) {
	if weeks == 0 {
		m.tlStart = m.svc.Today().WeekStart()
	} else {
		start, _ := m.timelineWindow()
		m.tlStart = start.AddDays(7 * weeks)
	}
	m.listOffset, m.cursor = 0, 0
	m.reload()
	start, days := m.timelineWindow()
	m.setStatus("타임라인 %s ~ %s · %d건", start, start.AddDays(days-1), len(m.rows))
}

// timelineView draws the chart: 눈금 세 줄 + 태스크 한 줄씩.
func (m *Model) timelineView(avail int) string {
	today := m.svc.Today()
	start, days := m.timelineWindow()
	labelW := m.innerWidth() - days*tlCell - 1
	if labelW < 8 {
		labelW = 8
	}

	var b strings.Builder
	b.WriteString(m.timelineScale(start, days, labelW, today))

	body := avail - tlHeadLines
	if m.tlUndated > 0 {
		body-- // the 미배정 footnote takes its line off the same budget
	}
	if body < 1 {
		body = 1
	}

	if len(m.rows) == 0 {
		b.WriteString(styMuted.Render("  (이 기간에 잡힌 일이 없습니다 — h/l 로 주 이동, t 로 이번 주)"))
		return b.String() + m.timelineFootnote()
	}

	m.ensureCursorVisible(body)
	first, end, above, below := m.viewport(body)
	y := bodyTop + tlHeadLines
	if above > 0 {
		b.WriteString(styMuted.Render(fmt.Sprintf("  ↑ %d건", above)) + "\n")
		y++
	}
	for i := first; i < end; i++ {
		t := m.rows[i].task
		if t == nil {
			continue
		}
		m.hits = append(m.hits, hit{y: y, x0: 0, x1: m.innerWidth(), kind: hitRow, a: i})
		y++
		b.WriteString(m.timelineRow(i, t, start, days, labelW, today) + "\n")
	}
	if below > 0 {
		b.WriteString(styMuted.Render(fmt.Sprintf("  ↓ %d건", below)) + "\n")
	}
	return strings.TrimRight(b.String(), "\n") + m.timelineFootnote()
}

// timelineFootnote reports the work the chart cannot draw. Silently dropping
// undated tasks would make the tab look like the whole backlog.
func (m *Model) timelineFootnote() string {
	if m.tlUndated == 0 {
		return ""
	}
	return "\n" + styMuted.Render(fmt.Sprintf("  날짜 없음 %d건 — D 로 기간을 넣으면 여기에 그려집니다", m.tlUndated))
}

// timelineScale renders the three header lines: 주 시작 날짜, 요일, 눈금.
// The label gutter carries the window caption, which would otherwise need a
// line of its own.
func (m *Model) timelineScale(start domain.Date, days, labelW int, today domain.Date) string {
	caption := fmt.Sprintf("%s ~ %s",
		start.Time().Format("01-02"), start.AddDays(days-1).Time().Format("01-02"))
	if m.tlStart.Equal(today.WeekStart()) {
		caption += " (이번 주부터)"
	}

	// The date row is built as cells so a "9/14" can never push the columns
	// behind it out of line with the weekday row below.
	cells := []rune(strings.Repeat(" ", days*tlCell))
	for i := 0; i < days; i++ {
		d := start.AddDays(i)
		if i != 0 && d.Weekday() != time.Monday {
			continue
		}
		for j, r := range []rune(d.Time().Format("1/2")) {
			if p := i*tlCell + j; p < len(cells) {
				cells[p] = r
			}
		}
	}

	var wk, rule strings.Builder
	for i := 0; i < days; i++ {
		d := start.AddDays(i)
		switch {
		case d.Equal(today):
			wk.WriteString(styTabActive.Render(d.WeekdayKO()))
			rule.WriteString(styDanger.Render("▼") + styRule.Render("─"))
		case d.Weekday() == time.Saturday || d.Weekday() == time.Sunday:
			wk.WriteString(styMuted.Render(d.WeekdayKO()))
			rule.WriteString(styRule.Render("──"))
		case d.Weekday() == time.Monday:
			wk.WriteString(d.WeekdayKO())
			rule.WriteString(styRule.Render("┬─"))
		default:
			wk.WriteString(d.WeekdayKO())
			rule.WriteString(styRule.Render("──"))
		}
	}

	gutter := strings.Repeat(" ", labelW+1)
	return styMuted.Render(pad(truncate(caption, labelW), labelW+1)) + styMuted.Render(string(cells)) + "\n" +
		gutter + wk.String() + "\n" +
		styRule.Render(strings.Repeat("─", labelW+1)) + rule.String() + "\n"
}

// timelineRow is one task: 라벨 + 막대. The bar is clipped to the window and
// marked with ◀ ▶ where the period continues outside it, so a task that runs
// past the edge never reads as one that ends there.
func (m *Model) timelineRow(i int, t *domain.Task, start domain.Date, days, labelW int, today domain.Date) string {
	label := m.timelineLabel(i, t, labelW, today)

	sty := lipgloss.NewStyle()
	switch {
	case t.Status.Terminal():
		sty = styMuted
	case t.Overdue(today):
		sty = styDanger
	case t.Status == domain.StatusDoing:
		sty = styDoing
	case t.Status == domain.StatusBlocked:
		sty = styBlocked
	}

	end := start.AddDays(days - 1)
	var bar strings.Builder
	for i := 0; i < days; i++ {
		d := start.AddDays(i)
		switch {
		case t.InSpan(d):
			cell := "██"
			if i == 0 && t.SpanStart().Before(start) {
				cell = "◀█"
			}
			if i == days-1 && t.SpanEnd().After(end) {
				cell = "█▶"
			}
			bar.WriteString(sty.Render(cell))
		case d.Equal(today):
			// 오늘 열은 막대가 없어도 이어져야 한다: 세로줄이 없으면 막대가
			// 오늘 앞인지 뒤인지 눈으로 세어야 한다.
			bar.WriteString(styRule.Render("┊ "))
		default:
			bar.WriteString("  ")
		}
	}
	return label + " " + bar.String()
}

// timelineLabel fills the gutter: 커서·상태·번호·제목 왼쪽, 마감 경고 오른쪽.
func (m *Model) timelineLabel(i int, t *domain.Task, labelW int, today domain.Date) string {
	cursor := selMark(i == m.cursor, m.marked[t.ID])
	note := m.dueNote(t, today, false)
	inner := labelW - lipgloss.Width(cursor)
	if w := lipgloss.Width(note); w > 0 {
		inner -= w + 1
	}
	if inner < 4 {
		inner = 4
	}

	body := fmt.Sprintf("%s %s %s", t.Status.Glyph(), pad(t.ShortID(), 5), t.Title)
	body = truncate(body, inner)
	switch {
	case i == m.cursor:
		body = stySelected.Render(body)
	case t.Status.Terminal():
		body = styDoneRow.Render(body)
	}
	line := cursor + pad(body, inner)
	if note != "" {
		line += " " + note
	}
	return line
}
