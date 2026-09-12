package cli

import (
	"task-planner/internal/domain"
	"task-planner/internal/service"
)

// parseDateFlag resolves a date flag against today. The shorthands themselves
// live in domain.ParseDateRef so the TUI prompts accept exactly the same forms.
func parseDateFlag(svc *service.Service, s string) (domain.Date, error) {
	return svc.ParseDate(s)
}
