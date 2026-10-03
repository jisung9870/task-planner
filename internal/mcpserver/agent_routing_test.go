package mcpserver

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"task-planner/internal/config"
	"task-planner/internal/service"
)

func TestClaimFlowOverMCP(t *testing.T) {
	cs, _ := newTestSession(t)
	info := call(t, cs, "vault_info", nil)
	if info["session"] == "" || info["models"].(map[string]any)["claude"].(map[string]any)["deep"] != "opus" {
		t.Fatalf("vault_info = %v", info)
	}

	added := call(t, cs, "task_add", map[string]any{"title": "인덱스 구조 검토", "agent": "claude", "tier": "deep"})
	task := added["task"].(map[string]any)
	if task["executor"] != "agent" || task["agent"] != "claude" || task["tier"] != "deep" {
		t.Fatalf("added = %v", task)
	}
	if msg := callErr(t, cs, "task_add", map[string]any{"title": "x", "agent": "gemini"}); !strings.Contains(msg, "허용되지 않은") {
		t.Fatalf("disallowed agent: %s", msg)
	}

	claimed := call(t, cs, "task_claim", map[string]any{"agent": "claude"})
	if claimed["model"] != "opus" {
		t.Fatalf("claim = %v", claimed)
	}
	ct := claimed["task"].(map[string]any)
	if ct["status"] != "doing" || !strings.HasPrefix(ct["claimed_by"].(string), "claude:mcp-") {
		t.Fatalf("claimed task = %v", ct)
	}
	if msg := callErr(t, cs, "task_claim", map[string]any{"ref": ct["id"], "agent": "claude", "session": "other"}); !strings.Contains(msg, "이미") {
		t.Fatalf("second claim: %s", msg)
	}
	if msg := callErr(t, cs, "task_release", map[string]any{"ref": ct["id"], "agent": "claude", "session": "other"}); !strings.Contains(msg, "놓을 수 없음") {
		t.Fatalf("foreign release: %s", msg)
	}
	released := call(t, cs, "task_release", map[string]any{"ref": ct["id"], "agent": "claude"})
	if rt := released["task"].(map[string]any); rt["status"] != "todo" || rt["claimed_by"] != nil {
		t.Fatalf("released = %v", rt)
	}
	got := call(t, cs, "task_query", map[string]any{"query": "pick:claude"})
	if n := len(got["tasks"].([]any)); n != 1 {
		t.Fatalf("pick:claude after release = %d", n)
	}
}

// A Claude Code client that does not pass agent is still identified - the
// client name is the one signal the session cannot misstate by accident.
func TestClaimInfersAgentFromClientName(t *testing.T) {
	svc, err := service.Init(config.Default(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	svc.SetClock(func() time.Time { return time.Date(2026, 9, 12, 9, 0, 0, 0, time.Local) })
	ct, st := mcp.NewInMemoryTransports()
	ctx := context.Background()
	ss, err := New(svc, "test").Connect(ctx, st)
	if err != nil {
		t.Fatal(err)
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "claude-code", Version: "0"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close(); ss.Wait(); svc.Close() })

	call(t, cs, "task_add", map[string]any{"title": "codex 일", "agent": "codex"})
	if msg := callErr(t, cs, "task_claim", map[string]any{}); !strings.Contains(msg, "claude 가 가져갈 일이 없음") {
		t.Fatalf("claude took codex work: %s", msg)
	}
}
