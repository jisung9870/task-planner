package tui

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"task-planner/internal/config"
	"task-planner/internal/domain"
	"task-planner/internal/service"
)

func TestProjectGroupingKeepsEveryTaskAndSourceOrder(t *testing.T) {
	tasks := []*domain.Task{
		{ID: "T-1", Project: "zeta"},
		{ID: "T-2", Project: "alpha"},
		{ID: "T-3"},
		{ID: "T-4", Project: "zeta"},
	}
	rows := groupProjectRows(tasks)
	var got []string
	for _, r := range rows {
		if r.task != nil {
			got = append(got, r.task.ID)
		} else {
			got = append(got, r.header)
		}
	}
	want := []string{"alpha (1)", "T-2", "zeta (2)", "T-1", "T-4", "(미지정) (1)", "T-3"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("프로젝트별 목록 = %v, want %v", got, want)
	}
	if got := groupProjectRows(nil); len(got) != 0 {
		t.Fatalf("빈 목록에 행이 생성됨: %v", got)
	}
}

func TestProjectStatusGroupingShowsNestedStatuses(t *testing.T) {
	tasks := []*domain.Task{
		{ID: "T-1", Project: "zeta", Status: domain.StatusDone},
		{ID: "T-2", Project: "alpha", Status: domain.StatusTodo},
		{ID: "T-3", Project: "zeta", Status: domain.StatusDoing},
		{ID: "T-4", Status: domain.StatusBlocked},
		{ID: "T-5", Project: "zeta", Status: domain.StatusDone},
	}
	rows := groupProjectStatusRows(tasks)
	var got []string
	for _, r := range rows {
		if r.task != nil {
			got = append(got, r.task.ID)
		} else {
			got = append(got, fmt.Sprintf("%d:%s", r.indent, r.header))
		}
	}
	want := []string{
		"0:alpha (1)", "1:대기중 (1)", "T-2",
		"0:zeta (3)", "1:진행중 (1)", "T-3", "1:완료 (2)", "T-1", "T-5",
		"0:(미지정) (1)", "1:보류 (1)", "T-4",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("프로젝트·상태별 목록 = %v, want %v", got, want)
	}
}

func TestAllShowsLatestThirtyDoneAndCanSearchOlder(t *testing.T) {
	cfg := config.Default(t.TempDir())
	svc, err := service.Init(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var oldestID string
	for i := 0; i < recentDoneLimit+3; i++ {
		day := time.Date(2026, time.August, 1+i, 12, 0, 0, 0, time.UTC)
		svc.SetClock(func() time.Time { return day })
		task, err := svc.Add(service.AddInput{Title: fmt.Sprintf("완료-%02d", i), Project: "alpha"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := svc.Done(task.ID); err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			oldestID = task.ID
		}
	}
	svc.SetClock(func() time.Time { return time.Date(2026, time.September, 26, 12, 0, 0, 0, time.UTC) })
	open, err := svc.Add(service.AddInput{Title: "열린 작업", Project: "alpha"})
	if err != nil {
		t.Fatal(err)
	}
	cancelled, err := svc.Add(service.AddInput{Title: "취소 작업", Project: "alpha"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Cancel(cancelled.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.SaveUIState(service.UIState{Tab: int(tabAll), WideDetail: true}); err != nil {
		t.Fatal(err)
	}
	m := New(svc)
	if got := countTasks(m.rows); got != recentDoneLimit+2 {
		t.Fatalf("All 태스크 수 = %d, want %d", got, recentDoneLimit+2)
	}
	var doneIDs []string
	seen := map[string]bool{}
	for _, r := range m.rows {
		if r.task == nil {
			continue
		}
		seen[r.task.ID] = true
		if r.task.Status == domain.StatusDone {
			doneIDs = append(doneIDs, r.task.ID)
		}
	}
	if seen[oldestID] || !seen[open.ID] || !seen[cancelled.ID] {
		t.Fatalf("All 포함 항목 오류: 가장 오래된 완료=%t, 열림=%t, 취소=%t", seen[oldestID], seen[open.ID], seen[cancelled.ID])
	}
	if len(doneIDs) != recentDoneLimit {
		t.Fatalf("완료 항목 수 = %d", len(doneIDs))
	}
	for i := 1; i < len(doneIDs); i++ {
		a, _ := svc.Resolve(doneIDs[i-1])
		b, _ := svc.Resolve(doneIDs[i])
		if a.Completed.Before(b.Completed) {
			t.Fatalf("완료일 순서가 뒤집힘: %s, %s", a.Completed, b.Completed)
		}
	}
	f, err := svc.Filter("id:" + oldestID)
	if err != nil {
		t.Fatal(err)
	}
	m.filter = f
	m.reload()
	if got := countTasks(m.rows); got != 1 || m.rows[1].task.ID != oldestID {
		t.Fatalf("오래된 완료 검색 결과 = %+v", m.rows)
	}

	m.filter = nil
	m.reload()
	for _, want := range []listGrouping{groupProject, groupProjectStatus, groupStatus} {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
		if m.grouping != want {
			t.Fatalf("f 전환 결과 = %q, want %q", m.grouping, want)
		}
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	m.saveUIState()
	if got := New(svc).grouping; got != groupProjectStatus {
		t.Fatalf("저장 후 목록 묶음 = %q", got)
	}
}
