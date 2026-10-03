package domain

import (
	"strings"
	"testing"
	"time"
)

var sinceNow = time.Date(2026, 10, 3, 12, 28, 30, 0, time.Local)

func TestParseSinceForms(t *testing.T) {
	cases := map[string]string{
		"10:30":            "2026-10-03 10:30",
		"2h":               "2026-10-03 10:28",
		"1h30m":            "2026-10-03 10:58",
		"어제 14:00":         "2026-10-02 14:00",
		"yesterday 09:05":  "2026-10-02 09:05",
		"10-01 14:00":      "2026-10-01 14:00",
		"2026-10-01 14:00": "2026-10-01 14:00",
		"2026-10-01T14:00": "2026-10-01 14:00",
	}
	for in, want := range cases {
		got, err := ParseSince(in, sinceNow)
		if err != nil {
			t.Fatalf("%q: %v", in, err)
		}
		if s := got.Format(sinceLayout); s != want {
			t.Errorf("%q → %s, want %s", in, s, want)
		}
	}
}

func TestParseSinceRefusesFutureAndJunk(t *testing.T) {
	for _, in := range []string{"13:00", "내일 09:00", "", "언젠가", "25:00"} {
		if _, err := ParseSince(in, sinceNow); err == nil {
			t.Errorf("%q: want error", in)
		}
	}
}

// The habit the vault showed: 대기중 → 완료 with nothing on the clock. The
// backfill closes one session from the typed start.
func TestBackfilledCompletionFromTodo(t *testing.T) {
	task := &Task{ID: "T-1", Title: "x", Status: StatusTodo}
	since := sinceNow.Add(-2 * time.Hour)
	if err := task.Transition(StatusDone, sinceNow, &TransitionOpts{Since: &since}); err != nil {
		t.Fatal(err)
	}
	if task.Actual != Duration(2*time.Hour) {
		t.Errorf("actual = %s, want 2h", task.Actual)
	}
	if task.StartedAt != nil {
		t.Errorf("timer left running")
	}
	logs := task.LogLines()
	last := logs[len(logs)-1]
	if !strings.Contains(last, "todo → done (착수 소급 2026-10-03 10:28) [+2h]") {
		t.Errorf("log = %q", last)
	}
	h := task.WorkHistory(time.Local)
	start, end, ok := h.Lead()
	if !ok || !start.Equal(since.Truncate(time.Minute)) || end.Format("15:04") != "12:28" {
		t.Errorf("lead = %s ~ %s (%v)", start, end, ok)
	}
	if len(h.Sessions) != 1 {
		t.Errorf("sessions = %d, want 1", len(h.Sessions))
	}
}

// Pressing 진행중 a minute before 완료 records a minute; the backfill moves the
// start of that session instead of adding a second one.
func TestBackfillReplacesAMomentarySession(t *testing.T) {
	task := &Task{ID: "T-1", Title: "x", Status: StatusTodo}
	_ = task.Transition(StatusDoing, sinceNow.Add(-time.Minute), nil)
	since := sinceNow.Add(-3 * time.Hour)
	if err := task.Transition(StatusDone, sinceNow, &TransitionOpts{Since: &since}); err != nil {
		t.Fatal(err)
	}
	if task.Actual != Duration(3*time.Hour) {
		t.Errorf("actual = %s, want 3h", task.Actual)
	}
	h := task.WorkHistory(time.Local)
	if len(h.Sessions) != 1 || h.Sessions[0].Start.Format("15:04") != "09:28" {
		t.Errorf("sessions = %+v", h.Sessions)
	}
}

