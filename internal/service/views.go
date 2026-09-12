package service

import (
	"fmt"
	"strings"

	"task-planner/internal/config"
	"task-planner/internal/domain"
)

// Views returns the saved filter expressions.
func (s *Service) Views() []config.View { return s.Cfg.Views }

// SaveView stores a query under a name, replacing a view of the same name, and
// writes config.yaml. Saving is what turns a query that worked into a query
// that gets reused; retyping it every morning is why nobody reuses them.
func (s *Service) SaveView(name, query string) error {
	name = strings.TrimSpace(name)
	query = strings.TrimSpace(query)
	if name == "" {
		return fmt.Errorf("뷰 이름이 비어 있음")
	}
	if query == "" {
		return fmt.Errorf("저장할 질의가 비어 있음")
	}
	if _, err := s.Filter(query); err != nil {
		return err
	}
	for i := range s.Cfg.Views {
		if strings.EqualFold(s.Cfg.Views[i].Name, name) {
			s.Cfg.Views[i].Query = query
			return config.Save(s.Cfg)
		}
	}
	s.Cfg.Views = append(s.Cfg.Views, config.View{Name: name, Query: query})
	return config.Save(s.Cfg)
}

// DeleteView removes a saved view.
func (s *Service) DeleteView(name string) error {
	for i, v := range s.Cfg.Views {
		if strings.EqualFold(v.Name, name) {
			s.Cfg.Views = append(s.Cfg.Views[:i], s.Cfg.Views[i+1:]...)
			return config.Save(s.Cfg)
		}
	}
	return fmt.Errorf("그런 뷰가 없음: %s", name)
}

// BlockingCounts maps a task id to how many open tasks wait on it.
//
// The list views need this per row. Asking Blocking() once per row walks the
// whole index per row; one pass here answers every row.
func (s *Service) BlockingCounts() map[string]int {
	counts := map[string]int{}
	for _, t := range s.All() {
		if !t.IsOpen() {
			continue
		}
		for _, dep := range t.BlockedBy {
			counts[dep]++
		}
	}
	return counts
}

// DayCounts is what the board footer summarises: work that finished today and
// therefore does not take a lane.
type DayCounts struct {
	Done      int
	Cancelled int
}

// ClosedOn counts the work that reached a terminal state on a given day.
func (s *Service) ClosedOn(d domain.Date) DayCounts {
	var c DayCounts
	for _, t := range s.All() {
		if !t.Completed.Equal(d) {
			continue
		}
		switch t.Status {
		case domain.StatusDone:
			c.Done++
		case domain.StatusCancelled:
			c.Cancelled++
		}
	}
	return c
}
