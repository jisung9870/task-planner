package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"task-planner/internal/config"
	"task-planner/internal/domain"
	"task-planner/internal/service"
)

func TestHourlyBarSeparatesSessionsAndShowsFinish(t *testing.T) {
	day := domain.NewDate(2026, 9, 26)
	at := func(h, m int) time.Time { return time.Date(2026, 9, 26, h, m, 0, 0, time.UTC) }
	h := domain.WorkHistory{
		Sessions: []domain.WorkSession{{Start: at(9, 15), End: at(10, 45)}, {Start: at(13, 0), End: at(14, 0)}},
		Finishes: []domain.WorkFinish{{At: at(14, 0), Status: domain.StatusDone}},
	}
	bar := []rune(hourlyBar(h, day, at(17, 0), 2))
	if len(bar) != 48 {
		t.Fatalf("width = %d", len(bar))
	}
	if string(bar[18:22]) != "████" || string(bar[22:26]) != "    " || string(bar[26:29]) != "██✓" {
		t.Fatalf("sessions/pauses: %q", string(bar))
	}
	// The current-time guide must not replace a completion that just occurred.
	if got := []rune(hourlyBar(h, day, at(14, 0), 2))[28]; got != '✓' {
		t.Fatalf("finish overwritten: %c", got)
	}
	h = domain.WorkHistory{Sessions: []domain.WorkSession{{Start: at(9, 15), End: at(9, 15)}}}
	if got := []rune(hourlyBar(h, day, at(17, 0), 2))[18]; got != '·' {
		t.Fatalf("instant missing: %c", got)
	}
	h = domain.WorkHistory{Finishes: []domain.WorkFinish{{At: at(14, 0), Status: domain.StatusCancelled}}}
	if got := []rune(hourlyBar(h, day, at(17, 0), 2))[28]; got != '×' {
		t.Fatalf("cancel missing: %c", got)
	}
}

func TestHourlyBarClipsMidnightAndRunningWork(t *testing.T) {
	loc := time.FixedZone("KST", 9*3600)
	at := func(d, h, m int) time.Time { return time.Date(2026, 9, d, h, m, 0, 0, loc) }
	h := domain.WorkHistory{Sessions: []domain.WorkSession{{Start: at(25, 23, 30), End: at(26, 1, 30)}}}
	bar := []rune(hourlyBar(h, domain.DateOf(at(26, 0, 0)), at(27, 12, 0), 2))
	if string(bar[:5]) != "◀██· " {
		t.Fatalf("midnight: %q", string(bar))
	}
	previous := []rune(hourlyBar(h, domain.DateOf(at(25, 0, 0)), at(27, 12, 0), 2))
	if previous[47] != '▶' || previous[46] != ' ' {
		t.Fatalf("previous day: %q", string(previous))
	}
	h = domain.WorkHistory{Sessions: []domain.WorkSession{{Start: at(26, 9, 0)}}}
	bar = []rune(hourlyBar(h, domain.DateOf(at(26, 0, 0)), at(26, 10, 15), 4))
	if string(bar[36:42]) != "█████┊" || strings.TrimSpace(string(bar[42:])) != "" {
		t.Fatalf("running extends past now: %q", string(bar))
	}
	if future := strings.TrimSpace(hourlyBar(h, domain.DateOf(at(27, 0, 0)), at(26, 10, 15), 4)); future != "" {
		t.Fatalf("future: %q", future)
	}
}

func TestHourlyTimelineNavigationAndPersistence(t *testing.T) {
	svc, err := service.Init(config.Default(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 26, 9, 15, 0, 0, time.UTC)
	svc.SetClock(func() time.Time { return now })
	task, err := svc.Add(service.AddInput{Title: "오전 작업", Status: domain.StatusDoing})
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Hour)
	if _, err := svc.Done(task.ID); err != nil {
		t.Fatal(err)
	}
	m := New(svc)
	m.tab, m.width, m.height, m.wideDetail = tabTimeline, 110, 30, false
	m.reload()
	key := func(s string) { m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}) }
	key("z")
	if !m.hourlyTimeline() || len(m.rows) != 1 {
		t.Fatal("z did not open actual hours")
	}
	if d, n := m.timelineWindow(); n != 1 || !d.Equal(svc.Today()) {
		t.Fatalf("window: %s/%d", d, n)
	}
	view := m.View()
	for _, want := range []string{"24시간", "00:00~24:00", "09:15", "11:15", "✓"} {
		if !strings.Contains(view, want) {
			t.Errorf("missing %q in\n%s", want, view)
		}
	}
	key("h")
	if !m.timelineDay().Equal(svc.Today().AddDays(-1)) || len(m.rows) != 0 {
		t.Fatal("h did not move a day")
	}
	key("l")
	if len(m.rows) != 1 {
		t.Fatal("l did not restore task")
	}
	key(".")
	key("t")
	if !m.timelineDay().Equal(svc.Today()) {
		t.Fatal("t did not return to today")
	}
	m.saveUIState()
	if restored := New(svc); !restored.hourlyTimeline() {
		t.Fatal("hourly preference not restored")
	}
	key("z")
	if m.hourlyTimeline() {
		t.Fatal("z did not restore the daily view")
	}
	if d, n := m.timelineWindow(); n < 7 || !d.Equal(svc.Today().WeekStart()) {
		t.Fatalf("daily window: %s/%d", d, n)
	}
	// Hour labels and bars fit both split and unsplit panes.
	key("z")
	for _, width := range []int{40, 60, 80, 110, 130, 200} {
		m.width = width
		labelW, cellW := m.hourlyGeometry()
		if cellW < 1 || cellW > 4 {
			t.Fatalf("cell width = %d", cellW)
		}
		for _, line := range strings.Split(m.hourlyScale(labelW), "\n") {
			if lipgloss.Width(line) > m.contentWidth() {
				t.Errorf("axis exceeds width %d: %q", width, line)
			}
		}
		if w := lipgloss.Width(m.hourlyRow(0, m.rows[0].task, labelW)); w > m.contentWidth() {
			t.Errorf("row width = %d", w)
		}
	}
	m.tlHistory[task.ID] = domain.WorkHistory{Completed: svc.Today()}
	labelW, _ := m.hourlyGeometry()
	unknown := m.hourlyRow(0, m.rows[0].task, labelW)
	if !strings.Contains(unknown, "시각 미기록") || strings.Contains(unknown, "█") {
		t.Fatalf("invented time: %s", unknown)
	}
}
