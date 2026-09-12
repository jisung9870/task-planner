package service

import (
	"fmt"
	"strings"

	"task-planner/internal/config"
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
