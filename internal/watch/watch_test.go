package watch

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func newWatcher(t *testing.T) (*Watcher, string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "2026-09"), 0o755); err != nil {
		t.Fatal(err)
	}
	w, err := New(dir, 50*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { w.Close() })
	return w, dir
}

func expectEvent(t *testing.T, w *Watcher, why string) {
	t.Helper()
	select {
	case <-w.Events():
	case <-time.After(3 * time.Second):
		t.Fatalf("%s: 이벤트가 오지 않음", why)
	}
}

func expectNoEvent(t *testing.T, w *Watcher, why string) {
	t.Helper()
	select {
	case <-w.Events():
		t.Fatalf("%s: 이벤트가 오면 안 됨", why)
	case <-time.After(400 * time.Millisecond):
	}
}

func TestNotifiesOnMarkdownWrite(t *testing.T) {
	w, dir := newWatcher(t)
	os.WriteFile(filepath.Join(dir, "2026-09", "a.md"), []byte("x"), 0o644)
	expectEvent(t, w, "md 생성")
}

// WriteAtomic and editors leave dot-prefixed temp files behind; reacting to
// those would double the reload rate for every single save.
func TestIgnoresHiddenTempFiles(t *testing.T) {
	w, dir := newWatcher(t)
	os.WriteFile(filepath.Join(dir, "2026-09", ".a.md.tmp123"), []byte("x"), 0o644)
	expectNoEvent(t, w, "임시 파일")
}

func TestIgnoresNonMarkdown(t *testing.T) {
	w, dir := newWatcher(t)
	os.WriteFile(filepath.Join(dir, "2026-09", "notes.txt"), []byte("x"), 0o644)
	expectNoEvent(t, w, "md 아닌 파일")
}

// The vault grows a directory every month; fsnotify is not recursive, so a new
// shard has to be picked up or the month's first task goes unnoticed.
func TestWatchesNewMonthDirectory(t *testing.T) {
	w, dir := newWatcher(t)
	next := filepath.Join(dir, "2026-10")
	if err := os.Mkdir(next, 0o755); err != nil {
		t.Fatal(err)
	}
	expectEvent(t, w, "새 월 디렉토리 생성")

	os.WriteFile(filepath.Join(next, "b.md"), []byte("x"), 0o644)
	expectEvent(t, w, "새 월 디렉토리 안의 md")
}

// A single save fires several filesystem events; the consumer should see one.
func TestDebouncesBurst(t *testing.T) {
	w, dir := newWatcher(t)
	for i := 0; i < 5; i++ {
		os.WriteFile(filepath.Join(dir, "2026-09", "a.md"), []byte("x"), 0o644)
	}
	expectEvent(t, w, "버스트")
	expectNoEvent(t, w, "버스트 이후 중복")
}
