package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"task-planner/internal/config"
	"task-planner/internal/domain"
	"task-planner/internal/editor"
	"task-planner/internal/query"
	"task-planner/internal/service"
	"task-planner/internal/watch"
)

// Update is the single event entry point.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		// The two flags belong to two layouts, and only one of them may be set
		// at a time or enter ends up cycling between the split and a bottom
		// pane with no way to close either. Widening carries the pane over
		// rather than dropping it: the user asking for detail on a narrow
		// terminal has not changed their mind by making the window bigger.
		if m.wide() && m.detail {
			m.wideDetail, m.detail = true, false
		}
		if m.tab == tabTimeline {
			// The window length is derived from the width, so the rows the
			// chart selected are stale as soon as the terminal is resized.
			m.reload()
		}
		return m, nil
	case editorDoneMsg:
		m.handleEditorDone(msg)
		return m, nil
	case projectEditedMsg:
		if msg.err != nil {
			m.setErr(msg.err)
			return m, nil
		}
		m.reload()
		m.selectProject(msg.slug)
		m.setStatus("프로젝트 %s 편집 반영", msg.slug)
		return m, nil
	case vaultChangedMsg:
		m.handleVaultChanged()
		return m, waitForChange(m.watcher)
	case tickMsg:
		// Only the elapsed column depends on wall-clock time, so the tick just
		// triggers a redraw.
		return m, tickCmd()
	case tea.MouseMsg:
		m.updateMouse(msg)
		return m, nil
	case tea.KeyMsg:
		switch {
		case m.mode.prompting():
			return m.updatePrompt(msg)
		case m.mode == modeHelp:
			m.updateHelp(msg)
			return m, nil
		case m.mode == modeForm:
			return m.updateForm(msg)
		case m.mode == modeViews:
			return m.updateViews(msg)
		case m.mode == modeConfirm:
			m.updateConfirm(msg)
			return m, nil
		default:
			return m.updateNormal(msg)
		}
	}
	return m, nil
}

// updateMouse turns clicks and wheel into the same moves the keyboard makes.
//
// Hit regions are recorded by View() while it draws, rather than recomputed
// here from the layout maths: the grid is built with lipgloss joins, and a
// second implementation of where things landed would drift from the first.
func (m *Model) updateMouse(msg tea.MouseMsg) {
	if m.mode != modeNormal {
		return
	}
	switch msg.Button {
	case tea.MouseButtonWheelUp:
		m.moveSelection(-1)
		return
	case tea.MouseButtonWheelDown:
		m.moveSelection(1)
		return
	}
	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return
	}
	for _, h := range m.hits {
		if msg.Y != h.y || msg.X < h.x0 || msg.X >= h.x1 {
			continue
		}
		switch h.kind {
		case hitTab:
			m.switchTab(tab(h.a))
		case hitRow:
			m.cursor = h.a
			if m.tab == tabProjects && !m.projDrill {
				m.drillProject()
			} else {
				m.openDetail()
			}
		case hitCard:
			m.colCursor, m.rowCursor = h.a, h.b
			m.openDetail()
		}
		return
	}
}

// toggleDetail flips the detail pane for the current layout: the right column
// on a wide terminal, the bottom pane on a narrow one. Every tab goes through
// here so enter means the same thing everywhere.
//
// Closing always works; opening goes through openDetail and so needs something
// to show. On an empty view - an empty board, a filter matching nothing - enter
// therefore folds the pane and a second enter does nothing until a task exists.
// That asymmetry is the point, not an oversight: see openDetail.
//
// The predicate reads the flags rather than asking splitActive whether a pane
// is on screen, so it stays right on a tab that draws no pane. esc is the
// opposite case and asks splitActive deliberately - it unwinds what the user
// can see, and must fall through to the filter on a tab with nothing to fold.
func (m *Model) toggleDetail() {
	if m.detail || (m.wide() && m.wideDetail) {
		m.setDetail(false)
		return
	}
	m.openDetail()
}

// openDetail shows the pane without closing an open one - what a click on a
// row or a card does, where toggling would make the second click on the same
// task hide the thing the click asked to see. With nothing selectable under
// the cursor it does nothing: an empty pane taking two fifths of a narrow
// screen is a worse answer than no pane.
func (m *Model) openDetail() {
	if m.current() == nil {
		return
	}
	m.setDetail(true)
}

// setDetail is the only writer of the two pane flags outside a resize. Each
// layout owns one flag and clears the other's, so "is the pane showing" has a
// single answer and no caller can forget the refit below - the esc path used
// to close the pane behind its back and leave the Timeline drawing a four-week
// axis over three weeks of rows.
func (m *Model) setDetail(on bool) {
	m.detailOffset = 0
	width := m.contentWidth()
	if m.wide() {
		m.wideDetail, m.detail = on, false
	} else {
		m.detail = on
	}
	// The width is the whole precondition: only the wide pane takes its share
	// of the columns, and only the Timeline turns columns into rows. A narrow
	// toggle splits the height instead, and a click on an already-open pane
	// changes nothing - neither is worth a query.
	if m.contentWidth() != width {
		m.refitTimeline()
	}
}

// refitTimeline rescans the chart after the content width changed. Its visible
// day range is derived from that width, so the rows picked against the old
// range no longer match the axis about to be drawn.
func (m *Model) refitTimeline() {
	if m.tab != tabTimeline {
		return
	}
	// reload rebuilds m.rows, so the cursor - a position in the old list - has
	// to be re-aimed at the task it was actually on.
	t := m.current()
	m.reload()
	if t == nil {
		return
	}
	m.selectID(t.ID)
	if cur := m.current(); cur != nil && cur.ID == t.ID {
		return
	}
	// The shorter window dropped the task the cursor was on: it sits past the
	// new right edge. Follow it rather than opening whatever inherited its row
	// - a click has to answer with the task it landed on.
	startDate, _ := m.timelineBounds(t, m.svc.Today())
	m.tlStart = startDate.WeekStart()
	m.reload()
	m.selectID(t.ID)
	start, days := m.timelineWindow()
	m.setStatus("타임라인 %s ~ %s (선택한 일에 맞춰 이동)", start, start.AddDays(days-1))
}

