// Package config resolves the vault location and user preferences.
package config

import (
	"fmt"
	"os"
	"os/exec"
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

	// Views are saved filter expressions the TUI binds to its view picker (v).
	// They live in config rather than in a dotfile because they are part of how
	// this vault is worked, and a vault is the thing people sync between
	// machines.
	Views []View `yaml:"views,omitempty"`

	Git GitConfig `yaml:"git"`

	// Agents is the one table both a plain session and 나루 read to decide who
	// may take a task and which model a tier means.
	Agents AgentsConfig `yaml:"agents"`
}

// AgentsConfig lists the agents a task may name and the model each tier maps
// to per agent. The tool never runs a model; it hands the name back.
type AgentsConfig struct {
	Allowed []domain.Agent                          `yaml:"allowed,omitempty"`
	Models  map[domain.Agent]map[domain.Tier]string `yaml:"models,omitempty"`

	// Source says where Allowed came from. A person who uses only one agent,
	// or a machine that has only one installed, must be able to tell why a
	// name was refused - see resolveAllowed for the order.
	Source AgentsSource `yaml:"-"`
}

// EnvAgents narrows the allowed agents for one machine, over the vault file:
// the vault is shared between machines through git, the installed CLIs are not.
const EnvAgents = "TP_AGENTS"

type AgentsSource string

const (
	AgentsFromEnv    AgentsSource = "env"
	AgentsFromConfig AgentsSource = "config"
	AgentsDetected   AgentsSource = "detected"
	AgentsDefault    AgentsSource = "default"
)

// Label is the source as a user-facing phrase for error messages.
func (s AgentsSource) Label() string {
	switch s {
	case AgentsFromEnv:
		return "$" + EnvAgents
	case AgentsFromConfig:
		return "config.yaml agents.allowed"
	case AgentsDetected:
		return "PATH 에서 감지"
	}
	return "기본값"
}

// Describe is "claude, codex (PATH 에서 감지)" for refusals.
func (a AgentsConfig) Describe() string {
	names := make([]string, len(a.Allowed))
	for i, n := range a.Allowed {
		names[i] = string(n)
	}
	list := strings.Join(names, ", ")
	if list == "" {
		list = "없음"
	}
	return list + " (" + a.Source.Label() + ")"
}

// lookPath is swapped in tests so detection does not depend on the machine.
var lookPath = exec.LookPath

// resolveAllowed picks the allowed list: the machine's TP_AGENTS, then the
// vault file, then the agent CLIs found on PATH, then both. Detection only
// sees installation, not login - an exact answer belongs in one of the first
// two.
func (a *AgentsConfig) resolveAllowed() error {
	if v := strings.TrimSpace(os.Getenv(EnvAgents)); v != "" {
		a.Allowed = nil
		for _, part := range strings.Split(v, ",") {
			if part = strings.TrimSpace(part); part != "" {
				a.Allowed = append(a.Allowed, domain.Agent(part))
			}
		}
		a.Source = AgentsFromEnv
		return nil
	}
	if a.Allowed != nil {
		a.Source = AgentsFromConfig
		return nil
	}
	for _, name := range defaultAgents().Allowed {
		if _, err := lookPath(string(name)); err == nil {
			a.Allowed = append(a.Allowed, name)
		}
	}
	if len(a.Allowed) > 0 {
		a.Source = AgentsDetected
		return nil
	}
	a.Allowed = defaultAgents().Allowed
	a.Source = AgentsDefault
	return nil
}

// Allows reports whether a task may name this agent. auto is always allowed:
// it means "any of the allowed ones".
func (a AgentsConfig) Allows(name domain.Agent) bool {
	if name == "" || name == domain.AgentAuto {
		return true
	}
	for _, x := range a.Allowed {
		if x == name {
			return true
		}
	}
	return false
}

