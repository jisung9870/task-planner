package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

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
		default:
			return m.updateNormal(msg)
		}
	}
	return m, nil
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
		if t := m.current(); t != nil {
			m.apply(t, domain.NextStatus(t.Status), nil)
		}
	case "s":
		if p := m.currentProj(); p != nil {
			m.cycleProject(p)
			break
		}
		m.applyCurrent(domain.StatusDoing)
	case "d":
		m.applyCurrent(domain.StatusDone)
	case "x":
		m.applyCurrent(domain.StatusCancelled)
	case "u":
		m.applyCurrent(domain.StatusTodo)
	case "b":
		if t := m.current(); t != nil {
			m.startPrompt(modeBlock, "보류 사유: ", t.BlockedReason)
			return m, textinput.Blink
		}
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
	case "!":
		m.jumpNext()
	case "N":
		if t := m.current(); t != nil {
			m.startPrompt(modeNote, "메모: ", "")
			return m, textinput.Blink
		}
	case "n":
		if m.tab == tabProjects && !m.projDrill {
			m.startPrompt(modeNewProject, "새 프로젝트 (slug [이름]): ", "")
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
}

func (m *Model) updatePrompt(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeNormal
		m.input.Blur()
		return m, nil
	case "enter":
		value := strings.TrimSpace(m.input.Value())
		md := m.mode
		m.mode = modeNormal
		m.input.Blur()
		switch md {
		case modeCapture:
			m.capture(value)
		case modeSearch:
			f, err := m.svc.Filter(value)
			if err != nil {
				m.setErr(err)
				return m, nil
			}
			m.search, m.filter = value, f
			m.cursor = 0
			m.reload()
			if value == "" {
				m.setStatus("필터 해제")
			} else {
				m.setStatus("필터: %s — %d건", value, countTasks(m.rows))
			}
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
			if t := m.current(); t != nil {
				if value == "" {
					m.setErr(domain.ErrBlockedNeedsReason)
					return m, nil
				}
				m.apply(t, domain.StatusBlocked, &domain.BlockInfo{Reason: value})
			}
		}
		return m, nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
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
	res, err := m.svc.AddWithResult(in)
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

func (m *Model) applyCurrent(to domain.Status) {
	if t := m.current(); t != nil {
		m.apply(t, to, nil)
	}
}

func (m *Model) apply(t *domain.Task, to domain.Status, block *domain.BlockInfo) {
	if to == domain.StatusBlocked && block == nil {
		m.startPrompt(modeBlock, "보류 사유: ", t.BlockedReason)
		return
	}
	res, err := m.svc.SetStatus(t.ID, to, block)
	if err != nil {
		m.setErr(err)
		return
	}
	id := res.Task.ID
	m.reload()
	m.selectID(id)
	m.setStatus("%s → %s", res.Task.ShortID(), to.Label())
	if res.Next != nil {
		m.status += fmt.Sprintf("  · 다음 회차 %s (%s)", res.Next.ShortID(), res.Next.Scheduled)
	}
	for _, w := range res.Warnings {
		m.status += "  · " + w
	}
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

// shiftSpan slides the selected task's whole 진행 기간 by a day, keeping its
// length - the re-planning gesture ("이건 하루 밀자"). An unscheduled task gets
// today first, so the first press pulls it out of the 미배정 lane instead of
// jumping blindly.
func (m *Model) shiftSpan(days int) {
	t := m.current()
	if t == nil {
		return
	}
	res, err := m.svc.ShiftSpan(t.ID, days)
	if err != nil {
		m.setErr(err)
		return
	}
	m.afterSpanChange(res.Task, "이동")
}

// resizeSpan moves only the end of the period: the task starts when it started
// and now takes longer (or less).
func (m *Model) resizeSpan(days int) {
	t := m.current()
	if t == nil {
		return
	}
	res, err := m.svc.ResizeSpan(t.ID, days)
	if err != nil {
		m.setErr(err)
		return
	}
	m.afterSpanChange(res.Task, "조정")
}

// setSpan applies a typed period ("09-15~09-19").
func (m *Model) setSpan(value string) {
	t := m.current()
	if t == nil || value == "" {
		return
	}
	sp, err := m.svc.ParseSpan(value)
	if err != nil {
		m.setErr(err)
		return
	}
	res, err := m.svc.SetSpan(t.ID, sp)
	if err != nil {
		m.setErr(err)
		return
	}
	m.afterSpanChange(res.Task, "설정")
}

// afterSpanChange re-reads the view and reports the period in one line.
func (m *Model) afterSpanChange(t *domain.Task, verb string) {
	m.reload()
	m.selectID(t.ID)
	switch {
	case t.SpanDays() == 0:
		m.setStatus("%s 기간 해제", t.ShortID())
	case t.HasSpan():
		m.setStatus("%s 기간 %s~%s (%d일) %s", t.ShortID(),
			t.SpanStart(), t.SpanEnd(), t.SpanDays(), verb)
	default:
		d := t.SpanStart()
		m.setStatus("%s 예정 → %s (%s) %s", t.ShortID(), d, d.WeekdayKO(), verb)
	}
}

// setProject re-homes the selected task. "-" clears the assignment.
func (m *Model) setProject(value string) {
	t := m.current()
	if t == nil {
		return
	}
	slug := value
	if slug == "-" || slug == "없음" {
		slug = ""
	} else if slug != "" {
		slug = service.ProjectSlug(slug)
	}
	res, err := m.svc.Edit(t.ID, service.EditInput{Project: &slug})
	if err != nil {
		m.setErr(err)
		return
	}
	m.reload()
	m.selectID(res.Task.ID)
	if slug == "" {
		m.setStatus("%s 프로젝트 해제", res.Task.ShortID())
		return
	}
	m.setStatus("%s → 프로젝트 %s", res.Task.ShortID(), slug)
}

// newProject creates projects/<slug>/project.md. The prompt takes "slug 이름"
// on one line: two prompts for two obvious fields is one prompt too many.
func (m *Model) newProject(value string) {
	if value == "" {
		return
	}
	slug, name, _ := strings.Cut(value, " ")
	p, err := m.svc.CreateProject(slug, strings.TrimSpace(name))
	if err != nil {
		m.setErr(err)
		return
	}
	m.reload()
	m.selectProject(p.Slug)
	m.setStatus("프로젝트 추가됨 %s  %s", p.Slug, p.Display())
}

// addNote appends a timestamped line to the selected task's note section -
// the thing you want to write down at the moment you hear it.
func (m *Model) addNote(text string) {
	t := m.current()
	if t == nil || text == "" {
		return
	}
	res, err := m.svc.AddNote(t.ID, text)
	if err != nil {
		m.setErr(err)
		return
	}
	m.reload()
	m.selectID(res.Task.ID)
	m.setStatus("%s 메모 추가", res.Task.ShortID())
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
	p, err := m.svc.EditProject(r.Slug, service.ProjectEditInput{Due: &d})
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
	p, err := m.svc.CycleProjectStatus(r.Slug)
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
