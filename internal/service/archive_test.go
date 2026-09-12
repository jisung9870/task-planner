package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"task-planner/internal/domain"
)

func TestArchiveMovesOldFinishedWork(t *testing.T) {
	svc := newTestService(t)
	old, _ := svc.Add(AddInput{Title: "지난 달 작업"})
	if _, err := svc.Done(old.ID); err != nil {
		t.Fatal(err)
	}
	// Backdate the completion: Add/Done stamp today, and the cutoff is a date.
	backdate(t, svc, old.ID, "2026-07-01")
	fresh, _ := svc.Add(AddInput{Title: "오늘 끝낸 일"})
	if _, err := svc.Done(fresh.ID); err != nil {
		t.Fatal(err)
	}
	open, _ := svc.Add(AddInput{Title: "아직 하는 중"})

	cutoff := svc.Today().AddDays(-30)
	rep, err := svc.Archive(cutoff, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Moved) != 1 || rep.Moved[0].Task.ID != old.ID {
		t.Fatalf("moved = %+v", rep.Moved)
	}
	if !strings.Contains(rep.Moved[0].To, filepath.Join("archive", "2026-Q3")) {
		t.Fatalf("아카이브 경로 = %s", rep.Moved[0].To)
	}
	if _, err := os.Stat(rep.Moved[0].From); !os.IsNotExist(err) {
		t.Fatal("원본 파일이 남아 있음")
	}
	if _, err := os.Stat(rep.Moved[0].To); err != nil {
		t.Fatal(err)
	}
	// The index must drop it: archived work is out of the working set.
	if _, err := svc.Resolve(old.ID); err == nil {
		t.Fatal("아카이브된 태스크가 인덱스에 남아 있음")
	}
	for _, id := range []string{fresh.ID, open.ID} {
		if _, err := svc.Resolve(id); err != nil {
			t.Fatalf("%s 가 사라짐: %v", id, err)
		}
	}
}

func TestArchiveDryRunTouchesNothing(t *testing.T) {
	svc := newTestService(t)
	task, _ := svc.Add(AddInput{Title: "지난 달 작업"})
	svc.Done(task.ID)
	backdate(t, svc, task.ID, "2026-07-01")

	rep, err := svc.Archive(svc.Today().AddDays(-30), true)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Moved) != 1 || !rep.DryRun {
		t.Fatalf("rep = %+v", rep)
	}
	if _, err := os.Stat(rep.Moved[0].From); err != nil {
		t.Fatal("dry-run 인데 파일이 옮겨짐")
	}
	if _, err := svc.Resolve(task.ID); err != nil {
		t.Fatal("dry-run 인데 인덱스에서 빠짐")
	}
}

// Open work is never archived, however old it is - "오래됐다"는 것은 완료의
// 속성이지 방치의 해법이 아니다.
func TestArchiveLeavesOpenWorkAlone(t *testing.T) {
	svc := newTestService(t)
	task, _ := svc.Add(AddInput{Title: "오래 묵은 할 일"})
	backdate(t, svc, task.ID, "2026-01-01")

	rep, err := svc.Archive(svc.Today(), false)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.Empty() {
		t.Fatalf("열린 작업이 아카이브됨: %+v", rep.Moved)
	}
}

// backdate rewrites completed/updated on disk, which is the only way to age a
// task in a test without a fake filesystem clock.
func backdate(t *testing.T, svc *Service, id, date string) {
	t.Helper()
	full, err := svc.Load(id)
	if err != nil {
		t.Fatal(err)
	}
	d, err := domain.ParseDate(date)
	if err != nil {
		t.Fatal(err)
	}
	if !full.Completed.IsZero() {
		full.Completed = d
	}
	full.Updated = d
	if err := svc.SaveTask(full); err != nil {
		t.Fatal(err)
	}
}
