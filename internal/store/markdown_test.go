package store

import (
	"strings"
	"testing"

	"task-planner/internal/domain"
)

const sample = `---
id: T-20260912-0001
title: 로그 파이프라인 PoC 결과 정리
status: doing
project: log-pipeline
priority: P1
created: 2026-09-12
updated: 2026-09-12
scheduled: 2026-09-12
due: 2026-09-15
estimate: 2h
tags: [ops, report]
links: [jira:ABC-123]
jira_sprint: S-42
---

## Note
결론은 A안.

## Log
- 2026-09-12 09:14 todo → doing
`

func TestDecodeTask(t *testing.T) {
	task, err := DecodeTask([]byte(sample), "x.md")
	if err != nil {
		t.Fatal(err)
	}
	if task.ID != "T-20260912-0001" || task.Status != domain.StatusDoing {
		t.Fatalf("%+v", task)
	}
	if task.Scheduled.String() != "2026-09-12" || task.Due.String() != "2026-09-15" {
		t.Fatalf("dates: %s %s", task.Scheduled, task.Due)
	}
	if task.Estimate.String() != "2h" {
		t.Fatalf("estimate = %s", task.Estimate)
	}
	if task.Note() != "결론은 A안." {
		t.Fatalf("note = %q", task.Note())
	}
	if got := task.Extra["jira_sprint"]; got != "S-42" {
		t.Fatalf("알 수 없는 키가 보존되지 않음: %v", task.Extra)
	}
	if _, dup := task.Extra["title"]; dup {
		t.Fatalf("알려진 키가 Extra 에 남음: %v", task.Extra)
	}
}

// A hand-added frontmatter key must survive a load/save cycle, otherwise
// editing a task in the TUI would silently strip the user's own fields.
func TestEncodeRoundTripPreservesUnknownKeys(t *testing.T) {
	task, err := DecodeTask([]byte(sample), "x.md")
	if err != nil {
		t.Fatal(err)
	}
	task.Status = domain.StatusDone
	raw, err := EncodeTask(task)
	if err != nil {
		t.Fatal(err)
	}
	out := string(raw)
	if !strings.Contains(out, "jira_sprint: S-42") {
		t.Fatalf("알 수 없는 키가 유실됨:\n%s", out)
	}
	if strings.Count(out, "status:") != 1 || !strings.Contains(out, "status: done") {
		t.Fatalf("status 중복 또는 미갱신:\n%s", out)
	}
	again, err := DecodeTask(raw, "x.md")
	if err != nil {
		t.Fatal(err)
	}
	if again.Title != task.Title || again.Note() != task.Note() {
		t.Fatalf("재파싱 불일치: %q / %q", again.Title, again.Note())
	}
}

func TestDecodeRejectsMissingFrontmatter(t *testing.T) {
	if _, err := DecodeTask([]byte("그냥 메모"), "x.md"); err == nil {
		t.Fatal("frontmatter 없는 파일이 통과함")
	}
	if _, err := DecodeTask([]byte("---\nid: T-1\n"), "x.md"); err == nil {
		t.Fatal("종료 표시 없는 frontmatter 가 통과함")
	}
}

func TestEncodeOmitsEmptyBody(t *testing.T) {
	raw, err := EncodeTask(&domain.Task{ID: "T-1", Title: "x", Status: domain.StatusTodo})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(raw), "---") != 2 {
		t.Fatalf("펜스 개수 이상:\n%s", raw)
	}
}
