package domain

import (
	"testing"
	"time"
)

var spanToday = NewDate(2026, time.September, 12) // 토요일

func TestParseSpanForms(t *testing.T) {
	cases := []struct {
		in               string
		start, end       string
		setStart, setEnd bool
	}{
		{"09-15~09-19", "2026-09-15", "2026-09-19", true, true},
		{"2026-09-15..2026-09-19", "2026-09-15", "2026-09-19", true, true},
		{"today~+4d", "2026-09-12", "2026-09-16", true, true},
		{"09-15", "2026-09-15", "", true, false},
		{"~09-19", "", "2026-09-19", false, true},
		{"09-15~", "2026-09-15", "", true, true},
		{"-", "", "", true, true},
		{"~", "", "", true, true},
		{"월", "2026-09-14", "", true, false},
	}
	for _, c := range cases {
		sp, err := ParseSpan(c.in, spanToday)
		if err != nil {
			t.Fatalf("%q: %v", c.in, err)
		}
		if sp.Start.String() != c.start || sp.End.String() != c.end {
			t.Errorf("%q → %s~%s, want %s~%s", c.in, sp.Start, sp.End, c.start, c.end)
		}
		if sp.SetStart != c.setStart || sp.SetEnd != c.setEnd {
			t.Errorf("%q → set %v/%v, want %v/%v", c.in, sp.SetStart, sp.SetEnd, c.setStart, c.setEnd)
		}
	}
}

func TestParseSpanRejectsBackwardRange(t *testing.T) {
	if _, err := ParseSpan("09-19~09-15", spanToday); err == nil {
		t.Fatal("끝이 시작보다 빠른 기간이 통과함")
	}
	if _, err := ParseSpan("", spanToday); err == nil {
		t.Fatal("빈 입력이 통과함")
	}
}

// A round trip matters because the TUI seeds the prompt with String() and the
// user edits it in place.
func TestSpanStringRoundTrips(t *testing.T) {
	for _, in := range []string{"2026-09-15~2026-09-19", "2026-09-15", "~2026-09-19"} {
		sp, err := ParseSpan(in, spanToday)
		if err != nil {
			t.Fatal(err)
		}
		if got := sp.String(); got != in {
			t.Errorf("%q → %q", in, got)
		}
	}
}

func TestTaskSpanCoversEveryDay(t *testing.T) {
	task := &Task{ID: "T-1", Title: "설계", Status: StatusTodo,
		Scheduled: NewDate(2026, time.September, 15),
		Due:       NewDate(2026, time.September, 19)}
	if !task.HasSpan() || task.SpanDays() != 5 {
		t.Fatalf("HasSpan=%v days=%d", task.HasSpan(), task.SpanDays())
	}
	for i := 0; i < 5; i++ {
		d := NewDate(2026, time.September, 15+i)
		if !task.InSpan(d) {
			t.Errorf("%s 가 기간 밖으로 판정됨", d)
		}
		if got := task.DayIndex(d); got != i+1 {
			t.Errorf("%s DayIndex=%d, want %d", d, got, i+1)
		}
	}
	if task.InSpan(NewDate(2026, time.September, 20)) {
		t.Error("기간 밖 날짜가 포함됨")
	}
}

// One date is one day of work, which keeps the old single-date behaviour intact.
func TestTaskSingleDateIsOneDay(t *testing.T) {
	only := &Task{Scheduled: NewDate(2026, time.September, 15)}
	if only.HasSpan() || only.SpanDays() != 1 {
		t.Fatalf("HasSpan=%v days=%d", only.HasSpan(), only.SpanDays())
	}
	if !only.InSpan(NewDate(2026, time.September, 15)) || only.InSpan(NewDate(2026, time.September, 16)) {
		t.Error("하루짜리 기간이 잘못 판정됨")
	}
	undated := &Task{}
	if undated.SpanDays() != 0 || undated.InSpan(spanToday) {
		t.Error("날짜 없는 태스크가 기간을 가짐")
	}
}

// A due date typed before the start is a typo; it must not produce a negative
// period that the week grid would then try to draw backwards.
func TestTaskBackwardDatesClampToOneDay(t *testing.T) {
	task := &Task{Scheduled: NewDate(2026, time.September, 19), Due: NewDate(2026, time.September, 15)}
	if task.SpanDays() != 1 {
		t.Fatalf("days=%d", task.SpanDays())
	}
	if !task.InSpan(NewDate(2026, time.September, 19)) {
		t.Error("시작일이 기간에서 빠짐")
	}
}

func TestParseDateRefShorthands(t *testing.T) {
	cases := map[string]string{
		"today": "2026-09-12", "내일": "2026-09-13", "+2w": "2026-09-26",
		"09-15": "2026-09-15", "2026-01-02": "2026-01-02", "화": "2026-09-15",
	}
	for in, want := range cases {
		got, err := ParseDateRef(in, spanToday)
		if err != nil {
			t.Fatalf("%q: %v", in, err)
		}
		if got.String() != want {
			t.Errorf("%q → %s, want %s", in, got, want)
		}
	}
}
