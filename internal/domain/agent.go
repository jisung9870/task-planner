package domain

import (
	"fmt"
	"strings"
	"time"
)

// Agent names who is meant to carry a task out. Empty means nobody was named;
// AgentAuto means any allowed agent may take it. Which names are allowed is
// vault config, not domain knowledge - the domain only checks the shape.
type Agent string

const AgentAuto Agent = "auto"

func ParseAgent(s string) (Agent, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return "", nil
	}
	for i, r := range s {
		ok := r >= 'a' && r <= 'z' || i > 0 && (r >= '0' && r <= '9' || r == '-')
		if !ok {
			return "", fmt.Errorf("agent 이름은 영문 소문자로 시작하는 [a-z0-9-] 이어야 함: %q", s)
		}
	}
	return Agent(s), nil
}

// Tier is how heavy the work is, which the reader maps to a concrete model.
type Tier string

const (
	TierFast     Tier = "fast"
	TierStandard Tier = "standard"
	TierDeep     Tier = "deep"
)

func ParseTier(s string) (Tier, error) {
	switch t := Tier(strings.ToLower(strings.TrimSpace(s))); t {
	case "", TierFast, TierStandard, TierDeep:
		return t, nil
	}
	return "", fmt.Errorf("tier 는 fast|standard|deep 이어야 함: %q", s)
}

// Accepts reports whether an agent may take this task: an unnamed or auto task
// is anyone's, a named one only its agent's.
func (t *Task) Accepts(a Agent) bool {
	return t.Agent == "" || t.Agent == AgentAuto || t.Agent == a
}

// ClaimRef is the claimed_by value for one run of an agent.
func ClaimRef(a Agent, session string) (string, error) {
	session = strings.TrimSpace(session)
	if a == "" || a == AgentAuto {
		return "", fmt.Errorf("가져가는 agent 를 지정하세요 (auto 는 불가)")
	}
	if session == "" || strings.ContainsAny(session, " \t\r\n:") {
		return "", fmt.Errorf("세션 식별자는 공백·콜론 없는 한 단어여야 함: %q", session)
	}
	return string(a) + ":" + session, nil
}

// Claim records that one run has taken the task and starts it. A second
// claimant is refused rather than silently sharing the work - two sessions
// editing the same thing is the failure this exists to prevent. Claiming
// again with the same ref is a no-op success so a resumed session can retry.
func (t *Task) Claim(a Agent, ref string, at time.Time, opts *TransitionOpts) error {
	if !t.IsOpen() {
		return fmt.Errorf("%s: 열린 태스크가 아님 (%s)", t.ID, t.Status.Label())
	}
	if t.Status == StatusBlocked {
		return fmt.Errorf("%s: 보류 중이라 가져갈 수 없음", t.ID)
	}
	if t.ClaimedBy != "" && t.ClaimedBy != ref {
		return fmt.Errorf("%s: 이미 %s 가 가져감", t.ID, t.ClaimedBy)
	}
	if !t.Accepts(a) {
		return fmt.Errorf("%s: %s 로 지정된 일이라 %s 가 가져갈 수 없음", t.ID, t.Agent, a)
	}
	if t.ClaimedBy == ref && t.Status == StatusDoing {
		return nil
	}
	t.ClaimedBy = ref
	t.Executor = ExecutorAgent
	t.Updated = DateOf(at)
	t.AppendLog(at, "claim: %s", ref)
	return t.Transition(StatusDoing, at, opts)
}

// Pickable reports whether a session of agent a may take this task without
// being pointed at it: named for a (or auto), open, not on hold, unclaimed.
// Unnamed tasks are left out on purpose - those are mostly the human's own
// work, and a session sweeping them up would be the wrong default.
func (t *Task) Pickable(a Agent) bool {
	return (t.Agent == a || t.Agent == AgentAuto) &&
		t.IsOpen() && t.Status != StatusBlocked && t.ClaimedBy == ""
}

// ClaimAgent is the agent half of claimed_by, empty when unclaimed.
func (t *Task) ClaimAgent() Agent {
	a, _, _ := strings.Cut(t.ClaimedBy, ":")
	return Agent(a)
}

// RunAgent is the agent that will (or did) run the task: the named one, or the
// claimant when the task left the choice open.
func (t *Task) RunAgent() Agent {
	if t.Agent != "" && t.Agent != AgentAuto {
		return t.Agent
	}
	return t.ClaimAgent()
}

// AgentBadge is the compact "@claude/deep" form list rows use; a trailing *
// marks a claimed task, and an auto task shows its claimant once taken.
func (t *Task) AgentBadge() string {
	b := "@" + string(t.Agent)
	if a := t.ClaimAgent(); a != "" && (t.Agent == "" || t.Agent == AgentAuto) {
		b = "@" + string(a)
	}
	if b == "@" {
		b = "@?"
	}
	if t.Tier != "" {
		b += "/" + string(t.Tier)
	}
	if t.ClaimedBy != "" {
		b += "*"
	}
	return b
}
