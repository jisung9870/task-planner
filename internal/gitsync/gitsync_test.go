package gitsync

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func repo(t *testing.T) string {
	t.Helper()
	if !Available() {
		t.Skip("git 없음")
	}
	dir := t.TempDir()
	if err := Init(dir); err != nil {
		t.Fatal(err)
	}
	for _, kv := range [][2]string{{"user.email", "t@example.com"}, {"user.name", "t"}} {
		if _, err := run(dir, "config", kv[0], kv[1]); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestInitAddsIndexIgnore(t *testing.T) {
	dir := repo(t)
	raw, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), ".index/") {
		t.Fatalf(".index/ 무시 규칙 없음:\n%s", raw)
	}
}

// EnsureIgnore must be idempotent: it runs on every `git init` call.
func TestEnsureIgnoreIsIdempotent(t *testing.T) {
	dir := repo(t)
	for i := 0; i < 3; i++ {
		if err := EnsureIgnore(dir); err != nil {
			t.Fatal(err)
		}
	}
	raw, _ := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if n := strings.Count(string(raw), ".index/"); n != 1 {
		t.Fatalf("규칙이 %d번 중복됨:\n%s", n, raw)
	}
}

func TestCommitAllReportsCleanTree(t *testing.T) {
	dir := repo(t)
	if ok, err := CommitAll(dir, "first"); err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	// A clean tree is the common case and must not surface as an error.
	ok, err := CommitAll(dir, "second")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("변경이 없는데 커밋됨")
	}
}

// The index is regenerated from markdown; committing it would conflict on
// every pull.
func TestCommitAllSkipsIndexDir(t *testing.T) {
	dir := repo(t)
	os.MkdirAll(filepath.Join(dir, ".index"), 0o755)
	os.WriteFile(filepath.Join(dir, ".index", "index.json"), []byte("{}"), 0o644)
	os.WriteFile(filepath.Join(dir, "task.md"), []byte("x"), 0o644)
	if _, err := CommitAll(dir, "c"); err != nil {
		t.Fatal(err)
	}
	out, err := run(dir, "ls-files")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, ".index/") {
		t.Fatalf("인덱스가 추적됨:\n%s", out)
	}
	if !strings.Contains(out, "task.md") {
		t.Fatalf("태스크 파일이 커밋되지 않음:\n%s", out)
	}
}
