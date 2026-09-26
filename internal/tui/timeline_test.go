package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"task-planner/internal/config"
	"task-planner/internal/domain"
	"task-planner/internal/service"
)

// The window is whole weeks and never eats the label gutter: a chart wide
// enough to draw two months and too narrow to read a title is not a plan.
func TestTimelineWindowFitsWidth(t *testing.T) {
	start := domain.NewDate(2026, time.September, 7)
	cases := []struct {
		width, days int
	}{
		{200, 28}, // capped at tlMaxWeeks
		{120, 28},
		{100, 28},
		{80, 21},
		{64, 14},
		{46, 7},
		{20, 7}, // never below one week, even when nothing fits
	}
	for _, c := range cases {
		m := &Model{tlStart: start, width: c.width}
		got, days := m.timelineWindow()
		if days != c.days {
			t.Errorf("width %d: %d일 창, want %d", c.width, days, c.days)
		}
		if days%7 != 0 {
			t.Errorf("width %d: 창이 주 단위가 아님 (%d일)", c.width, days)
		}
		if !got.Equal(start) {
			t.Errorf("width %d: 시작이 %s 로 밀림", c.width, got)
		}
		if label := c.width - days*tlCell - 1; label < 8 && c.width > 40 {
			t.Errorf("width %d: 라벨 폭이 %d 로 눌림", c.width, label)
		}
	}
}

func TestTimelineSwitchesBetweenPlanAndActualWithPersistedSelection(t *testing.T) {
	cfg := config.Default(t.TempDir())
	svc, err := service.Init(cfg)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 21, 9, 15, 0, 0, time.UTC)
	svc.SetClock(func() time.Time { return now })
	day := domain.DateOf(now)
	planned, err := svc.Add(service.AddInput{Title: "계획만 있음", Scheduled: day, Due: day.AddDays(2)})
	if err != nil {
		t.Fatal(err)
	}
	worked, err := svc.Add(service.AddInput{Title: "일정 없이 작업", Status: domain.StatusDoing})
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Hour)
	if _, err := svc.Done(worked.ID); err != nil {
		t.Fatal(err)
	}
	m := New(svc)
	m.tab, m.width, m.height, m.wideDetail = tabTimeline, 100, 30, false
	m.reload()
	if len(m.rows) != 1 || m.rows[0].task.ID != planned.ID {
		t.Fatalf("planned rows: %+v", m.rows)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	if !m.tlActual || len(m.rows) != 1 || m.rows[0].task.ID != worked.ID {
		t.Fatalf("actual rows: %+v", m.rows)
	}
	view := m.View()
	for _, want := range []string{"실제 작업", "2026-09-21 09:15", "2026-09-21 11:15", "실제 기록 없음 1건"} {
		if !strings.Contains(view, want) {
			t.Errorf("missing %q in\n%s", want, view)
		}
	}
	for _, hit := range m.hits {
		if hit.kind == hitRow && hit.y != bodyTop+tlHeadLines+3 {
			t.Errorf("row hit at %d", hit.y)
		}
	}
	m.saveUIState()
	if restored := New(svc); !restored.tlActual || restored.tab != tabTimeline {
		t.Fatal("view mode not restored")
	}
	m.filter, err = svc.Filter("status:todo")
	if err != nil {
		t.Fatal(err)
	}
	m.reload()
	if len(m.rows) != 0 {
		t.Fatal("actual view ignored filter")
	}
	m.filter = nil
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	if m.tlActual || len(m.rows) != 1 || m.rows[0].task.ID != planned.ID {
		t.Fatal("plan changed after toggle")
	}
}

func TestActualTimelineLeavesPausedDaysBlank(t *testing.T) {
	svc, err := service.Init(config.Default(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC)
	svc.SetClock(func() time.Time { return now })
	task, err := svc.Add(service.AddInput{Title: "중단과 재개", Status: domain.StatusDoing})
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Hour)
	if _, err := svc.SetStatus(task.ID, domain.StatusTodo, nil); err != nil {
		t.Fatal(err)
	}
	now = now.AddDate(0, 0, 14)
	if _, err := svc.Start(task.ID); err != nil {
		t.Fatal(err)
	}
	m := New(svc)
	m.tab, m.tlActual, m.width = tabTimeline, true, 46
	m.tlStart = domain.NewDate(2026, 9, 28)
	m.reload()
	if len(m.rows) != 0 {
		t.Fatal("paused week shown as worked")
	}
	m.shiftTimeline(1)
	if len(m.rows) != 1 {
		t.Fatal("running session missing")
	}
	view := m.timelineView(15)
	if !strings.Contains(view, "진행중") || !strings.Contains(view, "██") {
		t.Fatalf("running view:\n%s", view)
	}
}
