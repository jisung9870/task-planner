package domain

import (
	"fmt"
	"strings"
	"time"
)

const (
	noteHeading = "## Note"
	logHeading  = "## Log"
)

// AppendLog records a state change in the body's `## Log` section.
//
// The history lives in the markdown body rather than in frontmatter (which
// would make every diff noisy) or a side-car event file (which would mean the
// markdown is no longer the single source of truth).
func (t *Task) AppendLog(at time.Time, format string, args ...any) {
	line := fmt.Sprintf("- %s %s", at.Format("2006-01-02 15:04"), fmt.Sprintf(format, args...))
	body := strings.TrimRight(t.Body, "\n")
	idx := findHeading(body, logHeading)
	if idx < 0 {
		if body != "" {
			body += "\n\n"
		}
		t.Body = body + logHeading + "\n" + line + "\n"
		return
	}
	// Append at the end of the log section (i.e. before the next heading).
	end := nextHeading(body, idx+len(logHeading))
	if end < 0 {
		t.Body = strings.TrimRight(body, "\n") + "\n" + line + "\n"
		return
	}
	head := strings.TrimRight(body[:end], "\n")
	t.Body = head + "\n" + line + "\n\n" + strings.TrimLeft(body[end:], "\n") + "\n"
}

// Note returns the text under `## Note`, or the whole body when the task has no
// explicit sections (quick captures start that way).
func (t *Task) Note() string {
	body := strings.TrimSpace(t.Body)
	idx := findHeading(body, noteHeading)
	if idx < 0 {
		if findHeading(body, logHeading) >= 0 {
			return strings.TrimSpace(body[:findHeading(body, logHeading)])
		}
		return body
	}
	rest := body[idx+len(noteHeading):]
	if end := nextHeading(rest, 0); end >= 0 {
		rest = rest[:end]
	}
	return strings.TrimSpace(rest)
}

// LogLines returns the `## Log` entries oldest first.
func (t *Task) LogLines() []string {
	body := strings.TrimSpace(t.Body)
	idx := findHeading(body, logHeading)
	if idx < 0 {
		return nil
	}
	rest := body[idx+len(logHeading):]
	if end := nextHeading(rest, 0); end >= 0 {
		rest = rest[:end]
	}
	var out []string
	for _, l := range strings.Split(rest, "\n") {
		if l = strings.TrimSpace(l); strings.HasPrefix(l, "- ") {
			out = append(out, strings.TrimPrefix(l, "- "))
		}
	}
	return out
}

// findHeading locates a heading at the start of a line.
func findHeading(s, heading string) int {
	if strings.HasPrefix(s, heading) {
		return 0
	}
	if i := strings.Index(s, "\n"+heading); i >= 0 {
		return i + 1
	}
	return -1
}

// nextHeading returns the offset of the next `## ` heading after from, or -1.
func nextHeading(s string, from int) int {
	i := strings.Index(s[from:], "\n## ")
	if i < 0 {
		return -1
	}
	return from + i + 1
}