// drillProject opens the project under the cursor as a task list. enter and a
// click both land here: a click that opened a detail pane instead would make
// the mouse mean something the keyboard does not.
func (m *Model) drillProject() {
	if m.cursor < 0 || m.cursor >= len(m.rows) {
		return
	}
	r := m.rows[m.cursor]
	if r.proj == nil {
		return
	}
	m.projDrill, m.projSlug = true, r.proj.Slug
	m.cursor = 0
	m.reload()
	m.setStatus("프로젝트: %s (esc 로 목록)", query.ProjectLabel(m.projSlug))
}

// moveSelection is one step of cursor movement in whichever model the current
// tab uses.
func (m *Model) moveSelection(delta int) {
	if m.gridTab() {
		m.rowCursor += delta
		m.clampBoard()
	} else {
		m.moveCursor(delta)
	}
	m.detailOffset = 0
}

// updateConfirm answers a y/n question. Anything that is not a yes is a no:
// the prompts that use this delete files.
func (m *Model) updateConfirm(msg tea.KeyMsg) {
	action := m.confirm
	m.mode, m.confirm, m.confirmPrompt = modeNormal, nil, ""
	switch msg.String() {
	case "y", "Y", "enter":
		if action != nil {
			action()
		}
	default:
		m.setStatus("취소됨")
	}
}

// updateViews drives the saved-view picker.
func (m *Model) updateViews(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	views := m.svc.Views()
	key := msg.String()
	if len(key) == 1 && key[0] >= '1' && key[0] <= '9' {
		if i := int(key[0] - '1'); i < len(views) {
			m.mode = modeNormal
			m.applyView(views[i])
		}
		return m, nil
	}
	switch key {
	case "up", "k":
		if m.viewCursor > 0 {
			m.viewCursor--
		}
	case "down", "j":
		if m.viewCursor < len(views)-1 {
			m.viewCursor++
		}
	case "enter":
		m.mode = modeNormal
		if m.viewCursor < len(views) {
			m.applyView(views[m.viewCursor])
		}
	case "s":
		m.mode = modeNormal
		if m.search == "" {
			m.setErr(fmt.Errorf("저장할 질의가 없습니다 — / 로 먼저 걸러보세요"))
			return m, nil
		}
		m.startPrompt(modeSaveView, "뷰 이름: ", "")
		return m, textinput.Blink
	case "d", "x":
		if m.viewCursor < len(views) {
			name := views[m.viewCursor].Name
			if err := m.svc.DeleteView(name); err != nil {
				m.setErr(err)
				return m, nil
			}
			m.setStatus("뷰 삭제됨: %s", name)
			if m.viewCursor > 0 {
				m.viewCursor--
			}
		}
	case "esc", "q", "v":
		m.mode = modeNormal
	}
	return m, nil
}

// applyView runs a saved query as if it had been typed at the filter prompt.
func (m *Model) applyView(v config.View) {
	f, err := m.svc.Filter(v.Query)
	if err != nil {
		m.setErr(err)
		return
	}
	m.search, m.filter = v.Query, f
	m.cursor, m.listOffset = 0, 0
	m.reload()
	m.setStatus("뷰: %s (%s) — %d건", v.Name, v.Query, countTasks(m.rows))
}

// updateHelp scrolls the help screen; any other key closes it.
func (m *Model) updateHelp(msg tea.KeyMsg) {
	switch msg.String() {
	case "down", "j":
		m.helpOffset++
	case "up", "k":
		m.helpOffset--
	case " ", "pgdown", "ctrl+f":
		m.helpOffset += 10
	case "pgup", "ctrl+b":
		m.helpOffset -= 10
	case "g", "home":
		m.helpOffset = 0
	case "G", "end":
		m.helpOffset = len(helpLines())
	default:
		m.mode, m.helpOffset = modeNormal, 0
	}
}

