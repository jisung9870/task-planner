package service

import (
	"fmt"
	"path/filepath"
	"strings"

	"task-planner/internal/domain"
	"task-planner/internal/recur"
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
	if in.Recur != "" {
		if _, err := recur.Parse(in.Recur); err != nil {
			return nil, err
		}
	}
	today := s.Today()
	seq, err := s.nextSeq()
	if err != nil {
		return nil, err
	}
	status := in.Status
	if status == "" {
		status = domain.StatusTodo
	}
	t := &domain.Task{
		ID:        domain.NewID(today, seq),
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
	s.recordChange(t.ShortID()+" 추가", fmt.Sprintf("%s %s: 추가", t.ID, t.Title))
	return t, nil
}

// AddWithResult is Add plus the advisory the caller should surface (currently
// only the WIP warning, which applies when a task is captured as 진행중).
func (s *Service) AddWithResult(in AddInput) (*Result, error) {
	t, err := s.Add(in)
	if err != nil {
		return nil, err
	}
	res := &Result{Task: t}
	if t.Status == domain.StatusDoing {
		if w := s.wipWarning(t.ID); w != "" {
			res.Warnings = append(res.Warnings, w)
		}
	}
	return res, nil
}

// nextSeq continues numbering across dates. Archived files count too: moving
// a finished task out of the active index must not make its number reusable.
func (s *Service) nextSeq() (int, error) {
	max := 0
	for _, t := range s.All() {
		if n, ok := seqOf(t.ID); ok && n > max {
			max = n
		}
	}
	files, err := s.vault.TaskFiles(true)
	if err != nil {
		return 0, err
	}
	archivePrefix := s.vault.ArchiveDir() + string(filepath.Separator)
	for _, path := range files {
		if !strings.HasPrefix(path, archivePrefix) {
			continue
		}
		t, err := s.vault.LoadTask(path)
		if err != nil {
			return 0, fmt.Errorf("아카이브 번호 확인 실패: %w", err)
		}
		if n, ok := seqOf(t.ID); ok && n > max {
			max = n
		}
	}
	return max + 1, nil
}

// Result carries a mutation outcome plus any non-fatal advice for the user.
type Result struct {
	Task     *domain.Task
	Warnings []string
	// Unblocked lists tasks released by this change, if any.
	Unblocked []Unblocked
	// Next is the follow-up occurrence created for a recurring task.
	Next *domain.Task
}

// SetStatus moves a task between states, writing the change and its log line.
func (s *Service) SetStatus(ref string, to domain.Status, block *domain.BlockInfo) (*Result, error) {
	t, err := s.Load(ref)
	if err != nil {
		return nil, err
	}
	from := t.Status
	if err := t.Transition(to, s.now(), &domain.TransitionOpts{
		Block:      block,
		SessionCap: s.Cfg.SessionCap,
	}); err != nil {
		return nil, err
	}
	if err := s.save(t); err != nil {
		return nil, err
	}
	s.recordChange(fmt.Sprintf("%s %s", t.ShortID(), to.Label()),
		fmt.Sprintf("%s %s: %s → %s", t.ID, t.Title, from, to))

	res := &Result{Task: t}
	if to == domain.StatusDoing {
		if w := s.wipWarning(t.ID); w != "" {
			res.Warnings = append(res.Warnings, w)
		}
	}
	released, err := s.releaseDependents(t)
	if err != nil {
		return res, err
	}
	res.Unblocked = released
	for _, u := range released {
		res.Warnings = append(res.Warnings,
			fmt.Sprintf("%s 보류 해제됨 — %s", u.Task.ShortID(), u.Task.Title))
	}
	// Only completion rolls the series forward. Cancelling ends it - stopping a
	// recurring chore has to be possible without editing frontmatter.
	if to == domain.StatusDone {
		next, err := s.spawnNextOccurrence(t)
		if err != nil {
			return res, err
		}
		res.Next = next
	}
	return res, nil
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
	owner, err := s.Resolve(ref)
	if err != nil {
		return nil, err
	}
	ids, err := s.resolveRefs(owner.ID, by)
	if err != nil {
		return nil, err
	}
	// Blocking on work that is already finished is almost always a mistake in
	// the reference, so say so instead of creating a hold nothing will release.
	if len(ids) > 0 && reason == "" && s.allBlockersDone(ids) {
		return nil, fmt.Errorf("선행 태스크가 이미 완료 상태임: %s", strings.Join(ids, ", "))
	}
	return s.SetStatus(owner.ID, domain.StatusBlocked, &domain.BlockInfo{Reason: reason, By: ids})
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

// Any reports whether the input would change anything. The CLI needs it to
// tell "지정한 필드가 없다" from "지정했는데 값이 같다".
func (in EditInput) Any() bool {
	return in.Title != nil || in.Project != nil || in.Priority != nil ||
		in.Scheduled != nil || in.Due != nil || in.Estimate != nil ||
		in.Tags != nil || in.Links != nil || in.Recur != nil
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
		if *in.Recur != "" {
			if _, err := recur.Parse(*in.Recur); err != nil {
				return nil, err
			}
		}
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
	s.recordChange(t.ShortID()+" 수정", fmt.Sprintf("%s %s: %s", t.ID, t.Title, strings.Join(changes, " ")))
	return &Result{Task: t}, nil
}

// AddNote appends a timestamped line to the task's note section.
//
// It is separate from Edit because a note is not a field: it accumulates, and
// an Edit-shaped API would invite replacing the note instead of adding to it.
func (s *Service) AddNote(ref, text string) (*Result, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, fmt.Errorf("메모 내용이 비어 있음")
	}
	t, err := s.Load(ref)
	if err != nil {
		return nil, err
	}
	t.AppendNote(s.now(), text)
	t.Updated = s.Today()
	if err := s.save(t); err != nil {
		return nil, err
	}
	s.recordChange(t.ShortID()+" 메모", fmt.Sprintf("%s %s: 메모 추가", t.ID, t.Title))
	return &Result{Task: t}, nil
}

// Delete removes a task file outright. Cancelling is almost always the better
// move; this exists for captures made by mistake.
func (s *Service) Delete(ref string) error {
	t, err := s.Resolve(ref)
	if err != nil {
		return err
	}
	s.captureUndo(t.Path, t.ID, true)
	if err := s.vault.DeleteTask(t); err != nil {
		return err
	}
	s.idx.Remove(t.ID)
	s.recordChange(t.ShortID()+" 삭제", fmt.Sprintf("%s %s: 삭제", t.ID, t.Title))
	return s.idx.Flush()
}

// SaveTask persists a task the caller already holds (used after external edits).
func (s *Service) SaveTask(t *domain.Task) error { return s.save(t) }

// Refresh re-indexes a task that was changed outside the tool (external editor,
// another terminal). It validates before indexing so a broken hand-edit is
// reported at the point of editing rather than at the next query.
func (s *Service) Refresh(t *domain.Task) error {
	if err := t.Validate(); err != nil {
		return err
	}
	s.idx.Put(t)
	return s.idx.Flush()
}
