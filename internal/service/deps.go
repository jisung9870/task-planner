package service

import (
	"fmt"
	"strings"

	"task-planner/internal/domain"
)

// Unblocked describes a task released because its blockers finished.
type Unblocked struct {
	Task *domain.Task
	// By is the task whose completion released it.
	By string
}

// resolveRefs turns user-typed blocker references (#3, a title fragment) into
// canonical ids, rejecting self-reference and cycles. Storing raw input would
// make the dependency silently never match.
func (s *Service) resolveRefs(owner string, refs []string) ([]string, error) {
	out := make([]string, 0, len(refs))
	seen := map[string]bool{}
	for _, ref := range refs {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}
		dep, err := s.Resolve(ref)
		if err != nil {
			return nil, fmt.Errorf("선행 태스크 %q: %w", ref, err)
		}
		if dep.ID == owner {
			return nil, fmt.Errorf("자기 자신을 선행 태스크로 지정할 수 없음: %s", dep.ID)
		}
		if s.dependsOn(dep.ID, owner, map[string]bool{}) {
			return nil, fmt.Errorf("순환 의존: %s 가 이미 %s 에 막혀 있음", dep.ID, owner)
		}
		if !seen[dep.ID] {
			seen[dep.ID] = true
			out = append(out, dep.ID)
		}
	}
	return out, nil
}

// dependsOn reports whether from (transitively) waits on target.
func (s *Service) dependsOn(from, target string, seen map[string]bool) bool {
	if from == target {
		return true
	}
	if seen[from] {
		return false
	}
	seen[from] = true
	t, ok := s.idx.Get(from)
	if !ok {
		return false
	}
	for _, dep := range t.BlockedBy {
		if s.dependsOn(dep, target, seen) {
			return true
		}
	}
	return false
}

// Blockers returns the tasks a task is waiting on, in declaration order.
// Unknown ids are returned as nil entries' ids via the second result so the UI
// can still show something useful.
func (s *Service) Blockers(t *domain.Task) (known []*domain.Task, missing []string) {
	for _, id := range t.BlockedBy {
		if dep, err := s.Resolve(id); err == nil {
			known = append(known, dep)
		} else {
			missing = append(missing, id)
		}
	}
	return known, missing
}

// Blocking returns the open tasks that are waiting on t.
func (s *Service) Blocking(id string) []*domain.Task {
	var out []*domain.Task
	for _, t := range s.All() {
		if !t.IsOpen() {
			continue
		}
		for _, dep := range t.BlockedBy {
			if dep == id {
				out = append(out, t)
				break
			}
		}
	}
	domain.SortDefault(out, s.Today())
	return out
}

// releaseDependents unblocks every task whose blockers have all finished.
//
// Without this the dependency is only a note: the waiting task stays 보류 until
// someone remembers to look at it, which is exactly the failure the hold rule
// was meant to prevent.
func (s *Service) releaseDependents(finished *domain.Task) ([]Unblocked, error) {
	if !finished.Status.Terminal() {
		return nil, nil
	}
	var out []Unblocked
	for _, sum := range s.Blocking(finished.ID) {
		if sum.Status != domain.StatusBlocked {
			continue
		}
		if !s.allBlockersDone(sum.BlockedBy) {
			continue
		}
		t, err := s.Load(sum.ID)
		if err != nil {
			return out, err
		}
		if err := t.Transition(domain.StatusTodo, s.now(), &domain.TransitionOpts{SessionCap: s.Cfg.SessionCap}); err != nil {
			return out, err
		}
		t.AppendLog(s.now(), "선행 %s 완료로 보류 해제", finished.ID)
		if err := s.save(t); err != nil {
			return out, err
		}
		s.recordChange(t.ShortID()+" 보류해제",
			fmt.Sprintf("%s %s: 선행 %s 완료로 해제", t.ID, t.Title, finished.ID))
		out = append(out, Unblocked{Task: t, By: finished.ID})
	}
	return out, nil
}

func (s *Service) allBlockersDone(ids []string) bool {
	for _, id := range ids {
		dep, ok := s.idx.Get(id)
		if !ok {
			continue // an unknown blocker cannot hold the task hostage forever
		}
		if !dep.Status.Terminal() {
			return false
		}
	}
	return true
}