func (m *Model) updateNormal(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() != "q" {
		m.quitArmed = false
	}
	switch msg.String() {
	case "q", "ctrl+c":
		if msg.String() == "q" && !m.quitArmed {
			if t := m.runningTask(); t != nil {
				m.quitArmed = true
				m.setStatus("%s 진행중 (%s) — q 를 한 번 더 누르면 타이머를 켠 채 종료, d/u 로 멈춤",
					t.ShortID(), t.ElapsedLabel(m.svc.Now(), m.svc.Cfg.SessionCap))
				return m, nil
			}
		}
		m.saveUIState()
		m.quit = true
		return m, tea.Quit
	case "?":
		m.mode = modeHelp
	case "up", "k":
		m.moveSelection(-1)
	case "down", "j":
		m.moveSelection(1)
	case "J":
		m.detailOffset++
	case "K":
		if m.detailOffset > 0 {
			m.detailOffset--
		}
	case "left", "h":
		if m.tab == tabTimeline {
			m.shiftTimeline(-1)
		} else if m.gridTab() {
			m.moveColumn(-1)
		}
	case "right", "l":
		if m.tab == tabTimeline {
			m.shiftTimeline(1)
		} else if m.gridTab() {
			m.moveColumn(1)
		}
	case ",", "<":
		m.pageWindow(-1)
	case ".", ">":
		m.pageWindow(1)
	case "t":
		m.pageWindow(0)
	case "H":
		m.moveCard(-1)
	case "L":
		m.moveCard(1)
	case "[", "]":
		m.shiftSpan(map[string]int{"[": -1, "]": 1}[msg.String()])
	case "{", "}":
		m.resizeSpan(map[string]int{"{": -1, "}": 1}[msg.String()])
	case "g", "home":
		m.cursor, m.rowCursor = 0, 0
		m.clampCursor()
		m.clampBoard()
	case "G", "end":
		if m.gridTab() {
			if m.colCursor < len(m.cols) {
				m.rowCursor = len(m.cols[m.colCursor]) - 1
			}
			m.clampBoard()
		} else {
			m.cursor = len(m.rows) - 1
			m.clampCursor()
		}
	case "1", "2", "3", "4", "5", "6":
		m.switchTab(tab(int(msg.String()[0] - '1')))
	case "tab":
		m.switchTab((m.tab + 1) % tabCount)
	case "shift+tab":
		m.switchTab((m.tab + tabCount - 1) % tabCount)
	case "esc":
		switch {
		case len(m.marked) > 0:
			m.marked = map[string]bool{}
			m.setStatus("선택 해제")
		case m.detail || m.splitActive():
			m.setDetail(false)
		case m.search != "":
			m.search, m.filter = "", nil
			m.reload()
			m.setStatus("필터 해제")
		case m.projDrill:
			m.projDrill, m.projSlug = false, ""
			m.cursor = 0
			m.reload()
		}
	case "enter":
		if m.tab == tabProjects && !m.projDrill {
			m.drillProject()
			return m, nil
		}
		m.toggleDetail()
	case "r":
		m.refresh(false)
	case "R":
		m.refresh(true)
	case "a":
		m.startPrompt(modeCapture, "새 태스크: ", "")
		return m, textinput.Blink
	case "A":
		m.startForm()
		return m, textinput.Blink
	case "F":
		m.marked = map[string]bool{}
		switch m.scope {
		case "human":
			m.scope = "agent"
		case "agent":
			m.scope = "all"
		default:
			m.scope = "human"
		}
		m.reload()
		m.saveUIState()
		m.setStatus("작업 보기: %s", m.scope)
	case "/":
		m.startPrompt(modeSearch, "필터: ", m.search)
		return m, textinput.Blink
	case " ":
		m.applyNext()
	case "s":
		if p := m.currentProj(); p != nil {
			m.cycleProject(p)
			break
		}
		m.applyStatus(domain.StatusDoing, nil)
	case "d":
		m.applyStatus(domain.StatusDone, nil)
	case "x":
		m.applyStatus(domain.StatusCancelled, nil)
	case "u":
		m.applyStatus(domain.StatusTodo, nil)
	case "b":
		if t := m.current(); t != nil {
			m.startPrompt(modeBlock, "보류 사유: ", t.BlockedReason)
			return m, textinput.Blink
		}
	case "m":
		m.toggleMark()
	case "M":
		if len(m.marked) == 0 {
			break
		}
		n := len(m.marked)
		m.marked = map[string]bool{}
		m.setStatus("선택 %d건 해제", n)
	case "S":
		m.skipCurrent()
	case "X":
		m.confirmDelete()
	case "ctrl+z":
		m.undo()
	case "v":
		m.mode = modeViews
		m.viewCursor = 0
	case "z":
		if m.tab == tabTimeline {
			m.toggleTimelineScale()
		}
	case "f":
		if m.tab == tabTimeline {
			m.toggleTimelineMode()
			break
		}
		if m.tab != tabToday && m.tab != tabAll {
			break
		}
		selected := m.current()
		switch m.grouping {
		case groupStatus:
			m.grouping = groupProject
			m.setStatus("목록 묶음: 프로젝트별")
		case groupProject:
			m.grouping = groupProjectStatus
			m.setStatus("목록 묶음: 프로젝트 안에서 상태별")
		default:
			m.grouping = groupStatus
			m.setStatus("목록 묶음: 상태별")
		}
		m.reload()
		if selected != nil {
			m.selectID(selected.ID)
		}
		m.listOffset = 0
	case "e":
		if p := m.currentProj(); p != nil {
			return m, m.openProjectEditor(p)
		}
		return m, m.openEditor()
	case "p":
		if t := m.current(); t != nil {
			m.startPrompt(modeProject, "프로젝트: ", t.Project)
			return m, textinput.Blink
		}
	case "D":
		if p := m.currentProj(); p != nil {
			if p.Slug == "" {
				m.setErr(errNotAProject)
				break
			}
			m.startPrompt(modeProjectDue, "프로젝트 마감일: ", p.Due.String())
			return m, textinput.Blink
		}
		if t := m.current(); t != nil {
			m.startPrompt(modeSpan, "기간: ", t.Span().String())
			return m, textinput.Blink
		}
	case "n":
		if m.tab == tabProjects && !m.projDrill {
			m.startPrompt(modeNewProject, "새 프로젝트 (slug [이름]): ", "")
			return m, textinput.Blink
		}
	case "N":
		if t := m.current(); t != nil {
			m.startPrompt(modeNote, "메모: ", "")
			return m, textinput.Blink
		}
	case "!":
		m.jumpNext()
	}
	return m, nil
}

// tickMsg drives the running-timer display.
type tickMsg time.Time

