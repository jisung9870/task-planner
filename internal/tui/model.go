// Package tui is the terminal adapter. It renders what internal/service
// returns and never touches the vault directly.
package tui

import (
	"fmt"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"task-planner/internal/domain"
	"task-planner/internal/query"
	"task-planner/internal/service"
	"task-planner/internal/watch"
)

type tab int

const (
	tabToday tab = iota
	tabWeek
	tabBoard
	tabProjects
	tabAll
)

const tabCount = 5

var tabNames = []string{"Today", "Week", "Board", "Projects", "All"}

// boardColumns are the lanes of the kanban view. Terminal states are summarised
// in the header instead of taking a column: a done pile that grows forever
// squeezes the lanes that still need attention.
var boardColumns = []domain.Status{domain.StatusTodo, domain.StatusDoing, domain.StatusBlocked}

type mode int

const (
	modeNormal mode = iota
	modeCapture
	modeBlock
	modeSearch
	modeHelp
)

// row is one rendered line: a group header, a task, or a project rollup.
type row struct {
	header string
	task   *domain.Task
	proj   *query.ProjectCount
}

func (r row) selectable() bool { return r.task != nil || r.proj != nil }

// Model is the bubbletea state.
type Model struct {
	svc *service.Service

	tab    tab
	mode   mode
	rows   []row
	cursor int
	// detail: bottom pane toggle for narrow terminals.
	// wideDetail: right pane toggle for wide terminals - on by default, because
	// the pane is the point of the split layout.
	detail     bool
	wideDetail bool

	// listOffset is the first visible row of the list viewport. The list used
	// to render every row, which scrolled the header off screen past ~15 tasks.
	listOffset int

	// cols holds the board lanes; colCursor/rowCursor address the selected card.
	cols      [][]*domain.Task
	colCursor int
	rowCursor int

	// projDrill is true while the Projects tab shows one project's tasks;
	// projSlug may legitimately be empty (the "미지정" bucket).
	projDrill bool
	projSlug  string

	input  textinput.Model
	search string
	filter *query.Filter

	width, height int

	status string
	errMsg string
	quit   bool

	// watcher reports edits made outside this process.
	watcher *watch.Watcher
}

// New builds the initial model.
func New(svc *service.Service) *Model {
	in := textinput.New()
	in.Prompt = ""
	in.CharLimit = 400
	m := &Model{svc: svc, input: in, wideDetail: true}
	// Rolling over before the first render means the morning view is already
	// correct instead of showing yesterday's dates.
	if rep, err := svc.RolloverIfEnabled(); err != nil {
		m.setErr(err)
	} else if !rep.Empty() {
		m.setStatus("%d건 이월됨", len(rep.Rolled))
		if w := rep.StaleWarning(); w != "" {
			m.status += "  · " + w
		}
	}
	m.reload()
	return m
}

func (m *Model) Init() tea.Cmd {
	if m.watcher == nil {
		return tea.Batch(textinput.Blink, tickCmd())
	}
	return tea.Batch(textinput.Blink, tickCmd(), waitForChange(m.watcher))
}

// SetWatcher attaches an external-change source before the program starts.
func (m *Model) SetWatcher(w *watch.Watcher) { m.watcher = w }

// reload re-runs the current view's query. Cheap: it reads the in-memory index.
func (m *Model) reload() {
	today := m.svc.Today()
	var ts []*domain.Task
	switch m.tab {
	case tabToday:
		ts = m.svc.TodayList()
	case tabWeek:
		ts = m.svc.WeekList(today)
	case tabBoard:
		m.reloadBoard(today)
		return
	case tabProjects:
		if m.projDrill {
			ts = m.svc.ProjectList(m.projSlug, false)
		} else {
			m.rows = m.projectRows()
			m.clampCursor()
			return
		}
	case tabAll:
		ts = m.svc.All()
		domain.SortDefault(ts, today)
	}
	if m.filter != nil && !m.filter.Empty() {
		ts = m.svc.ApplyFilter(m.filter, ts)
	}
	m.rows = groupRows(ts)
	m.clampCursor()
}

