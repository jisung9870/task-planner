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
	case tea.KeyMsg:
		switch {
		case m.mode.prompting():
			return m.updatePrompt(msg)
		case m.mode == modeHelp:
			m.updateHelp(msg)
			return m, nil
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
	switch msg.String() {
	case "q", "ctrl+c":
		m.quit = true
		return m, tea.Quit
	case "?":
		m.mode = modeHelp
	case "up", "k":
		if m.gridTab() {
			m.rowCursor--
			m.clampBoard()
		} else {
			m.moveCursor(-1)
		}
	case "down", "j":
		if m.gridTab() {
			m.rowCursor++
			m.clampBoard()
		} else {
			m.moveCursor(1)
		}
	case "left", "h":
		if m.gridTab() {
			m.moveColumn(-1)
		}
	case "right", "l":
		if m.gridTab() {
			m.moveColumn(1)
		}
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
	case "1", "2", "3", "4", "5":
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
		case m.detail:
			m.detail = false
		case m.splitActive():
			m.wideDetail = false
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
			if r := m.rows[m.cursor]; r.proj != nil {
				m.projDrill, m.projSlug = true, r.proj.Slug
				m.cursor = 0
				m.reload()
				m.setStatus("프로젝트: %s (esc 로 목록)", query.ProjectLabel(m.projSlug))
			}
			return m, nil
		}
		if m.wide() && m.tab != tabBoard {
			m.wideDetail = !m.wideDetail
		} else {
			m.detail = !m.detail
		}
	case "r":
		m.refresh(false)
	case "R":
		m.refresh(true)
	case "a":
		m.startPrompt(modeCapture, "새 태스크: ", "")
		return m, textinput.Blink
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

func (m *Model) switchTab(t tab) {
	if t == m.tab {
		return
	}
	m.tab = t
	m.cursor, m.colCursor, m.rowCursor = 0, 0, 0
	m.listOffset = 0
	m.detail = false
	m.projDrill, m.projSlug = false, ""
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
	in := service.AddInput{Title: title}
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
			m.setStatus("%s 기간 %s~%s (%d일)", t.ShortID(), t.SpanStart(), t.SpanEnd(), t.SpanDays())
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
	sugg := m.svc.NextUp(1)
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
	cmd, err := editor.Command(m.svc.Cfg, p.Path)
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
