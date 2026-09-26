package service

import (
	"testing"
	"time"

	"task-planner/internal/domain"
)

func TestTimelineSurvivesReloadRebuildAndUndo(t *testing.T) {
	s := newTestService(t)
	now := time.Date(2026, 9, 26, 9, 15, 0, 0, time.UTC)
	s.SetClock(func() time.Time { return now })
	task, err := s.Add(AddInput{Title: "바로 착수", Status: domain.StatusDoing})
	if err != nil {
		t.Fatal(err)
	}
	if task.StartedAt == nil {
		t.Fatal("doing creation did not start timer")
	}
	now = now.Add(2 * time.Hour)
	if err := s.Undoable("완료", func() error { _, err := s.Done(task.ID); return err }); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Rebuild(); err != nil {
		t.Fatal(err)
	}
	h := s.WorkHistories(s.All())[task.ID]
	if len(h.Sessions) != 1 || len(h.Finishes) != 1 || !h.Finishes[0].At.Equal(now) {
		t.Fatalf("rebuild: %+v", h)
	}
	if _, err := s.Undo(); err != nil {
		t.Fatal(err)
	}
	h = s.WorkHistories(s.All())[task.ID]
	if len(h.Sessions) != 1 || !h.Sessions[0].End.IsZero() || len(h.Finishes) != 0 {
		t.Fatalf("undo: %+v", h)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(s.Cfg)
	if err != nil {
		t.Fatal(err)
	}
	reopened.SetClock(func() time.Time { return now })
	h = reopened.WorkHistories(reopened.All())[task.ID]
	if len(h.Sessions) != 1 || !h.Sessions[0].Start.Equal(now.Add(-2*time.Hour)) {
		t.Fatalf("reopen: %+v", h)
	}
}
