package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"task-planner/internal/domain"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// installed fakes PATH detection so tests do not depend on the machine.
func installed(t *testing.T, names ...string) {
	t.Helper()
	t.Setenv(EnvAgents, "")
	orig := lookPath
	t.Cleanup(func() { lookPath = orig })
	lookPath = func(file string) (string, error) {
		for _, n := range names {
			if n == file {
				return "/usr/bin/" + file, nil
			}
		}
		return "", errors.New("not found")
	}
}

func TestAgentsDefaults(t *testing.T) {
	installed(t)
	cfg, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Agents.Allows("claude") || !cfg.Agents.Allows("codex") || cfg.Agents.Allows("gemini") {
		t.Fatalf("allowed = %v", cfg.Agents.Allowed)
	}
	if m := cfg.Agents.Model("claude", domain.TierDeep); m != "opus" {
		t.Fatalf("claude deep = %q", m)
	}
	if m := cfg.Agents.Model(domain.AgentAuto, domain.TierDeep); m != "" {
		t.Fatalf("auto resolved to %q", m)
	}
}

func TestAgentsPartialOverrideKeepsDefaults(t *testing.T) {
	installed(t, "claude", "codex")
	dir := writeConfig(t, "agents:\n  allowed: [claude]\n  models:\n    claude:\n      deep: opus-max\n")
	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Agents.Allowed) != 1 || cfg.Agents.Allows("codex") {
		t.Fatalf("allowed = %v", cfg.Agents.Allowed)
	}
	if m := cfg.Agents.Model("claude", domain.TierDeep); m != "opus-max" {
		t.Fatalf("override lost: %q", m)
	}
	if m := cfg.Agents.Model("claude", domain.TierFast); m != "haiku" {
		t.Fatalf("default dropped: %q", m)
	}
}

func TestAgentsRejectsBadTable(t *testing.T) {
	installed(t)
	for _, body := range []string{
		"agents:\n  allowed: [claude, claude]\n",
		"agents:\n  allowed: [auto]\n",
		"agents:\n  models:\n    claude:\n      huge: x\n",
	} {
		if _, err := Load(writeConfig(t, body)); err == nil {
			t.Errorf("accepted %q", body)
		}
	}
}

func TestAgentsSourceOrder(t *testing.T) {
	installed(t, "codex")
	cfg, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Agents.Source != AgentsDetected || cfg.Agents.Allows("claude") || !cfg.Agents.Allows("codex") {
		t.Fatalf("detected = %v (%s)", cfg.Agents.Allowed, cfg.Agents.Source)
	}

	dir := writeConfig(t, "agents:\n  allowed: [claude]\n")
	cfg, err = Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Agents.Source != AgentsFromConfig || !cfg.Agents.Allows("claude") || cfg.Agents.Allows("codex") {
		t.Fatalf("config = %v (%s)", cfg.Agents.Allowed, cfg.Agents.Source)
	}

	t.Setenv(EnvAgents, " codex , ")
	cfg, err = Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Agents.Source != AgentsFromEnv || cfg.Agents.Allows("claude") || !cfg.Agents.Allows("codex") {
		t.Fatalf("env = %v (%s)", cfg.Agents.Allowed, cfg.Agents.Source)
	}
	if d := cfg.Agents.Describe(); d != "codex ($TP_AGENTS)" {
		t.Fatalf("Describe = %q", d)
	}

	t.Setenv(EnvAgents, "codex,auto")
	if _, err := Load(dir); err == nil || !strings.Contains(err.Error(), EnvAgents) {
		t.Fatalf("auto in env: %v", err)
	}
}

// Init must not pin one machine's detection into a vault other machines share.
func TestSaveDoesNotPinDetectedAgents(t *testing.T) {
	installed(t, "claude")
	dir := t.TempDir()
	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := Save(cfg); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "config.yaml"))
	if strings.Contains(string(raw), "allowed") {
		t.Fatalf("detected list written:\n%s", raw)
	}

	cfg.Agents.Allowed, cfg.Agents.Source = []domain.Agent{"codex"}, AgentsFromConfig
	if err := Save(cfg); err != nil {
		t.Fatal(err)
	}
	raw, _ = os.ReadFile(filepath.Join(dir, "config.yaml"))
	if !strings.Contains(string(raw), "allowed") {
		t.Fatalf("declared list dropped:\n%s", raw)
	}
}