// tickCmd redraws once a minute. Anything faster would burn CPU for a column
// that only changes at minute resolution.
func tickCmd() tea.Cmd {
	return tea.Tick(time.Minute, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// vaultChangedMsg means a file under tasks/ changed outside this process.
type vaultChangedMsg struct{}

// waitForChange blocks on the watcher until the next settled burst. It is
// re-issued after every delivery, which is how bubbletea models a stream.
func waitForChange(w *watch.Watcher) tea.Cmd {
	if w == nil {
		return nil
	}
	return func() tea.Msg {
		<-w.Events()
		return vaultChangedMsg{}
	}
}

// handleVaultChanged re-syncs after an external edit, keeping the cursor on the
// task the user was looking at.
func (m *Model) handleVaultChanged() {
	id := ""
	if t := m.current(); t != nil {
		id = t.ID
	}
	before := len(m.svc.All())
	if _, err := m.svc.Sync(); err != nil {
		m.setErr(err)
		return
	}
	m.reload()
	m.selectID(id)
	// Only announce changes the user did not make here; own edits already
	// produced their own status line.
	if after := len(m.svc.All()); after != before {
		m.setStatus("외부 변경 반영 — 태스크 %d건", after)
	}
}

// editorDoneMsg carries the result of an external edit back into the loop.
type editorDoneMsg struct {
	id  string
	err error
}

// openEditor suspends the TUI, runs $EDITOR on the selected file, and re-reads
// it on return. Markdown is the source of truth, so editing it by hand is a
// supported path rather than a workaround.
func (m *Model) openEditor() tea.Cmd {
	t := m.current()
	if t == nil {
		return nil
	}
	cmd, err := editor.Command(m.svc.Cfg, t.Path)
	if err != nil {
		m.setErr(err)
		return nil
	}
	id := t.ID
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return editorDoneMsg{id: id, err: err}
	})
}

// handleEditorDone re-indexes after an external edit and surfaces a parse error
// at the moment it was introduced.
func (m *Model) handleEditorDone(msg editorDoneMsg) {
	if msg.err != nil {
		m.setErr(msg.err)
		return
	}
	if _, err := m.svc.Sync(); err != nil {
		m.setErr(err)
		return
	}
	m.reload()
	m.selectID(msg.id)
	if t := m.current(); t != nil {
		m.setStatus("편집 반영 %s  %s", t.ShortID(), t.Title)
	} else {
		m.setStatus("편집 반영됨")
	}
}

// pageWindow moves the date window of whichever tab has one; 0 returns to
// 이번 주. Week and Timeline page with the same keys because they are the same
// question asked twice - a second set of keys would only be a second thing to
// remember.
func (m *Model) pageWindow(n int) {
	switch m.tab {
	case tabWeek:
		m.shiftWeek(n)
	case tabTimeline:
		m.shiftTimeline(n)
	}
}

func (m *Model) switchTab(t tab) {
	if t == m.tab {
		return
	}
	m.tabMem[m.tab] = tabState{
		cursor: m.cursor, listOffset: m.listOffset,
		colCursor: m.colCursor, rowCursor: m.rowCursor,
		projDrill: m.projDrill, projSlug: m.projSlug,
	}
	st := m.tabMem[t]
	m.tab = t
	m.cursor, m.listOffset = st.cursor, st.listOffset
	m.colCursor, m.rowCursor = st.colCursor, st.rowCursor
	m.projDrill, m.projSlug = st.projDrill, st.projSlug
	m.detail, m.detailOffset = false, 0
	m.reload()
}

func (m *Model) startPrompt(md mode, prompt, initial string) {
	m.mode = md
	m.input.Prompt = prompt
	m.input.SetValue(initial)
	m.input.CursorEnd()
	m.input.Focus()
	m.histPos, m.histDraft = len(m.history[md]), initial
	if md == modeSearch {
		// Keep what the view was showing so esc restores it rather than
		// leaving the half-typed filter applied.
		m.prevSearch, m.prevFilter = m.search, m.filter
	}
}

func (m *Model) updatePrompt(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		md := m.mode
		m.mode = modeNormal
		m.input.Blur()
		if md == modeSearch {
			// Restoring silently would leave the status line describing the
			// abandoned query, which reads as if it were still applied.
			m.search, m.filter = m.prevSearch, m.prevFilter
			m.reload()
			if m.search == "" {
				m.setStatus("필터 해제")
			} else {
				m.setStatus("필터: %s — %d건", m.search, m.countMatches())
			}
		}
		return m, nil
	case "up":
		m.recall(-1)
		return m, nil
	case "down":
		m.recall(1)
		return m, nil
	case "enter":
		value := strings.TrimSpace(m.input.Value())
		md := m.mode
		m.mode = modeNormal
		m.input.Blur()
		m.remember(md, value)
		switch md {
		case modeCapture:
			m.capture(value)
		case modeSearch:
			m.applySearch(value, true)
		case modeSpan:
			m.setSpan(value)
		case modeProject:
			m.setProject(value)
		case modeNewProject:
			m.newProject(value)
		case modeProjectDue:
			m.setProjectDue(value)
		case modeNote:
			m.addNote(value)
		case modeBlock:
			if value == "" {
				m.setErr(domain.ErrBlockedNeedsReason)
				return m, nil
			}
			m.applyStatus(domain.StatusBlocked, &domain.BlockInfo{Reason: value})
		case modeSaveView:
			m.saveView(value)
		}
		return m, nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	if m.mode == modeSearch {
		// Filter as it is typed: a query you cannot see the result of is a
		// query you have to run twice.
		m.applySearch(strings.TrimSpace(m.input.Value()), false)
	}
	return m, cmd
}

// applySearch compiles and applies a filter expression. committed is false
// while the user is still typing, which is when a body search is deferred -
// it reads files, and doing that per keystroke would stutter.
func (m *Model) applySearch(value string, committed bool) {
	f, err := m.svc.Filter(value)
	if err != nil {
		if committed {
			m.setErr(err)
		}
		return
	}
	if !committed && f.NeedsBody() {
		m.setStatus("본문 검색은 enter 를 눌러야 실행됩니다")
		return
	}
	m.search, m.filter = value, f
	m.cursor, m.listOffset = 0, 0
	m.reload()
	switch {
	case value == "":
		m.setStatus("필터 해제")
	default:
		m.setStatus("필터: %s — %d건", value, m.countMatches())
	}
}

