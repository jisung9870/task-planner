package service

import (
	"strings"
	"testing"
)

func TestAddNoteAppendsAndPersists(t *testing.T) {
	svc := newTestService(t)
	task, _ := svc.Add(AddInput{Title: "API 설계"})
	if _, err := svc.AddNote(task.ID, "보안팀 회신 대기"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AddNote(task.ID, "담당 바뀜"); err != nil {
		t.Fatal(err)
	}
	full, err := svc.Load(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	note := full.Note()
	if !strings.Contains(note, "보안팀 회신 대기") || !strings.Contains(note, "담당 바뀜") {
		t.Fatalf("note = %q", note)
	}
	if _, err := svc.AddNote(task.ID, "   "); err == nil {
		t.Fatal("빈 메모가 통과함")
	}
}

// The point of body search: a note written today is findable tomorrow without
// remembering which task it was on.
func TestQueryFindsNoteText(t *testing.T) {
	svc := newTestService(t)
	target, _ := svc.Add(AddInput{Title: "게이트웨이 점검"})
	svc.Add(AddInput{Title: "관계없는 작업"})
	if _, err := svc.AddNote(target.ID, "타임아웃을 30초로 조정함"); err != nil {
		t.Fatal(err)
	}
	got, err := svc.Query("body:타임아웃")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != target.ID {
		t.Fatalf("%d건: %+v", len(got), got)
	}
	// The cheap terms must still narrow first: this one excludes the match.
	got, err = svc.Query("project:infra body:타임아웃")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("프로젝트 조건이 무시됨: %+v", got)
	}
}
