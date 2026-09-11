package service

import (
	"fmt"
	"strings"

	"task-planner/internal/gitsync"
)

// change is one recorded mutation, used to build a commit message.
type change struct {
	short string // "#12 완료"
	full  string // "T-20260912-0012 배포 스크립트: doing → done"
}

// recordChange notes a mutation for the next commit. Committing per keystroke
// would bury the history in noise, so changes accumulate and are written once
// per session (CLI command or TUI run).
func (s *Service) recordChange(short, full string) {
	s.changes = append(s.changes, change{short: short, full: full})
}

// GitEnabled reports whether auto-commit can run for this vault.
func (s *Service) GitEnabled() bool {
	return s.Cfg.Git.AutoCommit && gitsync.Available() && gitsync.IsRepo(s.Cfg.Vault)
}

// CommitPending commits everything changed in this session. It is a no-op when
// auto-commit is off, the vault is not a repo, or nothing changed.
func (s *Service) CommitPending() (bool, error) {
	if len(s.changes) == 0 || !s.GitEnabled() {
		s.changes = nil
		return false, nil
	}
	msg := s.commitMessage()
	s.changes = nil
	committed, err := gitsync.CommitAll(s.Cfg.Vault, msg)
	if err != nil || !committed {
		return committed, err
	}
	if s.Cfg.Git.AutoPush {
		if err := gitsync.Push(s.Cfg.Vault, s.Cfg.Git.Remote, s.Cfg.Git.Branch); err != nil {
			// A failed push must not look like a failed commit: the work is
			// safely recorded locally either way.
			return true, fmt.Errorf("커밋은 됐으나 push 실패: %w", err)
		}
	}
	return true, nil
}

// commitMessage renders "tp: #1 완료, #5 보류" plus a body listing every change.
func (s *Service) commitMessage() string {
	shorts := make([]string, 0, len(s.changes))
	for _, c := range s.changes {
		shorts = append(shorts, c.short)
	}
	subject := strings.Join(shorts, ", ")
	if len(shorts) > 3 {
		subject = fmt.Sprintf("%s 외 %d건", strings.Join(shorts[:3], ", "), len(shorts)-3)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "tp: %s\n", subject)
	if len(s.changes) > 1 {
		b.WriteString("\n")
		for _, c := range s.changes {
			fmt.Fprintf(&b, "- %s\n", c.full)
		}
	}
	return b.String()
}

// GitInit turns the vault into a repository and adds the ignore rules.
func (s *Service) GitInit() error { return gitsync.Init(s.Cfg.Vault) }

// GitCommitNow commits regardless of the auto_commit setting, for `tp git sync`.
func (s *Service) GitCommitNow(message string) (bool, error) {
	if !gitsync.Available() {
		return false, fmt.Errorf("git 이 설치되어 있지 않음")
	}
	if !gitsync.IsRepo(s.Cfg.Vault) {
		return false, fmt.Errorf("vault 가 git 저장소가 아님: %s (tp git init)", s.Cfg.Vault)
	}
	if message == "" {
		if len(s.changes) > 0 {
			message = s.commitMessage()
		} else {
			message = "tp: vault 동기화"
		}
	}
	s.changes = nil
	committed, err := gitsync.CommitAll(s.Cfg.Vault, message)
	if err != nil || !committed {
		return committed, err
	}
	if s.Cfg.Git.AutoPush {
		if err := gitsync.Push(s.Cfg.Vault, s.Cfg.Git.Remote, s.Cfg.Git.Branch); err != nil {
			return true, fmt.Errorf("커밋은 됐으나 push 실패: %w", err)
		}
	}
	return true, nil
}