// countMatches counts tasks in the current view, whichever shape it has.
func (m *Model) countMatches() int {
	if m.gridTab() {
		n := 0
		for _, col := range m.cols {
			n += len(col)
		}
		return n
	}
	return countTasks(m.rows)
}

// remember appends to the prompt's recall list, newest last and no immediate
// repeats.
func (m *Model) remember(md mode, value string) {
	if value == "" || md == modeConfirm {
		return
	}
	h := m.history[md]
	if len(h) > 0 && h[len(h)-1] == value {
		return
	}
	h = append(h, value)
	if len(h) > 30 {
		h = h[1:]
	}
	m.history[md] = h
}

// recall walks the prompt's history with ↑/↓, keeping the half-typed line at
// the bottom of the walk.
func (m *Model) recall(delta int) {
	h := m.history[m.mode]
	if len(h) == 0 {
		return
	}
	if m.histPos == len(h) {
		m.histDraft = m.input.Value()
	}
	pos := m.histPos + delta
	if pos < 0 {
		pos = 0
	}
	if pos > len(h) {
		pos = len(h)
	}
	m.histPos = pos
	if pos == len(h) {
		m.input.SetValue(m.histDraft)
	} else {
		m.input.SetValue(h[pos])
	}
	m.input.CursorEnd()
	if m.mode == modeSearch {
		m.applySearch(strings.TrimSpace(m.input.Value()), false)
	}
}

func (m *Model) capture(title string) {
	if title == "" {
		return
	}
	in, projRow := m.captureInput(title)
	m.addTask(in, projRow)
}

// captureInput derives the defaults a new task inherits from where it was
// captured; projRow is the project row the cursor should stay on.
func (m *Model) captureInput(title string) (service.AddInput, string) {
	in := service.AddInput{Title: title}
	if m.scope == "agent" {
		in.Executor = domain.ExecutorAgent
	}
	// A task captured from the Today view is meant for today; from a project
	// view it belongs to that project. Both save a follow-up edit.
	if m.tab == tabToday {
		in.Scheduled = m.svc.Today()
	}
	if m.tab == tabBoard && m.colCursor < len(boardColumns) && boardColumns[m.colCursor] == domain.StatusDoing {
		in.Status = domain.StatusDoing
	}
	// Capturing while a Week day column is selected schedules for that day.
	if m.tab == tabWeek && m.colCursor < len(m.weekDays) {
		in.Scheduled = m.weekDays[m.colCursor]
	}
	// Capturing with a project row selected files the task under it. The slug
	// is read before the reload: the row moves once its open count changes.
	projRow := ""
	if m.projDrill {
		in.Project = m.projSlug
	} else if p := m.currentProj(); p != nil {
		in.Project, projRow = p.Slug, p.Slug
	}
	return in, projRow
}

// addTask files the task and keeps the cursor somewhere sensible.
func (m *Model) addTask(in service.AddInput, projRow string) {
	var res *service.Result
	err := m.svc.Undoable("추가", func() error {
		var e error
		res, e = m.svc.AddWithResult(in)
		return e
	})
	if err != nil {
		m.setErr(err)
		return
	}
	t := res.Task
	m.reload()
	if projRow != "" {
		m.selectProject(projRow)
	} else {
		m.selectID(t.ID)
	}
	m.setStatus("추가됨 %s  %s", t.ShortID(), t.Title)
	for _, w := range res.Warnings {
		m.status += "  · " + w
	}
}

// startForm opens the multi-field capture (A): Jira 식 제목/설명/기간/태그를
// 한 화면에서 받는다. 캡처 문맥(탭·프로젝트 드릴인)은 a 와 똑같이 적용된다.
func (m *Model) startForm() {
	m.mode = modeForm
	m.formVals = [formCount]string{}
	m.formFocus = formTitle
	m.input.Prompt = ""
	m.input.SetValue("")
	m.input.Focus()
}

// updateForm drives the form: enter walks the fields and submits at the end,
// ctrl+s submits from anywhere, esc discards.
func (m *Model) updateForm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeNormal
		m.input.Blur()
		m.setStatus("취소됨")
		return m, nil
	case "enter":
		m.formVals[m.formFocus] = m.input.Value()
		if m.formFocus == formCount-1 {
			m.submitForm()
		} else {
			m.formSetFocus(m.formFocus + 1)
		}
		return m, nil
	case "tab", "down":
		m.formVals[m.formFocus] = m.input.Value()
		m.formSetFocus((m.formFocus + 1) % formCount)
		return m, nil
	case "shift+tab", "up":
		m.formVals[m.formFocus] = m.input.Value()
		m.formSetFocus((m.formFocus + formCount - 1) % formCount)
		return m, nil
	case "ctrl+s":
		m.formVals[m.formFocus] = m.input.Value()
		m.submitForm()
		return m, nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m *Model) formSetFocus(i int) {
	m.formFocus = i
	m.input.SetValue(m.formVals[i])
	m.input.CursorEnd()
}

// submitForm validates and files the task. A bad field puts the focus back on
// it instead of throwing the rest of the input away.
func (m *Model) submitForm() {
	title := strings.TrimSpace(m.formVals[formTitle])
	if title == "" {
		m.formSetFocus(formTitle)
		m.setErr(fmt.Errorf("제목이 비어 있음"))
		return
	}
	in, projRow := m.captureInput(title)
	in.Note = strings.TrimSpace(m.formVals[formDesc])
	if v := strings.TrimSpace(m.formVals[formSpan]); v != "" {
		sp, err := m.svc.ParseSpan(v)
		if err != nil {
			m.formSetFocus(formSpan)
			m.setErr(err)
			return
		}
		// A typed period wins over the tab's contextual default.
		in.Scheduled, in.Due = sp.Start, sp.End
	}
	if tags := splitTags(m.formVals[formTags]); len(tags) > 0 {
		in.Tags = tags
	}
	m.mode = modeNormal
	m.input.Blur()
	m.addTask(in, projRow)
}

