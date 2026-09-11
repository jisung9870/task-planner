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
	if err := task.Transition(StatusBlocked, at, &BlockInfo{Reason: "인프라팀 회신 대기"}); err != nil {
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
