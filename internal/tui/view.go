package tui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"task-planner/internal/domain"
	"task-planner/internal/query"
	"task-planner/internal/service"
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
		return m.helpView()
	}
	// Hit regions describe this frame only; a click is always answered against
	// what is currently on screen.
	m.hits = m.hits[:0]

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
	var body string
	switch {
	case m.mode == modeForm:
		body = m.formPane()
	case m.mode == modeViews:
		body = m.viewsPane()
	case m.splitActive():
		body = m.splitBody(avail)
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
		body = fitHeight(m.list(listH), listH) + "\n" +
			styRule.Render(strings.Repeat("─", m.innerWidth())) + "\n" +
			m.detailBlock(detailH)
	default:
		body = m.list(avail)
	}
	b.WriteString(fitHeight(body, avail))
	b.WriteString("\n")
	b.WriteString(styRule.Render(strings.Repeat("─", m.innerWidth())))
	b.WriteString("\n")
	b.WriteString(m.footer())
	return b.String()
}

// helpView windows the help text to the terminal height. The text is longer
// than a 30-row terminal, and the keys it describes are at the top - clipping
// from the bottom without a way to scroll would hide exactly what is needed.
func (m *Model) helpView() string {
	lines := helpLines()
	h := m.height
	if h <= 0 {
		h = 24
	}
	avail := h - 2 // the footer line plus its blank separator
	if avail < 3 {
		avail = 3
	}
	if max := len(lines) - avail; m.helpOffset > max {
		m.helpOffset = max
	}
	if m.helpOffset < 0 {
		m.helpOffset = 0
	}
	end := m.helpOffset + avail
	if end > len(lines) {
		end = len(lines)
	}
	foot := "아무 키나 누르면 돌아갑니다"
	if len(lines) > avail {
		foot = fmt.Sprintf("↑/↓ 스크롤 (%d-%d / %d줄)  ·  다른 키를 누르면 돌아갑니다",
			m.helpOffset+1, end, len(lines))
	}
	return strings.Join(lines[m.helpOffset:end], "\n") + "\n" + styHelp.Render(foot) + "\n"
}

// viewsPane is the saved-view picker. Views live in config.yaml; the picker is
// what makes them worth having, since a query you have to retype is a query
// you stop using.
func (m *Model) viewsPane() string {
	views := m.svc.Views()
	var b strings.Builder
	b.WriteString(styGroup.Render("저장된 뷰") + "\n\n")
	if len(views) == 0 {
		b.WriteString(styMuted.Render("  (없음)  / 로 걸러본 뒤 v → s 로 저장하세요") + "\n")
	}
	for i, v := range views {
		cursor := "  "
		if i == m.viewCursor {
			cursor = stySelected.Render("▸ ")
		}
		num := " "
		if i < 9 {
			num = fmt.Sprint(i + 1)
		}
		line := fmt.Sprintf("%s %s  %s", styMuted.Render(num), pad(v.Name, 14), styMuted.Render(v.Query))
		if i == m.viewCursor {
			line = fmt.Sprintf("%s %s  %s", styMuted.Render(num), stySelected.Render(pad(v.Name, 14)), styMuted.Render(v.Query))
		}
		b.WriteString("  " + cursor + line + "\n")
	}
	b.WriteString("\n" + styHelp.Render("1-9 또는 enter 선택  ·  s 현재 필터 저장  ·  d 삭제  ·  esc 닫기"))
	return b.String()
}

// formHints show what each empty field accepts, in place of its value.
var formHints = [formCount]string{
	"(필수) 짧은 명사구 한 줄",
	"(선택) 본문 ## Note 로 들어갑니다",
	"(선택) 09-15~09-19 · today~+4d · 09-15",
	"(선택) 쉼표로 구분 (ops, backend)",
}