// splitTags accepts "ops, backend" and "#ops backend" alike.
func splitTags(s string) []string {
	var out []string
	for _, f := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' }) {
		if f = strings.TrimPrefix(strings.TrimSpace(f), "#"); f != "" {
			out = append(out, f)
		}
	}
	return out
}

// applyNext cycles each target through its own next status; a bulk cycle over
// tasks in different states is still one step per task.
func (m *Model) applyNext() {
	ts := m.targets()
	if len(ts) == 0 {
		return
	}
	m.mutate(m.bulkLabel(ts, "상태 변경"), ts, func(t *domain.Task) (*service.Result, error) {
		return m.svc.SetStatus(t.ID, domain.NextStatus(t.Status), nil)
	})
}

// applyStatus moves every target to one status.
func (m *Model) applyStatus(to domain.Status, block *domain.BlockInfo) {
	ts := m.targets()
	if len(ts) == 0 {
		return
	}
	if to == domain.StatusBlocked && block == nil {
		// One reason for the whole selection: a hold that needs five different
		// reasons is five holds, and the prompt asks once.
		m.startPrompt(modeBlock, "보류 사유: ", ts[0].BlockedReason)
		return
	}
	m.mutate(m.bulkLabel(ts, to.Label()), ts, func(t *domain.Task) (*service.Result, error) {
		return m.svc.SetStatus(t.ID, to, block)
	})
}

// mutate runs one change per target inside a single undo step, then reports
// what happened in one status line.
func (m *Model) mutate(label string, ts []*domain.Task, fn func(*domain.Task) (*service.Result, error)) {
	var last *service.Result
	var warnings []string
	n := 0
	err := m.svc.Undoable(label, func() error {
		for _, t := range ts {
			res, err := fn(t)
			if err != nil {
				return err
			}
			last, n = res, n+1
			warnings = append(warnings, res.Warnings...)
			if res.Next != nil {
				warnings = append(warnings,
					fmt.Sprintf("다음 회차 %s (%s)", res.Next.ShortID(), res.Next.Scheduled))
			}
		}
		return nil
	})
	if err != nil {
		m.setErr(err)
	}
	if n == 0 {
		return
	}
	m.reload()
	if last != nil {
		m.selectID(last.Task.ID)
	}
	if err == nil {
		if n == 1 && last != nil {
			m.setStatus("%s  %s", last.Task.ShortID(), label)
		} else {
			m.setStatus("%d건  %s", n, label)
		}
	}
	// Marks are consumed by the action that used them; leaving them set makes
	// the next keystroke act on a selection the user has stopped thinking about.
	if len(m.marked) > 0 {
		m.marked = map[string]bool{}
	}
	for _, w := range warnings {
		m.status += "  · " + w
	}
}

// bulkLabel names an undo step and the status line after it.
func (m *Model) bulkLabel(ts []*domain.Task, what string) string {
	if len(ts) == 1 {
		return what
	}
	return fmt.Sprintf("%s (%d건)", what, len(ts))
}

// moveCard moves the selected work one lane over: a status on the Board, a day
// on Week. Moving the card is the gesture a board implies; h/l only moved the
// cursor, which left the actual move to keys you had to already know.
func (m *Model) moveCard(delta int) {
	if !m.gridTab() {
		return
	}
	if m.tab == tabWeek {
		m.shiftSpan(delta)
		return
	}
	ts := m.targets()
	if len(ts) == 0 {
		return
	}
	to := domain.Status("")
	for i, st := range boardColumns {
		if st == ts[0].Status {
			j := i + delta
			if j < 0 || j >= len(boardColumns) {
				return
			}
			to = boardColumns[j]
			break
		}
	}
	if to == "" {
		return
	}
	m.applyStatus(to, nil)
}

// skipCurrent advances a recurring task to its next occurrence.
func (m *Model) skipCurrent() {
	ts := m.targets()
	if len(ts) == 0 {
		return
	}
	m.mutate(m.bulkLabel(ts, "이번 회차 건너뜀"), ts, func(t *domain.Task) (*service.Result, error) {
		return m.svc.Skip(t.ID)
	})
}

// confirmDelete asks before removing files. Undo can put them back, but a file
// disappearing from the vault is not something to do on a single keystroke.
func (m *Model) confirmDelete() {
	ts := m.targets()
	if len(ts) == 0 {
		return
	}
	what := fmt.Sprintf("%s %s", ts[0].ShortID(), ts[0].Title)
	if len(ts) > 1 {
		what = fmt.Sprintf("%d건", len(ts))
	}
	m.confirmPrompt = fmt.Sprintf("%s 을(를) 삭제할까요? 파일이 지워집니다 (y/n)  — 접을 일이면 x 로 취소가 낫습니다", what)
	m.mode = modeConfirm
	m.confirm = func() {
		ids := make([]string, len(ts))
		for i, t := range ts {
			ids[i] = t.ID
		}
		err := m.svc.Undoable(m.bulkLabel(ts, "삭제"), func() error {
			for _, id := range ids {
				if err := m.svc.Delete(id); err != nil {
					return err
				}
			}
			return nil
		})
		m.marked = map[string]bool{}
		m.reload()
		if err != nil {
			m.setErr(err)
			return
		}
		m.setStatus("%s 삭제됨 (ctrl+z 로 복구)", what)
	}
}

