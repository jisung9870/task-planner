package service

import (
	"testing"
	"time"
)

func TestSummarizeCountsEachSignal(t *testing.T) {
	svc := newTestService(t)
	today := svc.Today()

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
	// Surfaced five days ago and never started: past stale_days.
	svc.Add(AddInput{Title: "꺼낸 채 둔 것", Scheduled: today.AddDays(-5)})
	svc.Add(AddInput{Title: "어제 꺼낸 것", Scheduled: today.AddDays(-1)})
	// Running for six days.
	long, _ := svc.Add(AddInput{Title: "큰 일"})
	svc.Start(long.ID)
	l, _ := svc.Load(long.ID)
	started := svc.Now().Add(-6 * 24 * time.Hour)
	l.StartedAt = &started
	if err := svc.SaveTask(l); err != nil {
		t.Fatal(err)
	}
	// Closed work must not count anywhere.
	done, _ := svc.Add(AddInput{Title: "끝난 것", Scheduled: today.AddDays(-9)})
	svc.Done(done.ID)

	sum := svc.Summarize()
	if sum.Doing != 2 || sum.Blocked != 1 || sum.BlockedMaxDay != 12 {
		t.Fatalf("doing=%d blocked=%d maxday=%d", sum.Doing, sum.Blocked, sum.BlockedMaxDay)
	}
	if sum.Stale != 2 {
		t.Fatalf("stale=%d, want the 5-day wait and the 6-day run", sum.Stale)
	}
}