// formPane is the Jira-style capture: every field on one screen, the focused
// one live. a 의 3초 캡처를 대체하지 않는다 — 필드를 이미 아는 순간을 위한
// 화면이다.
func (m *Model) formPane() string {
	var b strings.Builder
	b.WriteString(styGroup.Render("새 태스크") + "\n\n")
	if m.projDrill && m.projSlug != "" {
		b.WriteString("  " + styMuted.Render("프로젝트: "+m.projSlug) + "\n\n")
	}
	for i, label := range formLabels {
		if i == m.formFocus {
			b.WriteString(fmt.Sprintf("  %s%s  %s\n", stySelected.Render("▸ "),
				stySelected.Render(pad(label, 4)), m.input.View()))
			continue
		}
		val := m.formVals[i]
		if val == "" {
			val = styMuted.Render(formHints[i])
		}
		b.WriteString(fmt.Sprintf("    %s  %s\n", pad(label, 4), val))
	}
	b.WriteString("\n" + styHelp.Render("enter 다음 필드 (마지막에서 저장)  ·  tab/↑↓ 이동  ·  ctrl+s 바로 저장  ·  esc 취소"))
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
	right := fitBlock(m.detailBlock(avail), detailW)

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

// footerLines is how many lines the footer occupies this frame.
func (m *Model) footerLines() int {
	if m.mode.prompting() || m.mode == modeConfirm {
		return 2 // prompt + hint
	}
	if m.status != "" || m.errMsg != "" {
		return 2 // message + help
	}
	return 1
}

// bodyHeight is the exact line budget for the body: terminal height minus the
// chrome. Exact matters - the body is padded to this size so the footer sits
// on the terminal's last row instead of floating under short content.
func (m *Model) bodyHeight() int {
	h := m.height
	if h <= 0 {
		h = 24
	}
	avail := h - (4 + 1 + m.footerLines()) // header/stats/tabs/rule + rule + footer
	if avail < 4 {
		avail = 4
	}
	return avail
}

// fitHeight pads (or clips) a block to exactly n lines.
func fitHeight(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[:n]
	}
	for len(lines) < n {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

// innerWidth is the full terminal width - the TUI owns the whole screen, like
// any full-screen terminal app. (An earlier 140-column cap left wide terminals
// half empty.)
func (m *Model) innerWidth() int {
	if m.width <= 0 {
		return 78
	}
	return m.width
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

// tabRow is the screen line the tab bar occupies; bodyTop is the first line of
// the body. Both are fixed by View()'s header, so a click can be resolved
// without re-deriving the layout.
const (
	tabRow  = 2
	bodyTop = 4
)

func (m *Model) tabs() string {
	parts := make([]string, 0, len(tabNames)+1)
	x := 0
	for i, name := range tabNames {
		label := fmt.Sprintf("[%d]%s", i+1, name)
		w := lipgloss.Width(label)
		m.hits = append(m.hits, hit{y: tabRow, x0: x, x1: x + w, kind: hitTab, a: i})
		x += w + 2
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
	sum := m.summary
	var parts []string
	if t := m.runningTask(); t != nil {
		parts = append(parts, styDoing.Render(fmt.Sprintf("▶ %s %s %s",
			t.ShortID(), truncate(t.Title, 20), t.ElapsedActual(m.svc.Now()))))
	}
	if sum.DueToday > 0 {
		parts = append(parts, styBlocked.Render(fmt.Sprintf("오늘마감 %d", sum.DueToday)))
	}
	if sum.Overdue > 0 {
		parts = append(parts, styDanger.Render(fmt.Sprintf("마감초과 %d", sum.Overdue)))
	}
	parts = append(parts, m.wipNote())
	if note := m.loadNote(); note != "" {
		parts = append(parts, note)
	}
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

// loadNote reports how much work today is carrying. It is omitted when nothing
// is estimated: "0h" would read as a free day when it actually means unknown.
func (m *Model) loadNote() string {
	l := m.todayLoad
	if l.Estimated == 0 {
		return ""
	}
	label := "배정 " + l.Planned.String()
	if l.Limit > 0 {
		label = fmt.Sprintf("배정 %s/%s", l.Planned, l.Limit)
	}
	if l.Estimated < l.Tasks {
		label += fmt.Sprintf(" (%d/%d)", l.Estimated, l.Tasks)
	}
	if l.Over() {
		return styDanger.Render(label + " ⚠")
	}
	return label
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
	y := bodyTop
	if above > 0 {
		b.WriteString(styMuted.Render(fmt.Sprintf("  ↑ %d줄", above)) + "\n")
		y++
	}
	for i := start; i < end; i++ {
		r := m.rows[i]
		if r.selectable() {
			m.hits = append(m.hits, hit{y: y, x0: 0, x1: m.innerWidth(), kind: hitRow, a: i})
		}
		y++
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

// selMark is the two-cell prefix every selectable line carries: the cursor in
// the first cell, the multi-select tick in the second. Encoding both in a
// fixed width keeps every column below it aligned.
func selMark(cursor, marked bool) string {
	c, k := " ", " "
	if cursor {
		c = "▸"
	}
	if marked {
		k = "✓"
	}
	switch {
	case cursor:
		return stySelected.Render(c + k)
	case marked:
		return styDoing.Render(c + k)
	}
	return c + k
}

func (m *Model) renderTaskRow(i int, t *domain.Task, today domain.Date) string {
	cursor := selMark(i == m.cursor, m.marked[t.ID])
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
	if t.HasSpan() {
		meta = append(meta, t.SpanLabelShort())
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
	if note := m.dueNote(t, today, !t.HasSpan()); note != "" {
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
	if n := m.blockingCount[t.ID]; n > 0 && t.IsOpen() {
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

// dueNote flags the deadline. far decides whether a deadline still comfortably
// ahead is printed at all - a row that already shows the 기간 has said it once.
func (m *Model) dueNote(t *domain.Task, today domain.Date, far bool) string {
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
	case !far:
		return ""
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
		panes[i] = m.boardColumn(i, st, colWidth, maxCards, today, i*(colWidth+2))
	}
	out := lipgloss.JoinHorizontal(lipgloss.Top, panes...)
	return strings.TrimRight(out, "\n") + "\n" + m.boardFooter(today)
}

// boardStacked renders the same lanes one under another for narrow terminals.
func (m *Model) boardStacked(width int, today domain.Date) string {
	var b strings.Builder
	y := bodyTop
	for i, st := range boardColumns {
		b.WriteString(styGroup.Render(fmt.Sprintf("▾ %s (%d)", st.Label(), len(m.cols[i]))) + "\n")
		y++
		for r, t := range m.cols[i] {
			b.WriteString(m.card(t, domain.Date{}, width, i == m.colCursor && r == m.rowCursor, today) + "\n")
			m.hitCard(y, 0, width, i, r)
			y += 2
		}
	}
	return b.String() + m.boardFooter(today)
}

// boardColumn renders one lane, windowed around the cursor when the lane holds
// more cards than fit.
func (m *Model) boardColumn(idx int, st domain.Status, width, maxCards int, today domain.Date, x0 int) string {
	cards := m.cols[idx]
	head := fmt.Sprintf("%s %s (%d)", st.Glyph(), st.Label(), len(cards))
	if st == domain.StatusDoing && m.svc.Cfg.WIPLimit > 0 && len(cards) > m.svc.Cfg.WIPLimit {
		head += " ⚠"
	}
	var b strings.Builder
	y := bodyTop
	b.WriteString(styGroup.Render(truncate(head, width)) + "\n")
	b.WriteString(styRule.Render(strings.Repeat("─", width)) + "\n")
	y += 2
	if len(cards) == 0 {
		b.WriteString(styMuted.Render(truncate("  (없음)", width)) + "\n")
		y++
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
		y++
	}
	for r := start; r < end; r++ {
		selected := idx == m.colCursor && r == m.rowCursor
		b.WriteString(m.card(cards[r], domain.Date{}, width, selected, today) + "\n")
		m.hitCard(y, x0, width, idx, r)
		y += 2
	}
	if end < len(cards) {
		b.WriteString(styMuted.Render(fmt.Sprintf("  ↓ %d건", len(cards)-end)) + "\n")
	}
	return lipgloss.NewStyle().Width(width).MarginRight(2).Render(b.String())
}

// card is the two-line cell used on the board and the week grid.
//
// day is the column's date on the Week grid and the zero Date on the Board.
// A task whose 진행 기간 covers several days draws on each of them; every day
// after the first is a continuation cell, marked so the grid reads as one bar
// rather than as five separate tasks.
func (m *Model) card(t *domain.Task, day domain.Date, width int, selected bool, today domain.Date) string {
	marker := selMark(selected, m.marked[t.ID])
	cont := !day.IsZero() && t.MultiDay() && day.After(t.SpanStart())
	title := t.Title
	if cont {
		title = "╌ " + title
	}
	title = truncate(title, width-4)
	head := marker + title
	switch {
	case selected:
		head = marker + stySelected.Render(title)
	case cont:
		head = marker + styMuted.Render(title)
	}

	var meta []string
	meta = append(meta, t.ShortID())
	if !day.IsZero() && t.MultiDay() {
		// How far into the period this day is, and how much is left. On a
		// narrow column this is the first thing to survive truncation - it is
		// what turns repeated cards into one bar.
		meta = append(meta, fmt.Sprintf("%d/%d일", t.DayIndex(day), t.SpanDays()))
	}
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
	c := m.boardClosed
	return styMuted.Render(fmt.Sprintf("오늘 완료 %d · 취소 %d   h/l 열 이동  H/L 카드를 옆 열로  j/k 카드 이동",
		c.Done, c.Cancelled))
}

// hitCard records the two screen lines a card occupies, so a click selects the
// same card the eye did.
func (m *Model) hitCard(y, x0, width, col, row int) {
	m.hits = append(m.hits,
		hit{y: y, x0: x0, x1: x0 + width, kind: hitCard, a: col, b: row},
		hit{y: y + 1, x0: x0, x1: x0 + width, kind: hitCard, a: col, b: row})
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
		panes[i] = m.weekColumn(i, colW, maxCards, today, i*(colW+2))
	}
	out := strings.TrimRight(lipgloss.JoinHorizontal(lipgloss.Top, panes...), "\n")
	if laneH > 0 {
		out += "\n" + m.weekLane(lane, width, laneMax, bodyTop+lipgloss.Height(out))
	}
	return out
}

// weekColumn renders one day.
func (m *Model) weekColumn(idx, width, maxCards int, today domain.Date, x0 int) string {
	d := m.weekDays[idx]
	cards := m.cols[idx]
	head := fmt.Sprintf("%s %s", d.WeekdayKO(), d.Time().Format("01-02"))
	if len(cards) > 0 {
		head += fmt.Sprintf(" %d", len(cards))
	}

	var b strings.Builder
	y := bodyTop
	if d.Equal(today) {
		b.WriteString(styTabActive.Render(truncate("▾ "+head, width)) + "\n")
	} else {
		b.WriteString(styGroup.Render(truncate("  "+head, width)) + "\n")
	}
	b.WriteString(m.weekRule(idx, width) + "\n")
	y += 2
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
		y++
	}
	for r := start; r < end; r++ {
		b.WriteString(m.card(cards[r], d, width, idx == m.colCursor && r == m.rowCursor, today) + "\n")
		m.hitCard(y, x0, width, idx, r)
		y += 2
	}
	if end < len(cards) {
		b.WriteString(styMuted.Render(fmt.Sprintf(" ↓%d", len(cards)-end)) + "\n")
	}
	return lipgloss.NewStyle().Width(width).MarginRight(2).Render(b.String())
}

// weekRule draws a day column's underline and hangs that day's planned hours
// off its right end. The rule has columns to spare while the header does not,
// and a day's load belongs next to the day, not in a separate legend.
//
// The load is hidden while a filter is active: it counts the whole day's work,
// so printing it beside a filtered card count would look like a contradiction.
func (m *Model) weekRule(idx, width int) string {
	if m.filter != nil && !m.filter.Empty() || idx >= len(m.weekLoads) {
		return styRule.Render(strings.Repeat("─", width))
	}
	l := m.weekLoads[idx]
	tag := compactHours(l.Planned)
	if l.Estimated == 0 || lipgloss.Width(tag)+2 > width {
		return styRule.Render(strings.Repeat("─", width))
	}
	style := styMuted
	if l.Over() {
		style = styDanger
		tag += "!"
	}
	dashes := width - lipgloss.Width(tag) - 1
	return styRule.Render(strings.Repeat("─", dashes)) + " " + style.Render(tag)
}

// compactHours shortens a duration for a grid header, where "3h30m" costs more
// columns than the weekday it sits next to. Minutes below an hour stay minutes:
// "0.5h" is a worse answer to "how long" than "30m".
func compactHours(d domain.Duration) string {
	h := d.Std().Hours()
	if h < 1 {
		return d.String()
	}
	s := strconv.FormatFloat(h, 'f', 1, 64)
	return strings.TrimSuffix(s, ".0") + "h"
}

// weekLane renders the 미배정 strip: work that belongs to the week but has no
// day yet. `]` pulls a task onto today.
func (m *Model) weekLane(lane []*domain.Task, width, laneMax, y int) string {
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
	y++ // the lane heading
	end := min(start+laneMax, len(lane))
	for r := start; r < end; r++ {
		// A lane entry is a single line, not a two-line card.
		m.hits = append(m.hits, hit{y: y, x0: 0, x1: width, kind: hitCard, a: weekLaneUnassigned, b: r})
		y++
		t := lane[r]
		cursor := selMark(m.colCursor == weekLaneUnassigned && r == m.rowCursor, m.marked[t.ID])
		line := fmt.Sprintf("%s %s %s", t.Status.Glyph(), pad(t.ShortID(), 5), t.Title)
		if t.Project != "" {
			line += "  " + styMuted.Render(t.Project)
		}
		if note := m.dueNote(t, today, true); note != "" {
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
	y := bodyTop
	for i, d := range m.weekDays {
		if len(m.cols[i]) == 0 && !d.Equal(today) {
			continue // an empty past/future day is noise in a narrow screen
		}
		mark := "  "
		if d.Equal(today) {
			mark = "▾ "
		}
		b.WriteString(styGroup.Render(fmt.Sprintf("%s%s %s (%d)", mark, d.WeekdayKO(), d.Time().Format("01-02"), len(m.cols[i]))) + "\n")
		y++
		for r, t := range m.cols[i] {
			b.WriteString(m.card(t, d, width, i == m.colCursor && r == m.rowCursor, today) + "\n")
			m.hitCard(y, 0, width, i, r)
			y += 2
		}
	}
	if lane := m.cols[weekLaneUnassigned]; len(lane) > 0 {
		b.WriteString(m.weekLane(lane, width, len(lane), bodyTop+lipgloss.Height(b.String())))
	}
	return strings.TrimRight(b.String(), "\n")
}

func (m *Model) renderProjRow(i int, r row) string {
	cursor := selMark(i == m.cursor, false)
	c := r.proj
	line := fmt.Sprintf("%s %s %s 열림 %-3d 진행 %-3d 완료 %-3d",
		pad(query.ProjectLabel(c.Slug), 16), pad(projStatusMark(c), 6),
		progressBar(c.Progress(), 5), c.Open, c.Doing, c.Done)
	if !c.Remain.IsZero() {
		line += "  " + styMuted.Render("남은 ~"+c.Remain.String())
	}
	if note := m.projDueNote(c); note != "" {
		line += "  " + note
	}
	if c.Overdue > 0 {
		line += "  " + styDanger.Render(fmt.Sprintf("마감초과 %d", c.Overdue))
	}
	if c.Name != "" && c.Name != c.Slug {
		line += "  " + styMuted.Render(truncate(c.Name, 20))
	}
	if !c.Active() {
		line = styDoneRow.Render(stripStyles(line))
	}
	return "  " + cursor + line
}

// progressBar renders done-vs-open. The percentage alone gets skimmed past; a
// bar is what makes a project that has not moved visible in a list.
func progressBar(ratio float64, width int) string {
	filled := int(ratio*float64(width) + 0.5)
	if filled > width {
		filled = width
	}
	bar := styDoing.Render(strings.Repeat("▰", filled)) + styMuted.Render(strings.Repeat("▱", width-filled))
	return fmt.Sprintf("%s %3.0f%%", bar, ratio*100)
}

// projDueNote counts down to the project milestone, which is the form the
// answer is wanted in ("2주 남았다") rather than a bare date.
func (m *Model) projDueNote(c *service.ProjectRow) string {
	if c.Due.IsZero() {
		return ""
	}
	d := c.Due.DaysUntil(m.svc.Today())
	switch {
	case d < 0:
		return styDanger.Render(fmt.Sprintf("마감 %d일 초과", -d))
	case d == 0:
		return styDanger.Render("오늘 마감")
	case d <= 14:
		return styBlocked.Render(fmt.Sprintf("D-%d", d))
	}
	return styMuted.Render("~" + c.Due.String())
}

// projStatusMark shows where a project stands. A slug that exists only on
// tasks has no project.md and so no status to show - "·" says that plainly
// instead of pretending it is active.
func projStatusMark(c *service.ProjectRow) string {
	if !c.Defined {
		return styMuted.Render("·")
	}
	switch c.Status {
	case "paused":
		return styBlocked.Render("중지")
	case "done":
		return styMuted.Render("완료")
	}
	return styDoing.Render("진행")
}

func (m *Model) detailPane() string {
	t := m.current()
	if t == nil {
		return styMuted.Render("  (선택된 태스크 없음)") + "\n"
	}
	d, err := m.fullTask(t.ID)
	if err != nil {
		return styErr.Render("  "+err.Error()) + "\n"
	}
	full := d.task
	var b strings.Builder
	fmt.Fprintf(&b, "  %s  %s\n", styTitle.Render(full.ID), full.Title)
	fmt.Fprintf(&b, "  %s\n", styMuted.Render(detailMeta(full)))
	if note := full.Note(); note != "" {
		b.WriteString("\n")
		for _, l := range strings.Split(note, "\n") {
			b.WriteString("  " + l + "\n")
		}
	}
	if len(d.known)+len(d.missing) > 0 {
		b.WriteString("\n  " + styGroup.Render("선행") + "\n")
		for _, dep := range d.known {
			b.WriteString("  " + styMuted.Render(fmt.Sprintf("%s %s  %s", dep.Status.Glyph(), dep.ShortID(), dep.Title)) + "\n")
		}
		for _, id := range d.missing {
			b.WriteString("  " + styDanger.Render("? "+id+"  (인덱스에 없음)") + "\n")
		}
	}
	if len(d.blocking) > 0 {
		b.WriteString("\n  " + styGroup.Render("후행") + "\n")
		for _, dep := range d.blocking {
			b.WriteString("  " + styMuted.Render(fmt.Sprintf("%s %s  %s", dep.Status.Glyph(), dep.ShortID(), dep.Title)) + "\n")
		}
	}
	if logs := full.LogLines(); len(logs) > 0 {
		b.WriteString("\n  " + styGroup.Render("Log") + "\n")
		for _, l := range logs {
			b.WriteString("  " + styMuted.Render("· "+l) + "\n")
		}
	}
	b.WriteString("\n  " + styMuted.Render(full.Path) + "\n")
	return b.String()
}

// detailBlock windows the pane to the height available, scrolled by J/K.
//
// The pane used to print the last five log lines and cut the rest with "e 로
// 열람". Notes accumulate now, so the answer to a long body is scrolling, not
// a trip to the editor.
func (m *Model) detailBlock(avail int) string {
	lines := strings.Split(strings.TrimRight(m.detailPane(), "\n"), "\n")
	if avail < 1 {
		avail = 1
	}
	if len(lines) <= avail {
		m.detailOffset = 0
		return strings.Join(lines, "\n")
	}
	// One line of the budget goes to whichever indicator is showing.
	body := avail - 1
	if max := len(lines) - body; m.detailOffset > max {
		m.detailOffset = max
	}
	if m.detailOffset < 0 {
		m.detailOffset = 0
	}
	end := m.detailOffset + body
	if end > len(lines) {
		end = len(lines)
	}
	out := strings.Join(lines[m.detailOffset:end], "\n")
	hint := fmt.Sprintf("  … %d줄 더 (J/K 스크롤)", len(lines)-end)
	if m.detailOffset > 0 && end >= len(lines) {
		hint = fmt.Sprintf("  ↑ %d줄 (J/K 스크롤)", m.detailOffset)
	} else if m.detailOffset > 0 {
		hint = fmt.Sprintf("  ↑%d ↓%d (J/K 스크롤)", m.detailOffset, len(lines)-end)
	}
	return out + "\n" + styMuted.Render(hint)
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
	if t.HasSpan() {
		// Two dates that bound a period read better as one fact than as two.
		parts = append(parts, "기간 "+t.SpanLabel())
	} else {
		if !t.Scheduled.IsZero() {
			parts = append(parts, "예정 "+t.Scheduled.String())
		}
		if !t.Due.IsZero() {
			parts = append(parts, "마감 "+t.Due.String())
		}
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
	if m.mode == modeConfirm {
		return styDanger.Render(m.confirmPrompt) + "\n" + styHelp.Render("y 확인  ·  다른 키는 취소")
	}
	if m.mode.prompting() {
		// input.View() already renders its own Prompt - do not prepend it again.
		return m.input.View() + "\n" + styHelp.Render(m.promptHint())
	}
	msg := ""
	switch {
	case m.errMsg != "":
		msg = styErr.Render("! " + m.errMsg)
	case m.status != "":
		msg = styStatus.Render(m.status)
	}
	if n := len(m.marked); n > 0 {
		mark := styDoing.Render(fmt.Sprintf("✓ %d건 선택됨", n))
		if msg == "" {
			msg = mark
		} else {
			msg = mark + styMuted.Render("  ·  ") + msg
		}
	}
	if msg != "" {
		return msg + "\n" + styHelp.Render(helpLine(m.innerWidth()))
	}
	return styHelp.Render(helpLine(m.innerWidth()))
}

// promptHint spells out the accepted syntax under the input. A prompt whose
// grammar is only in the help screen gets guessed at, and a wrong guess here
// writes a wrong date into a file.
func (m *Model) promptHint() string {
	switch m.mode {
	case modeSpan:
		return "enter 확인  esc 취소   예: 09-15~09-19 · today~+4d · 09-15 (시작만) · ~09-19 (마감만) · - 해제"
	case modeProject:
		hint := "enter 확인  esc 취소   - 입력 시 해제"
		if slugs := m.svc.ProjectSlugs(); len(slugs) > 0 {
			if len(slugs) > 8 {
				slugs = slugs[:8]
			}
			hint += "   기존: " + strings.Join(slugs, " ")
		}
		return hint
	case modeNewProject:
		return "enter 확인  esc 취소   첫 낱말이 slug, 나머지가 이름 (예: infra-2026 인프라 개편)"
	case modeProjectDue:
		return "enter 확인  esc 취소   예: 2026-10-31 · +2w · none(해제)"
	case modeNote:
		return "enter 확인  esc 취소   본문 ## Note 에 시각과 함께 한 줄 추가됩니다"
	case modeSaveView:
		return "enter 저장  esc 취소   현재 필터를 이름으로 저장합니다 (config.yaml)"
	case modeSearch:
		return "입력하는 대로 걸러집니다  ·  ↑/↓ 이전 질의  ·  enter 확정  esc 취소"
	case modeBlock:
		return "enter 확인  esc 취소   사유 없이는 보류되지 않습니다"
	}
	return "enter 확인  esc 취소"
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