// undo reverses the last action, leaving the cursor where it was - the point
// of undo is to carry on from where the mistake happened.
func (m *Model) undo() {
	id := ""
	if t := m.current(); t != nil {
		id = t.ID
	}
	label, err := m.svc.Undo()
	if err != nil {
		m.setErr(err)
		return
	}
	m.reload()
	m.selectID(id)
	m.setStatus("되돌림: %s", label)
}

// runningTask is the task whose timer is going, if any.
func (m *Model) runningTask() *domain.Task {
	for _, t := range m.svc.All() {
		if t.Status == domain.StatusDoing && t.StartedAt != nil {
			return t
		}
	}
	return nil
}

// saveUIState persists the layout choices worth restoring next launch.
func (m *Model) saveUIState() {
	_ = m.svc.SaveUIState(service.UIState{Tab: int(m.tab), WideDetail: m.wideDetail, ListGrouping: string(m.grouping), TimelineActual: m.tlActual, TimelineHourly: m.tlHourly, ExecutorScope: m.scope})
}

// selectID keeps the cursor on the same task across a reload. On a grid tab a
// status or date change moves the card to another lane, so the cursor has to
// follow it there or the next keystroke would act on an unrelated task.
func (m *Model) selectID(id string) {
	if m.gridTab() {
		for c, col := range m.cols {
			for r, t := range col {
				if t.ID == id {
					m.colCursor, m.rowCursor = c, r
					return
				}
			}
		}
		m.clampBoard()
		return
	}
	for i, r := range m.rows {
		if r.task != nil && r.task.ID == id {
			m.cursor = i
			return
		}
	}
	m.clampCursor()
}

// shiftSpan slides each target's whole 진행 기간 by a day, keeping its length -
// the re-planning gesture ("이건 하루 밀자"). An unscheduled task gets today
// first, so the first press pulls it out of the 미배정 lane instead of jumping
// blindly.
func (m *Model) shiftSpan(days int) {
	ts := m.targets()
	if len(ts) == 0 {
		return
	}
	verb := "기간 하루 뒤로"
	if days < 0 {
		verb = "기간 하루 앞으로"
	}
	m.spanMutate(m.bulkLabel(ts, verb), ts, func(t *domain.Task) (*service.Result, error) {
		return m.svc.ShiftSpan(t.ID, days)
	})
}

// resizeSpan moves only the end of the period: the task starts when it started
// and now takes longer (or less).
func (m *Model) resizeSpan(days int) {
	ts := m.targets()
	if len(ts) == 0 {
		return
	}
	verb := "기간 늘림"
	if days < 0 {
		verb = "기간 줄임"
	}
	m.spanMutate(m.bulkLabel(ts, verb), ts, func(t *domain.Task) (*service.Result, error) {
		return m.svc.ResizeSpan(t.ID, days)
	})
}

// setSpan applies a typed period ("09-15~09-19").
func (m *Model) setSpan(value string) {
	ts := m.targets()
	if len(ts) == 0 || value == "" {
		return
	}
	sp, err := m.svc.ParseSpan(value)
	if err != nil {
		m.setErr(err)
		return
	}
	m.spanMutate(m.bulkLabel(ts, "기간 설정"), ts, func(t *domain.Task) (*service.Result, error) {
		return m.svc.SetSpan(t.ID, sp)
	})
}

// spanMutate is mutate() with a status line that states the resulting period,
// which is the whole point of the keys that call it.
func (m *Model) spanMutate(label string, ts []*domain.Task, fn func(*domain.Task) (*service.Result, error)) {
	bulk := len(ts) > 1
	m.mutate(label, ts, fn)
	if bulk || m.errMsg != "" {
		return
	}
	if t := m.current(); t != nil {
		switch {
		case t.SpanDays() == 0:
			m.setStatus("%s 기간 해제", t.ShortID())
		case t.HasSpan():
			m.setStatus("%s 기간 %s", t.ShortID(), t.SpanLabel())
		default:
			d := t.SpanStart()
			m.setStatus("%s 예정 → %s (%s)", t.ShortID(), d, d.WeekdayKO())
		}
	}
}

// setProject re-homes every target. "-" clears the assignment.
func (m *Model) setProject(value string) {
	ts := m.targets()
	if len(ts) == 0 {
		return
	}
	slug := value
	if slug == "-" || slug == "없음" {
		slug = ""
	} else if slug != "" {
		slug = service.ProjectSlug(slug)
	}
	label := "프로젝트 해제"
	if slug != "" {
		label = "프로젝트 → " + slug
	}
	m.mutate(m.bulkLabel(ts, label), ts, func(t *domain.Task) (*service.Result, error) {
		s := slug
		return m.svc.Edit(t.ID, service.EditInput{Project: &s})
	})
}

// newProject creates projects/<slug>/project.md. The prompt takes "slug 이름"
// on one line: two prompts for two obvious fields is one prompt too many.
func (m *Model) newProject(value string) {
	if value == "" {
		return
	}
	slug, name, _ := strings.Cut(value, " ")
	var p *domain.Project
	err := m.svc.Undoable("프로젝트 추가", func() error {
		var e error
		p, e = m.svc.CreateProject(slug, strings.TrimSpace(name))
		return e
	})
	if err != nil {
		m.setErr(err)
		return
	}
	m.reload()
	m.selectProject(p.Slug)
	m.setStatus("프로젝트 추가됨 %s  %s", p.Slug, p.Display())
}

// addNote appends a timestamped line to every target's note section - the
// thing you want to write down at the moment you hear it.
func (m *Model) addNote(text string) {
	ts := m.targets()
	if len(ts) == 0 || text == "" {
		return
	}
	m.mutate(m.bulkLabel(ts, "메모 추가"), ts, func(t *domain.Task) (*service.Result, error) {
		return m.svc.AddNote(t.ID, text)
	})
}

