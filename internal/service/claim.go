package service

import (
	"fmt"

	"task-planner/internal/domain"
)

// ClaimInput names the run taking a task. Ref empty means "the most urgent
// task this agent may pick".
type ClaimInput struct {
	Ref     string
	Agent   domain.Agent
	Session string
}

// Claim lets one run take a task. It is the only write guarded across
// processes: under the vault lock the index is re-synced and the file re-read,
// so two sessions racing for the same task see each other's claim.
func (s *Service) Claim(in ClaimInput) (*Result, error) {
	if in.Agent == "" || in.Agent == domain.AgentAuto || !s.Cfg.Agents.Allows(in.Agent) {
		return nil, fmt.Errorf("허용되지 않은 agent: %q (허용: %s)", in.Agent, s.Cfg.Agents.Describe())
	}
	ref, err := domain.ClaimRef(in.Agent, in.Session)
	if err != nil {
		return nil, err
	}
	unlock, err := s.vault.Lock("claim")
	if err != nil {
		return nil, fmt.Errorf("vault 잠금 실패: %w", err)
	}
	defer unlock()
	if _, err := s.idx.Sync(); err != nil {
		return nil, err
	}
	target := in.Ref
	if target == "" {
		if target = s.pick(in.Agent); target == "" {
			return nil, fmt.Errorf("%s 가 가져갈 일이 없음 (agent:%s 또는 auto 로 지정된 열린 일)", in.Agent, in.Agent)
		}
	}
	t, err := s.Load(target)
	if err != nil {
		return nil, err
	}
	if err := t.Claim(in.Agent, ref, s.now(), &domain.TransitionOpts{SessionCap: s.Cfg.SessionCap}); err != nil {
		return nil, err
	}
	if err := s.save(t); err != nil {
		return nil, err
	}
	s.recordChange(t.ShortID()+" 가져감", fmt.Sprintf("%s %s: claim %s", t.ID, t.Title, ref))
	res := &Result{Task: t}
	if w := s.wipWarning(t.ID); w != "" {
		res.Warnings = append(res.Warnings, w)
	}
	return res, nil
}

// pick returns the id of the task NextUp ranks first among those the agent
// may take, so an unpointed claim follows the same urgency order a human sees.
func (s *Service) pick(a domain.Agent) string {
	for _, sg := range s.NextUp(0) {
		if sg.Task.Pickable(a) {
			return sg.Task.ID
		}
	}
	return ""
}

// Release gives a claimed task back. Only the claimant may, unless forced - a
// session that died leaves its claim behind, and clearing that is a human
// decision.
func (s *Service) Release(ref, by string, force bool) (*Result, error) {
	unlock, err := s.vault.Lock("claim")
	if err != nil {
		return nil, fmt.Errorf("vault 잠금 실패: %w", err)
	}
	defer unlock()
	if _, err := s.idx.Sync(); err != nil {
		return nil, err
	}
	t, err := s.Load(ref)
	if err != nil {
		return nil, err
	}
	if t.ClaimedBy == "" {
		return nil, fmt.Errorf("%s: 가져간 실행이 없음", t.ID)
	}
	if !t.IsOpen() {
		return nil, fmt.Errorf("%s: 이미 %s 상태라 기록으로 남김", t.ID, t.Status.Label())
	}
	if t.ClaimedBy != by && !force {
		return nil, fmt.Errorf("%s: %s 가 가져간 일이라 놓을 수 없음 (force 로 강제)", t.ID, t.ClaimedBy)
	}
	prev := t.ClaimedBy
	now := s.now()
	if t.Status == domain.StatusDoing {
		if err := t.Transition(domain.StatusTodo, now, &domain.TransitionOpts{SessionCap: s.Cfg.SessionCap}); err != nil {
			return nil, err
		}
	} else {
		t.ClaimedBy = ""
		t.Updated = domain.DateOf(now)
	}
	t.AppendLog(now, "release: %s", prev)
	if err := s.save(t); err != nil {
		return nil, err
	}
	s.recordChange(t.ShortID()+" 놓음", fmt.Sprintf("%s %s: release %s", t.ID, t.Title, prev))
	return &Result{Task: t}, nil
}

// ModelFor resolves the concrete model for a task from the vault's table;
// empty while the agent or tier is still open.
func (s *Service) ModelFor(t *domain.Task) string {
	return s.Cfg.Agents.Model(t.RunAgent(), t.Tier)
}
