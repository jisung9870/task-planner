package tui

import (
	"strings"

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
	case vaultChangedMsg:
		m.handleVaultChanged()
		return m, waitForChange(m.watcher)
	case tea.KeyMsg:
		switch m.mode {
		case modeCapture, modeBlock, modeSearch:
			return m.updatePrompt(msg)
		case modeHelp:
			m.mode = modeNormal
			return m, nil
		default:
			return m.updateNormal(msg)
		}
	}
	return m, nil
}

func (m *Model) updateNormal(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		m.quit = true
		return m, tea.Quit
	case "?":
		m.mode = modeHelp
	case "up", "k":
		m.moveCursor(-1)
	case "down", "j":
		m.moveCursor(1)
	case "g", "home":
		m.cursor = 0
		m.clampCursor()
	case "G", "end":
		m.cursor = len(m.rows) - 1
		m.clampCursor()
	case "1", "2", "3", "4":
		m.switchTab(tab(int(msg.String()[0] - '1')))
	case "tab":
		m.switchTab((m.tab + 1) % 4)
	case "shift+tab":
		m.switchTab((m.tab + 3) % 4)
	case "esc":
		switch {
		case m.detail:
			m.detail = false
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
		m.detail = !m.detail
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
		return m, m.openEditor()
	}
	return m, nil
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
	m.cursor = 0
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
	if m.projDrill {
		in.Project = m.projSlug
	}
	t, err := m.svc.Add(in)
	if err != nil {
		m.setErr(err)
		return
	}
	m.reload()
	m.selectID(t.ID)
	m.setStatus("추가됨 %s  %s", t.ShortID(), t.Title)
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
	for _, w := range res.Warnings {
		m.status += "  · " + w
	}
}

// selectID keeps the cursor on the same task across a reload.
func (m *Model) selectID(id string) {
	for i, r := range m.rows {
		if r.task != nil && r.task.ID == id {
			m.cursor = i
			return
		}
	}
	m.clampCursor()
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