// saveView stores the active filter under a name so tomorrow it is one key.
func (m *Model) saveView(name string) {
	if name == "" {
		return
	}
	if err := m.svc.SaveView(name, m.search); err != nil {
		m.setErr(err)
		return
	}
	m.setStatus("뷰 저장됨: %s (%s) — v 로 불러옵니다", name, m.search)
}

// jumpNext moves the cursor onto the task worth doing next and says why.
func (m *Model) jumpNext() {
	sugg := m.svc.NextUp(0)
	if m.scope != "all" {
		filtered := sugg[:0]
		for _, sg := range sugg {
			if string(sg.Task.Executor.Effective()) == m.scope {
				filtered = append(filtered, sg)
			}
		}
		sugg = filtered
	}
	if len(sugg) == 0 {
		m.setStatus("지금 바로 할 수 있는 일이 없습니다 (전부 완료이거나 보류 중)")
		return
	}
	sg := sugg[0]
	if !m.hasTask(sg.Task.ID) {
		// The suggestion may live outside the current tab; All shows everything.
		m.switchTab(tabAll)
	}
	m.selectID(sg.Task.ID)
	m.setStatus("다음: %s %s — %s", sg.Task.ShortID(), sg.Task.Title, sg.Reason)
	if !m.hasTask(sg.Task.ID) {
		m.status += "  · 현재 필터에 가려져 있습니다 (esc 로 해제)"
	}
}

// hasTask reports whether the current view contains a task.
func (m *Model) hasTask(id string) bool {
	if m.gridTab() {
		for _, col := range m.cols {
			for _, t := range col {
				if t.ID == id {
					return true
				}
			}
		}
		return false
	}
	for _, r := range m.rows {
		if r.task != nil && r.task.ID == id {
			return true
		}
	}
	return false
}

// setProjectDue writes the project milestone that the Projects tab counts down.
func (m *Model) setProjectDue(value string) {
	r := m.currentProj()
	if r == nil {
		return
	}
	d, err := m.svc.ParseDate(value)
	if err != nil {
		m.setErr(err)
		return
	}
	if _, err := m.svc.EnsureProject(r.Slug); err != nil {
		m.setErr(err)
		return
	}
	var p *domain.Project
	err = m.svc.Undoable("프로젝트 마감 변경", func() error {
		var e error
		p, e = m.svc.EditProject(r.Slug, service.ProjectEditInput{Due: &d})
		return e
	})
	if err != nil {
		m.setErr(err)
		return
	}
	m.reload()
	m.selectProject(p.Slug)
	if d.IsZero() {
		m.setStatus("프로젝트 %s 마감 해제", p.Slug)
		return
	}
	m.setStatus("프로젝트 %s 마감 → %s", p.Slug, d)
}

// cycleProject advances the project status (active → paused → done).
func (m *Model) cycleProject(r *service.ProjectRow) {
	if r.Slug == "" {
		m.setErr(errNotAProject)
		return
	}
	if _, err := m.svc.EnsureProject(r.Slug); err != nil {
		m.setErr(err)
		return
	}
	var p *domain.Project
	err := m.svc.Undoable("프로젝트 상태 변경", func() error {
		var e error
		p, e = m.svc.CycleProjectStatus(r.Slug)
		return e
	})
	if err != nil {
		m.setErr(err)
		return
	}
	m.reload()
	m.selectProject(p.Slug)
	m.setStatus("프로젝트 %s → %s", p.Slug, p.Status)
}

// selectProject keeps the cursor on a project row across a reload.
func (m *Model) selectProject(slug string) {
	for i, r := range m.rows {
		if r.proj != nil && r.proj.Slug == slug {
			m.cursor = i
			return
		}
	}
	m.clampCursor()
}

// openProjectEditor edits project.md, creating it first when the project only
// exists as a slug on some task - which is the usual way one comes into being.
func (m *Model) openProjectEditor(r *service.ProjectRow) tea.Cmd {
	if r.Slug == "" {
		m.setErr(errNotAProject)
		return nil
	}
	p, err := m.svc.EnsureProject(r.Slug)
	if err != nil {
		m.setErr(err)
		return nil
	}
	path := p.Path
	cmd, err := editor.Command(m.svc.Cfg, path)
	if err != nil {
		m.setErr(err)
		return nil
	}
	slug := r.Slug
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return projectEditedMsg{slug: slug, err: err}
	})
}

// errNotAProject guards the 미지정 bucket: it is where tasks without a project
// collect, not a project that could have a file.
var errNotAProject = fmt.Errorf("(미지정) 은 프로젝트가 아닙니다 — 태스크에 p 로 프로젝트를 지정하세요")

// projectEditedMsg carries the result of editing project.md.
type projectEditedMsg struct {
	slug string
	err  error
}

func (m *Model) refresh(full bool) {
	var err error
	if full {
		_, err = m.svc.Rebuild()
		// Pre-images describe files as this session last wrote them; a full
		// rebuild is the moment to admit that may no longer hold.
		m.svc.ResetUndo()
	} else {
		_, err = m.svc.Sync()
	}
	if err != nil {
		m.setErr(err)
		return
	}
	id := ""
	if t := m.current(); t != nil {
		id = t.ID
	}
	m.reload()
	m.selectID(id)
	st := m.svc.IndexStats()
	m.setStatus("새로고침 — 태스크 %d (열림 %d)", st.Total, st.Open)
}

func countTasks(rows []row) int {
	n := 0
	for _, r := range rows {
		if r.task != nil {
			n++
		}
	}
	return n
}
