package cli

import (
	"github.com/spf13/cobra"

	"task-planner/internal/service"
	"task-planner/internal/tui"
)

// runTUI is the bare `tp` entry point.
func runTUI(cmd *cobra.Command) error {
	return withService(func(svc *service.Service) error {
		return tui.Run(svc)
	})
}
