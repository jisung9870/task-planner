package tui

import (
	"reflect"
	"testing"

	"task-planner/internal/domain"
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
