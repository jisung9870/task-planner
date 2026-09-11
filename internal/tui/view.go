package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"task-planner/internal/domain"
	"task-planner/internal/query"
)

// pad left-aligns to a display width; %-Ns counts bytes and misaligns on CJK.
//
// Width is measured with lipgloss.Width, the same function lipgloss uses when
// it pads a column. Mixing measurement libraries makes board lanes drift by a
// cell wherever the two disagree (⏱ and … are the usual culprits).
func pad(s string, w int) string {
	d := w - lipgloss.Width(s)
	if d <= 0 {
		return s
	}
	return s + strings.Repeat(" ", d)
}

// View renders the whole screen: header, tabs, list, optional detail, footer.
func (m *Model) View() string {
	if m.quit {
		return ""
	}
	if m.mode == modeHelp {
		return helpFull + "\n\n" + styHelp.Render("아무 키나 누르면 돌아갑니다") + "\n"
	}

	var b strings.Builder
	b.WriteString(m.header())
	b.WriteString("\n")
	b.WriteString(m.statsLine())
	b.WriteString("\n")
	b.WriteString(m.tabs())
	b.WriteString("\n")
	b.WriteString(styRule.Render(strings.Repeat("─", m.innerWidth())))
	b.WriteString("\n")

	avail := m.bodyHeight()
	switch {
	case m.splitActive():
		b.WriteString(m.splitBody(avail))
	case m.detail && m.tab != tabBoard:
		// Narrow terminal: the detail pane borrows from the same budget so
		// the footer never scrolls off; give it the smaller share.
		detailH := avail * 2 / 5
		if detailH < 4 {
			detailH = 4
		}
		listH := avail - detailH - 1 // -1 for the divider
		if listH < 3 {
			listH = 3
		}
		b.WriteString(m.list(listH))
		b.WriteString("\n")
		b.WriteString(styRule.Render(strings.Repeat("─", m.innerWidth())))
		b.WriteString("\n")
		b.WriteString(clipLines(m.detailPane(), detailH))
	default:
		b.WriteString(m.list(avail))
	}
	b.WriteString("\n")
	b.WriteString(styRule.Render(strings.Repeat("─", m.innerWidth())))
	b.WriteString("\n")
	b.WriteString(m.footer())
	return b.String()
}

// wide reports whether the terminal can afford a side-by-side split.
func (m *Model) wide() bool { return m.innerWidth() >= 100 }

// splitActive: the right-hand detail pane renders on wide terminals for list
// tabs whenever a task is selected. Grid tabs draw their own columns.
func (m *Model) splitActive() bool {
	return m.wide() && m.wideDetail && !m.gridTab() && m.current() != nil
}

// splitBody renders list and detail side by side, both clipped to the height
// budget and to their column widths (ANSI-aware via lipgloss MaxWidth).
func (m *Model) splitBody(avail int) string {
	width := m.innerWidth()
	listW := width * 11 / 20
	detailW := width - listW - 3 // " │ " divider

	// lipgloss Width() word-wraps long lines; a list row must truncate instead,
	// so fit each line by hand with the ANSI-aware truncator.
	left := fitBlock(m.list(avail), listW)
	right := fitBlock(clipLines(m.detailPane(), avail), detailW)

	divider := strings.TrimRight(strings.Repeat(styRule.Render("│")+"\n", avail), "\n")
	return lipgloss.JoinHorizontal(lipgloss.Top, left, " ", divider, " ", right)
}

// fitBlock truncates and pads every line of a block to exactly w columns, so a
// horizontal join produces a straight divider regardless of content.
func fitBlock(block string, w int) string {
	lines := strings.Split(block, "\n")
	for i, l := range lines {
		l = ansi.Truncate(l, w, "…")
		if d := w - lipgloss.Width(l); d > 0 {
			l += strings.Repeat(" ", d)
		}
		lines[i] = l
	}
	return strings.Join(lines, "\n")
}

