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
	modeSpan
	modeProject
	modeNewProject
	modeProjectDue
	modeNote
	modeSaveView
	modeViews
	modeConfirm
	modeHelp
)

// prompting reports whether the mode is a one-line text prompt.
func (md mode) prompting() bool {
	switch md {
	case modeCapture, modeBlock, modeSearch, modeSpan, modeProject, modeNewProject,
		modeProjectDue, modeNote, modeSaveView:
		return true
	}
	return false
}

// row is one rendered line: a group header, a task, or a project rollup.
type row struct {
	header string
	task   *domain.Task
	proj   *service.ProjectRow
}

func (r row) selectable() bool { return r.task != nil || r.proj != nil }

// tabState is what a tab remembers while another one is on screen. Resetting
// the cursor on every switch made a round trip to Week and back lose the row
// the user was reading.
type tabState struct {
	cursor     int
	listOffset int
	colCursor  int
	rowCursor  int
	projDrill  bool
	projSlug   string
}

// detailCache holds everything the detail pane needs for one task. Without it
// the pane re-reads the markdown file and rescans the index on every redraw -
// once per keystroke while the cursor moves.
type detailCache struct {
	id       string
	task     *domain.Task
	known    []*domain.Task
	missing  []string
	blocking []*domain.Task
}

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

	// helpOffset scrolls the help screen: it is longer than a short terminal,
	// and a help text whose first half is unreachable is worse than none.
	helpOffset int

	// listOffset is the first visible row of the list viewport. The list used
	// to render every row, which scrolled the header off screen past ~15 tasks.
	listOffset int
	// detailOffset scrolls the detail pane (J/K), which grows unbounded once
	// notes start accumulating.
	detailOffset int

	// tabMem remembers each tab's cursor across switches.
	tabMem [tabCount]tabState

	// marked holds the ids selected for a bulk action, keyed by id so the
	// selection survives a reload that reorders or re-buckets rows.
	marked map[string]bool

	// cols holds the lanes of grid tabs (Board: 상태 3열, Week: 요일 7열 +
	// 미배정); colCursor/rowCursor address the selected card.
	cols      [][]*domain.Task
	colCursor int
	rowCursor int
	// weekDays are the 7 dates of the Week grid, parallel to cols[0..6].
	weekDays []domain.Date
	// weekLoads is the planned work per weekday, parallel to weekDays.
	weekLoads []service.DayLoad

	// projDrill is true while the Projects tab shows one project's tasks;
	// projSlug may legitimately be empty (the "미지정" bucket).
	projDrill bool
	projSlug  string

	input  textinput.Model
	search string
	filter *query.Filter
	// pendingFilter is applied while the search prompt is being typed; the
	// previous one is kept so esc restores the view instead of clearing it.
	prevSearch string
	prevFilter *query.Filter
	// history is the per-prompt recall list walked with ↑/↓.
	history   map[mode][]string
	histPos   int
	histDraft string

	// viewCursor is the selection in the saved-view picker.
	viewCursor int
	// confirm holds the action a y/n prompt will run when answered.
	confirm       func()
	confirmPrompt string

	// Per-reload aggregates. Every one of these used to be recomputed inside
	// View(), i.e. on every keystroke and every tick.
	summary       service.Summary
	todayLoad     service.DayLoad
	blockingCount map[string]int
	boardClosed   service.DayCounts
	detail_       detailCache

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
	in.PromptStyle = styPrompt
	in.CharLimit = 400
	m := &Model{svc: svc, input: in, wideDetail: true,
		marked: map[string]bool{}, history: map[mode][]string{}}
	// The tab and panel layout are the two things a user notices resetting on
	// every launch; both are disposable state living next to the index.
	ui := svc.LoadUIState()
	if ui.Tab >= 0 && ui.Tab < tabCount {
		m.tab = tab(ui.Tab)
	}
	m.wideDetail = ui.WideDetail
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

