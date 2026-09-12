package service

import (
	"os"
	"strings"
	"testing"

	"task-planner/internal/domain"
)

func TestUndoRestoresStatusAndLog(t *testing.T) {
	svc := newTestService(t)
	task, _ := svc.Add(AddInput{Title: "API 설계"})
	if err := svc.Undoable("완료", func() error {
		_, err := svc.Done(task.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if label := svc.UndoLabel(); label != "완료" {
		t.Fatalf("label = %q", label)
	}
	if _, err := svc.Undo(); err != nil {
		t.Fatal(err)
	}
	after, err := svc.Load(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Status != domain.StatusTodo || !after.Completed.IsZero() {
		t.Fatalf("status=%s completed=%s", after.Status, after.Completed)
	}
	// The pre-image restores the whole file, so the transition log line that
	// the change added is gone too.
	for _, l := range after.LogLines() {
		if strings.Contains(l, "done") {
			t.Fatalf("로그가 남아 있음: %v", after.LogLines())
		}
	}
	if svc.CanUndo() {
		t.Fatal("스택이 비워지지 않음")
	}
}

// One user action is one undo step even when it writes several files.
func TestUndoTreatsBulkChangeAsOneStep(t *testing.T) {
	svc := newTestService(t)
	var ids []string
	for _, title := range []string{"A", "B", "C"} {
		task, _ := svc.Add(AddInput{Title: title})
		ids = append(ids, task.ID)
	}
	if err := svc.Undoable("완료 (3건)", func() error {
		for _, id := range ids {
			if _, err := svc.Done(id); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Undo(); err != nil {
		t.Fatal(err)
	}
	for _, id := range ids {
		after, err := svc.Load(id)
		if err != nil {
			t.Fatal(err)
		}
		if after.Status != domain.StatusTodo {
			t.Fatalf("%s = %s", id, after.Status)
		}
	}
}

func TestUndoRestoresDeletedFile(t *testing.T) {
	svc := newTestService(t)
	task, _ := svc.Add(AddInput{Title: "잘못 만든 태스크"})
	path := task.Path
	if err := svc.Undoable("삭제", func() error { return svc.Delete(task.ID) }); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("파일이 지워지지 않음")
	}
	if _, err := svc.Undo(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("파일이 복구되지 않음")
	}
	if _, err := svc.Resolve(task.ID); err != nil {
		t.Fatalf("인덱스에 돌아오지 않음: %v", err)
	}
}

// A task added inside a step has no pre-image; undoing removes the file.
func TestUndoRemovesAddedTask(t *testing.T) {
	svc := newTestService(t)
	var task *domain.Task
	if err := svc.Undoable("추가", func() error {
		var err error
		task, err = svc.Add(AddInput{Title: "실수로 캡처"})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Undo(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(task.Path); !os.IsNotExist(err) {
		t.Fatal("추가된 파일이 남아 있음")
	}
	if _, err := svc.Resolve(task.ID); err == nil {
		t.Fatal("인덱스에 유령이 남음")
	}
}

// Undo must not clobber an edit made in an editor or another terminal.
func TestUndoRefusesAfterExternalChange(t *testing.T) {
	svc := newTestService(t)
	task, _ := svc.Add(AddInput{Title: "API 설계"})
	if err := svc.Undoable("완료", func() error {
		_, err := svc.Done(task.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddNote(task.ID, "밖에서 고친 줄"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Undo(); err == nil {
		t.Fatal("외부 변경이 있는데 되돌려짐")
	}
	// The step stays on the stack: refusing is not the same as consuming it.
	if !svc.CanUndo() {
		t.Fatal("거부된 단계가 스택에서 사라짐")
	}
}

// Changes made outside a step are not undoable - the boundary is the caller's.
func TestChangesOutsideAStepAreNotUndoable(t *testing.T) {
	svc := newTestService(t)
	task, _ := svc.Add(AddInput{Title: "API 설계"})
	if _, err := svc.Done(task.ID); err != nil {
		t.Fatal(err)
	}
	if svc.CanUndo() {
		t.Fatal("Undoable 밖의 변경이 스택에 쌓임")
	}
	if _, err := svc.Undo(); err == nil {
		t.Fatal("빈 스택에서 되돌려짐")
	}
}

func TestUndoRestoresProjectFile(t *testing.T) {
	svc := newTestService(t)
	if _, err := svc.CreateProject("infra", "인프라"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Undoable("프로젝트 상태 변경", func() error {
		_, err := svc.CycleProjectStatus("infra")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if p, _ := svc.ProjectBySlug("infra"); p.Status != "paused" {
		t.Fatalf("status = %q", p.Status)
	}
	if _, err := svc.Undo(); err != nil {
		t.Fatal(err)
	}
	p, err := svc.ProjectBySlug("infra")
	if err != nil {
		t.Fatal(err)
	}
	if p.Status != "active" {
		t.Fatalf("되돌린 status = %q", p.Status)
	}
	// A project file must never end up in the task index.
	for _, task := range svc.All() {
		if task.Title == "" {
			t.Fatalf("인덱스에 태스크가 아닌 항목이 들어감: %+v", task)
		}
	}
}

func TestUndoDepthIsBounded(t *testing.T) {
	svc := newTestService(t)
	task, _ := svc.Add(AddInput{Title: "메모 대상"})
	for i := 0; i < undoDepth+5; i++ {
		if err := svc.Undoable("메모 추가", func() error {
			_, err := svc.AddNote(task.ID, "줄")
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	if got := len(svc.undoStack); got != undoDepth {
		t.Fatalf("스택 깊이 = %d", got)
	}
}