// bodyHeight is the line budget left for the body once the fixed chrome
// (header, tabs, rules, footer) is subtracted.
func (m *Model) bodyHeight() int {
	h := m.height
	if h <= 0 {
		h = 24
	}
	chrome := 4 + 3 // header/stats/tabs/rule + rule/footer help line
	if m.status != "" || m.errMsg != "" || m.mode == modeCapture || m.mode == modeBlock || m.mode == modeSearch {
		chrome++
	}
	avail := h - chrome
	if avail < 4 {
		avail = 4
	}
	return avail
}

// clipLines truncates a multi-line block to max lines, marking the cut.
func clipLines(s string, max int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) <= max {
		return strings.Join(lines, "\n")
	}
	out := lines[:max-1]
	return strings.Join(out, "\n") + "\n" + styMuted.Render(fmt.Sprintf("  … %d줄 더 (e 편집기로 열람)", len(lines)-max+1))
}

func (m *Model) innerWidth() int {
	if m.width <= 0 {
		return 78
	}
	if m.width > 140 {
		return 140
	}
	return m.width - 1
}

func (m *Model) header() string {
	today := m.svc.Today()
	left := styTitle.Render("task-planner")
	right := styDate.Render(fmt.Sprintf("%s (%s) %s", today, today.WeekdayKO(), today.WeekLabel()))
	gap := m.innerWidth() - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
}

func (m *Model) tabs() string {
	parts := make([]string, 0, len(tabNames)+1)
	for i, name := range tabNames {
		label := fmt.Sprintf("[%d]%s", i+1, name)
		if tab(i) == m.tab {
			parts = append(parts, styTabActive.Render(label))
		} else {
			parts = append(parts, styTabInactive.Render(label))
		}
	}
	line := strings.Join(parts, "  ")
	var suffix []string
	if m.search != "" {
		suffix = append(suffix, "필터:"+m.search)
	}
	if m.projDrill {
		suffix = append(suffix, "프로젝트:"+query.ProjectLabel(m.projSlug))
	}
	if len(suffix) > 0 {
		line += "   " + styMuted.Render(strings.Join(suffix, "  "))
	}
	return line
}

// statsLine is the morning briefing: what is due, what slipped, what is in
// flight, what has been stuck. Zero-valued signals are omitted - a row of
// zeros is noise, and the point is that anything printed here needs a look.
func (m *Model) statsLine() string {
	sum := m.svc.Summarize()
	var parts []string
	if sum.DueToday > 0 {
		parts = append(parts, styBlocked.Render(fmt.Sprintf("오늘마감 %d", sum.DueToday)))
	}
	if sum.Overdue > 0 {
		parts = append(parts, styDanger.Render(fmt.Sprintf("마감초과 %d", sum.Overdue)))
	}
	parts = append(parts, m.wipNote())
	if sum.Blocked > 0 {
		label := fmt.Sprintf("보류 %d", sum.Blocked)
		if sum.BlockedMaxDay > 0 {
			label += fmt.Sprintf(" (최장 %d일)", sum.BlockedMaxDay)
		}
		if sum.BlockedMaxDay >= 7 {
			parts = append(parts, styDanger.Render(label))
		} else {
			parts = append(parts, styBlocked.Render(label))
		}
	}
	if sum.Carried > 0 {
		parts = append(parts, styMuted.Render(fmt.Sprintf("이월 %d", sum.Carried)))
	}
	return " " + strings.Join(parts, styMuted.Render("  ·  "))
}

func (m *Model) wipNote() string {
	w := m.svc.WIP()
	if w.Limit <= 0 {
		return fmt.Sprintf("WIP %d", w.Count)
	}
	label := fmt.Sprintf("WIP %d/%d", w.Count, w.Limit)
	if w.Exceeded() {
		return styDanger.Render(label + " ⚠")
	}
	return label
}

