package service

import (
	"testing"

	"task-planner/internal/config"
)

func TestSaveViewPersistsToConfig(t *testing.T) {
	svc := newTestService(t)
	before := len(svc.Views())
	if err := svc.SaveView("막힌 것", "is:blocked"); err != nil {
		t.Fatal(err)
	}
	if got := len(svc.Views()); got != before+1 {
		t.Fatalf("뷰 %d개", got)
	}
	// Re-reading config is what proves it survived the session.
	reloaded, err := config.Load(svc.Cfg.Vault)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, v := range reloaded.Views {
		if v.Name == "막힌 것" && v.Query == "is:blocked" {
			found = true
		}
	}
	if !found {
		t.Fatalf("config.yaml 에 없음: %+v", reloaded.Views)
	}
}

func TestSaveViewReplacesSameNameAndRejectsBadQuery(t *testing.T) {
	svc := newTestService(t)
	if err := svc.SaveView("내 뷰", "is:blocked"); err != nil {
		t.Fatal(err)
	}
	if err := svc.SaveView("내 뷰", "status:doing"); err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, v := range svc.Views() {
		if v.Name == "내 뷰" {
			n++
			if v.Query != "status:doing" {
				t.Fatalf("query = %q", v.Query)
			}
		}
	}
	if n != 1 {
		t.Fatalf("같은 이름이 %d개", n)
	}
	// A view that cannot be parsed would fail every morning instead of now.
	if err := svc.SaveView("깨진 뷰", "없는필드:x"); err == nil {
		t.Fatal("잘못된 질의가 저장됨")
	}
	if err := svc.DeleteView("내 뷰"); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteView("내 뷰"); err == nil {
		t.Fatal("없는 뷰 삭제가 성공함")
	}
}

func TestDefaultViewsParse(t *testing.T) {
	svc := newTestService(t)
	views := svc.Views()
	if len(views) == 0 {
		t.Fatal("기본 뷰가 없음")
	}
	for _, v := range views {
		if _, err := svc.Filter(v.Query); err != nil {
			t.Fatalf("기본 뷰 %q 가 파싱되지 않음: %v", v.Name, err)
		}
	}
}

func TestBlockingCountsMatchesBlocking(t *testing.T) {
	svc := newTestService(t)
	blocker, _ := svc.Add(AddInput{Title: "선행"})
	for _, title := range []string{"뒤1", "뒤2"} {
		waiting, _ := svc.Add(AddInput{Title: title})
		if _, err := svc.Block(waiting.ID, "", []string{blocker.ID}); err != nil {
			t.Fatal(err)
		}
	}
	counts := svc.BlockingCounts()
	if got, want := counts[blocker.ID], len(svc.Blocking(blocker.ID)); got != want {
		t.Fatalf("count = %d, Blocking = %d", got, want)
	}
	if counts[blocker.ID] != 2 {
		t.Fatalf("count = %d", counts[blocker.ID])
	}
}

func TestClosedOnCountsTerminalWork(t *testing.T) {
	svc := newTestService(t)
	done, _ := svc.Add(AddInput{Title: "완료할 것"})
	svc.Done(done.ID)
	cancelled, _ := svc.Add(AddInput{Title: "접을 것"})
	svc.Cancel(cancelled.ID)
	svc.Add(AddInput{Title: "그대로 둘 것"})

	c := svc.ClosedOn(svc.Today())
	if c.Done != 1 || c.Cancelled != 1 {
		t.Fatalf("%+v", c)
	}
	if other := svc.ClosedOn(svc.Today().AddDays(-1)); other.Done != 0 || other.Cancelled != 0 {
		t.Fatalf("%+v", other)
	}
}
