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
