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
		t.Fatalf("%s 결과 파싱 실패: %v", name, err)
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
		if tool.OutputSchema != nil {
			raw, err := json.Marshal(tool.OutputSchema)
			if err != nil {
				t.Fatal(err)
			}
			var schema map[string]any
			if err := json.Unmarshal(raw, &schema); err != nil || schema["type"] != "object" {
				t.Fatalf("%s outputSchema 는 object 여야 한다: %s", tool.Name, raw)
			}
		}
		if strings.Contains(tool.Name, "delete") || strings.Contains(tool.Name, "remove") {
			t.Fatalf("삭제 도구가 노출됨: %s", tool.Name)
		}
	}
	for _, want := range []string{
		"task_today", "task_query", "task_add", "task_status", "report_week", "summary",
		"task_note", "task_next", "day_load", "project_create", "project_set", "archive",
		"task_claim", "task_release",
	} {
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

func TestNoteIsFindableByBodySearch(t *testing.T) {
	cs, _ := newTestSession(t)
	call(t, cs, "task_add", map[string]any{"title": "게이트웨이 점검"})
	call(t, cs, "task_add", map[string]any{"title": "관계없는 작업"})
	call(t, cs, "task_note", map[string]any{"ref": "#1", "text": "타임아웃을 30초로 조정함"})

	out := call(t, cs, "task_get", map[string]any{"ref": "#1"})
	if note := out["note"].(string); !strings.Contains(note, "타임아웃") {
		t.Fatalf("note = %q", note)
	}
	out = call(t, cs, "task_query", map[string]any{"query": "body:타임아웃"})
	tasks := out["tasks"].([]any)
	if len(tasks) != 1 {
		t.Fatalf("본문 검색 결과 = %v", tasks)
	}
}

func TestEditSpanWritesBothDates(t *testing.T) {
	cs, _ := newTestSession(t)
	call(t, cs, "task_add", map[string]any{"title": "리팩터링"})

	out := call(t, cs, "task_edit", map[string]any{"ref": "#1", "span": "2026-09-15~2026-09-19"})
	task := out["task"].(map[string]any)
	if task["scheduled"] != "2026-09-15" || task["due"] != "2026-09-19" {
		t.Fatalf("task = %v", task)
	}
	// span and the individual dates write the same fields; the overlap is an
	// error rather than a silent winner.
	if msg := callErr(t, cs, "task_edit", map[string]any{
		"ref": "#1", "span": "2026-09-15~2026-09-19", "due": "2026-09-30",
	}); !strings.Contains(msg, "함께 지정할 수 없음") {
		t.Fatalf("msg = %q", msg)
	}
	// Any other field rides along with the span instead of being dropped.
	out = call(t, cs, "task_edit", map[string]any{"ref": "#1", "span": "2026-10-06~2026-10-09", "priority": "P1"})
	task = out["task"].(map[string]any)
	if task["scheduled"] != "2026-10-06" || task["due"] != "2026-10-09" || task["priority"] != "P1" {
		t.Fatalf("span with priority = %v", task)
	}
}

func TestProjectCreateSetAndStatus(t *testing.T) {
	cs, _ := newTestSession(t)
	out := call(t, cs, "project_create", map[string]any{"slug": "infra", "name": "인프라 개편"})
	if out["slug"] != "infra" || out["status"] != "active" {
		t.Fatalf("create = %v", out)
	}
	call(t, cs, "project_set", map[string]any{"slug": "infra", "due": "2026-10-31", "status": "paused"})
	call(t, cs, "task_add", map[string]any{"title": "방화벽", "project": "infra", "estimate": "2h"})
	call(t, cs, "task_add", map[string]any{"title": "라우팅", "project": "infra"})
	call(t, cs, "task_status", map[string]any{"ref": "#2", "status": "done"})

	rows := call(t, cs, "project_status", nil)["rows"].([]any)
	row := rows[0].(map[string]any)
	if row["project"] != "infra" || row["due"] != "2026-10-31" || row["status"] != "paused" {
		t.Fatalf("row = %v", row)
	}
	if p := row["progress"].(float64); p < 0.49 || p > 0.51 {
		t.Fatalf("progress = %v", p)
	}
	if row["remain_estimate"] != "2h" {
		t.Fatalf("remain = %v", row["remain_estimate"])
	}
}

func TestNextAndDayLoad(t *testing.T) {
	cs, _ := newTestSession(t)
	call(t, cs, "task_add", map[string]any{"title": "오늘 마감", "due": "today", "estimate": "2h", "scheduled": "today"})
	call(t, cs, "task_add", map[string]any{"title": "언젠가"})

	rows := call(t, cs, "task_next", map[string]any{"limit": 1})["rows"].([]any)
	if len(rows) != 1 {
		t.Fatalf("next = %v", rows)
	}
	first := rows[0].(map[string]any)
	if first["task"].(map[string]any)["title"] != "오늘 마감" {
		t.Fatalf("추천 = %v", first)
	}
	if reason := first["reason"].(string); !strings.Contains(reason, "오늘 마감") {
		t.Fatalf("reason = %q", reason)
	}

	loads := call(t, cs, "day_load", map[string]any{})["rows"].([]any)
	day := loads[0].(map[string]any)
	if day["planned"] != "2h" || day["estimated"].(float64) != 1 {
		t.Fatalf("load = %v", day)
	}
	if week := call(t, cs, "day_load", map[string]any{"week": true})["rows"].([]any); len(week) != 7 {
		t.Fatalf("주간 load = %d일", len(week))
	}
}

func TestArchiveDryRunReportsWithoutMoving(t *testing.T) {
	cs, svc := newTestSession(t)
	call(t, cs, "task_add", map[string]any{"title": "끝난 일"})
	call(t, cs, "task_status", map[string]any{"ref": "#1", "status": "done"})

	// cutoff 를 내일로 두면 오늘 완료분이 대상에 들어온다.
	out := call(t, cs, "archive", map[string]any{"before": "tomorrow", "dry_run": true})
	if len(out["moved"].([]any)) != 1 || out["dry_run"] != true {
		t.Fatalf("archive = %v", out)
	}
	if _, err := svc.Resolve("#1"); err != nil {
		t.Fatalf("dry-run 인데 인덱스에서 빠짐: %v", err)
	}
	out = call(t, cs, "archive", map[string]any{"before": "tomorrow"})
	if len(out["moved"].([]any)) != 1 {
		t.Fatalf("archive = %v", out)
	}
	if _, err := svc.Resolve("#1"); err == nil {
		t.Fatal("아카이브 후에도 인덱스에 남아 있음")
	}
}

func TestStyleGateRejectsDocumentsAndAdvisesTone(t *testing.T) {
	cs, _ := newTestSession(t)

	// Structure is refused, and the refusal says where the material goes.
	msg := callErr(t, cs, "task_add", map[string]any{
		"title": "결제 콜백 중복 수신",
		"note":  "## Context\n배경\n## Approach\n- [ ] 수정\n- [ ] 테스트",
	})
	if !strings.Contains(msg, "tp://conventions") {
		t.Errorf("거부가 규약을 가리키지 않음: %s", msg)
	}
	if msg = callErr(t, cs, "task_add", map[string]any{"title": strings.Repeat("가", 61)}); !strings.Contains(msg, "note") {
		t.Errorf("긴 제목 거부에 고치는 방법이 없음: %s", msg)
	}

	// Tone is advice, not a wall: the task is created and the warning rides back.
	out := call(t, cs, "task_add", map[string]any{
		"title": "주간 보고서 작성 및 공유",
		"note":  "확인 필요",
	})
	warns := out["warnings"].([]any)
	if len(warns) < 2 {
		t.Fatalf("경고가 모자람: %v", warns)
	}
	if out["task"].(map[string]any)["short_id"] != "#1" {
		t.Fatalf("경고 때문에 생성이 막힘: %v", out)
	}

	// A line written the way the conventions ask draws nothing.
	out = call(t, cs, "task_add", map[string]any{
		"title": "task_add 동시 호출 때 중복 생성",
		"note":  "9/12 Codex 세션에서 같은 태스크가 두 번 생겼다. 재현은 아직 한 번뿐.",
	})
	if w, ok := out["warnings"]; ok {
		t.Errorf("잘 쓴 태스크에 경고: %v", w)
	}
}

func TestStyleGateOnHoldReason(t *testing.T) {
	cs, _ := newTestSession(t)
	call(t, cs, "task_add", map[string]any{"title": "정산 배치 시각 조정"})

	out := call(t, cs, "task_status", map[string]any{
		"ref": "#1", "status": "blocked", "reason": "외부 의존성으로 인한 대기",
	})
	if _, ok := out["warnings"]; !ok {
		t.Error("상대도 기한도 없는 보류에 경고가 없음")
	}
	out = call(t, cs, "task_status", map[string]any{
		"ref": "#1", "status": "blocked", "reason": "A팀 스펙 대기 — 9/12 요청, 18일까지 준다고 함",
	})
	if w, ok := out["warnings"]; ok {
		t.Errorf("잘 쓴 사유에 경고: %v", w)
	}
}

func TestWriteToolsEchoThePreview(t *testing.T) {
	cs, _ := newTestSession(t)
	out := call(t, cs, "task_add", map[string]any{
		"title": "결제 콜백 중복 수신", "project": "billing", "priority": "P1",
		"scheduled": "2026-09-15", "due": "2026-09-19",
	})
	want := "○ #1 결제 콜백 중복 수신  [billing · P1]  09-15~09-19 5일"
	if out["preview"] != want {
		t.Fatalf("preview = %q, want %q", out["preview"], want)
	}
	out = call(t, cs, "task_status", map[string]any{"ref": "#1", "status": "doing"})
	if !strings.HasPrefix(out["preview"].(string), "● #1") {
		t.Fatalf("전이 후 preview = %q", out["preview"])
	}
}

func TestConventionsResourceIsReadable(t *testing.T) {
	cs, _ := newTestSession(t)
	res, err := cs.ReadResource(context.Background(),
		&mcp.ReadResourceParams{URI: "tp://conventions"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Contents) != 1 || !strings.Contains(res.Contents[0].Text, "명사구") {
		t.Fatalf("규약 문서가 비어 있음: %v", res.Contents)
	}
}

// time_summary takes the periods `tp time` takes and refuses the rest, rather
// than reading "month" as everything.
func TestTimeSummaryPeriods(t *testing.T) {
	cs, _ := newTestSession(t)
	call(t, cs, "task_add", map[string]any{"title": "측정", "estimate": "1h"})
	for _, p := range []string{"", "all", "week"} {
		call(t, cs, "time_summary", map[string]any{"period": p})
	}
	if msg := callErr(t, cs, "time_summary", map[string]any{"period": "month"}); !strings.Contains(msg, "week 또는 all") {
		t.Fatalf("msg = %q", msg)
	}
}
