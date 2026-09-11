package service

import (
	"fmt"
	"os"
	"path/filepath"

	"task-planner/internal/domain"
	"task-planner/internal/query"
	"task-planner/internal/report"
	"task-planner/internal/store"
)

// WeekReport renders the weekly report for the week containing ref.
func (s *Service) WeekReport(ref domain.Date) string {
	if ref.IsZero() {
		ref = s.Today()
	}
	return report.Week(report.WeekInput{Tasks: s.All(), Ref: ref, Today: s.Today()})
}

// WriteWeekReport saves the report under reports/YYYY-Www.md and returns the
// path. Regenerating overwrites: the report is derived from task data, so an
// edited copy belongs somewhere else.
func (s *Service) WriteWeekReport(ref domain.Date) (string, error) {
	if ref.IsZero() {
		ref = s.Today()
	}
	content := s.WeekReport(ref)
	dir := s.vault.ReportsDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, ref.WeekLabel()+".md")
	if err := store.WriteAtomic(path, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("%s 쓰기 실패: %w", path, err)
	}
	return path, nil
}

// TimeSummary aggregates tracked effort for a period. Zero dates mean "all".
func (s *Service) TimeSummary(from, to domain.Date) []query.ProjectTime {
	return query.TimeSummary(s.All(), from, to, s.now())
}

// Running returns the task currently being timed, if any.
func (s *Service) Running() *domain.Task {
	for _, t := range s.All() {
		if t.Status == domain.StatusDoing && t.StartedAt != nil {
			return t
		}
	}
	return nil
}
