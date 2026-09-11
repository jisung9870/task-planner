package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"task-planner/internal/service"
)

// Run starts the TUI against an open service.
func Run(svc *service.Service) error {
	p := tea.NewProgram(New(svc), tea.WithAltScreen())
	_, err := p.Run()
	return err
}
