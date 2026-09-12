package service

import (
	"fmt"

	"task-planner/internal/domain"
)

// Archived is one task moved out of tasks/.
type Archived struct {
	Task *domain.Task
	From string
	To   string
}

// ArchiveReport is the outcome of an archive run.
type ArchiveReport struct {
	Moved  []Archived
	DryRun bool
	// Cutoff is the date used, echoed back so the caller can say what it did.
	Cutoff domain.Date
}

func (r ArchiveReport) Empty() bool { return len(r.Moved) == 0 }

// Archive moves finished work completed before cutoff into archive/<year>-Q<n>/.
//
// The vault is the source of truth and markdown is cheap, but the index scans
// every file under tasks/ on every sync - so a year of done tasks is a tax on
// every command. Archiving keeps the working set the size of the work.
func (s *Service) Archive(cutoff domain.Date, dryRun bool) (ArchiveReport, error) {
	rep := ArchiveReport{DryRun: dryRun, Cutoff: cutoff}
	if cutoff.IsZero() {
		return rep, fmt.Errorf("아카이브 기준일이 비어 있음")
	}
	for _, sum := range s.All() {
		if !sum.Status.Terminal() {
			continue
		}
		done := archiveDate(sum)
		// A finished task with no date at all is left alone: moving a file on a
		// guess is not worth the tidiness.
		if done.IsZero() || !done.Before(cutoff) {
			continue
		}
		item := Archived{Task: sum, From: sum.Path}
		if dryRun {
			item.To = s.vault.ArchivePath(sum)
			rep.Moved = append(rep.Moved, item)
			continue
		}
		t, err := s.Load(sum.ID)
		if err != nil {
			return rep, err
		}
		to, err := s.vault.ArchiveTask(t)
		if err != nil {
			return rep, err
		}
		s.idx.Remove(t.ID)
		item.Task, item.To = t, to
		rep.Moved = append(rep.Moved, item)
		s.recordChange(t.ShortID()+" 아카이브",
			fmt.Sprintf("%s %s: 아카이브 (%s)", t.ID, t.Title, to))
	}
	if !dryRun && !rep.Empty() {
		if err := s.idx.Flush(); err != nil {
			return rep, err
		}
	}
	return rep, nil
}

// archiveDate is when the task stopped mattering: its completion date, or the
// last edit when a hand-written file has none.
func archiveDate(t *domain.Task) domain.Date {
	if !t.Completed.IsZero() {
		return t.Completed
	}
	return t.Updated
}
