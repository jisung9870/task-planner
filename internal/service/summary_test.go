package service

import (
	"testing"
)

func TestSummarizeCountsEachSignal(t *testing.T) {
	svc := newTestService(t)
	today := svc.Today()

	dueToday, _ := svc.Add(AddInput{Title: "오늘 마감", Due: today})
	_ = dueToday
	overdue, _ := svc.Add(AddInput{Title: "밀린 것", Due: today.AddDays(-3)})
	_ = overdue
	doing, _ := svc.Add(AddInput{Title: "진행중"})
	svc.Start(doing.ID)
	blocked, _ := svc.Add(AddInput{Title: "보류"})
	svc.Block(blocked.ID, "회신 대기", nil)
	// Push the hold's start date back so BlockedMaxDay is non-zero.
	b, _ := svc.Load(blocked.ID)
	b.BlockedSince = today.AddDays(-12)
	if err := svc.SaveTask(b); err != nil {
		t.Fatal(err)
	}
	carried, _ := svc.Add(AddInput{Title: "이월된 것"})
	c, _ := svc.Load(carried.ID)
	c.RolloverCount = 2
	if err := svc.SaveTask(c); err != nil {
		t.Fatal(err)
	}
	// Closed work must not count anywhere.
	done, _ := svc.Add(AddInput{Title: "끝난 것", Due: today})
	svc.Done(done.ID)

	sum := svc.Summarize()
	if sum.DueToday != 1 || sum.Overdue != 1 {
		t.Fatalf("due=%d overdue=%d", sum.DueToday, sum.Overdue)
	}
	if sum.Doing != 1 || sum.Blocked != 1 || sum.BlockedMaxDay != 12 {
		t.Fatalf("doing=%d blocked=%d maxday=%d", sum.Doing, sum.Blocked, sum.BlockedMaxDay)
	}
	if sum.Carried != 1 {
		t.Fatalf("carried=%d", sum.Carried)
	}
}