// reload re-runs the current view's query and refreshes everything the frame
// would otherwise recompute. Cheap: it reads the in-memory index.
func (m *Model) reload() {
	today := m.svc.Today()
	m.summary = m.svc.Summarize()
	m.todayLoad = m.svc.DayLoad(today)
	m.blockingCount = m.svc.BlockingCounts()
	m.detail_ = detailCache{}
	m.detailOffset = 0
	var ts []*domain.Task
	switch m.tab {
	case tabToday:
		ts = m.svc.TodayList()
	case tabWeek:
		m.reloadWeek(today)
		return
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
	m.boardClosed = m.svc.ClosedOn(today)
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

// weekLaneUnassigned is the index of the 미배정 lane in the Week grid.
const weekLaneUnassigned = 7

// reloadWeek buckets the week's work by day, plus a lane for tasks that belong
// to the week but have no date inside it. This is the view that finally uses
// query.WeekDays - a status-grouped list cannot show how the week is laid out.
func (m *Model) reloadWeek(today domain.Date) {
	ts := m.svc.WeekList(today)
	if m.filter != nil && !m.filter.Empty() {
		ts = m.svc.ApplyFilter(m.filter, ts)
	}
	buckets, days := query.WeekDays(ts, today)
	m.weekDays = days
	m.weekLoads = m.svc.WeekLoad(today)
	m.cols = make([][]*domain.Task, 8)
	for i, d := range days {
		m.cols[i] = buckets[d]
	}
	m.cols[weekLaneUnassigned] = buckets[domain.Date{}]
	m.clampBoard()
}

// fullTask returns the selected task with its body, cached for the frame. The
// detail pane asks for this on every redraw.
func (m *Model) fullTask(id string) (detailCache, error) {
	if m.detail_.id == id && m.detail_.task != nil {
		return m.detail_, nil
	}
	full, err := m.svc.Load(id)
	if err != nil {
		return detailCache{}, err
	}
	c := detailCache{id: id, task: full}
	c.known, c.missing = m.svc.Blockers(full)
	c.blocking = m.svc.Blocking(id)
	m.detail_ = c
	return c, nil
}

// targets are the tasks the next action applies to: the marked set when there
// is one, otherwise the task under the cursor. Every mutating key goes through
// this, which is what makes marking work everywhere at once.
func (m *Model) targets() []*domain.Task {
	if len(m.marked) == 0 {
		if t := m.current(); t != nil {
			return []*domain.Task{t}
		}
		return nil
	}
	var out []*domain.Task
	seen := map[string]bool{}
	for _, t := range m.visibleTasks() {
		if m.marked[t.ID] && !seen[t.ID] {
			seen[t.ID] = true
			out = append(out, t)
		}
	}
	// A marked task can be filtered out of the current view; it is still
	// marked, and dropping it silently would make bulk actions unpredictable.
	for id := range m.marked {
		if seen[id] {
			continue
		}
		if t, err := m.svc.Resolve(id); err == nil {
			out = append(out, t)
		}
	}
	return out
}

// visibleTasks lists the tasks of the current view in display order.
func (m *Model) visibleTasks() []*domain.Task {
	var out []*domain.Task
	if m.gridTab() {
		for _, col := range m.cols {
			out = append(out, col...)
		}
		return out
	}
	for _, r := range m.rows {
		if r.task != nil {
			out = append(out, r.task)
		}
	}
	return out
}

// toggleMark selects or deselects the task under the cursor.
func (m *Model) toggleMark() {
	t := m.current()
	if t == nil {
		return
	}
	if m.marked[t.ID] {
		delete(m.marked, t.ID)
	} else {
		m.marked[t.ID] = true
	}
	m.setStatus("선택 %d건  (m 토글 · M 해제 · 상태·기간·프로젝트 키가 선택 전체에 적용)", len(m.marked))
	if m.gridTab() {
		m.rowCursor++
		m.clampBoard()
		return
	}
	m.moveCursor(1)
}

// gridTab reports whether the current tab uses the lane/card cursor model.
func (m *Model) gridTab() bool { return m.tab == tabBoard || m.tab == tabWeek }

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

// projectRows renders the per-project rollup, open work first. Projects with a
// file but no tasks are included: a project is created before its work exists,
// and a row that only appears once a task references it would look like the
// creation failed.
func (m *Model) projectRows() []row {
	list, err := m.svc.ProjectRows()
	if err != nil {
		m.setErr(err)
	}
	rows := []row{{header: fmt.Sprintf("프로젝트 (%d)", len(list))}}
	for i := range list {
		p := list[i]
		rows = append(rows, row{proj: &p})
	}
	return rows
}

// currentProj returns the selected project row, if the cursor is on one.
func (m *Model) currentProj() *service.ProjectRow {
	if m.gridTab() || m.cursor < 0 || m.cursor >= len(m.rows) {
		return nil
	}
	return m.rows[m.cursor].proj
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
	if m.gridTab() {
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