func (m *Model) list(avail int) string {
	if m.tab == tabBoard {
		return m.board(avail)
	}
	if m.tab == tabWeek {
		return m.weekGrid(avail)
	}
	if len(m.rows) == 0 {
		return styMuted.Render("  (표시할 항목 없음 — a 로 추가)")
	}
	today := m.svc.Today()

	m.ensureCursorVisible(avail)
	start, end, above, below := m.viewport(avail)

	var b strings.Builder
	if above > 0 {
		b.WriteString(styMuted.Render(fmt.Sprintf("  ↑ %d줄", above)) + "\n")
	}
	for i := start; i < end; i++ {
		r := m.rows[i]
		switch {
		case r.proj != nil:
			b.WriteString(m.renderProjRow(i, r))
		case r.task != nil:
			b.WriteString(m.renderTaskRow(i, r.task, today))
		default:
			b.WriteString(styGroup.Render("▾ " + r.header))
		}
		b.WriteString("\n")
	}
	if below > 0 {
		b.WriteString(styMuted.Render(fmt.Sprintf("  ↓ %d줄", below)) + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// viewport resolves the visible slice for the current offset: which rows fit,
// and how many are hidden on each side (each hidden side costs one indicator
// line from the budget).
func (m *Model) viewport(avail int) (start, end, above, below int) {
	total := len(m.rows)
	start = m.listOffset
	if start > total {
		start = total
	}
	vis := avail
	if start > 0 {
		vis--
	}
	if start+vis < total {
		vis--
	}
	if vis < 1 {
		vis = 1
	}
	end = start + vis
	if end > total {
		end = total
	}
	return start, end, start, total - end
}

// ensureCursorVisible slides the offset so the cursor stays inside the
// viewport. Run twice because indicator lines change the capacity, which can
// change whether an indicator is needed.
func (m *Model) ensureCursorVisible(avail int) {
	if m.listOffset > len(m.rows) {
		m.listOffset = 0
	}
	for i := 0; i < 2; i++ {
		start, end, _, _ := m.viewport(avail)
		switch {
		case m.cursor < start:
			m.listOffset = m.cursor
			// Pull the group header above the cursor into view when adjacent:
			// a task row with its heading is far easier to read.
			if m.listOffset > 0 && m.rows[m.listOffset-1].header != "" {
				m.listOffset--
			}
		case m.cursor >= end:
			m.listOffset += m.cursor - end + 1
		}
		if m.listOffset < 0 {
			m.listOffset = 0
		}
	}
}

func (m *Model) renderTaskRow(i int, t *domain.Task, today domain.Date) string {
	cursor := "  "
	if i == m.cursor {
		cursor = stySelected.Render("▸ ")
	}
	glyph := t.Status.Glyph()
	body := fmt.Sprintf("%s %s %s", glyph, pad(t.ShortID(), 5), t.Title)

	var meta []string
	if t.Project != "" {
		meta = append(meta, t.Project)
	}
	if t.Priority != "" {
		meta = append(meta, string(t.Priority))
	}
	if !t.Estimate.IsZero() {
		meta = append(meta, "~"+t.Estimate.String())
	}
	line := body
	if t.Status == domain.StatusDoing && t.StartedAt != nil {
		meta = append(meta, "⏱"+t.ElapsedActual(m.svc.Now()).String())
	} else if !t.Actual.IsZero() {
		meta = append(meta, "⏱"+t.Actual.String())
	}
	if len(meta) > 0 {
		line += "  " + styMuted.Render(strings.Join(meta, " · "))
	}
	if note := m.dueNote(t, today); note != "" {
		line += "  " + note
	}
	if t.Recur != "" && t.IsOpen() {
		line += "  " + styMuted.Render("↻")
	}
	if n := t.RolloverCount; n >= m.svc.Cfg.RolloverWarnAt && t.IsOpen() {
		line += "  " + styBlocked.Render(fmt.Sprintf("↻%d", n))
	}
	if t.Status == domain.StatusBlocked {
		line += "  " + styBlocked.Render("← "+m.blockNote(t, today))
	}
	if n := len(m.svc.Blocking(t.ID)); n > 0 && t.IsOpen() {
		line += "  " + styMuted.Render(fmt.Sprintf("→%d대기", n))
	}

	switch t.Status {
	case domain.StatusDoing:
		line = styDoing.Render(glyph) + line[len(glyph):]
	case domain.StatusDone, domain.StatusCancelled:
		line = styDoneRow.Render(stripStyles(line))
	}
	return "  " + cursor + line
}

func (m *Model) blockNote(t *domain.Task, today domain.Date) string {
	reason := t.BlockedReason
	if reason == "" && len(t.BlockedBy) > 0 {
		reason = "선행 " + strings.Join(domain.ShortRefs(t.BlockedBy), ", ")
	}
	if d := t.BlockedDays(today); d > 0 {
		reason = fmt.Sprintf("%s (%d일 경과)", reason, d)
	}
	return reason
}

func (m *Model) dueNote(t *domain.Task, today domain.Date) string {
	if t.Due.IsZero() || !t.IsOpen() {
		return ""
	}
	d := t.Due.DaysUntil(today)
	switch {
	case d < 0:
		return styDanger.Render(fmt.Sprintf("마감 %d일 초과", -d))
	case d == 0:
		return styDanger.Render("오늘 마감")
	case d <= m.svc.Cfg.DueSoonDays:
		return styBlocked.Render(fmt.Sprintf("D-%d", d))
	}
	return styMuted.Render("~" + t.Due.String())
}

// board renders the kanban lanes side by side.
func (m *Model) board(avail int) string {
	width := m.innerWidth()
	colWidth := (width - 2*(len(boardColumns)-1)) / len(boardColumns)
	today := m.svc.Today()
	// Cards are two lines; reserve the column header (2) and footer (1).
	maxCards := (avail - 3) / 2
	if maxCards < 1 {
		maxCards = 1
	}
	if colWidth < 18 {
		// Narrow terminals stack the lanes instead of rendering unreadable
		// slivers. Cursor addressing is unchanged, so keys behave the same.
		return m.boardStacked(width, today)
	}
	panes := make([]string, len(boardColumns))
	for i, st := range boardColumns {
		panes[i] = m.boardColumn(i, st, colWidth, maxCards, today)
	}
	out := lipgloss.JoinHorizontal(lipgloss.Top, panes...)
	return strings.TrimRight(out, "\n") + "\n" + m.boardFooter(today)
}

// boardStacked renders the same lanes one under another for narrow terminals.
func (m *Model) boardStacked(width int, today domain.Date) string {
	var b strings.Builder
	for i, st := range boardColumns {
		b.WriteString(styGroup.Render(fmt.Sprintf("▾ %s (%d)", st.Label(), len(m.cols[i]))) + "\n")
		for r, t := range m.cols[i] {
			b.WriteString(m.card(t, width, i == m.colCursor && r == m.rowCursor, today) + "\n")
		}
	}
	return b.String() + m.boardFooter(today)
}

// boardColumn renders one lane, windowed around the cursor when the lane holds
// more cards than fit.
func (m *Model) boardColumn(idx int, st domain.Status, width, maxCards int, today domain.Date) string {
	cards := m.cols[idx]
	head := fmt.Sprintf("%s %s (%d)", st.Glyph(), st.Label(), len(cards))
	if st == domain.StatusDoing && m.svc.Cfg.WIPLimit > 0 && len(cards) > m.svc.Cfg.WIPLimit {
		head += " ⚠"
	}
	var b strings.Builder
	b.WriteString(styGroup.Render(truncate(head, width)) + "\n")
	b.WriteString(styRule.Render(strings.Repeat("─", width)) + "\n")
	if len(cards) == 0 {
		b.WriteString(styMuted.Render(truncate("  (없음)", width)) + "\n")
	}
	start := 0
	if idx == m.colCursor && m.rowCursor >= maxCards {
		start = m.rowCursor - maxCards + 1
	}
	end := start + maxCards
	if end > len(cards) {
		end = len(cards)
	}
	if start > 0 {
		b.WriteString(styMuted.Render(fmt.Sprintf("  ↑ %d건", start)) + "\n")
	}
	for r := start; r < end; r++ {
		selected := idx == m.colCursor && r == m.rowCursor
		b.WriteString(m.card(cards[r], width, selected, today) + "\n")
	}
	if end < len(cards) {
		b.WriteString(styMuted.Render(fmt.Sprintf("  ↓ %d건", len(cards)-end)) + "\n")
	}
	return lipgloss.NewStyle().Width(width).MarginRight(2).Render(b.String())
}

// card is the two-line cell used on the board.
func (m *Model) card(t *domain.Task, width int, selected bool, today domain.Date) string {
	marker := "  "
	if selected {
		marker = stySelected.Render("▸ ")
	}
	title := truncate(t.Title, width-4)
	head := marker + title
	if selected {
		head = marker + stySelected.Render(title)
	}

	var meta []string
	meta = append(meta, t.ShortID())
	if t.Project != "" {
		meta = append(meta, t.Project)
	}
	if t.Priority != "" {
		meta = append(meta, string(t.Priority))
	}
	if t.Status == domain.StatusDoing && t.StartedAt != nil {
		meta = append(meta, "⏱"+t.ElapsedActual(m.svc.Now()).String())
	}
	if t.Recur != "" {
		meta = append(meta, "↻")
	}
	if n := t.RolloverCount; n >= m.svc.Cfg.RolloverWarnAt {
		meta = append(meta, fmt.Sprintf("↻%d", n))
	}
	sub := "    " + truncate(strings.Join(meta, " · "), width-6)
	line := styMuted.Render(sub)
	if t.Overdue(today) {
		line = styDanger.Render(sub)
	}
	return head + "\n" + line
}

// boardFooter summarises what the lanes deliberately leave out.
func (m *Model) boardFooter(today domain.Date) string {
	doneToday, cancelled := 0, 0
	for _, t := range m.svc.All() {
		switch {
		case t.Status == domain.StatusDone && t.Completed.Equal(today):
			doneToday++
		case t.Status == domain.StatusCancelled && t.Completed.Equal(today):
			cancelled++
		}
	}
	return styMuted.Render(fmt.Sprintf("오늘 완료 %d · 취소 %d   h/l 열 이동  j/k 카드 이동",
		doneToday, cancelled))
}

// truncate cuts to a display width, appending an ellipsis when it had to cut.
// ANSI-aware: styled input keeps its escape sequences intact.
func truncate(s string, w int) string {
	if w <= 0 {
		return ""
	}
	return ansi.Truncate(s, w, "…")
}

// weekGrid renders 요일 7컬럼 + 하단 미배정 lane. Answers "이번 주 뭐가 어디
// 배치돼 있나" with an actual time axis - the grouped list could not.
func (m *Model) weekGrid(avail int) string {
	width := m.innerWidth()
	colW := (width - 2*6) / 7
	today := m.svc.Today()
	if colW < 13 {
		return m.weekStacked(width, today)
	}

	// The 미배정 lane takes its share off the top of the budget.
	lane := m.cols[weekLaneUnassigned]
	laneMax := 3
	laneH := 0
	if len(lane) > 0 {
		laneH = 1 + min(len(lane), laneMax)
		if len(lane) > laneMax {
			laneH++ // ↓ indicator
		}
	}
	dayAvail := avail - laneH
	maxCards := (dayAvail - 2) / 2
	if maxCards < 1 {
		maxCards = 1
	}

	panes := make([]string, 7)
	for i := range m.weekDays {
		panes[i] = m.weekColumn(i, colW, maxCards, today)
	}
	out := strings.TrimRight(lipgloss.JoinHorizontal(lipgloss.Top, panes...), "\n")
	if laneH > 0 {
		out += "\n" + m.weekLane(lane, width, laneMax)
	}
	return out
}

// weekColumn renders one day.
func (m *Model) weekColumn(idx, width, maxCards int, today domain.Date) string {
	d := m.weekDays[idx]
	cards := m.cols[idx]
	head := fmt.Sprintf("%s %s", d.WeekdayKO(), d.Time().Format("01-02"))
	if len(cards) > 0 {
		head += fmt.Sprintf(" %d", len(cards))
	}
	var b strings.Builder
	if d.Equal(today) {
		b.WriteString(styTabActive.Render(truncate("▾ "+head, width)) + "\n")
	} else {
		b.WriteString(styGroup.Render(truncate("  "+head, width)) + "\n")
	}
	b.WriteString(styRule.Render(strings.Repeat("─", width)) + "\n")
	start := 0
	if idx == m.colCursor && m.rowCursor >= maxCards {
		start = m.rowCursor - maxCards + 1
	}
	end := start + maxCards
	if end > len(cards) {
		end = len(cards)
	}
	if start > 0 {
		b.WriteString(styMuted.Render(fmt.Sprintf(" ↑%d", start)) + "\n")
	}
	for r := start; r < end; r++ {
		b.WriteString(m.card(cards[r], width, idx == m.colCursor && r == m.rowCursor, today) + "\n")
	}
	if end < len(cards) {
		b.WriteString(styMuted.Render(fmt.Sprintf(" ↓%d", len(cards)-end)) + "\n")
	}
	return lipgloss.NewStyle().Width(width).MarginRight(2).Render(b.String())
}

// weekLane renders the 미배정 strip: work that belongs to the week but has no
// day yet. `]` pulls a task onto today.
func (m *Model) weekLane(lane []*domain.Task, width, laneMax int) string {
	today := m.svc.Today()
	var b strings.Builder
	head := fmt.Sprintf("미배정 (%d)", len(lane))
	if m.colCursor == weekLaneUnassigned {
		b.WriteString(styTabActive.Render("▾ "+head) + styMuted.Render("   ] 로 오늘에 배정") + "\n")
	} else {
		b.WriteString(styGroup.Render("▾ "+head) + "\n")
	}
	start := 0
	if m.colCursor == weekLaneUnassigned && m.rowCursor >= laneMax {
		start = m.rowCursor - laneMax + 1
	}
	end := min(start+laneMax, len(lane))
	for r := start; r < end; r++ {
		t := lane[r]
		cursor := "  "
		if m.colCursor == weekLaneUnassigned && r == m.rowCursor {
			cursor = stySelected.Render("▸ ")
		}
		line := fmt.Sprintf("%s %s %s", t.Status.Glyph(), pad(t.ShortID(), 5), t.Title)
		if t.Project != "" {
			line += "  " + styMuted.Render(t.Project)
		}
		if note := m.dueNote(t, today); note != "" {
			line += "  " + note
		}
		b.WriteString("  " + cursor + truncate(line, width-6) + "\n")
	}
	if rest := len(lane) - end + start; rest > 0 {
		b.WriteString(styMuted.Render(fmt.Sprintf("    ↓ %d건", rest)) + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// weekStacked lists the days vertically for narrow terminals.
func (m *Model) weekStacked(width int, today domain.Date) string {
	var b strings.Builder
	for i, d := range m.weekDays {
		if len(m.cols[i]) == 0 && !d.Equal(today) {
			continue // an empty past/future day is noise in a narrow screen
		}
		mark := "  "
		if d.Equal(today) {
			mark = "▾ "
		}
		b.WriteString(styGroup.Render(fmt.Sprintf("%s%s %s (%d)", mark, d.WeekdayKO(), d.Time().Format("01-02"), len(m.cols[i]))) + "\n")
		for r, t := range m.cols[i] {
			b.WriteString(m.card(t, width, i == m.colCursor && r == m.rowCursor, today) + "\n")
		}
	}
	if lane := m.cols[weekLaneUnassigned]; len(lane) > 0 {
		b.WriteString(m.weekLane(lane, width, len(lane)))
	}
	return strings.TrimRight(b.String(), "\n")
}

func (m *Model) renderProjRow(i int, r row) string {
	cursor := "  "
	if i == m.cursor {
		cursor = stySelected.Render("▸ ")
	}
	c := r.proj
	line := fmt.Sprintf("%s 열림 %-3d 진행 %-3d 보류 %-3d 완료 %-3d", pad(query.ProjectLabel(c.Slug), 24), c.Open, c.Doing, c.Blocked, c.Done)
	if c.Overdue > 0 {
		line += "  " + styDanger.Render(fmt.Sprintf("마감초과 %d", c.Overdue))
	}
	return "  " + cursor + line
}

func (m *Model) detailPane() string {
	t := m.current()
	if t == nil {
		return styMuted.Render("  (선택된 태스크 없음)") + "\n"
	}
	full, err := m.svc.Load(t.ID)
	if err != nil {
		return styErr.Render("  "+err.Error()) + "\n"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "  %s  %s\n", styTitle.Render(full.ID), full.Title)
	fmt.Fprintf(&b, "  %s\n", styMuted.Render(detailMeta(full)))
	if note := full.Note(); note != "" {
		b.WriteString("\n")
		for _, l := range strings.Split(note, "\n") {
			b.WriteString("  " + l + "\n")
		}
	}
	if known, missing := m.svc.Blockers(full); len(known)+len(missing) > 0 {
		b.WriteString("\n  " + styGroup.Render("선행") + "\n")
		for _, d := range known {
			b.WriteString("  " + styMuted.Render(fmt.Sprintf("%s %s  %s", d.Status.Glyph(), d.ShortID(), d.Title)) + "\n")
		}
		for _, id := range missing {
			b.WriteString("  " + styDanger.Render("? "+id+"  (인덱스에 없음)") + "\n")
		}
	}
	if blocking := m.svc.Blocking(full.ID); len(blocking) > 0 {
		b.WriteString("\n  " + styGroup.Render("후행") + "\n")
		for _, d := range blocking {
			b.WriteString("  " + styMuted.Render(fmt.Sprintf("%s %s  %s", d.Status.Glyph(), d.ShortID(), d.Title)) + "\n")
		}
	}
	if logs := full.LogLines(); len(logs) > 0 {
		b.WriteString("\n  " + styGroup.Render("Log") + "\n")
		start := 0
		if len(logs) > 5 {
			start = len(logs) - 5
		}
		for _, l := range logs[start:] {
			b.WriteString("  " + styMuted.Render("· "+l) + "\n")
		}
	}
	b.WriteString("\n  " + styMuted.Render(full.Path) + "\n")
	return b.String()
}

func detailMeta(t *domain.Task) string {
	var parts []string
	parts = append(parts, "상태 "+t.Status.Label())
	if t.Project != "" {
		parts = append(parts, "프로젝트 "+t.Project)
	}
	if t.Priority != "" {
		parts = append(parts, string(t.Priority))
	}
	if !t.Scheduled.IsZero() {
		parts = append(parts, "예정 "+t.Scheduled.String())
	}
	if !t.Due.IsZero() {
		parts = append(parts, "마감 "+t.Due.String())
	}
	if !t.Estimate.IsZero() {
		parts = append(parts, "예상 "+t.Estimate.String())
	}
	if !t.Actual.IsZero() || t.StartedAt != nil {
		parts = append(parts, "실소요 "+t.ElapsedActual(time.Now()).String())
	}
	if t.Recur != "" {
		parts = append(parts, "반복 "+t.Recur)
	}
	if t.RolloverCount > 0 {
		parts = append(parts, fmt.Sprintf("이월 %d회", t.RolloverCount))
	}
	if len(t.Tags) > 0 {
		parts = append(parts, "#"+strings.Join(t.Tags, " #"))
	}
	if len(t.Links) > 0 {
		parts = append(parts, strings.Join(t.Links, " "))
	}
	return strings.Join(parts, "  ·  ")
}

func (m *Model) footer() string {
	if m.mode == modeCapture || m.mode == modeBlock || m.mode == modeSearch {
		return styPrompt.Render(m.input.Prompt) + m.input.View() + "\n" +
			styHelp.Render("enter 확인  esc 취소")
	}
	msg := ""
	switch {
	case m.errMsg != "":
		msg = styErr.Render("! " + m.errMsg)
	case m.status != "":
		msg = styStatus.Render(m.status)
	}
	if msg != "" {
		return msg + "\n" + styHelp.Render(helpLine)
	}
	return styHelp.Render(helpLine)
}

// stripStyles removes ANSI sequences so a strikethrough row renders uniformly.
func stripStyles(s string) string {
	var b strings.Builder
	skip := false
	for i := 0; i < len(s); i++ {
		switch {
		case s[i] == 0x1b:
			skip = true
		case skip && s[i] == 'm':
			skip = false
		case !skip:
			b.WriteByte(s[i])
		}
	}
	return b.String()
}