func TestBackfillHonoursSessionCap(t *testing.T) {
	task := &Task{ID: "T-1", Title: "x", Status: StatusTodo}
	since := sinceNow.AddDate(0, 0, -2)
	err := task.Transition(StatusDone, sinceNow, &TransitionOpts{Since: &since, SessionCap: Duration(8 * time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if task.Actual != Duration(8*time.Hour) {
		t.Errorf("actual = %s, want the 8h cap", task.Actual)
	}
	// The cap bounds 작업 시간, not 걸린 기간: the start stays two days back.
	start, _, _ := task.WorkHistory(time.Local).Lead()
	if start.Format(sinceLayout) != "2026-10-01 12:28" {
		t.Errorf("lead start = %s", start)
	}
}

func TestBackfillRefusesOverlapAndOtherStates(t *testing.T) {
	task := &Task{ID: "T-1", Title: "x", Status: StatusTodo}
	_ = task.Transition(StatusDoing, sinceNow.Add(-4*time.Hour), nil)
	_ = task.Transition(StatusTodo, sinceNow.Add(-3*time.Hour), nil)
	since := sinceNow.Add(-5 * time.Hour)
	if err := task.Transition(StatusDone, sinceNow, &TransitionOpts{Since: &since}); err == nil {
		t.Error("overlap with a recorded session accepted")
	}
	later := sinceNow.Add(-time.Hour)
	if err := task.Transition(StatusCancelled, sinceNow, &TransitionOpts{Since: &later}); err == nil {
		t.Error("since accepted on 취소")
	}
	future := sinceNow.Add(time.Hour)
	if err := task.Transition(StatusDone, sinceNow, &TransitionOpts{Since: &future}); err == nil {
		t.Error("since after completion accepted")
	}
	if err := task.Transition(StatusDone, sinceNow, &TransitionOpts{Since: &later}); err != nil {
		t.Fatalf("since after the earlier session refused: %v", err)
	}
	if task.Actual != Duration(2*time.Hour) {
		t.Errorf("actual = %s, want 1h earlier + 1h backfilled", task.Actual)
	}
	start, _, _ := task.WorkHistory(time.Local).Lead()
	if start.Format("15:04") != "08:28" {
		t.Errorf("lead starts at %s, want the first session", start.Format("15:04"))
	}
}

// A task put back to 대기중 has started but is neither running nor finished;
// its label must not count days as if the clock were still on.
func TestLeadLabelDistinguishesPausedFromRunning(t *testing.T) {
	task := &Task{ID: "T-1", Title: "x", Status: StatusTodo}
	_ = task.Transition(StatusDoing, sinceNow.Add(-3*time.Hour), nil)
	if got := task.WorkHistory(time.Local).LeadLabel(sinceNow); !strings.Contains(got, "3h째") {
		t.Errorf("running = %q", got)
	}
	_ = task.Transition(StatusTodo, sinceNow.Add(-time.Hour), nil)
	if got := task.WorkHistory(time.Local).LeadLabel(sinceNow); !strings.Contains(got, "멈춤 (완료 전)") {
		t.Errorf("paused = %q", got)
	}
	_ = task.Transition(StatusDone, sinceNow, nil)
	if got := task.WorkHistory(time.Local).LeadLabel(sinceNow); !strings.Contains(got, "→ 완료") {
		t.Errorf("done = %q", got)
	}
}

// "2h" means two hours on both clocks: 작업 시간 and the 걸린 기간 read back
// from the minute-resolution log.
func TestRelativeSinceAgreesOnBothClocks(t *testing.T) {
	task := &Task{ID: "T-1", Title: "x", Status: StatusTodo}
	since, err := ParseSince("2h", sinceNow)
	if err != nil {
		t.Fatal(err)
	}
	if err := task.Transition(StatusDone, sinceNow, &TransitionOpts{Since: &since}); err != nil {
		t.Fatal(err)
	}
	start, end, _ := task.WorkHistory(time.Local).Lead()
	if task.Actual.String() != "2h" || SpanText(end.Sub(start)) != "2h" {
		t.Fatalf("actual=%s lead=%s", task.Actual, SpanText(end.Sub(start)))
	}
}

func TestAbsoluteSinceAgreesOnBothClocks(t *testing.T) {
	task := &Task{ID: "T-1", Title: "x", Status: StatusTodo}
	since, _ := ParseSince("10:30", sinceNow) // now 12:28:30
	if err := task.Transition(StatusDone, sinceNow, &TransitionOpts{Since: &since}); err != nil {
		t.Fatal(err)
	}
	start, end, _ := task.WorkHistory(time.Local).Lead()
	if task.Actual.String() != "1h58m" || SpanText(end.Sub(start)) != "1h58m" {
		t.Fatalf("actual=%s lead=%s", task.Actual, SpanText(end.Sub(start)))
	}
}