// Model resolves the concrete model for an agent and tier; empty when either
// is open or the table has no entry.
func (a AgentsConfig) Model(name domain.Agent, tier domain.Tier) string {
	if name == "" || name == domain.AgentAuto || tier == "" {
		return ""
	}
	return a.Models[name][tier]
}

func defaultAgents() AgentsConfig {
	return AgentsConfig{
		Source:  AgentsDefault,
		Allowed: []domain.Agent{"claude", "codex"},
		Models: map[domain.Agent]map[domain.Tier]string{
			"claude": {domain.TierFast: "haiku", domain.TierStandard: "sonnet", domain.TierDeep: "opus"},
			"codex":  {domain.TierFast: "gpt-6-luna", domain.TierStandard: "gpt-6-sol", domain.TierDeep: "gpt-6-astra"},
		},
	}
}

// normalize validates the agents table. A config that lists the same agent
// twice or a bad tier would make the 허용 check silently disagree with what
// the file says, so it is a load error.
func (a *AgentsConfig) normalize() error {
	def := defaultAgents()
	if err := a.resolveAllowed(); err != nil {
		return err
	}
	where := a.Source.Label()
	seen := map[domain.Agent]bool{}
	for i, name := range a.Allowed {
		n, err := domain.ParseAgent(string(name))
		if err != nil {
			return fmt.Errorf("%s: %w", where, err)
		}
		if n == "" || n == domain.AgentAuto {
			return fmt.Errorf("%s 에 %q 는 넣을 수 없음", where, name)
		}
		if seen[n] {
			return fmt.Errorf("%s 중복: %s", where, n)
		}
		seen[n] = true
		a.Allowed[i] = n
	}
	if a.Models == nil {
		a.Models = map[domain.Agent]map[domain.Tier]string{}
	}
	for name, tiers := range a.Models {
		for tier := range tiers {
			if _, err := domain.ParseTier(string(tier)); err != nil || tier == "" {
				return fmt.Errorf("agents.models.%s: %w", name, err)
			}
		}
	}
	// A partial table keeps the defaults for tiers it does not mention, so
	// overriding one model does not erase the rest.
	for name, tiers := range def.Models {
		if a.Models[name] == nil {
			a.Models[name] = map[domain.Tier]string{}
		}
		for tier, model := range tiers {
			if _, ok := a.Models[name][tier]; !ok {
				a.Models[name][tier] = model
			}
		}
	}
	return nil
}

// View is one saved query. A list rather than a map: the picker numbers them,
// and a map would renumber itself on every save.
type View struct {
	Name  string `yaml:"name"`
	Query string `yaml:"query"`
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
		// Seeds, not policy: they are the questions this tool was built to
		// answer, and they are editable like any other config key.
		Views: []View{
			{Name: "진행중", Query: "status:doing"},
			{Name: "마감 임박", Query: "is:duesoon"},
			{Name: "마감 초과", Query: "is:overdue"},
			{Name: "반복 이월", Query: "is:carried rollover>2"},
			{Name: "보류", Query: "is:blocked"},
			{Name: "날짜 없음", Query: "is:open is:unscheduled"},
		},
		Git:    GitConfig{Remote: "origin"},
		Agents: defaultAgents(),
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
	// Allowed starts empty so the file's own list is distinguishable from
	// "not set" - the latter falls through to machine detection.
	cfg.Agents.Allowed = nil
	path := filepath.Join(vault, "config.yaml")
	raw, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("%s 읽기 실패: %w", path, err)
	}
	if err == nil {
		if err := yaml.Unmarshal(raw, cfg); err != nil {
			return nil, fmt.Errorf("%s 파싱 실패: %w", path, err)
		}
	}
	cfg.Vault = vault
	if err := cfg.Agents.normalize(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
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
	// Only a list the file itself declared is written back. Pinning a detected
	// or default list into a vault that git carries to other machines would
	// turn one machine's installation into everyone's policy.
	out := *cfg
	if out.Agents.Source != AgentsFromConfig {
		out.Agents.Allowed = nil
	}
	raw, err := yaml.Marshal(&out)
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
