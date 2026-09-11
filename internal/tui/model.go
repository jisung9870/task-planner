// Package tui is the terminal adapter. It renders what internal/service
// returns and never touches the vault directly.
package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"task-planner/internal/domain"
	"task-planner/internal/query"
	"task-planner/internal/service"
)

type tab int

const (
	tabToday tab = iota
	tabWeek
	tabProjects
	tabAll
)

var tabNames = []string{"Today", "Week", "Projects", "All"}

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
	detail bool

	// projDrill is true while the Projects tab shows one project's tasks;
	// projSlug may legitimately be empty (the "미지정" bucket).
	projDrill bool
	projSlug  string

	input  textinput.Model
	search string

	width, height int

	status string
	errMsg string
	quit   bool
}

// New builds the initial model.
func New(svc *service.Service) *Model {
	in := textinput.New()
	in.Prompt = ""
	in.CharLimit = 400
	m := &Model{svc: svc, input: in}
	m.reload()
	return m
}

func (m *Model) Init() tea.Cmd { return textinput.Blink }

// reload re-runs the current view's query. Cheap: it reads the in-memory index.
func (m *Model) reload() {
	today := m.svc.Today()
	var ts []*domain.Task
	switch m.tab {
	case tabToday:
		ts = m.svc.TodayList()
	case tabWeek:
		ts = m.svc.WeekList(today)
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
	if m.search != "" {
		ts = filterSearch(ts, m.search)
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

func filterSearch(ts []*domain.Task, q string) []*domain.Task {
	q = strings.ToLower(q)
	var out []*domain.Task
	for _, t := range ts {
		hay := strings.ToLower(t.Title + " " + t.Project + " " + strings.Join(t.Tags, " "))
		if strings.Contains(hay, q) {
			out = append(out, t)
		}
	}
	return out
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
