package editor

import (
	"strings"
	"testing"

	"task-planner/internal/config"
)

// A configured editor may carry flags ("code -w"); splitting matters because
// exec would otherwise look for a binary literally named "code -w".
func TestCommandSplitsArguments(t *testing.T) {
	cfg := config.Default(t.TempDir())
	cfg.Editor = "sh -c"
	cmd, err := Command(cfg, "/tmp/x.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(cmd.Path, "/sh") {
		t.Fatalf("path = %s", cmd.Path)
	}
	if got := cmd.Args[len(cmd.Args)-1]; got != "/tmp/x.md" {
		t.Fatalf("마지막 인자 = %q", got)
	}
	if len(cmd.Args) != 3 {
		t.Fatalf("args = %v", cmd.Args)
	}
}

func TestCommandReportsMissingEditor(t *testing.T) {
	cfg := config.Default(t.TempDir())
	cfg.Editor = "정말없는편집기x"
	if _, err := Command(cfg, "/tmp/x.md"); err == nil {
		t.Fatal("없는 편집기가 통과함")
	}
}
