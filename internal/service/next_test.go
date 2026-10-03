package service

import (
	"strings"
	"testing"

	"task-planner/internal/domain"
)

func TestNextUpSkipsBlockedAndClosed(t *testing.T) {
	svc := newTestService(t)
	blocker, _ := svc.Add(AddInput{Title: "선행 작업"})
	waiting, _ := svc.Add(AddInput{Title: "뒤따르는 작업"})
	if _, err := svc.Block(waiting.ID, "", []string{blocker.ID}); err != nil {
		t.Fatal(err)
	}
	free, _ := svc.Add(AddInput{Title: "막힌 것 없는 작업", Priority: domain.P1})
	closed, _ := svc.Add(AddInput{Title: "끝난 작업"})
	svc.Done(closed.ID)

	got := svc.NextUp(0)
	ids := map[string]bool{}
	for _, sg := range got {
		ids[sg.Task.ID] = true
	}
	if !ids[free.ID] || !ids[blocker.ID] {
		t.Fatalf("추천에 빠짐: %v", ids)
	}
	if ids[waiting.ID] {
		t.Fatal("보류 태스크가 추천됨")
	}
	if ids[closed.ID] {
		t.Fatal("완료 태스크가 추천됨")
	}
}

func TestNextUpRanksPriorityFirstWithReason(t *testing.T) {
	svc := newTestService(t)
	svc.Add(AddInput{Title: "언젠가", Priority: domain.P3})
	urgent, _ := svc.Add(AddInput{Title: "오늘 꺼낸 일", Priority: domain.P1, Scheduled: svc.Today()})

	got := svc.NextUp(1)
	if len(got) != 1 || got[0].Task.ID != urgent.ID {
		t.Fatalf("%+v", got)
	}
	if !strings.Contains(got[0].Reason, "P1") || !strings.Contains(got[0].Reason, "오늘 꺼낸 일") {
		t.Fatalf("reason = %q", got[0].Reason)
	}
}

// The stale signal replaced the rollover count: a task surfaced and left for
// stale_days says it is time to split or drop it.
func TestNextUpReasonFlagsStaleWork(t *testing.T) {
	svc := newTestService(t)
	old, _ := svc.Add(AddInput{Title: "꺼낸 채 둔 일", Scheduled: svc.Today().AddDays(-6)})
	got := svc.NextUp(0)
	if len(got) != 1 || got[0].Task.ID != old.ID || !strings.Contains(got[0].Reason, "꺼낸 지 6일") {
		t.Fatalf("%+v", got)
	}
}

// A task other work waits on should say so: finishing it releases the queue.
func TestNextUpReasonMentionsWaitingWork(t *testing.T) {
	svc := newTestService(t)
	blocker, _ := svc.Add(AddInput{Title: "선행 작업"})
	waiting, _ := svc.Add(AddInput{Title: "뒤따르는 작업"})
	svc.Block(waiting.ID, "", []string{blocker.ID})

	for _, sg := range svc.NextUp(0) {
		if sg.Task.ID == blocker.ID {
			if !strings.Contains(sg.Reason, "기다림") {
				t.Fatalf("reason = %q", sg.Reason)
			}
			return
		}
	}
	t.Fatal("선행 작업이 추천에 없음")
}
