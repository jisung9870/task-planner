package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

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
	b.WriteString(m.tabs())
	b.WriteString("\n")
	b.WriteString(styRule.Render(strings.Repeat("─", m.innerWidth())))
	b.WriteString("\n")
	b.WriteString(m.list())
	if m.detail {
		b.WriteString("\n")
		b.WriteString(styRule.Render(strings.Repeat("─", m.innerWidth())))
		b.WriteString("\n")
		b.WriteString(m.detailPane())
	}
	b.WriteString("\n")
	b.WriteString(styRule.Render(strings.Repeat("─", m.innerWidth())))
	b.WriteString("\n")
	b.WriteString(m.footer())
	return b.String()
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
	if wip := m.wipNote(); wip != "" {
		suffix = append(suffix, wip)
	}
	if len(suffix) > 0 {
		line += "   " + styMuted.Render(strings.Join(suffix, "  "))
	}
	return line
}

// wipNote surfaces the in-progress count next to the tabs; the limit itself is
// enforced elsewhere.
func (m *Model) wipNote() string {
	doing := 0
	for _, t := range m.svc.All() {
		if t.Status == domain.StatusDoing {
			doing++
		}
	}
	if m.svc.Cfg.WIPLimit > 0 {
		return fmt.Sprintf("WIP %d/%d", doing, m.svc.Cfg.WIPLimit)
	}
	return fmt.Sprintf("WIP %d", doing)
}

func (m *Model) list() string {
	if m.tab == tabBoard {
		return m.board()
	}
	if len(m.rows) == 0 {
		return styMuted.Render("  (표시할 항목 없음 — a 로 추가)") + "\n"
	}
	today := m.svc.Today()
	var b strings.Builder
	for i, r := range m.rows {
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
	return b.String()
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
func (m *Model) board() string {
	width := m.innerWidth()
	colWidth := (width - 2*(len(boardColumns)-1)) / len(boardColumns)
	today := m.svc.Today()
	if colWidth < 18 {
		// Narrow terminals stack the lanes instead of rendering unreadable
		// slivers. Cursor addressing is unchanged, so keys behave the same.
		return m.boardStacked(width, today)
	}
	panes := make([]string, len(boardColumns))
	for i, st := range boardColumns {
		panes[i] = m.boardColumn(i, st, colWidth, today)
	}
	out := lipgloss.JoinHorizontal(lipgloss.Top, panes...)
	return out + "\n" + m.boardFooter(today)
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

// boardColumn renders one lane.
func (m *Model) boardColumn(idx int, st domain.Status, width int, today domain.Date) string {
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
	for r, t := range cards {
		selected := idx == m.colCursor && r == m.rowCursor
		b.WriteString(m.card(t, width, selected, today) + "\n")
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
func truncate(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= w {
		return s
	}
	const ellipsis = "…"
	budget := w - lipgloss.Width(ellipsis)
	if budget <= 0 {
		return ellipsis
	}
	var b strings.Builder
	for _, r := range s {
		if lipgloss.Width(b.String()+string(r)) > budget {
			break
		}
		b.WriteRune(r)
	}
	return strings.TrimRight(b.String(), " ") + ellipsis
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
