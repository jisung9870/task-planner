package service

import "task-planner/internal/domain"

// WorkHistories reads logs through the service, including legacy tasks whose
// timer start was cleared on completion. No source files need migration.
func (s *Service) WorkHistories(tasks []*domain.Task) map[string]domain.WorkHistory {
	out := make(map[string]domain.WorkHistory, len(tasks))
	for _, t := range s.hydrate(tasks) {
		out[t.ID] = t.WorkHistory(s.Now().Location())
	}
	return out
}