// groupRows flattens a task list into header + task rows.
func groupRows(ts []*domain.Task) []row {
	groups := domain.GroupByStatus(ts)
	var rows []row
	for _, st := range domain.AllStatuses {
		g := groups[st]
		if len(g) == 0 {
			continue
		}
		rows = append(rows, row{header: fmt.Sprintf("%s (%d)", st.Label(), len(g))})
		for _, t := range g {
			rows = append(rows, row{task: t})
		}
	}
	return rows
}

// reloadBoard buckets open work into lanes. The board always shows the whole
// open backlog: its job is to answer "진행중이 몇 개인가", which a filtered
// subset cannot do.
func (m *Model) reloadBoard(today domain.Date) {
	ts := m.svc.OpenList()
	if m.filter != nil && !m.filter.Empty() {
		ts = m.svc.ApplyFilter(m.filter, ts)
	}
	m.cols = make([][]*domain.Task, len(boardColumns))
	for _, t := range ts {
		for i, st := range boardColumns {
			if t.Status == st {
				m.cols[i] = append(m.cols[i], t)
				break
			}
		}
	}
	m.clampBoard()
}

// clampBoard keeps the cursor on a real card, preferring to stay in the current
// lane and falling back to the nearest non-empty one.
func (m *Model) clampBoard() {
	if len(m.cols) == 0 {
		return
	}
	if m.colCursor < 0 {
		m.colCursor = 0
	}
	if m.colCursor >= len(m.cols) {
		m.colCursor = len(m.cols) - 1
	}
	if len(m.cols[m.colCursor]) == 0 {
		for i := range m.cols {
			if len(m.cols[i]) > 0 {
				m.colCursor = i
				break
			}
		}
	}
	if n := len(m.cols[m.colCursor]); m.rowCursor >= n {
		m.rowCursor = n - 1
	}
	if m.rowCursor < 0 {
		m.rowCursor = 0
	}
}

// moveColumn jumps to the next lane that has cards.
func (m *Model) moveColumn(delta int) {
	if len(m.cols) == 0 {
		return
	}
	i := m.colCursor
	for n := 0; n < len(m.cols); n++ {
		i += delta
		if i < 0 || i >= len(m.cols) {
			return
		}
		if len(m.cols[i]) > 0 {
			m.colCursor, m.rowCursor = i, 0
			return
		}
	}
}

// projectRows renders the per-project rollup, open work first.
func (m *Model) projectRows() []row {
	counts := m.svc.ProjectCounts()
	rows := []row{{header: fmt.Sprintf("프로젝트 (%d)", len(counts))}}
	for i := range counts {
		c := counts[i]
		rows = append(rows, row{proj: &c})
	}
	return rows
}

func (m *Model) clampCursor() {
	if m.cursor >= len(m.rows) {
		m.cursor = len(m.rows) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor < len(m.rows) && !m.rows[m.cursor].selectable() {
		m.moveCursor(1)
	}
}

// moveCursor skips header rows so the selection always lands on a task.
func (m *Model) moveCursor(delta int) {
	if len(m.rows) == 0 {
		return
	}
	i := m.cursor
	for n := 0; n < len(m.rows); n++ {
		i += delta
		if i < 0 || i >= len(m.rows) {
			return
		}
		if m.rows[i].selectable() {
			m.cursor = i
			return
		}
	}
}

// current returns the selected task, if any.
func (m *Model) current() *domain.Task {
	if m.tab == tabBoard {
		if m.colCursor < len(m.cols) && m.rowCursor < len(m.cols[m.colCursor]) {
			return m.cols[m.colCursor][m.rowCursor]
		}
		return nil
	}
	if m.cursor < 0 || m.cursor >= len(m.rows) {
		return nil
	}
	return m.rows[m.cursor].task
}

func (m *Model) setStatus(format string, args ...any) {
	m.status = fmt.Sprintf(format, args...)
	m.errMsg = ""
}

func (m *Model) setErr(err error) {
	if err == nil {
		return
	}
	m.errMsg = err.Error()
	m.status = ""
}
