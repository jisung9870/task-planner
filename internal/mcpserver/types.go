package mcpserver

import (
	"strings"

	"task-planner/internal/domain"
)

// taskJSON is the wire shape of a task summary. Field names mirror the
// frontmatter keys so what an agent reads over MCP matches what it would see
// in the markdown itself.
type taskJSON struct {
	ID            string   `json:"id"`
	ShortID       string   `json:"short_id"`
	Title         string   `json:"title"`
	Status        string   `json:"status"`
	StatusLabel   string   `json:"status_label"`
	Project       string   `json:"project,omitempty"`
	Executor      string   `json:"executor"`
	Priority      string   `json:"priority,omitempty"`
	Scheduled     string   `json:"scheduled,omitempty"`
	Due           string   `json:"due,omitempty"`
	Estimate      string   `json:"estimate,omitempty"`
	Actual        string   `json:"actual,omitempty"`
	Tags          []string `json:"tags,omitempty"`
	Links         []string `json:"links,omitempty"`
	BlockedReason string   `json:"blocked_reason,omitempty"`
	BlockedBy     []string `json:"blocked_by,omitempty"`
	RolloverCount int      `json:"rollover_count,omitempty"`
	Recur         string   `json:"recur,omitempty"`
	Overdue       bool     `json:"overdue,omitempty"`
}

func toTaskJSON(t *domain.Task, today domain.Date) taskJSON {
	return taskJSON{
		ID:            t.ID,
		ShortID:       t.ShortID(),
		Title:         t.Title,
		Status:        string(t.Status),
		StatusLabel:   t.Status.Label(),
		Project:       t.Project,
		Executor:      string(t.Executor.Effective()),
		Priority:      string(t.Priority),
		Scheduled:     t.Scheduled.String(),
		Due:           t.Due.String(),
		Estimate:      t.Estimate.String(),
		Actual:        t.Actual.String(),
		Tags:          t.Tags,
		Links:         t.Links,
		BlockedReason: t.BlockedReason,
		BlockedBy:     t.BlockedBy,
		RolloverCount: t.RolloverCount,
		Recur:         t.Recur,
		Overdue:       t.Overdue(today),
	}
}

func toTaskList(ts []*domain.Task, today domain.Date) []taskJSON {
	out := make([]taskJSON, len(ts))
	for i, t := range ts {
		out[i] = toTaskJSON(t, today)
	}
	return out
}

// parseDate resolves a date argument through the same domain parser the CLI
// flags and TUI prompts use. An adapter that understood fewer date forms than
// its siblings would just be a trap for the calling model.
func parseDate(s string, today domain.Date) (domain.Date, error) {
	if strings.TrimSpace(s) == "" {
		return domain.Date{}, nil
	}
	return domain.ParseDateRef(s, today)
}
