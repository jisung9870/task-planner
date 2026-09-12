package domain

import (
	"strings"
	"testing"
	"time"
)

func TestParseDate(t *testing.T) {
	d, err := ParseDate("2026-09-12")
	if err != nil {
		t.Fatal(err)
	}
	if got := d.String(); got != "2026-09-12" {
		t.Fatalf("String() = %q", got)
	}
	if _, err := ParseDate("2026/09/12"); err == nil {
		t.Fatal("잘못된 형식이 통과함")
	}
}

func TestWeekStartAndLabel(t *testing.T) {
	// 2026-09-12 is a Saturday; its ISO week starts Monday 2026-09-07.
	d := NewDate(2026, time.September, 12)
	if got := d.WeekStart().String(); got != "2026-09-07" {
		t.Fatalf("WeekStart = %q", got)
	}
	if got := d.WeekLabel(); got != "2026-W37" {
		t.Fatalf("WeekLabel = %q", got)
	}
}

func TestDaysUntil(t *testing.T) {
	from := NewDate(2026, time.September, 12)
	if got := NewDate(2026, time.September, 15).DaysUntil(from); got != 3 {
		t.Fatalf("DaysUntil = %d", got)
	}
	if got := NewDate(2026, time.September, 10).DaysUntil(from); got != -2 {
		t.Fatalf("DaysUntil = %d", got)
	}
}

func TestParseDuration(t *testing.T) {
	cases := map[string]string{"2h": "2h", "30m": "30m", "1h30m": "1h30m", "90": "1h30m", "1d": "24h"}
	for in, want := range cases {
		d, err := ParseDuration(in)
		if err != nil {
			t.Fatalf("%q: %v", in, err)
		}
		if got := d.String(); got != want {
			t.Errorf("%q -> %q, want %q", in, got, want)
		}
	}
	if _, err := ParseDuration("주"); err == nil {
		t.Error("잘못된 형식이 통과함")
	}
}

func TestTransitionRecordsLogAndCompleted(t *testing.T) {
	at := time.Date(2026, 9, 12, 9, 14, 0, 0, time.Local)
	task := &Task{ID: "T-1", Title: "x", Status: StatusTodo}
	if err := task.Transition(StatusDoing, at, nil); err != nil {
		t.Fatal(err)
	}
	if task.Status != StatusDoing {
		t.Fatalf("status = %s", task.Status)
	}
	if !strings.Contains(task.Body, "todo → doing") {
		t.Fatalf("로그 누락:\n%s", task.Body)
	}
	if err := task.Transition(StatusDone, at, nil); err != nil {
		t.Fatal(err)
	}
	if task.Completed.String() != "2026-09-12" {
		t.Fatalf("completed = %q", task.Completed)
	}
	if got := len(task.LogLines()); got != 2 {
		t.Fatalf("log lines = %d", got)
	}
}

func TestBlockedRequiresReason(t *testing.T) {
	at := time.Now()
	task := &Task{ID: "T-1", Title: "x", Status: StatusTodo}
	if err := task.Transition(StatusBlocked, at, nil); err == nil {
		t.Fatal("사유 없는 보류가 통과함")
	}
	if err := task.Transition(StatusBlocked, at, &TransitionOpts{Block: &BlockInfo{Reason: "인프라팀 회신 대기"}}); err != nil {
		t.Fatal(err)
	}
	if task.BlockedSince.IsZero() {
		t.Fatal("blocked_since 가 설정되지 않음")
	}
	// Leaving the blocked state must clear the hold metadata, otherwise a
	// stale reason keeps rendering in list views.
	if err := task.Transition(StatusDoing, at, nil); err != nil {
		t.Fatal(err)
	}
	if task.BlockedReason != "" || !task.BlockedSince.IsZero() {
		t.Fatalf("보류 정보가 남아 있음: %q %v", task.BlockedReason, task.BlockedSince)
	}
}

func TestAppendLogKeepsSectionOrder(t *testing.T) {
	task := &Task{Body: "## Note\n메모 본문\n\n## Log\n- 2026-09-11 10:00 created\n"}
	task.AppendLog(time.Date(2026, 9, 12, 9, 0, 0, 0, time.Local), "todo → doing")
	if task.Note() != "메모 본문" {
		t.Fatalf("Note() = %q", task.Note())
	}
	logs := task.LogLines()
	if len(logs) != 2 || !strings.HasSuffix(logs[1], "todo → doing") {
		t.Fatalf("logs = %v", logs)
	}
}

