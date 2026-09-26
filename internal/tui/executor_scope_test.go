package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"task-planner/internal/config"
	"task-planner/internal/domain"
	"task-planner/internal/service"
)

func TestExecutorScopeCyclesAndPersists(t *testing.T) {
	svc, err := service.Init(config.Default(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()
	if _, err := svc.Add(service.AddInput{Title: "사람 작업", Project: "same"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Add(service.AddInput{Title: "agent 작업", Project: "same", Executor: domain.ExecutorAgent}); err != nil {
		t.Fatal(err)
	}
	m := New(svc)
	m.tab = tabAll
	m.reload()
	if m.scope != "human" || countTasks(m.rows) != 1 {
		t.Fatalf("human scope = %s, rows=%d", m.scope, countTasks(m.rows))
	}
	m.updateNormal(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'F'}})
	if m.scope != "agent" || countTasks(m.rows) != 1 || m.rows[len(m.rows)-1].task.Executor != domain.ExecutorAgent {
		t.Fatalf("agent scope = %s, rows=%v", m.scope, m.rows)
	}
	m.updateNormal(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'F'}})
	if m.scope != "all" || countTasks(m.rows) != 2 {
		t.Fatalf("all scope = %s, rows=%d", m.scope, countTasks(m.rows))
	}
	if got := m.projectRows(); len(got) != 2 || got[1].proj.Open != 2 {
		t.Fatalf("shared project = %v", got)
	}
	if New(svc).scope != "all" {
		t.Fatal("scope was not persisted")
	}
}
