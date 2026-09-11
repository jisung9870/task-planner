package service

import (
	"fmt"
	"strings"

	"task-planner/internal/domain"
)

// AddInput is the full set of fields a capture can carry. Only Title is
// required: the point of quick capture is that everything else can wait.
type AddInput struct {
	Title     string
	Project   string
	Priority  domain.Priority
	Status    domain.Status
	Scheduled domain.Date
	Due       domain.Date
	Estimate  domain.Duration
	Tags      []string
	Links     []string
	Note      string
	Recur     string
	RecurOf   string
}

// Add creates a task file and returns it.
func (s *Service) Add(in AddInput) (*domain.Task, error) {
	title := strings.TrimSpace(in.Title)
	if title == "" {
		return nil, fmt.Errorf("제목이 비어 있음")
	}
	today := s.Today()
	status := in.Status
	if status == "" {
		status = domain.StatusTodo
	}
	t := &domain.Task{
		ID:        domain.NewID(today, s.nextSeq(today)),
		Title:     title,
		Status:    status,
		Project:   strings.TrimSpace(in.Project),
		Priority:  in.Priority,
		Created:   today,
		Updated:   today,
		Scheduled: in.Scheduled,
		Due:       in.Due,
		Estimate:  in.Estimate,
		Tags:      in.Tags,
		Links:     in.Links,
		Recur:     strings.TrimSpace(in.Recur),
		RecurOf:   strings.TrimSpace(in.RecurOf),
	}
	if note := strings.TrimSpace(in.Note); note != "" {
		t.Body = "## Note\n" + note + "\n"
	}
	t.AppendLog(s.now(), "created")
	if err := s.save(t); err != nil {
		return nil, err
	}
	return t, nil
}

// nextSeq finds the next free per-day sequence number. Scanning the index is
// fine at this scale and avoids a counter file that could drift from reality.
func (s *Service) nextSeq(d domain.Date) int {
	prefix := "T-" + d.Time().Format("20060102") + "-"
	max := 0
	for _, t := range s.All() {
		if !strings.HasPrefix(t.ID, prefix) {
			continue
		}
		if n, ok := seqOf(t.ID); ok && n > max {
			max = n
		}
	}
	return max + 1
}

// Result carries a mutation outcome plus any non-fatal advice for the user.
type Result struct {
	Task     *domain.Task
	Warnings []string
}

// SetStatus moves a task between states, writing the change and its log line.
func (s *Service) SetStatus(ref string, to domain.Status, block *domain.BlockInfo) (*Result, error) {
	t, err := s.Load(ref)
	if err != nil {
		return nil, err
	}
	if err := t.Transition(to, s.now(), block); err != nil {
		return nil, err
	}
	if err := s.save(t); err != nil {
		return nil, err
	}
	return &Result{Task: t}, nil
}

// Start, Done, Cancel and Reopen are the shorthands the CLI and TUI bind to.
func (s *Service) Start(ref string) (*Result, error) {
	return s.SetStatus(ref, domain.StatusDoing, nil)
}

func (s *Service) Done(ref string) (*Result, error) {
	return s.SetStatus(ref, domain.StatusDone, nil)
}

func (s *Service) Cancel(ref string) (*Result, error) {
	return s.SetStatus(ref, domain.StatusCancelled, nil)
}

func (s *Service) Reopen(ref string) (*Result, error) {
	return s.SetStatus(ref, domain.StatusTodo, nil)
}

// Block puts a task on hold. A reason (or a blocking task) is mandatory - an
// unexplained hold is how the backlog fills with zombies.
func (s *Service) Block(ref, reason string, by []string) (*Result, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" && len(by) == 0 {
		return nil, domain.ErrBlockedNeedsReason
	}
	return s.SetStatus(ref, domain.StatusBlocked, &domain.BlockInfo{Reason: reason, By: by})
}

// EditInput carries partial updates; nil pointers mean "leave alone".
type EditInput struct {
	Title     *string
	Project   *string
	Priority  *domain.Priority
	Scheduled *domain.Date
	Due       *domain.Date
	Estimate  *domain.Duration
	Tags      *[]string
	Links     *[]string
	Recur     *string
}

// Edit applies field updates and records what changed.
func (s *Service) Edit(ref string, in EditInput) (*Result, error) {
	t, err := s.Load(ref)
	if err != nil {
		return nil, err
	}
	var changes []string
	if in.Title != nil && *in.Title != t.Title {
		changes = append(changes, fmt.Sprintf("title=%q", *in.Title))
		t.Title = *in.Title
	}
	if in.Project != nil && *in.Project != t.Project {
		changes = append(changes, "project="+*in.Project)
		t.Project = *in.Project
	}
	if in.Priority != nil && *in.Priority != t.Priority {
		changes = append(changes, "priority="+string(*in.Priority))
		t.Priority = *in.Priority
	}
	if in.Scheduled != nil && !in.Scheduled.Equal(t.Scheduled) {
		changes = append(changes, "scheduled="+in.Scheduled.String())
		t.Scheduled = *in.Scheduled
	}
	if in.Due != nil && !in.Due.Equal(t.Due) {
		changes = append(changes, "due="+in.Due.String())
		t.Due = *in.Due
	}
	if in.Estimate != nil && *in.Estimate != t.Estimate {
		changes = append(changes, "estimate="+in.Estimate.String())
		t.Estimate = *in.Estimate
	}
	if in.Tags != nil {
		changes = append(changes, "tags="+strings.Join(*in.Tags, ","))
		t.Tags = *in.Tags
	}
	if in.Links != nil {
		changes = append(changes, "links="+strings.Join(*in.Links, ","))
		t.Links = *in.Links
	}
	if in.Recur != nil && *in.Recur != t.Recur {
		changes = append(changes, "recur="+*in.Recur)
		t.Recur = *in.Recur
	}
	if len(changes) == 0 {
		return &Result{Task: t}, nil
	}
	t.Updated = s.Today()
	t.AppendLog(s.now(), "edit: %s", strings.Join(changes, " "))
	if err := s.save(t); err != nil {
		return nil, err
	}
	return &Result{Task: t}, nil
}

// Delete removes a task file outright. Cancelling is almost always the better
// move; this exists for captures made by mistake.
func (s *Service) Delete(ref string) error {
	t, err := s.Resolve(ref)
	if err != nil {
		return err
	}
	if err := s.vault.DeleteTask(t); err != nil {
		return err
	}
	s.idx.Remove(t.ID)
	return s.idx.Flush()
}

// SaveTask persists a task the caller already holds (used after external edits).
func (s *Service) SaveTask(t *domain.Task) error { return s.save(t) }
