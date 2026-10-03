package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"task-planner/internal/config"
	"task-planner/internal/domain"
	"task-planner/internal/service"
)

func sinceModel(t *testing.T) (*Model, *service.Service) {
	t.Helper()
	svc, err := service.Init(config.Default(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { svc.Close() })
	m := New(svc)
	m.tab = tabAll
	return m, svc
}

func key(r rune) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}} }

// d on a task the timer never saw asks when it started; the answer lands in
// the log and the task completes.
func TestDoneAsksStartAndBackfills(t *testing.T) {
	m, svc := sinceModel(t)
	task, _ := svc.Add(service.AddInput{Title: "보고서"})
	m.reload()
	m.selectID(task.ID)
	m.updateNormal(key('d'))
	if m.mode != modeSince {
		t.Fatalf("mode = %v, want the 착수 시각 prompt", m.mode)
	}
	m.input.SetValue("1h")
	m.updatePrompt(tea.KeyMsg{Type: tea.KeyEnter})
	full, _ := svc.Load(task.ID)
	if full.Status != domain.StatusDone {
		t.Fatalf("status = %s", full.Status)
	}
	logs := full.LogLines()
	if !strings.Contains(logs[len(logs)-1], "착수 소급") {
		t.Errorf("log = %q", logs[len(logs)-1])
	}
}

// Enter on an empty answer completes as recorded, so asking never costs more
// than one key.
func TestDoneSkipsStartWithEmptyAnswer(t *testing.T) {
	m, svc := sinceModel(t)
	task, _ := svc.Add(service.AddInput{Title: "보고서"})
	m.reload()
	m.selectID(task.ID)
	m.updateNormal(key('d'))
	m.updatePrompt(tea.KeyMsg{Type: tea.KeyEnter})
	full, _ := svc.Load(task.ID)
	if full.Status != domain.StatusDone || strings.Contains(full.Body, "착수 소급") {
		t.Fatalf("status=%s body=%q", full.Status, full.Body)
	}
}

// A bad time keeps the prompt open with the text, the task still open.
func TestDoneKeepsPromptOnBadTime(t *testing.T) {
	m, svc := sinceModel(t)
	task, _ := svc.Add(service.AddInput{Title: "보고서"})
	m.reload()
	m.selectID(task.ID)
	m.updateNormal(key('d'))
	m.input.SetValue("언젠가")
	m.updatePrompt(tea.KeyMsg{Type: tea.KeyEnter})
	if m.mode != modeSince || m.input.Value() != "언젠가" {
		t.Fatalf("mode=%v value=%q", m.mode, m.input.Value())
	}
	if full, _ := svc.Load(task.ID); full.Status != domain.StatusTodo {
		t.Fatalf("status = %s", full.Status)
	}
}

// Agent work and bulk completion never ask.
func TestDoneDoesNotAskForAgentWork(t *testing.T) {
	m, svc := sinceModel(t)
	m.scope = "all"
	task, _ := svc.Add(service.AddInput{Title: "agent 작업", Executor: domain.ExecutorAgent})
	m.reload()
	m.selectID(task.ID)
	m.updateNormal(key('d'))
	if m.mode == modeSince {
		t.Fatal("agent work asked for a start time")
	}
	if full, _ := svc.Load(task.ID); full.Status != domain.StatusDone {
		t.Fatalf("status = %s", full.Status)
	}
}
