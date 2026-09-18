package tui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"task-planner/internal/domain"
)

// A grid column plus its gutter has to fit inside the width it was handed.
// Overflow was invisible while the grids owned the whole screen - the spare
// columns fell off the right edge - and became a torn divider the moment the
// detail pane started taking half the frame.
func TestGridColumnsFitTheirWidth(t *testing.T) {
	for _, n := range []int{len(boardColumns), 7} {
		for _, w := range []int{60, 71, 80, 100, 110, 130, 200} {
			col := gridColWidth(w, n)
			if col < 1 {
				continue // too narrow for a grid at all; the stacked layout takes over
			}
			if got := (col + 2) * n; got > w {
				t.Errorf("%d열 · 폭 %d: 열이 %d 칸을 차지 (넘침)", n, w, got)
			}
		}
	}
}

// The detail pane is not a list-tab feature: every tab that lists tasks opens
// it, and the content shrinks into the left column to make room. Board and Week
// are the regression - they used to swallow enter and render nothing.
func TestSplitActiveOnEveryTaskTab(t *testing.T) {
	for tb := tab(0); tb < tabCount; tb++ {
		if tb == tabProjects {
			continue // covered below: the rollup lists projects, not tasks
		}
		m := &Model{width: 130, tab: tb, wideDetail: true}
		if !m.splitActive() {
			t.Errorf("%s 탭: 상세 패널이 열리지 않음", tabNames[tb])
		}
		if got := m.contentWidth(); got >= m.innerWidth() {
			t.Errorf("%s 탭: 본문이 %d 칸 — 패널 자리를 안 비움", tabNames[tb], got)
		}
	}
}

// The Projects rollup is the one screen with no task under the cursor, so it
// keeps the whole width until the user drills into a project.
func TestProjectsRollupKeepsFullWidth(t *testing.T) {
	m := &Model{width: 130, tab: tabProjects, wideDetail: true}
	if m.splitActive() {
		t.Error("프로젝트 목록에서 빈 패널이 열림")
	}
	if m.contentWidth() != m.innerWidth() {
		t.Error("프로젝트 목록인데 본문 폭이 줄어듦")
	}
	m.projDrill = true
	if !m.splitActive() {
		t.Error("프로젝트 드릴인 후에도 패널이 안 열림")
	}
}

// The reserved width must not depend on what the cursor is on. The Timeline
// picks its rows from a day range derived from contentWidth, so a pane that
// appeared only once a row existed would let the width decide the rows and the
// rows decide the width.
func TestPaneWidthIgnoresSelection(t *testing.T) {
	task := &domain.Task{ID: "T-1", Title: "확인용"}
	start := domain.NewDate(2026, time.September, 7) // set so timelineWindow needs no clock
	empty := &Model{width: 130, tab: tabTimeline, wideDetail: true, tlStart: start}
	filled := &Model{width: 130, tab: tabTimeline, wideDetail: true, tlStart: start,
		rows: []row{{task: task}}}
	if empty.contentWidth() != filled.contentWidth() {
		t.Errorf("선택 여부로 본문 폭이 달라짐: %d vs %d",
			empty.contentWidth(), filled.contentWidth())
	}
	if _, days := empty.timelineWindow(); days != 21 {
		t.Errorf("빈 타임라인의 창이 %d일 — 패널을 세지 않음", days)
	}
}

// A narrow terminal has no room for a side pane, and a folded pane stays
// folded.
func TestSplitActiveNeedsWidthAndConsent(t *testing.T) {
	if m := (&Model{width: 80, wideDetail: true}); m.splitActive() {
		t.Error("좁은 화면에서 우측 패널이 열림")
	}
	if m := (&Model{width: 130}); m.splitActive() {
		t.Error("접어둔 패널이 열림")
	}
	if m := (&Model{width: 80}); m.contentWidth() != m.innerWidth() {
		t.Error("패널이 없는데 본문 폭이 줄어듦")
	}
}

// m.detail and m.wideDetail belong to two layouts, and only one may be set at
// a time: a pane opened on a narrow terminal that is then widened would
// otherwise leave enter cycling between the split and a bottom pane with no
// way to close either. The resize carries the pane over rather than dropping
// it - a bigger window is not a change of mind.
func TestResizeMigratesTheDetailPane(t *testing.T) {
	task := &domain.Task{ID: "T-1", Title: "확인용"}
	m := &Model{width: 130, height: 30, tab: tabAll, rows: []row{{task: task}}}

	m.toggleDetail() // wide: open the right pane
	if !m.wideDetail || m.detail {
		t.Fatalf("넓은 화면 토글: wideDetail=%v detail=%v", m.wideDetail, m.detail)
	}
	m.toggleDetail()
	if m.wideDetail || m.detail {
		t.Fatal("다시 눌러도 패널이 안 닫힘")
	}

	m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	m.toggleDetail() // narrow: open the bottom pane
	if !m.detail {
		t.Fatal("좁은 화면에서 하단 패널이 안 열림")
	}

	m.Update(tea.WindowSizeMsg{Width: 130, Height: 30})
	if m.detail {
		t.Error("넓혔는데 좁은 화면 플래그가 남음")
	}
	if !m.wideDetail {
		t.Error("넓혔더니 열어둔 패널이 사라짐")
	}
	m.toggleDetail()
	if m.wideDetail || m.detail {
		t.Error("넓힌 뒤 enter 한 번으로 안 닫힘")
	}
}

// Nothing to show must not hand two fifths of a narrow screen to an empty pane -
// by click or by key. An empty drilled-in project is the reachable case.
func TestDetailNeedsATask(t *testing.T) {
	m := &Model{width: 80, tab: tabProjects, projDrill: true}
	m.openDetail()
	if m.detail {
		t.Error("클릭: 선택된 태스크가 없는데 패널이 열림")
	}
	m.toggleDetail()
	if m.detail {
		t.Error("enter: 선택된 태스크가 없는데 패널이 열림")
	}
	m.rows = []row{{task: &domain.Task{ID: "T-1", Title: "확인용"}}}
	m.toggleDetail()
	if !m.detail {
		t.Error("태스크가 있는데 패널이 안 열림")
	}
}
