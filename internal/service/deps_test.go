package service

import (
	"strings"
	"testing"

	"task-planner/internal/domain"
)

// The whole point of recording a dependency is that finishing the blocker
// releases the waiting task without anyone remembering to look.
func TestCompletingBlockerReleasesDependent(t *testing.T) {
	svc := newTestService(t)
	dep, _ := svc.Add(AddInput{Title: "인프라팀 승인"})
	waiting, _ := svc.Add(AddInput{Title: "DB 마이그레이션"})

	if _, err := svc.Block(waiting.ID, "", []string{dep.ID}); err != nil {
		t.Fatal(err)
	}
	res, err := svc.Done(dep.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Unblocked) != 1 || res.Unblocked[0].Task.ID != waiting.ID {
		t.Fatalf("해제 결과 = %+v", res.Unblocked)
	}
	got, _ := svc.Load(waiting.ID)
	if got.Status != domain.StatusTodo {
		t.Fatalf("상태 = %s", got.Status)
	}
	if len(got.BlockedBy) != 0 || got.BlockedReason != "" {
		t.Fatalf("보류 정보가 남음: %v %q", got.BlockedBy, got.BlockedReason)
	}
	raw := strings.Join(got.LogLines(), "\n")
	if !strings.Contains(raw, "선행 "+dep.ID+" 완료로 보류 해제") {
		t.Fatalf("해제 로그 없음:\n%s", raw)
	}
}

func TestDependentStaysBlockedUntilAllBlockersDone(t *testing.T) {
	svc := newTestService(t)
	a, _ := svc.Add(AddInput{Title: "선행 A"})
	b, _ := svc.Add(AddInput{Title: "선행 B"})
	waiting, _ := svc.Add(AddInput{Title: "대기"})
	if _, err := svc.Block(waiting.ID, "", []string{a.ID, b.ID}); err != nil {
		t.Fatal(err)
	}

	res, _ := svc.Done(a.ID)
	if len(res.Unblocked) != 0 {
		t.Fatalf("선행 하나만 끝났는데 해제됨: %+v", res.Unblocked)
	}
	res, _ = svc.Done(b.ID)
	if len(res.Unblocked) != 1 {
		t.Fatalf("마지막 선행 완료 후에도 해제되지 않음: %+v", res.Unblocked)
	}
}

// Cancelling a blocker also releases: the waiting task is no longer waiting on
// anything that will ever happen.
func TestCancellingBlockerAlsoReleases(t *testing.T) {
	svc := newTestService(t)
	dep, _ := svc.Add(AddInput{Title: "접힌 선행"})
	waiting, _ := svc.Add(AddInput{Title: "대기"})
	svc.Block(waiting.ID, "", []string{dep.ID})

	res, err := svc.Cancel(dep.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Unblocked) != 1 {
		t.Fatalf("취소로 해제되지 않음: %+v", res.Unblocked)
	}
}

// Raw user input must be canonicalised; storing "#1" would never match an id.
func TestBlockResolvesShortReferences(t *testing.T) {
	svc := newTestService(t)
	dep, _ := svc.Add(AddInput{Title: "선행 승인"})
	waiting, _ := svc.Add(AddInput{Title: "대기 작업"})

	if _, err := svc.Block(waiting.ID, "", []string{"#1"}); err != nil {
		t.Fatal(err)
	}
	got, _ := svc.Load(waiting.ID)
	if len(got.BlockedBy) != 1 || got.BlockedBy[0] != dep.ID {
		t.Fatalf("blocked_by = %v", got.BlockedBy)
	}
}

func TestBlockRejectsSelfAndCycles(t *testing.T) {
	svc := newTestService(t)
	a, _ := svc.Add(AddInput{Title: "A"})
	b, _ := svc.Add(AddInput{Title: "B"})

	if _, err := svc.Block(a.ID, "", []string{a.ID}); err == nil {
		t.Fatal("자기 참조가 통과함")
	}
	if _, err := svc.Block(a.ID, "", []string{b.ID}); err != nil {
		t.Fatal(err)
	}
	_, err := svc.Block(b.ID, "", []string{a.ID})
	if err == nil || !strings.Contains(err.Error(), "순환") {
		t.Fatalf("순환 의존이 통과함: %v", err)
	}
}

// A hold nothing will ever release is a typo, not a plan.
func TestBlockRejectsAlreadyFinishedBlocker(t *testing.T) {
	svc := newTestService(t)
	dep, _ := svc.Add(AddInput{Title: "이미 끝난 것"})
	svc.Done(dep.ID)
	waiting, _ := svc.Add(AddInput{Title: "대기"})

	if _, err := svc.Block(waiting.ID, "", []string{dep.ID}); err == nil {
		t.Fatal("완료된 선행으로 보류가 걸림")
	}
	// With an explicit reason it is a deliberate hold, so it is allowed.
	if _, err := svc.Block(waiting.ID, "다른 이유", []string{dep.ID}); err != nil {
		t.Fatal(err)
	}
}

func TestBlockersAndBlockingBothDirections(t *testing.T) {
	svc := newTestService(t)
	dep, _ := svc.Add(AddInput{Title: "선행"})
	waiting, _ := svc.Add(AddInput{Title: "후행"})
	svc.Block(waiting.ID, "", []string{dep.ID})

	w, _ := svc.Load(waiting.ID)
	known, missing := svc.Blockers(w)
	if len(known) != 1 || known[0].ID != dep.ID || len(missing) != 0 {
		t.Fatalf("선행 = %v / %v", known, missing)
	}
	if blocking := svc.Blocking(dep.ID); len(blocking) != 1 || blocking[0].ID != waiting.ID {
		t.Fatalf("후행 = %v", blocking)
	}
}

// An id typed by hand into the markdown may not exist; an unknown blocker must
// not wedge the task permanently.
func TestUnknownBlockerDoesNotHoldTaskForever(t *testing.T) {
	svc := newTestService(t)
	dep, _ := svc.Add(AddInput{Title: "실재하는 선행"})
	waiting, _ := svc.Add(AddInput{Title: "대기"})
	svc.Block(waiting.ID, "", []string{dep.ID})

	// Simulate a hand-edited file referencing an id that no longer exists.
	w, _ := svc.Load(waiting.ID)
	w.BlockedBy = append(w.BlockedBy, "T-19990101-0001")
	if err := svc.SaveTask(w); err != nil {
		t.Fatal(err)
	}

	res, err := svc.Done(dep.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Unblocked) != 1 {
		t.Fatalf("없는 선행 때문에 해제되지 않음: %+v", res.Unblocked)
	}
}
