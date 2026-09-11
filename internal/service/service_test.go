package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"task-planner/internal/config"
	"task-planner/internal/domain"
)

// newTestService builds a vault in a temp dir with a frozen clock so every
// date-dependent assertion is stable.
func newTestService(t *testing.T) *Service {
	t.Helper()
	cfg := config.Default(t.TempDir())
	svc, err := Init(cfg)
	if err != nil {
		t.Fatal(err)
	}
	svc.SetClock(func() time.Time { return time.Date(2026, 9, 12, 9, 14, 0, 0, time.Local) })
	t.Cleanup(func() { svc.Close() })
	return svc
}

func TestAddAssignsSequentialIDs(t *testing.T) {
	svc := newTestService(t)
	a, err := svc.Add(AddInput{Title: "첫 번째"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := svc.Add(AddInput{Title: "두 번째"})
	if err != nil {
		t.Fatal(err)
	}
	if a.ID != "T-20260912-0001" || b.ID != "T-20260912-0002" {
		t.Fatalf("ids = %s, %s", a.ID, b.ID)
	}
	if _, err := os.Stat(a.Path); err != nil {
		t.Fatal(err)
	}
}

func TestAddRejectsEmptyTitle(t *testing.T) {
	svc := newTestService(t)
	if _, err := svc.Add(AddInput{Title: "   "}); err == nil {
		t.Fatal("빈 제목이 통과함")
	}
}

func TestResolveByShortIDAndTitle(t *testing.T) {
	svc := newTestService(t)
	task, _ := svc.Add(AddInput{Title: "로그 파이프라인 PoC"})
	svc.Add(AddInput{Title: "배포 스크립트"})

	for _, ref := range []string{task.ID, "#1", "1", "파이프라인", "poc"} {
		got, err := svc.Resolve(ref)
		if err != nil {
			t.Fatalf("%q: %v", ref, err)
		}
		if got.ID != task.ID {
			t.Fatalf("%q -> %s", ref, got.ID)
		}
	}
	if _, err := svc.Resolve("없는것"); err == nil {
		t.Fatal("없는 태스크가 해석됨")
	}
}

func TestResolveReportsAmbiguity(t *testing.T) {
	svc := newTestService(t)
	svc.Add(AddInput{Title: "배포 스크립트 A"})
	svc.Add(AddInput{Title: "배포 스크립트 B"})
	_, err := svc.Resolve("배포")
	if err == nil || !strings.Contains(err.Error(), "2개") {
		t.Fatalf("err = %v", err)
	}
}

func TestStatusFlowWritesFileAndIndex(t *testing.T) {
	svc := newTestService(t)
	task, _ := svc.Add(AddInput{Title: "작업"})

	if _, err := svc.Start(task.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := svc.idx.Get(task.ID); got.Status != domain.StatusDoing {
		t.Fatalf("인덱스 상태 = %s", got.Status)
	}
	res, err := svc.Done(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if res.Task.Completed.String() != "2026-09-12" {
		t.Fatalf("completed = %s", res.Task.Completed)
	}
	raw, _ := os.ReadFile(res.Task.Path)
	if !strings.Contains(string(raw), "status: done") {
		t.Fatalf("파일에 반영되지 않음:\n%s", raw)
	}
	if !strings.Contains(string(raw), "doing → done") {
		t.Fatalf("로그가 기록되지 않음:\n%s", raw)
	}
}

func TestBlockRequiresReason(t *testing.T) {
	svc := newTestService(t)
	task, _ := svc.Add(AddInput{Title: "작업"})
	if _, err := svc.Block(task.ID, "", nil); err == nil {
		t.Fatal("사유 없는 보류가 통과함")
	}
	res, err := svc.Block(task.ID, "인프라팀 회신 대기", nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Task.BlockedSince.IsZero() {
		t.Fatal("blocked_since 미설정")
	}
}

func TestEditRenamesFileAndLogsChange(t *testing.T) {
	svc := newTestService(t)
	task, _ := svc.Add(AddInput{Title: "원래 제목"})
	old := task.Path

	title := "바뀐 제목"
	res, err := svc.Edit(task.ID, EditInput{Title: &title})
	if err != nil {
		t.Fatal(err)
	}
	if res.Task.Path == old {
		t.Fatal("파일명이 갱신되지 않음")
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatal("이전 파일이 남아 있음")
	}
	if len(svc.All()) != 1 {
		t.Fatalf("인덱스에 중복: %d", len(svc.All()))
	}
	raw, _ := os.ReadFile(res.Task.Path)
	if !strings.Contains(string(raw), "edit: title=") {
		t.Fatalf("변경 로그 없음:\n%s", raw)
	}
}

// The index is a derived artifact: deleting it must lose nothing.
func TestIndexRebuildsFromMarkdown(t *testing.T) {
	svc := newTestService(t)
	svc.Add(AddInput{Title: "하나", Project: "infra"})
	svc.Add(AddInput{Title: "둘"})
	svc.Close()

	if err := os.RemoveAll(svc.Vault().IndexDir()); err != nil {
		t.Fatal(err)
	}
	fresh, err := Open(config.Default(svc.Cfg.Vault))
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Close()
	if got := len(fresh.All()); got != 2 {
		t.Fatalf("재생성 후 %d건", got)
	}
}

// A task edited outside the tool must be picked up on the next sync.
func TestSyncPicksUpExternalEdits(t *testing.T) {
	svc := newTestService(t)
	task, _ := svc.Add(AddInput{Title: "외부 편집 대상"})

	raw, _ := os.ReadFile(task.Path)
	edited := strings.Replace(string(raw), "status: todo", "status: doing", 1)
	// mtime granularity: force a different value rather than relying on timing.
	os.WriteFile(task.Path, []byte(edited), 0o644)
	future := time.Now().Add(2 * time.Second)
	os.Chtimes(task.Path, future, future)

	if _, err := svc.Sync(); err != nil {
		t.Fatal(err)
	}
	got, ok := svc.idx.Get(task.ID)
	if !ok || got.Status != domain.StatusDoing {
		t.Fatalf("외부 편집이 반영되지 않음: %+v", got)
	}
}

func TestDeleteRemovesFileAndEntry(t *testing.T) {
	svc := newTestService(t)
	task, _ := svc.Add(AddInput{Title: "삭제 대상"})
	if err := svc.Delete(task.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(task.Path); !os.IsNotExist(err) {
		t.Fatal("파일이 남아 있음")
	}
	if len(svc.All()) != 0 {
		t.Fatalf("인덱스에 남음: %d", len(svc.All()))
	}
}

func TestOpenFailsOnMissingVault(t *testing.T) {
	cfg := config.Default(filepath.Join(t.TempDir(), "없는경로"))
	if _, err := Open(cfg); err == nil {
		t.Fatal("없는 vault 가 열림")
	}
}
