package service

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"task-planner/internal/config"
	"task-planner/internal/domain"
	"task-planner/internal/gitsync"
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

func TestNumberingContinuesAcrossDatesAndArchive(t *testing.T) {
	svc := newTestService(t)
	first, err := svc.Add(AddInput{Title: "첫 작업"})
	if err != nil {
		t.Fatal(err)
	}
	archived, err := svc.Add(AddInput{Title: "아카이브 작업"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.vault.ArchiveTask(archived); err != nil {
		t.Fatal(err)
	}
	svc.idx.Remove(archived.ID)
	svc.SetClock(func() time.Time { return time.Date(2026, 9, 13, 9, 0, 0, 0, time.Local) })
	next, err := svc.Add(AddInput{Title: "다음 날 작업"})
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != "T-20260912-0001" || next.ID != "T-20260913-0003" || next.ShortID() != "#3" {
		t.Fatalf("번호가 이어지지 않음: %s, %s (%s)", first.ID, next.ID, next.ShortID())
	}
}

func TestLegacyDuplicateShortNumbersRemainDistinct(t *testing.T) {
	svc := newTestService(t)
	first, err := svc.Add(AddInput{Title: "첫날 작업"})
	if err != nil {
		t.Fatal(err)
	}
	otherDay := domain.NewDate(2026, 9, 13)
	second := &domain.Task{
		ID: domain.NewID(otherDay, 1), Title: "둘째 날 작업", Status: domain.StatusTodo,
		Created: otherDay, Updated: otherDay,
	}
	if err := svc.vault.SaveTask(second); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Sync(); err != nil {
		t.Fatal(err)
	}
	for id, want := range map[string]string{
		first.ID: "#20260912-1", second.ID: "#20260913-1",
	} {
		resolved, err := svc.Resolve(want)
		if err != nil || resolved.ID != id || resolved.ShortID() != want {
			t.Fatalf("%s → %v, %v", want, resolved, err)
		}
		loaded, err := svc.Load(id)
		if err != nil || loaded.ShortID() != want {
			t.Fatalf("load %s → %v, %v", id, loaded, err)
		}
	}
	if _, err := svc.Resolve("#1"); err == nil || !strings.Contains(err.Error(), "2개") {
		t.Fatalf("중복 번호를 거부하지 않음: %v", err)
	}
	newTask, err := svc.Add(AddInput{Title: "새 작업"})
	if err != nil || newTask.ID != "T-20260912-0002" || newTask.ShortID() != "#2" {
		t.Fatalf("새 번호: %v, %v", newTask, err)
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

// Git integration: a session's changes become exactly one commit.
func TestAutoCommitProducesOneCommitPerSession(t *testing.T) {
	if !gitsync.Available() {
		t.Skip("git 없음")
	}
	svc := newTestService(t)
	if err := svc.GitInit(); err != nil {
		t.Fatal(err)
	}
	gitConfig(t, svc.Cfg.Vault)
	svc.Cfg.Git.AutoCommit = true

	a, _ := svc.Add(AddInput{Title: "하나"})
	svc.Add(AddInput{Title: "둘"})
	svc.Done(a.ID)

	committed, err := svc.CommitPending()
	if err != nil {
		t.Fatal(err)
	}
	if !committed {
		t.Fatal("커밋되지 않음")
	}
	log := gitOut(t, svc.Cfg.Vault, "log", "--format=%s%n%b", "-1")
	if !strings.Contains(log, "tp: #1 추가, #2 추가, #1 완료") {
		t.Fatalf("커밋 제목: %q", log)
	}
	if !strings.Contains(log, "doing → done") && !strings.Contains(log, "todo → done") {
		t.Fatalf("본문에 변경 내역 없음: %q", log)
	}
	// Second call has nothing pending.
	if again, err := svc.CommitPending(); err != nil || again {
		t.Fatalf("중복 커밋: %v %v", again, err)
	}
}

func TestAutoCommitOffLeavesRepoUntouched(t *testing.T) {
	if !gitsync.Available() {
		t.Skip("git 없음")
	}
	svc := newTestService(t)
	if err := svc.GitInit(); err != nil {
		t.Fatal(err)
	}
	gitConfig(t, svc.Cfg.Vault)
	svc.Add(AddInput{Title: "하나"})
	if committed, err := svc.CommitPending(); err != nil || committed {
		t.Fatalf("auto_commit=false 인데 커밋됨: %v %v", committed, err)
	}
}

func gitConfig(t *testing.T, dir string) {
	t.Helper()
	for _, kv := range [][2]string{{"user.email", "t@example.com"}, {"user.name", "t"}} {
		cmd := exec.Command("git", "config", kv[0], kv[1])
		cmd.Dir = dir
		if err := cmd.Run(); err != nil {
			t.Fatal(err)
		}
	}
}

func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	raw, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
