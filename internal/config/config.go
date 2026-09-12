// Package config resolves the vault location and user preferences.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/goccy/go-yaml"

	"task-planner/internal/domain"
)

// EnvVault overrides the vault path for a single invocation.
const EnvVault = "TP_VAULT"

// DefaultVaultDir is used when nothing else says otherwise. It is deliberately
// separate from the journal tree: journal holds "왜/경과", the vault holds
// "지금 할 일".
const DefaultVaultDir = "tasks"

// Config is read from <vault>/config.yaml. Every field has a working default so
// a vault with no config file behaves sensibly.
type Config struct {
	Vault string `yaml:"-"`

	Editor string `yaml:"editor,omitempty"` // falls back to $VISUAL, $EDITOR, vi

	// WIPLimit warns (never blocks) when too many tasks are in progress.
	WIPLimit int `yaml:"wip_limit,omitempty"`

	// AutoRollover moves yesterday's unfinished work onto today at startup.
	AutoRollover bool `yaml:"auto_rollover"`
	// RolloverWarnAt is the carry count that marks a task as badly scoped.
	RolloverWarnAt int `yaml:"rollover_warn_at,omitempty"`

	// DueSoonDays controls the deadline warning window in list views.
	DueSoonDays int `yaml:"due_soon_days,omitempty"`

	// DailyCapacity is how much work a day is expected to hold. It drives the
	// over-commitment warning; it is a planning number, not a time sheet.
	DailyCapacity domain.Duration `yaml:"daily_capacity,omitempty"`

	// SessionCap bounds one 진행중 session when computing actual time. A task
	// left running overnight would otherwise record the whole night.
	SessionCap domain.Duration `yaml:"session_cap,omitempty"`

	Git GitConfig `yaml:"git"`
}

type GitConfig struct {
	AutoCommit bool   `yaml:"auto_commit"`
	AutoPush   bool   `yaml:"auto_push"`
	Remote     string `yaml:"remote,omitempty"`
	Branch     string `yaml:"branch,omitempty"`
}

// Default returns the baseline config for a vault path.
func Default(vault string) *Config {
	return &Config{
		Vault:          vault,
		WIPLimit:       3,
		AutoRollover:   true,
		RolloverWarnAt: 3,
		DueSoonDays:    3,
		DailyCapacity:  domain.Duration(6 * time.Hour),
		SessionCap:     domain.Duration(8 * time.Hour),
		Git:            GitConfig{Remote: "origin"},
	}
}

// ResolveVault picks the vault directory: explicit flag, then $TP_VAULT, then
// ~/tasks.
func ResolveVault(flagVault string) (string, error) {
	if v := strings.TrimSpace(flagVault); v != "" {
		return expand(v)
	}
	if v := strings.TrimSpace(os.Getenv(EnvVault)); v != "" {
		return expand(v)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("홈 디렉토리를 찾을 수 없음: %w", err)
	}
	return filepath.Join(home, DefaultVaultDir), nil
}

// Load reads <vault>/config.yaml, filling in defaults for absent keys.
func Load(vault string) (*Config, error) {
	cfg := Default(vault)
	path := filepath.Join(vault, "config.yaml")
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return cfg, nil
	}
	if err != nil {
		return nil, fmt.Errorf("%s 읽기 실패: %w", path, err)
	}
	if err := yaml.Unmarshal(raw, cfg); err != nil {
		return nil, fmt.Errorf("%s 파싱 실패: %w", path, err)
	}
	cfg.Vault = vault
	if cfg.WIPLimit < 0 {
		cfg.WIPLimit = 0
	}
	if cfg.RolloverWarnAt <= 0 {
		cfg.RolloverWarnAt = 3
	}
	if cfg.DueSoonDays <= 0 {
		cfg.DueSoonDays = 3
	}
	if cfg.SessionCap < 0 {
		cfg.SessionCap = 0
	}
	if cfg.DailyCapacity < 0 {
		cfg.DailyCapacity = 0
	}
	return cfg, nil
}

// Save writes the config back, used by `tp init`.
func Save(cfg *Config) error {
	raw, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	header := "# task-planner vault 설정. 상세: docs/product-task-planner-202609.md\n"
	return os.WriteFile(filepath.Join(cfg.Vault, "config.yaml"), append([]byte(header), raw...), 0o644)
}

// EditorCommand returns the editor to shell out to.
func (c *Config) EditorCommand() string {
	for _, v := range []string{c.Editor, os.Getenv("VISUAL"), os.Getenv("EDITOR")} {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return "vi"
}

func expand(p string) (string, error) {
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		p = filepath.Join(home, strings.TrimPrefix(p, "~"))
	}
	return filepath.Abs(p)
}
