package domain

import (
	"testing"
	"time"
)

func TestWorkHistoryPreservesPausesAndReopenedWork(t *testing.T) {
	loc := time.FixedZone("KST", 9*60*60)
	at := func(day, hour int) time.Time { return time.Date(2026, 9, day, hour, 30, 0, 0, loc) }
	task := &Task{Status: StatusTodo}
	steps := []struct {
		status Status
		at     time.Time
	}{
		{StatusDoing, at(21, 9)}, {StatusTodo, at(21, 11)},
		{StatusDoing, at(23, 13)}, {StatusDone, at(23, 17)},
		{StatusDoing, at(25, 10)},
	}
	for _, step := range steps {
		if err := task.Transition(step.status, step.at, nil); err != nil {
			t.Fatal(err)
		}
	}
	today := DateOf(at(26, 12))
	h := task.WorkHistory(loc)
	if len(h.Sessions) != 3 || len(h.Finishes) != 1 {
		t.Fatalf("history: %+v", h)
	}
	if !h.Sessions[0].Start.Equal(at(21, 9)) || !h.Sessions[0].End.Equal(at(21, 11)) {
		t.Fatalf("first: %+v", h.Sessions[0])
	}
	if !h.Sessions[2].End.IsZero() {
		t.Fatal("running session has an end")
	}
	for day, want := range map[int]bool{21: true, 22: false, 23: true, 24: false, 25: true, 26: true, 27: false} {
		if got := h.InDay(DateOf(at(day, 12)), today); got != want {
			t.Errorf("day %d = %v", day, got)
		}
	}
	if err := task.Transition(StatusCancelled, at(26, 14), nil); err != nil {
		t.Fatal(err)
	}
	h = task.WorkHistory(loc)
	if len(h.Finishes) != 2 || h.Finishes[1].Status != StatusCancelled {
		t.Fatalf("finishes: %+v", h.Finishes)
	}
}

func TestWorkHistoryFallbacksDoNotInventTimes(t *testing.T) {
	day := NewDate(2026, 9, 26)
	task := &Task{Status: StatusDone, Completed: day,
		Body: "## Note\n- 2026-09-20 12:00 todo → doing\n## Log\n- not a timestamp\n- 2026-09-26 13:00 note todo → done\n"}
	h := task.WorkHistory(time.UTC)
	if len(h.Sessions) != 0 || len(h.Finishes) != 0 || !h.Completed.Equal(day) {
		t.Fatalf("history: %+v", h)
	}
	if !h.InDay(day, day) || h.InDay(day.AddDays(-1), day) {
		t.Fatal("wrong fallback date")
	}
	start := time.Date(2026, 9, 25, 23, 50, 0, 0, time.UTC)
	running := (&Task{Status: StatusDoing, StartedAt: &start}).WorkHistory(time.UTC)
	if len(running.Sessions) != 1 || !running.InDay(day, day) {
		t.Fatalf("running: %+v", running)
	}
}
