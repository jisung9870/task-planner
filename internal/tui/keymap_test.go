package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// The footer is the whole discovery surface, so it must never be clipped by
// the terminal - a clipped footer loses its last item, which is "? 도움말".
func TestHelpLineFitsWidth(t *testing.T) {
	for _, w := range []int{40, 80, 100, 110, 120, 200} {
		line := helpLine(w)
		if got := lipgloss.Width(line); got > w {
			t.Errorf("width %d: 푸터가 %d 칸 (넘침)", w, got)
		}
		if !strings.HasSuffix(line, helpSegments[len(helpSegments)-1]) {
			t.Errorf("width %d: 도움말 키가 잘려나감: %q", w, line)
		}
	}
}

// Every key the footer advertises has to appear in the full help too, or ?
// answers a question the footer raised with silence.
func TestFooterKeysAppearInFullHelp(t *testing.T) {
	for _, seg := range helpSegments {
		key, _, ok := strings.Cut(seg, " ")
		if !ok {
			t.Fatalf("푸터 항목 형식이 %q", seg)
		}
		if key == "?" || key == "1-6" {
			continue // the help screen itself, and the tab row
		}
		if !strings.Contains(helpFull, key) {
			t.Errorf("푸터의 %q 가 전체 도움말에 없음", key)
		}
	}
}
