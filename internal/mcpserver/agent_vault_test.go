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

func TestSharedVaultAgentPlanWorkflow(t *testing.T) {
	base := t.TempDir()
	cfg := config.Default(base)
	svc, err := service.Init(cfg)
	if err != nil {
		t.Fatal(err)
	}
	svc.SetClock(func() time.Time { return time.Date(2026, 9, 26, 9, 0, 0, 0, time.Local) })
	srv := New(svc, "test")
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	ctx := context.Background()
	serverSession, err := srv.Connect(ctx, serverTransport)
	if err != nil {
		t.Fatal(err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0"}, nil)
	cs, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cs.Close()
		serverSession.Wait()
		svc.Close()
	})

	info := call(t, cs, "vault_info", nil)
	if info["mode"] != "shared" || info["path"] != cfg.Vault {
		t.Fatalf("vault_info = %v", info)
	}
	plan := "목표: 작업 이력 기록\n1. 경로 확인\n2. 스킬 작성\n3. 검증\n완료 기준: 기록 조회"
	if msg := callErr(t, cs, "task_add", map[string]any{"title": "사람 기록 검증", "note": plan}); !strings.Contains(msg, "계획") {
		t.Fatalf("human note gate = %q", msg)
	}
	added := call(t, cs, "task_add", map[string]any{"title": "에이전트 기록 검증", "executor": "agent", "project": "shared", "note": plan, "start": true})
	id := added["task"].(map[string]any)["id"].(string)
	if added["task"].(map[string]any)["executor"] != "agent" {
		t.Fatalf("executor = %v", added)
	}
	human := call(t, cs, "task_add", map[string]any{"title": "사람 검증", "project": "shared"})
	if human["task"].(map[string]any)["executor"] != "human" {
		t.Fatalf("human executor = %v", human)
	}
	rows := call(t, cs, "project_status", nil)["rows"].([]any)
	if len(rows) != 1 || rows[0].(map[string]any)["open"] != float64(2) {
		t.Fatalf("shared project = %v", rows)
	}
	filtered := call(t, cs, "task_query", map[string]any{"query": "executor:agent project:shared"})["tasks"].([]any)
	if len(filtered) != 1 {
		t.Fatalf("agent query = %v", filtered)
	}
	call(t, cs, "task_note", map[string]any{"ref": id, "text": "2. 스킬 작성 완료, 검증 시작"})
	call(t, cs, "task_status", map[string]any{"ref": id, "status": "done"})
	got := call(t, cs, "task_get", map[string]any{"ref": id})
	if got["task"].(map[string]any)["status"] != "done" {
		t.Fatalf("task_get = %v", got)
	}
	note := got["note"].(string)
	if !strings.Contains(note, plan) || !strings.Contains(note, "스킬 작성 완료") {
		t.Fatalf("note = %q", note)
	}
}
