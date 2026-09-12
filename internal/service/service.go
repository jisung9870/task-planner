// Package service is the single facade every adapter goes through.
//
// TUI, CLI and (later) the HTTP/MCP servers call this package and nothing
// below it. Keeping that rule is what makes those later adapters cheap: the
// use cases are already written once, here.
package service

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"task-planner/internal/config"
	"task-planner/internal/domain"
	"task-planner/internal/index"
	"task-planner/internal/query"
	"task-planner/internal/store"
)

// Service owns the vault, the index and the clock.
type Service struct {
	Cfg   *config.Config
	vault *store.Vault
	idx   index.Index
	now   func() time.Time

	// changes accumulates this session's mutations for one git commit.
	changes []change

	// undo is the step currently collecting pre-images; undoStack holds the
	// finished ones, newest last.
	undo      *undoStep
	undoStack []*undoStep
}

// Open loads a vault and syncs its index.
func Open(cfg *config.Config) (*Service, error) {
	v := store.New(cfg.Vault)
	if !v.Exists() {
		return nil, fmt.Errorf("vault 가 없음: %s\n  `tp init` 으로 생성하거나 --vault / $%s 로 경로를 지정하세요", cfg.Vault, config.EnvVault)
	}
	idx, err := index.NewJSON(v)
	if err != nil {
		return nil, err
	}
	s := &Service{Cfg: cfg, vault: v, idx: idx, now: time.Now}
	if _, err := s.idx.Sync(); err != nil {
		return nil, err
	}
	return s, nil
}

// Init creates a vault skeleton and returns a service on it.
func Init(cfg *config.Config) (*Service, error) {
	v := store.New(cfg.Vault)
	if err := v.Init(); err != nil {
		return nil, err
	}
	if err := config.Save(cfg); err != nil {
		return nil, err
	}
	return Open(cfg)
}

// Close commits any pending changes and flushes the index. One session (a CLI
// command, or a TUI run) produces one commit.
func (s *Service) Close() error {
	if _, err := s.CommitPending(); err != nil {
		return err
	}
	return s.idx.Flush()
}

// Vault exposes the underlying paths for adapters that need them (editor,
// reports, git).
func (s *Service) Vault() *store.Vault { return s.vault }

// Now is the injectable clock; tests override it.
func (s *Service) Now() time.Time     { return s.now() }
func (s *Service) Today() domain.Date { return domain.DateOf(s.now()) }

// SetClock is used by tests to make date-dependent behaviour deterministic.
func (s *Service) SetClock(f func() time.Time) { s.now = f }

// Sync re-reads changed files. Cheap enough to call on every redraw.
func (s *Service) Sync() (index.Stats, error) { return s.idx.Sync() }

// Rebuild discards the index and re-reads the vault.
func (s *Service) Rebuild() (index.Stats, error) { return s.idx.Rebuild() }

func (s *Service) IndexStats() index.Stats { return s.idx.Stats() }

// All returns every task summary (no body).
func (s *Service) All() []*domain.Task { return s.idx.Tasks() }

// TodayList answers "오늘 뭘 해야 하지".
func (s *Service) TodayList() []*domain.Task { return query.Today(s.All(), s.Today()) }

// WeekList answers "이번 주에 뭐가 남았지".
func (s *Service) WeekList(ref domain.Date) []*domain.Task {
	if ref.IsZero() {
		ref = s.Today()
	}
	return query.Week(s.All(), ref, s.Today())
}

// OpenList returns everything still needing attention.
func (s *Service) OpenList() []*domain.Task { return query.Open(s.All(), s.Today()) }

// ProjectCounts aggregates open work per project.
func (s *Service) ProjectCounts() []query.ProjectCount {
	return query.ProjectCounts(s.All(), s.Today())
}

// ProjectList returns the tasks of one project.
func (s *Service) ProjectList(slug string, includeDone bool) []*domain.Task {
	return query.ByProject(s.All(), slug, s.Today(), includeDone)
}

// Projects reads the project metadata files.
func (s *Service) Projects() ([]*domain.Project, error) { return s.vault.LoadProjects() }

// Load returns the full task including its body, read fresh from disk.
func (s *Service) Load(ref string) (*domain.Task, error) {
	sum, err := s.Resolve(ref)
	if err != nil {
		return nil, err
	}
	return s.vault.LoadTask(sum.Path)
}

