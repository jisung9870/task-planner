package config

import (
	"os"
	"path/filepath"
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

func TestAgentsDefaults(t *testing.T) {
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