func TestAppendLogBeforeFollowingSection(t *testing.T) {
	task := &Task{Body: "## Log\n- a\n\n## Refs\n- link\n"}
	task.AppendLog(time.Now(), "b")
	if !strings.Contains(task.Body, "## Refs") {
		t.Fatalf("뒤따르는 섹션이 사라짐:\n%s", task.Body)
	}
	logs := task.LogLines()
	if len(logs) != 2 {
		t.Fatalf("logs = %v\nbody:\n%s", logs, task.Body)
	}
}

func TestSlug(t *testing.T) {
	if got := Slug("로그 파이프라인 PoC 결과 정리"); got != "로그-파이프라인-poc-결과-정리" {
		t.Fatalf("Slug = %q", got)
	}
	if got := Slug("!!!"); got != "task" {
		t.Fatalf("Slug = %q", got)
	}
}

func TestOverdueAndDueSoon(t *testing.T) {
	today := NewDate(2026, time.September, 12)
	task := &Task{Status: StatusTodo, Due: NewDate(2026, time.September, 10)}
	if !task.Overdue(today) {
		t.Error("마감 초과를 감지하지 못함")
	}
	task.Status = StatusDone
	if task.Overdue(today) {
		t.Error("완료된 태스크가 마감 초과로 잡힘")
	}
	task = &Task{Status: StatusTodo, Due: NewDate(2026, time.September, 14)}
	if !task.DueSoon(today, 3) {
		t.Error("D-2 를 임박으로 보지 않음")
	}
}

func TestTimerAccumulatesAcrossSessions(t *testing.T) {
	start := time.Date(2026, 9, 12, 9, 0, 0, 0, time.Local)
	task := &Task{ID: "T-1", Title: "x", Status: StatusTodo}

	if err := task.Transition(StatusDoing, start, nil); err != nil {
		t.Fatal(err)
	}
	if task.StartedAt == nil {
		t.Fatal("started_at 미설정")
	}
	if err := task.Transition(StatusTodo, start.Add(90*time.Minute), nil); err != nil {
		t.Fatal(err)
	}
	if task.StartedAt != nil {
		t.Fatal("started_at 이 남아 있음")
	}
	if got := task.Actual.String(); got != "1h30m" {
		t.Fatalf("actual = %s", got)
	}

	// A second session adds to the first.
	task.Transition(StatusDoing, start.Add(3*time.Hour), nil)
	task.Transition(StatusDone, start.Add(3*time.Hour+30*time.Minute), nil)
	if got := task.Actual.String(); got != "2h" {
		t.Fatalf("누적 actual = %s", got)
	}
	logs := strings.Join(task.LogLines(), "\n")
	if !strings.Contains(logs, "[+1h30m]") || !strings.Contains(logs, "[+30m]") {
		t.Fatalf("세션 시간이 로그에 없음:\n%s", logs)
	}
}

// A task left running overnight would record the whole night and destroy the
// estimate-vs-actual signal, which is the only reason to track time at all.
func TestTimerAppliesSessionCap(t *testing.T) {
	start := time.Date(2026, 9, 12, 17, 0, 0, 0, time.Local)
	task := &Task{ID: "T-1", Title: "x", Status: StatusTodo}
	opts := &TransitionOpts{SessionCap: Duration(8 * time.Hour)}

	task.Transition(StatusDoing, start, opts)
	task.Transition(StatusDone, start.Add(16*time.Hour), opts)

	if got := task.Actual.String(); got != "8h" {
		t.Fatalf("actual = %s", got)
	}
	if !strings.Contains(strings.Join(task.LogLines(), "\n"), "상한 적용") {
		t.Fatalf("상한 적용 표시 없음: %v", task.LogLines())
	}
}

