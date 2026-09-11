package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"task-planner/internal/domain"
)

func newTestVault(t *testing.T) *Vault {
	t.Helper()
	v := New(t.TempDir())
	if err := v.Init(); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestSaveTaskShardsByMonth(t *testing.T) {
	v := newTestVault(t)
	task := &domain.Task{
		ID: "T-20260912-0001", Title: "로그 정리", Status: domain.StatusTodo,
		Created: domain.NewDate(2026, 9, 12),
	}
	if err := v.SaveTask(task); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(v.TasksDir(), "2026-09", "T-20260912-0001-로그-정리.md")
	if task.Path != want {
		t.Fatalf("path = %s, want %s", task.Path, want)
	}
	if _, err := os.Stat(want); err != nil {
		t.Fatal(err)
	}
}

// Renaming a task must not leave the old file behind: the index rebuilds from
// the filesystem, so a leftover would come back as a duplicate.
func TestSaveTaskRemovesStalePathOnRename(t *testing.T) {
	v := newTestVault(t)
	task := &domain.Task{
		ID: "T-20260912-0001", Title: "원래 제목", Status: domain.StatusTodo,
		Created: domain.NewDate(2026, 9, 12),
	}
	if err := v.SaveTask(task); err != nil {
		t.Fatal(err)
	}
	old := task.Path
	task.Title = "바뀐 제목"
	if err := v.SaveTask(task); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatalf("이전 파일이 남아 있음: %s", old)
	}
	files, err := v.TaskFiles(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("파일 %d개: %v", len(files), files)
	}
}

func TestSaveTaskRejectsInvalid(t *testing.T) {
	v := newTestVault(t)
	err := v.SaveTask(&domain.Task{ID: "T-1", Title: "x", Status: domain.StatusBlocked})
	if err == nil || !strings.Contains(err.Error(), "보류") {
		t.Fatalf("err = %v", err)
	}
}

func TestTaskFilesSkipsHiddenAndNonMarkdown(t *testing.T) {
	v := newTestVault(t)
	dir := filepath.Join(v.TasksDir(), "2026-09")
	os.MkdirAll(dir, 0o755)
	for _, name := range []string{"a.md", ".hidden.md", "notes.txt"} {
		os.WriteFile(filepath.Join(dir, name), []byte("---\nid: x\n---\n"), 0o644)
	}
	files, err := v.TaskFiles(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || filepath.Base(files[0]) != "a.md" {
		t.Fatalf("files = %v", files)
	}
}

func TestWriteAtomicReplaces(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.md")
	if err := WriteAtomic(p, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WriteAtomic(p, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(p)
	if string(raw) != "new" {
		t.Fatalf("content = %q", raw)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("임시 파일이 남음: %d", len(entries))
	}
}
