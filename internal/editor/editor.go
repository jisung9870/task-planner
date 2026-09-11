// Package editor shells out to the user's editor.
//
// Long notes and decision records belong in vim, not in a one-line TUI prompt.
// Because markdown is the source of truth, an external edit is a first-class
// way to change a task - the tool just has to notice afterwards.
package editor

import (
	"fmt"
	"os/exec"
	"strings"

	"task-planner/internal/config"
)

// Command builds the editor invocation for a file.
//
// The configured editor may carry arguments ("code -w", "emacsclient -nw"), so
// it is split on whitespace rather than executed as a single binary name.
func Command(cfg *config.Config, path string) (*exec.Cmd, error) {
	fields := strings.Fields(cfg.EditorCommand())
	if len(fields) == 0 {
		return nil, fmt.Errorf("편집기를 찾을 수 없음 ($EDITOR 또는 config.yaml 의 editor 설정)")
	}
	bin, err := exec.LookPath(fields[0])
	if err != nil {
		return nil, fmt.Errorf("편집기 %q 를 실행할 수 없음: %w", fields[0], err)
	}
	args := append(append([]string{}, fields[1:]...), path)
	return exec.Command(bin, args...), nil
}