// Resolve accepts a full id (T-20260912-0001), a short id (#421 or 421) or a
// case-insensitive title substring, and requires the match to be unique.
func (s *Service) Resolve(ref string) (*domain.Task, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, fmt.Errorf("태스크를 지정하세요")
	}
	all := s.All()
	for _, t := range all {
		if strings.EqualFold(t.ID, ref) {
			return t, nil
		}
	}
	if n, err := strconv.Atoi(strings.TrimPrefix(ref, "#")); err == nil {
		var hits []*domain.Task
		for _, t := range all {
			if seq, ok := seqOf(t.ID); ok && seq == n {
				hits = append(hits, t)
			}
		}
		if len(hits) == 1 {
			return hits[0], nil
		}
		if len(hits) > 1 {
			return nil, ambiguous(ref, hits)
		}
	}
	var hits []*domain.Task
	for _, t := range all {
		if strings.Contains(strings.ToLower(t.Title), strings.ToLower(ref)) {
			hits = append(hits, t)
		}
	}
	switch len(hits) {
	case 1:
		return hits[0], nil
	case 0:
		return nil, fmt.Errorf("일치하는 태스크 없음: %q", ref)
	default:
		return nil, ambiguous(ref, hits)
	}
}

func ambiguous(ref string, hits []*domain.Task) error {
	var b strings.Builder
	fmt.Fprintf(&b, "%q 에 %d개가 일치함:", ref, len(hits))
	for i, t := range hits {
		if i == 6 {
			fmt.Fprintf(&b, "\n  ... 외 %d개", len(hits)-i)
			break
		}
		fmt.Fprintf(&b, "\n  %s  %s", t.ID, t.Title)
	}
	return fmt.Errorf("%s", b.String())
}

func seqOf(id string) (int, bool) {
	i := strings.LastIndexByte(id, '-')
	if i < 0 {
		return 0, false
	}
	n, err := strconv.Atoi(id[i+1:])
	return n, err == nil
}

// save persists a task and refreshes its index entry.
func (s *Service) save(t *domain.Task) error {
	// Both locations are captured before the write: a retitled task moves, and
	// undo has to put the old file back and remove the new one.
	s.captureUndo(t.Path, t.ID, true)
	s.captureUndo(s.vault.TaskPath(t), t.ID, true)
	if err := s.vault.SaveTask(t); err != nil {
		return err
	}
	s.idx.Put(t)
	return s.idx.Flush()
}

// Query runs a filter expression over every task.
func (s *Service) Query(expr string) ([]*domain.Task, error) {
	f, err := query.ParseFilter(expr, s.Today())
	if err != nil {
		return nil, err
	}
	return s.ApplyFilter(f, s.All()), nil
}

// Filter compiles an expression against today's date, for callers that want to
// apply it to a list they already have (the TUI filters the active view).
func (s *Service) Filter(expr string) (*query.Filter, error) {
	return query.ParseFilter(expr, s.Today())
}

// ApplyFilter narrows a list the caller already holds.
//
// A body search cannot run on index summaries, which carry no body. The cheap
// terms run first and only the survivors are read from disk, so `body:에러
// project:infra` reads one project's files rather than the whole vault.
func (s *Service) ApplyFilter(f *query.Filter, ts []*domain.Task) []*domain.Task {
	if f.Empty() {
		return ts
	}
	today := s.Today()
	if f.NeedsBody() {
		var cand []*domain.Task
		for _, t := range ts {
			if f.MatchCheap(t, today, s.Cfg.DueSoonDays) {
				cand = append(cand, t)
			}
		}
		ts = s.hydrate(cand)
	}
	return f.Apply(ts, today, s.Cfg.DueSoonDays)
}

// hydrate re-reads full task files for summaries that lack a body. A file that
// fails to parse is skipped rather than aborting the search: a broken file
// should not make every query fail.
func (s *Service) hydrate(ts []*domain.Task) []*domain.Task {
	out := make([]*domain.Task, 0, len(ts))
	for _, sum := range ts {
		if sum.Body != "" || sum.Path == "" {
			out = append(out, sum)
			continue
		}
		full, err := s.vault.LoadTask(sum.Path)
		if err != nil {
			continue
		}
		out = append(out, full)
	}
	return out
}
