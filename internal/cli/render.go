package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/mattn/go-runewidth"

	"task-planner/internal/domain"
)

// pad left-aligns to a display width. %-Ns counts bytes, which misaligns every
// column the moment a Korean title appears.
func pad(s string, w int) string {
	d := w - runewidth.StringWidth(s)
	if d <= 0 {
		return s
	}
	return s + strings.Repeat(" ", d)
}

// padLeft right-aligns to a display width, for numeric column headings whose
// %Nd counterparts count cells rather than bytes.
func padLeft(s string, w int) string {
	d := w - runewidth.StringWidth(s)
	if d <= 0 {
		return s
	}
	return strings.Repeat(" ", d) + s
}

// renderList prints a task list grouped by status - the same grouping the TUI
// shows, so switching between the two does not require re-learning the layout.
func renderList(w io.Writer, ts []*domain.Task, today domain.Date, staleDays int) {
	if len(ts) == 0 {
		fmt.Fprintln(w, "  (없음)")
		return
	}
	groups := domain.GroupByStatus(ts)
	for _, st := range domain.AllStatuses {
		g := groups[st]
		if len(g) == 0 {
			continue
		}
		fmt.Fprintf(w, "\n%s (%d)\n", st.Label(), len(g))
		for _, t := range g {
			fmt.Fprintln(w, "  "+taskLine(t, today, staleDays))
		}
	}
}

// taskLine is the one-line form used by every list output.
func taskLine(t *domain.Task, today domain.Date, staleDays int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s %s", t.Status.Glyph(), pad(t.ShortID(), 5), t.Title)
	var meta []string
	if t.Project != "" {
		meta = append(meta, t.Project)
	}
	if t.Priority != "" {
		meta = append(meta, string(t.Priority))
	}
	if t.Agent != "" || t.ClaimedBy != "" {
		meta = append(meta, t.AgentBadge())
	}
	if len(t.Tags) > 0 {
		meta = append(meta, "#"+strings.Join(t.Tags, " #"))
	}
	if len(meta) > 0 {
		fmt.Fprintf(&b, "  [%s]", strings.Join(meta, " · "))
	}
	if when := t.When(today); when != "" {
		if t.Stale(today, staleDays) {
			when = "! " + when
		}
		b.WriteString("  " + when)
	}
	if t.Status == domain.StatusBlocked {
		reason := t.BlockedReason
		if reason == "" && len(t.BlockedBy) > 0 {
			reason = "선행 " + strings.Join(domain.ShortRefs(t.BlockedBy), ", ")
		}
		if d := t.BlockedDays(today); d > 0 {
			reason = fmt.Sprintf("%s (%d일 경과)", reason, d)
		}
		b.WriteString("  ← " + reason)
	}
	return b.String()
}
