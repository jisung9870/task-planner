// Package gitsync commits the vault to git.
//
// A vault that is a git repo gets history, backup and cross-machine sync for
// free, which is why the planning doc lists conflict merging as a non-goal:
// git already solves it better than a bespoke sync layer would.
package gitsync

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Available reports whether git is on PATH.
func Available() bool {
	_, err := exec.LookPath("git")
	return err == nil
}

// IsRepo reports whether dir is inside a git work tree.
func IsRepo(dir string) bool {
	out, err := run(dir, "rev-parse", "--is-inside-work-tree")
	return err == nil && strings.TrimSpace(out) == "true"
}

// Init creates a repository in dir and adds the ignore rules the vault needs.
func Init(dir string) error {
	if !Available() {
		return fmt.Errorf("git 이 설치되어 있지 않음")
	}
	if IsRepo(dir) {
		return EnsureIgnore(dir)
	}
	if _, err := run(dir, "init"); err != nil {
		return err
	}
	return EnsureIgnore(dir)
}

// EnsureIgnore makes sure the derived index is never committed. Tracking it
// would produce a conflict on every pull for a file that is regenerated
// from the markdown anyway.
func EnsureIgnore(dir string) error {
	path := filepath.Join(dir, ".gitignore")
	raw, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.TrimSpace(line) == ".index/" {
			return nil
		}
	}
	body := string(raw)
	if body != "" && !strings.HasSuffix(body, "\n") {
		body += "\n"
	}
	body += "# 인덱스는 markdown 에서 재생성되는 파생물\n.index/\n"
	return os.WriteFile(path, []byte(body), 0o644)
}

// CommitAll stages everything and commits. It reports false when the work tree
// was already clean, which is the common case and not an error.
func CommitAll(dir, message string) (bool, error) {
	if _, err := run(dir, "add", "-A"); err != nil {
		return false, err
	}
	if out, err := run(dir, "status", "--porcelain"); err != nil {
		return false, err
	} else if strings.TrimSpace(out) == "" {
		return false, nil
	}
	if _, err := run(dir, "commit", "-m", message); err != nil {
		return false, err
	}
	return true, nil
}

// Push sends the current branch to the remote.
func Push(dir, remote, branch string) error {
	if remote == "" {
		remote = "origin"
	}
	if branch == "" {
		out, err := run(dir, "rev-parse", "--abbrev-ref", "HEAD")
		if err != nil {
			return err
		}
		branch = strings.TrimSpace(out)
	}
	_, err := run(dir, "push", remote, branch)
	return err
}

func run(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = strings.TrimSpace(stdout.String())
		}
		return stdout.String(), fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
	}
	return stdout.String(), nil
}
