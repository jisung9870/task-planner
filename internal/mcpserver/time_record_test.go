package mcpserver

import (
	"fmt"
	"strings"
	"testing"
)

// due·estimate·span still save for one release, but say they are on the way
// out so an agent following an older skill learns to stop sending them.
func TestRetiredFieldsWarnButSave(t *testing.T) {
	cs, svc := newTestSession(t)
	out := call(t, cs, "task_add", map[string]any{
		"title": "보고서 정리", "executor": "agent", "due": "2026-09-15", "estimate": "2h",
	})
	if w := fmt.Sprint(out["warnings"]); !strings.Contains(w, "due") || !strings.Contains(w, "estimate") {
		t.Fatalf("warnings = %v", w)
	}
	id := out["task"].(map[string]any)["id"].(string)
	got, _ := svc.Load(id)
	if got.Due.String() != "2026-09-15" || got.Estimate.String() != "2h" {
		t.Fatalf("due=%s estimate=%s were not kept", got.Due, got.Estimate)
	}
	out = call(t, cs, "task_edit", map[string]any{"ref": id, "span": "09-15~09-19"})
	if w := fmt.Sprint(out["warnings"]); !strings.Contains(w, "span") {
		t.Fatalf("span warnings = %v", w)
	}
	out = call(t, cs, "task_edit", map[string]any{"ref": id, "scheduled": "09-20"})
	if out["warnings"] != nil {
		t.Fatalf("scheduled alone warned: %v", out["warnings"])
	}
}

// since backfills the start on done and is refused elsewhere.
func TestStatusDoneSince(t *testing.T) {
	cs, svc := newTestSession(t) // now = 2026-09-12 09:00
	out := call(t, cs, "task_add", map[string]any{"title": "보고서 정리", "executor": "agent"})
	id := out["task"].(map[string]any)["id"].(string)
	if msg := callErr(t, cs, "task_status", map[string]any{"ref": id, "status": "doing", "since": "08:00"}); !strings.Contains(msg, "done") {
		t.Fatalf("since on doing: %s", msg)
	}
	call(t, cs, "task_status", map[string]any{"ref": id, "status": "done", "since": "07:30"})
	got, _ := svc.Load(id)
	if got.Actual.String() != "1h30m" || !strings.Contains(got.Body, "(착수 소급 2026-09-12 07:30)") {
		t.Fatalf("actual=%s body=%q", got.Actual, got.Body)
	}
}