func TestElapsedActualIncludesRunningSession(t *testing.T) {
	start := time.Date(2026, 9, 12, 9, 0, 0, 0, time.Local)
	task := &Task{ID: "T-1", Title: "x", Status: StatusTodo, Actual: Duration(time.Hour)}
	task.Transition(StatusDoing, start, nil)
	if got := task.ElapsedActual(start.Add(30 * time.Minute)).String(); got != "1h30m" {
		t.Fatalf("elapsed = %s", got)
	}
}

// Re-entering 진행중 while already running must not reset the clock.
func TestTimerIgnoresRedundantDoingTransition(t *testing.T) {
	start := time.Date(2026, 9, 12, 9, 0, 0, 0, time.Local)
	task := &Task{ID: "T-1", Title: "x", Status: StatusTodo}
	task.Transition(StatusDoing, start, nil)
	first := *task.StartedAt
	task.Transition(StatusDoing, start.Add(time.Hour), nil)
	if !task.StartedAt.Equal(first) {
		t.Fatalf("타이머가 재설정됨: %v != %v", task.StartedAt, first)
	}
}

func TestAppendNoteCreatesSectionAboveLog(t *testing.T) {
	at := time.Date(2026, 9, 12, 16, 30, 0, 0, time.UTC)
	task := &Task{ID: "T-1", Title: "작업", Status: StatusTodo}
	task.AppendLog(at, "created")
	task.AppendNote(at, "보안팀 회신 대기")

	note, log := task.Note(), task.LogLines()
	if !strings.Contains(note, "보안팀 회신 대기") {
		t.Fatalf("note = %q", note)
	}
	if len(log) != 1 || !strings.Contains(log[0], "created") {
		t.Fatalf("log = %v", log)
	}
	// The note section has to sit above the log, or the file stops reading
	// top-down as history accumulates.
	if strings.Index(task.Body, "## Note") > strings.Index(task.Body, "## Log") {
		t.Fatalf("Note 가 Log 아래에 있음:\n%s", task.Body)
	}
}

func TestAppendNoteAccumulates(t *testing.T) {
	at := time.Date(2026, 9, 12, 16, 30, 0, 0, time.UTC)
	task := &Task{ID: "T-1", Title: "작업", Status: StatusTodo}
	task.AppendNote(at, "첫 줄")
	task.AppendNote(at.Add(time.Hour), "둘째 줄")
	note := task.Note()
	if !strings.Contains(note, "첫 줄") || !strings.Contains(note, "둘째 줄") {
		t.Fatalf("메모가 덮어써짐: %q", note)
	}
	if strings.Count(task.Body, "## Note") != 1 {
		t.Fatalf("Note 섹션이 중복됨:\n%s", task.Body)
	}
}

// A quick capture's body is bare prose; a note must not orphan it.
func TestAppendNoteKeepsExistingProse(t *testing.T) {
	at := time.Date(2026, 9, 12, 16, 30, 0, 0, time.UTC)
	task := &Task{ID: "T-1", Title: "작업", Status: StatusTodo, Body: "원래 적어둔 내용\n"}
	task.AppendNote(at, "새 메모")
	note := task.Note()
	if !strings.Contains(note, "원래 적어둔 내용") || !strings.Contains(note, "새 메모") {
		t.Fatalf("note = %q", note)
	}
}

// Cancelling ends the work as surely as finishing does; the views that report
// "그 날 끝난 일" all read Completed, so it has to be set for both.
func TestTransitionRecordsCompletedForTerminalStates(t *testing.T) {
	at := time.Date(2026, 9, 12, 10, 0, 0, 0, time.UTC)
	today := DateOf(at)
	for _, st := range []Status{StatusDone, StatusCancelled} {
		task := &Task{ID: "T-1", Title: "작업", Status: StatusTodo}
		if err := task.Transition(st, at, nil); err != nil {
			t.Fatal(err)
		}
		if !task.Completed.Equal(today) {
			t.Fatalf("%s: completed = %q", st, task.Completed)
		}
	}
	// Reopening clears it: the work is not finished any more.
	task := &Task{ID: "T-1", Title: "작업", Status: StatusDone, Completed: today}
	if err := task.Transition(StatusTodo, at, nil); err != nil {
		t.Fatal(err)
	}
	if !task.Completed.IsZero() {
		t.Fatalf("completed = %q", task.Completed)
	}
}
