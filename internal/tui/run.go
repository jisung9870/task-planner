package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"task-planner/internal/service"
	"task-planner/internal/watch"
)

// Run starts the TUI against an open service.
//
// A failed watcher is not fatal: the TUI still works, the user just has to
// press r after editing files elsewhere.
func Run(svc *service.Service) error {
	initTheme()
	m := New(svc)
	if w, err := watch.New(svc.Vault().TasksDir(), watch.DefaultDebounce); err != nil {
		m.setStatus("파일 감시 비활성 (%v) — r 로 새로고침하세요", err)
	} else {
		m.SetWatcher(w)
		defer w.Close()
	}
	// Mouse support is additive: every key binding keeps working, and clicking
	// a card beats pressing h/l six times to reach Thursday.
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	_, err := p.Run()
	return err
}
