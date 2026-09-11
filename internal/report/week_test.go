package report

import (
	"strings"
	"testing"
	"time"

	"task-planner/internal/domain"
)

var today = domain.NewDate(2026, time.September, 12) // week 37: 09-07 ~ 09-13

func mk(id, title string, st domain.Status) *domain.Task {
	return &domain.Task{ID: "T-2026-" + id, Title: title, Status: st}
}

func render(ts ...*domain.Task) string {
	return Week(WeekInput{Tasks: ts, Ref: today, Today: today})
}

func TestWeekGroupsByStatus(t *testing.T) {
	done := mk("0001", "PoC 정리", domain.StatusDone)
	done.Completed = domain.NewDate(2026, time.September, 10)
	done.Project = "log-pipeline"

	doing := mk("0002", "권한 이슈", domain.StatusDoing)
	doing.Project = "infra"

	blocked := mk("0003", "DB 마이그레이션", domain.StatusBlocked)
	blocked.BlockedReason = "인프라팀 회신 대기"
	blocked.BlockedSince = domain.NewDate(2026, time.September, 9)

	out := render(done, doing, blocked)
	for _, want := range []string{
		"# 2026-W37 주간 (2026-09-07 ~ 2026-09-13)",
		"완료 1 · 진행중 1 · 보류 1 · 이월 0",
		"## 완료 (1)", "PoC 정리",
		"## 진행중 (1)", "권한 이슈",
		"## 보류 (1)", "인프라팀 회신 대기 · 3일 경과",
		"**log-pipeline**", "**infra**",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("누락: %q\n---\n%s", want, out)
		}
	}
}

// Work completed in a different week must not appear; otherwise the report
// double-counts every time it is regenerated.
func TestWeekExcludesOtherWeeks(t *testing.T) {
	lastWeek := mk("0001", "지난주 완료", domain.StatusDone)
	lastWeek.Completed = domain.NewDate(2026, time.September, 4)
	out := render(lastWeek)
	if strings.Contains(out, "지난주 완료") {
		t.Fatalf("다른 주 항목이 포함됨:\n%s", out)
	}
	if !strings.Contains(out, "기록된 항목이 없습니다") {
		t.Fatalf("빈 리포트 안내 없음:\n%s", out)
	}
}

func TestWeekReportsCarryCountAndUpcoming(t *testing.T) {
	carried := mk("0001", "계속 미뤄진 것", domain.StatusTodo)
	carried.RolloverCount = 4

	next := mk("0002", "다음 주 마감", domain.StatusTodo)
	next.Due = domain.NewDate(2026, time.September, 16)

	out := render(carried, next)
	if !strings.Contains(out, "## 이월 (1)") || !strings.Contains(out, "4회") {
		t.Errorf("이월 섹션 누락:\n%s", out)
	}
	if !strings.Contains(out, "## 다음 주 예정 (1)") || !strings.Contains(out, "마감 2026-09-16") {
		t.Errorf("다음 주 섹션 누락:\n%s", out)
	}
}

func TestWeekSeparatesCancelledFromDone(t *testing.T) {
	cancelled := mk("0001", "접은 일", domain.StatusCancelled)
	cancelled.Completed = domain.NewDate(2026, time.September, 11)
	out := render(cancelled)
	if strings.Contains(out, "## 완료") {
		t.Errorf("취소가 완료로 집계됨:\n%s", out)
	}
	if !strings.Contains(out, "## 취소 (1)") {
		t.Errorf("취소 섹션 없음:\n%s", out)
	}
}

func TestWeekEmitsFrontmatter(t *testing.T) {
	out := render()
	if !strings.HasPrefix(out, "---\ntype: weekly-report\n") {
		t.Fatalf("frontmatter 없음:\n%s", out)
	}
}
