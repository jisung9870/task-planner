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

// AppendNote adds a timestamped line to the body's `## Note` section, creating
// the section above `## Log` when it is missing.
//
// A note is the thing you want to write at the moment you learn it - who said
// what, which value was wrong. Making that require an editor round trip means
// it does not get written.
func (t *Task) AppendNote(at time.Time, text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	line := fmt.Sprintf("- %s %s", at.Format("2006-01-02 15:04"), text)
	body := strings.TrimRight(t.Body, "\n")

	if idx := findHeading(body, noteHeading); idx >= 0 {
		end := nextHeading(body, idx+len(noteHeading))
		if end < 0 {
			t.Body = body + "\n" + line + "\n"
			return
		}
		head := strings.TrimRight(body[:end], "\n")
		t.Body = head + "\n" + line + "\n\n" + strings.TrimLeft(body[end:], "\n") + "\n"
		return
	}

	section := noteHeading + "\n" + line
	// The log is history and belongs at the bottom; a new note section goes in
	// front of it so the file still reads top-down.
	if idx := findHeading(body, logHeading); idx >= 0 {
		head := strings.TrimRight(body[:idx], "\n")
		if head != "" {
			head += "\n\n"
		}
		t.Body = head + section + "\n\n" + strings.TrimLeft(body[idx:], "\n") + "\n"
		return
	}
	if body != "" {
		// A quick capture's body is bare prose with no headings: keep it as the
		// note's first paragraph instead of orphaning it above the section.
		t.Body = noteHeading + "\n" + body + "\n" + line + "\n"
		return
	}
	t.Body = section + "\n"
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
