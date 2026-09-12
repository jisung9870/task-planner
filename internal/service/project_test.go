package service

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCreateProjectWritesFileAndShowsUpEmpty(t *testing.T) {
	svc := newTestService(t)
	p, err := svc.CreateProject("Infra 2026", "인프라 개편")
	if err != nil {
		t.Fatal(err)
	}
	if p.Slug != "infra-2026" {
		t.Fatalf("slug = %q", p.Slug)
	}
	want := filepath.Join(svc.Vault().ProjectsDir(), "infra-2026", "project.md")
	if p.Path != want {
		t.Fatalf("path = %q, want %q", p.Path, want)
	}
	if _, err := os.Stat(want); err != nil {
		t.Fatal(err)
	}
	// A project with no tasks yet must still be listed, or creating one looks
	// like it silently failed.
	rows, err := svc.ProjectRows()
	if err != nil {
		t.Fatal(err)
	}
	r := findRow(t, rows, "infra-2026")
	if !r.Defined || r.Name != "인프라 개편" || r.Status != "active" || r.Open != 0 {
		t.Fatalf("row = %+v", r)
	}
}

func TestCreateProjectRejectsDuplicate(t *testing.T) {
	svc := newTestService(t)
	if _, err := svc.CreateProject("infra", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateProject("infra", ""); err == nil {
		t.Fatal("중복 slug 가 통과함")
	}
}

func TestProjectRowsMergeCountsWithMetadata(t *testing.T) {
	svc := newTestService(t)
	if _, err := svc.CreateProject("infra", "인프라"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Add(AddInput{Title: "방화벽 정리", Project: "infra"}); err != nil {
		t.Fatal(err)
	}
	// A slug that only ever appears on a task is still a project row, just
	// without metadata to edit.
	if _, err := svc.Add(AddInput{Title: "임시 작업", Project: "adhoc"}); err != nil {
		t.Fatal(err)
	}
	rows, err := svc.ProjectRows()
	if err != nil {
		t.Fatal(err)
	}
	if r := findRow(t, rows, "infra"); r.Open != 1 || !r.Defined || r.Name != "인프라" {
		t.Fatalf("infra = %+v", r)
	}
	if r := findRow(t, rows, "adhoc"); r.Open != 1 || r.Defined {
		t.Fatalf("adhoc = %+v", r)
	}
}

func TestCycleProjectStatus(t *testing.T) {
	svc := newTestService(t)
	if _, err := svc.CreateProject("infra", ""); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"paused", "done", "active"} {
		p, err := svc.CycleProjectStatus("infra")
		if err != nil {
			t.Fatal(err)
		}
		if p.Status != want {
			t.Fatalf("status = %q, want %q", p.Status, want)
		}
	}
	// The change has to survive a re-read: project.md is the source of truth.
	p, err := svc.ProjectBySlug("infra")
	if err != nil {
		t.Fatal(err)
	}
	if p.Status != "active" {
		t.Fatalf("재읽기 status = %q", p.Status)
	}
}

func findRow(t *testing.T, rows []ProjectRow, slug string) ProjectRow {
	t.Helper()
	for _, r := range rows {
		if r.Slug == slug {
			return r
		}
	}
	t.Fatalf("%q 행이 없음: %+v", slug, rows)
	return ProjectRow{}
}
