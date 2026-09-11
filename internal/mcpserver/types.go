package mcpserver

import (
	"fmt"
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

// parseDate accepts the forms worth exposing over MCP. Anything fancier the
// calling model can compute itself before passing an ISO date.
func parseDate(s string, today domain.Date) (domain.Date, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "":
		return domain.Date{}, nil
	case "today", "오늘":
		return today, nil
	case "tomorrow", "내일":
		return today.AddDays(1), nil
	case "none", "clear":
		return domain.Date{}, nil
	}
	d, err := domain.ParseDate(s)
	if err != nil {
		return domain.Date{}, fmt.Errorf("날짜는 YYYY-MM-DD, today, tomorrow, none 중 하나: %q", s)
	}
	return d, nil
}
