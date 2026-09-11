package mcpserver

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"task-planner/internal/config"
	"task-planner/internal/service"
)

// newTestSession wires a client to the server over in-memory transports, so
// tests exercise the real protocol path (schema validation included).
func newTestSession(t *testing.T) (*mcp.ClientSession, *service.Service) {
	t.Helper()
	cfg := config.Default(t.TempDir())
	svc, err := service.Init(cfg)
	if err != nil {
		t.Fatal(err)
	}
	svc.SetClock(func() time.Time { return time.Date(2026, 9, 12, 9, 0, 0, 0, time.Local) })

	srv := New(svc, "test")
	ct, st := mcp.NewInMemoryTransports()
	ctx := context.Background()
	srvSession, err := srv.Connect(ctx, st)
	if err != nil {
		t.Fatal(err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0"}, nil)
	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cs.Close()
		srvSession.Wait()
		svc.Close()
	})
	return cs, svc
}

func call(t *testing.T, cs *mcp.ClientSession, name string, args map[string]any) map[string]any {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	if res.IsError {
		t.Fatalf("%s 가 오류를 반환: %v", name, res.Content)
	}
	raw, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		// Some tools return arrays; wrap them for uniform access.
		var arr []any
		if err2 := json.Unmarshal(raw, &arr); err2 != nil {
			t.Fatalf("%s 결과 파싱 실패: %v", name, err)
		}
		return map[string]any{"rows": arr}
	}
	return out
}

func callErr(t *testing.T, cs *mcp.ClientSession, name string, args map[string]any) string {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		return err.Error()
	}
	if !res.IsError {
		t.Fatalf("%s 가 성공함 (오류를 기대)", name)
	}
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}

// The whole point of the MCP surface: no delete tool exists.
func TestNoDeleteToolExposed(t *testing.T) {
	cs, _ := newTestSession(t)
	tools, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(tools.Tools))
	for _, tool := range tools.Tools {
		names = append(names, tool.Name)
		if strings.Contains(tool.Name, "delete") || strings.Contains(tool.Name, "remove") {
			t.Fatalf("삭제 도구가 노출됨: %s", tool.Name)
		}
	}
	for _, want := range []string{"task_today", "task_query", "task_add", "task_status", "report_week", "summary"} {
		found := false
		for _, n := range names {
			if n == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("%s 도구가 없음: %v", want, names)
		}
	}
}

func TestAddAndTodayRoundTrip(t *testing.T) {
	cs, svc := newTestSession(t)
	out := call(t, cs, "task_add", map[string]any{
		"title": "MCP 로 추가한 태스크", "project": "infra",
		"scheduled": "today", "due": "2026-09-15", "estimate": "2h",
		"note": "본문 메모",
	})
	task := out["task"].(map[string]any)
	if task["id"] != "T-20260912-0001" || task["project"] != "infra" {
		t.Fatalf("task = %v", task)
	}
	// The file really exists and went through the domain layer.
	loaded, err := svc.Load(task["id"].(string))
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Note() != "본문 메모" {
		t.Fatalf("note = %q", loaded.Note())
	}
	if _, err := os.Stat(loaded.Path); err != nil {
		t.Fatal(err)
	}

	today := call(t, cs, "task_today", nil)
	tasks := today["tasks"].([]any)
	if len(tasks) != 1 {
		t.Fatalf("today = %v", tasks)
	}
}

// The blocked-needs-a-reason rule must hold over MCP exactly as in the TUI.
func TestBlockedWithoutReasonRejected(t *testing.T) {
	cs, _ := newTestSession(t)
	call(t, cs, "task_add", map[string]any{"title": "대상"})
	msg := callErr(t, cs, "task_status", map[string]any{"ref": "#1", "status": "blocked"})
	if !strings.Contains(msg, "사유") {
		t.Fatalf("오류 메시지: %q", msg)
	}
	out := call(t, cs, "task_status", map[string]any{
		"ref": "#1", "status": "blocked", "reason": "외부 회신 대기",
	})
	if out["task"].(map[string]any)["blocked_reason"] != "외부 회신 대기" {
		t.Fatalf("out = %v", out)
	}
}

func TestStatusDoneReportsUnblockedAndNextOccurrence(t *testing.T) {
	cs, _ := newTestSession(t)
	call(t, cs, "task_add", map[string]any{"title": "선행 승인"})
	call(t, cs, "task_add", map[string]any{"title": "후행 작업"})
	call(t, cs, "task_status", map[string]any{"ref": "#2", "status": "blocked", "blocked_by": []string{"#1"}})
	call(t, cs, "task_add", map[string]any{"title": "주간 점검", "recur": "weekly", "scheduled": "today"})

	out := call(t, cs, "task_status", map[string]any{"ref": "#1", "status": "done"})
	unblocked := out["unblocked"].([]any)
	if len(unblocked) != 1 {
		t.Fatalf("unblocked = %v", out)
	}

	out = call(t, cs, "task_status", map[string]any{"ref": "주간 점검", "status": "done"})
	next := out["next_occurrence"].(map[string]any)
	if next["scheduled"] != "2026-09-19" {
		t.Fatalf("next = %v", next)
	}
}

func TestQueryAndEdit(t *testing.T) {
	cs, _ := newTestSession(t)
	call(t, cs, "task_add", map[string]any{"title": "인프라 정리", "project": "infra"})
	call(t, cs, "task_add", map[string]any{"title": "문서 작업"})

	out := call(t, cs, "task_query", map[string]any{"query": "project:infra"})
	if tasks := out["tasks"].([]any); len(tasks) != 1 {
		t.Fatalf("query 결과 = %v", tasks)
	}
	if msg := callErr(t, cs, "task_query", map[string]any{"query": "없는필드:x"}); !strings.Contains(msg, "알 수 없는 필드") {
		t.Fatalf("msg = %q", msg)
	}

	out = call(t, cs, "task_edit", map[string]any{"ref": "#2", "due": "2026-09-20", "priority": "P1"})
	task := out["task"].(map[string]any)
	if task["due"] != "2026-09-20" || task["priority"] != "P1" {
		t.Fatalf("edit 결과 = %v", task)
	}
}

func TestSummaryAndReport(t *testing.T) {
	cs, _ := newTestSession(t)
	call(t, cs, "task_add", map[string]any{"title": "오늘 마감", "due": "today"})
	call(t, cs, "task_add", map[string]any{"title": "진행", "start": true})

	sum := call(t, cs, "summary", nil)
	if sum["due_today"].(float64) != 1 || sum["doing"].(float64) != 1 {
		t.Fatalf("summary = %v", sum)
	}

	rep := call(t, cs, "report_week", map[string]any{})
	md := rep["markdown"].(string)
	if !strings.Contains(md, "# 2026-W37 주간") || !strings.Contains(md, "진행") {
		t.Fatalf("report:\n%s", md)
	}
}
