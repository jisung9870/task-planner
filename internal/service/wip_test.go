package service

import (
	"strings"
	"testing"

	"task-planner/internal/domain"
)

func TestWIPWarningFiresOnlyPastTheLimit(t *testing.T) {
	svc := newTestService(t)
	svc.Cfg.WIPLimit = 2

	a, _ := svc.Add(AddInput{Title: "A"})
	b, _ := svc.Add(AddInput{Title: "B"})
	c, _ := svc.Add(AddInput{Title: "C"})

	res, err := svc.Start(a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Warnings) != 0 {
		t.Fatalf("한도 내인데 경고: %v", res.Warnings)
	}
	if res, _ = svc.Start(b.ID); len(res.Warnings) != 0 {
		t.Fatalf("한도 도달인데 경고: %v", res.Warnings)
	}
	res, _ = svc.Start(c.ID)
	if len(res.Warnings) != 1 {
		t.Fatalf("한도 초과인데 경고 없음: %v", res.Warnings)
	}
	// The warning has to name what to finish, or it is just noise.
	w := res.Warnings[0]
	if !strings.Contains(w, a.ShortID()) || !strings.Contains(w, b.ShortID()) {
		t.Fatalf("경고에 진행중 목록이 없음: %q", w)
	}
	if strings.Contains(w, c.ShortID()) {
		t.Fatalf("방금 시작한 태스크가 정리 대상으로 나옴: %q", w)
	}
}

// The limit warns but never blocks: a tool that refuses to record what someone
// is actually doing gets bypassed.
func TestWIPLimitDoesNotBlockTransition(t *testing.T) {
	svc := newTestService(t)
	svc.Cfg.WIPLimit = 1
	a, _ := svc.Add(AddInput{Title: "A"})
	b, _ := svc.Add(AddInput{Title: "B"})
	svc.Start(a.ID)

	res, err := svc.Start(b.ID)
	if err != nil {
		t.Fatal(err)
	}
	if res.Task.Status != domain.StatusDoing {
		t.Fatalf("상태 = %s", res.Task.Status)
	}
	if svc.WIP().Count != 2 {
		t.Fatalf("진행중 = %d", svc.WIP().Count)
	}
}

func TestWIPLimitZeroDisablesWarning(t *testing.T) {
	svc := newTestService(t)
	svc.Cfg.WIPLimit = 0
	for _, title := range []string{"A", "B", "C"} {
		task, _ := svc.Add(AddInput{Title: title})
		res, err := svc.Start(task.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(res.Warnings) != 0 {
			t.Fatalf("한도 0(비활성)인데 경고: %v", res.Warnings)
		}
	}
}

// Capturing a task straight into 진행중 must warn too, otherwise `tp add
// --start` is a hole in the limit.
func TestAddWithStartWarnsOnLimit(t *testing.T) {
	svc := newTestService(t)
	svc.Cfg.WIPLimit = 1
	first, _ := svc.Add(AddInput{Title: "A"})
	svc.Start(first.ID)

	res, err := svc.AddWithResult(AddInput{Title: "B", Status: domain.StatusDoing})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Warnings) != 1 {
		t.Fatalf("경고 = %v", res.Warnings)
	}
}

func TestWIPStatusReportsRunningTasks(t *testing.T) {
	svc := newTestService(t)
	svc.Cfg.WIPLimit = 3
	a, _ := svc.Add(AddInput{Title: "A"})
	svc.Start(a.ID)

	w := svc.WIP()
	if w.Count != 1 || w.Limit != 3 || len(w.Running) != 1 {
		t.Fatalf("wip = %+v", w)
	}
	if w.Exceeded() {
		t.Fatal("초과로 잘못 판정")
	}
	if w.AtLimit() {
		t.Fatal("한도 도달로 잘못 판정")
	}
}
