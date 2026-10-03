package mcpserver

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"task-planner/internal/domain"
	"task-planner/internal/service"
	"task-planner/internal/style"
)

type addArgs struct {
	Title     string   `json:"title" jsonschema:"태스크 제목 (필수). 40자 안쪽 명사구 한 줄, 한 제목에 한 가지 일. 상세는 note 로 (규약: tp://conventions)"`
	Project   string   `json:"project,omitempty" jsonschema:"프로젝트 slug"`
	Executor  string   `json:"executor,omitempty" jsonschema:"실행 주체 human|agent. 에이전트 수행 작업은 agent 필수"`
	Agent     string   `json:"agent,omitempty" jsonschema:"실행할 agent: vault_info.agents 중 하나 또는 auto(누구든). 지정하면 executor=agent"`
	Tier      string   `json:"tier,omitempty" jsonschema:"작업 무게 fast|standard|deep. 모델은 vault_info.models 로 정해짐"`
	Priority  string   `json:"priority,omitempty" jsonschema:"우선순위 P0~P3"`
	Scheduled string   `json:"scheduled,omitempty" jsonschema:"꺼낼 날: 이날부터 Today 에 보인다. 마감 아님. YYYY-MM-DD | today | tomorrow | +3d | mon"`
	Due       string   `json:"due,omitempty" jsonschema:"폐지 예정(2026-10-03) — 저장은 되지만 어디에도 쓰이지 않는다. 보내지 말 것"`
	Estimate  string   `json:"estimate,omitempty" jsonschema:"폐지 예정(2026-10-03) — 저장은 되지만 어디에도 쓰이지 않는다. 소요는 진행중→완료 기록에서 계산된다"`
	Tags      []string `json:"tags,omitempty"`
	Links     []string `json:"links,omitempty" jsonschema:"외부 링크 (jira:ABC-123 등)"`
	Note      string   `json:"note,omitempty" jsonschema:"메모 본문. human 작업은 사실 2~4줄, agent 작업은 목표·단계·완료 기준 (규약: tp://conventions)"`
	Recur     string   `json:"recur,omitempty" jsonschema:"반복 규칙: daily|weekly|monthly|weekdays|every N days|every monday|monthly on 15"`
	Start     bool     `json:"start,omitempty" jsonschema:"true 면 추가와 동시에 진행중으로 (타이머 시작)"`
}

// withWarnings attaches the style advice to a tool result. Warnings ride back
// with the task itself rather than blocking it: the caller sees how the line it
// just wrote falls short while the write still succeeds.
func withWarnings(out mutateOut, is []style.Issue) mutateOut {
	out.Warnings = append(out.Warnings, style.Warnings(is)...)
	return out
}

type mutateOut struct {
	Task taskJSON `json:"task"`
	// Preview is the row as a human will meet it in the TUI. It rides back on
	// writes only: seeing the line it just wrote is what keeps the caller's
	// next title from being a paragraph, and a list view repeating it for
	// twenty rows would only cost tokens.
	Preview   string     `json:"preview" jsonschema:"방금 쓴 태스크가 목록에서 보이는 한 줄"`
	Warnings  []string   `json:"warnings,omitempty"`
	Unblocked []taskJSON `json:"unblocked,omitempty" jsonschema:"이 변경으로 보류가 자동 해제된 태스크"`
	Next      *taskJSON  `json:"next_occurrence,omitempty" jsonschema:"반복 태스크 완료로 생성된 다음 회차"`
}

type statusArgs struct {
	Ref       string   `json:"ref" jsonschema:"태스크 지정: id, 짧은 번호, 제목 부분일치"`
	Status    string   `json:"status" jsonschema:"todo|doing|blocked|done|cancelled. doing 은 타이머 시작, done 은 반복이면 다음 회차 생성과 후행 보류 해제"`
	Reason    string   `json:"reason,omitempty" jsonschema:"보류(blocked) 사유 — 누구를 언제까지 기다리는지 한 줄. blocked 는 reason 또는 blocked_by 없이는 거부됨"`
	BlockedBy []string `json:"blocked_by,omitempty" jsonschema:"선행 태스크 참조 목록. 선행이 전부 끝나면 자동으로 대기중 복귀"`
	Since     string   `json:"since,omitempty" jsonschema:"done 에만: 실제 착수 시각 (10:30, 2h, 어제 14:00, 2026-10-01 14:00). 타이머를 켜지 않았거나 늦게 켠 일의 시작을 남긴다"`
}

type editArgs struct {
	Ref       string    `json:"ref" jsonschema:"태스크 지정"`
	Title     *string   `json:"title,omitempty"`
	Project   *string   `json:"project,omitempty" jsonschema:"빈 문자열이면 프로젝트 해제"`
	Executor  *string   `json:"executor,omitempty" jsonschema:"실행 주체 human|agent"`
	Agent     *string   `json:"agent,omitempty" jsonschema:"실행할 agent 또는 auto. 빈 문자열이면 지정 해제"`
	Tier      *string   `json:"tier,omitempty" jsonschema:"fast|standard|deep. 빈 문자열이면 해제"`
	Priority  *string   `json:"priority,omitempty" jsonschema:"P0~P3, 빈 문자열이면 해제"`
	Scheduled *string   `json:"scheduled,omitempty" jsonschema:"꺼낼 날: YYYY-MM-DD | today | tomorrow | +3d | none(해제 — 백로그로)"`
	Due       *string   `json:"due,omitempty" jsonschema:"폐지 예정(2026-10-03) — 보내지 말 것"`
	Estimate  *string   `json:"estimate,omitempty" jsonschema:"폐지 예정(2026-10-03) — 보내지 말 것"`
	Tags      *[]string `json:"tags,omitempty" jsonschema:"전체 교체"`
	Recur     *string   `json:"recur,omitempty" jsonschema:"반복 규칙, 빈 문자열이면 반복 중단"`
	Span      *string   `json:"span,omitempty" jsonschema:"폐지 예정(2026-10-03) — 진행 기간(scheduled~due)은 없어졌다. 꺼낼 날은 scheduled 로"`
}

type claimArgs struct {
	Ref     string `json:"ref,omitempty" jsonschema:"가져갈 태스크. 비우면 이 agent 몫 중 가장 급한 일"`
	Agent   string `json:"agent,omitempty" jsonschema:"가져가는 agent. 생략하면 연결한 클라이언트 이름(claude·codex)으로 판단"`
	Session string `json:"session,omitempty" jsonschema:"claimed_by 에 남길 세션 식별자. 생략하면 이 연결의 식별자(vault_info.session). 재시작 후 이어가려면 같은 값을 줄 것"`
}

type releaseArgs struct {
	Ref     string `json:"ref" jsonschema:"놓을 태스크"`
	Agent   string `json:"agent,omitempty"`
	Session string `json:"session,omitempty"`
	Force   bool   `json:"force,omitempty" jsonschema:"다른 세션의 claim 도 해제. 그 세션이 끝난 것을 사용자가 확인했을 때만"`
}

type claimOut struct {
	mutateOut
	Model string `json:"model,omitempty" jsonschema:"agent·tier 로 정해진 모델. 비어 있으면 tier 미정"`
}

type noteArgs struct {
	Ref  string `json:"ref" jsonschema:"태스크 지정"`
	Text string `json:"text" jsonschema:"시각이 붙어 누적되는 메모. human 작업은 확인한 사실, agent 작업은 실행 단계·결과·계획 변경·검증 근거를 기록"`
}

type projectCreateArgs struct {
	Slug string `json:"slug" jsonschema:"프로젝트 slug (태스크의 project 필드가 참조하는 값)"`
	Name string `json:"name,omitempty" jsonschema:"표시 이름. 생략 시 slug"`
}

type projectSetArgs struct {
	Slug   string  `json:"slug" jsonschema:"프로젝트 slug"`
	Name   *string `json:"name,omitempty"`
	Status *string `json:"status,omitempty" jsonschema:"active|paused|done"`
	Owner  *string `json:"owner,omitempty" jsonschema:"담당. 빈 문자열이면 해제"`
	Due    *string `json:"due,omitempty" jsonschema:"프로젝트 마감(마일스톤). none 이면 해제"`
}

type projectOut struct {
	Slug   string `json:"slug"`
	Name   string `json:"name"`
	Status string `json:"status,omitempty"`
	Owner  string `json:"owner,omitempty"`
	Due    string `json:"due,omitempty"`
	Path   string `json:"path"`
}

type archiveArgs struct {
	Before string `json:"before,omitempty" jsonschema:"이 날짜 이전 완료분 (YYYY-MM-DD | -3m). days 보다 우선"`
	Days   int    `json:"days,omitempty" jsonschema:"며칠 이전 완료분을 옮길지 (기본 30)"`
	DryRun bool   `json:"dry_run,omitempty" jsonschema:"true 면 옮기지 않고 목록만"`
}

type archiveOut struct {
	Cutoff string     `json:"cutoff"`
	DryRun bool       `json:"dry_run,omitempty"`
	Moved  []taskJSON `json:"moved"`
}

type rolloverOut struct {
	Rolled  []taskJSON `json:"rolled"`
	Warning string     `json:"warning,omitempty"`
}

// deprecatedFields warns about inputs retired on 2026-10-03 (기획서 "시간: 계획이
// 아니라 기록"). They are still saved for one release so an agent following an
// older skill does not fail mid-task; the warning is how it learns to stop.
func deprecatedFields(due, estimate, span bool) []string {
	var out []string
	if due || span {
		out = append(out, "due·span 은 폐지 예정입니다 — 마감은 더 쓰지 않습니다. 언제 꺼낼지는 scheduled 로")
	}
	if estimate {
		out = append(out, "estimate 는 폐지 예정입니다 — 소요는 진행중→완료 기록에서 계산합니다. 시작을 놓쳤으면 task_status done 에 since")
	}
	return out
}

func (s *Server) registerWriteTools() {
	mcp.AddTool(s.mcp, &mcp.Tool{
		Name:        "task_add",
		Description: "공유 vault에 태스크 추가. 에이전트 수행 작업은 executor=agent로 등록하고 note에 목표·계획·완료 기준을 기록한다. 작성 규약: tp://conventions.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in addArgs) (*mcp.CallToolResult, mutateOut, error) {
		defer s.begin()()
		agent, err := domain.ParseAgent(in.Agent)
		if err != nil {
			return nil, mutateOut{}, err
		}
		tier, err := domain.ParseTier(in.Tier)
		if err != nil {
			return nil, mutateOut{}, err
		}
		// Naming an agent makes it agent work; an omitted executor must not
		// default to human and then collide with that.
		if in.Executor == "" && agent != "" {
			in.Executor = string(domain.ExecutorAgent)
		}
		executor, err := domain.ParseExecutor(in.Executor)
		if err != nil {
			return nil, mutateOut{}, err
		}
		issues := style.CheckTitle(in.Title)
		if executor == domain.ExecutorHuman {
			issues = append(issues, style.CheckNote(in.Note)...)
		}
		if err := style.Err(issues); err != nil {
			return nil, mutateOut{}, err
		}
		today := s.svc.Today()
		ai := service.AddInput{
			Title: in.Title, Project: in.Project, Executor: executor, Agent: agent, Tier: tier,
			Tags: in.Tags, Links: in.Links, Note: in.Note, Recur: in.Recur,
		}
		// The executor controls note conventions independently of vault path.
		if ai.Priority, err = domain.ParsePriority(in.Priority); err != nil {
			return nil, mutateOut{}, err
		}
		if ai.Scheduled, err = parseDate(in.Scheduled, today); err != nil {
			return nil, mutateOut{}, err
		}
		if ai.Due, err = parseDate(in.Due, today); err != nil {
			return nil, mutateOut{}, err
		}
		if ai.Estimate, err = domain.ParseDuration(in.Estimate); err != nil {
			return nil, mutateOut{}, err
		}
		deprecated := deprecatedFields(in.Due != "", in.Estimate != "", false)
		if in.Start {
			ai.Status = domain.StatusDoing
		}
		res, err := s.svc.AddWithResult(ai)
		if err != nil {
			return nil, mutateOut{}, err
		}
		if err := s.finish(); err != nil {
			return nil, mutateOut{}, err
		}
		out := withWarnings(toMutateOut(res, today), issues)
		out.Warnings = append(out.Warnings, deprecated...)
		return nil, out, nil
	})

	mcp.AddTool(s.mcp, &mcp.Tool{
		Name:        "task_status",
		Description: "상태 전이. 전이 로그·완료일·타이머·보류 검증·반복 회차 생성·후행 자동 해제가 함께 처리됨. 삭제는 제공하지 않음 — 접을 일은 cancelled 로.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in statusArgs) (*mcp.CallToolResult, mutateOut, error) {
		defer s.begin()()
		st, err := domain.ParseStatus(in.Status)
		if err != nil {
			return nil, mutateOut{}, err
		}
		var issues []style.Issue
		var res *service.Result
		if st == domain.StatusBlocked {
			issues = style.CheckReason(in.Reason)
			if err := style.Err(issues); err != nil {
				return nil, mutateOut{}, err
			}
			res, err = s.svc.Block(in.Ref, in.Reason, in.BlockedBy)
		} else if in.Since != "" {
			if st != domain.StatusDone {
				return nil, mutateOut{}, fmt.Errorf("since 는 status=done 에만 쓸 수 있음")
			}
			since, perr := s.svc.ParseSince(in.Since)
			if perr != nil {
				return nil, mutateOut{}, perr
			}
			res, err = s.svc.DoneSince(in.Ref, &since)
		} else {
			res, err = s.svc.SetStatus(in.Ref, st, nil)
		}
		if err != nil {
			return nil, mutateOut{}, err
		}
		if err := s.finish(); err != nil {
			return nil, mutateOut{}, err
		}
		return nil, withWarnings(toMutateOut(res, s.svc.Today()), issues), nil
	})

	mcp.AddTool(s.mcp, &mcp.Tool{
		Name:        "task_skip",
		Description: "반복 태스크의 이번 회차만 건너뛰고 다음 회차를 생성. 시리즈를 끝내려면 task_status cancelled.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in refArgs) (*mcp.CallToolResult, mutateOut, error) {
		defer s.begin()()
		res, err := s.svc.Skip(in.Ref)
		if err != nil {
			return nil, mutateOut{}, err
		}
		if err := s.finish(); err != nil {
			return nil, mutateOut{}, err
		}
		return nil, toMutateOut(res, s.svc.Today()), nil
	})

	mcp.AddTool(s.mcp, &mcp.Tool{
		Name:        "task_edit",
		Description: "필드 수정 (부분 갱신 — 지정한 필드만 바뀜). 변경 내역이 태스크 로그에 남음. 미리 정하는 값은 priority 와 scheduled(꺼낼 날)뿐이다.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in editArgs) (*mcp.CallToolResult, mutateOut, error) {
		defer s.begin()()
		var issues []style.Issue
		if in.Title != nil {
			issues = style.CheckTitle(*in.Title)
			if err := style.Err(issues); err != nil {
				return nil, mutateOut{}, err
			}
		}
		today := s.svc.Today()
		ei := service.EditInput{Title: in.Title, Project: in.Project, Tags: in.Tags, Recur: in.Recur}
		if in.Executor != nil {
			e, err := domain.ParseExecutor(*in.Executor)
			if err != nil {
				return nil, mutateOut{}, err
			}
			ei.Executor = &e
		}
		if in.Agent != nil {
			a, err := domain.ParseAgent(*in.Agent)
			if err != nil {
				return nil, mutateOut{}, err
			}
			ei.Agent = &a
		}
		if in.Tier != nil {
			tr, err := domain.ParseTier(*in.Tier)
			if err != nil {
				return nil, mutateOut{}, err
			}
			ei.Tier = &tr
		}
		if in.Priority != nil {
			p, err := domain.ParsePriority(*in.Priority)
			if err != nil {
				return nil, mutateOut{}, err
			}
			ei.Priority = &p
		}
		if in.Scheduled != nil {
			d, err := parseDate(*in.Scheduled, today)
			if err != nil {
				return nil, mutateOut{}, err
			}
			ei.Scheduled = &d
		}
		if in.Due != nil {
			d, err := parseDate(*in.Due, today)
			if err != nil {
				return nil, mutateOut{}, err
			}
			ei.Due = &d
		}
		if in.Estimate != nil {
			e, err := domain.ParseDuration(*in.Estimate)
			if err != nil {
				return nil, mutateOut{}, err
			}
			ei.Estimate = &e
		}
		var span *domain.Span
		if in.Span != nil {
			sp, err := s.svc.ParseSpan(*in.Span)
			if err != nil {
				return nil, mutateOut{}, err
			}
			span = &sp
		}
		res, err := s.svc.EditWithSpan(in.Ref, ei, span)
		if err != nil {
			return nil, mutateOut{}, err
		}
		if err := s.finish(); err != nil {
			return nil, mutateOut{}, err
		}
		out := withWarnings(toMutateOut(res, today), issues)
		out.Warnings = append(out.Warnings, deprecatedFields(in.Due != nil, in.Estimate != nil, in.Span != nil)...)
		return nil, out, nil
	})

	mcp.AddTool(s.mcp, &mcp.Tool{
		Name:        "task_claim",
		Description: "이 세션이 일을 가져가 진행중으로 바꾼다. ref 를 비우면 이 agent 몫(agent 가 같거나 auto, 미보류, 아무도 안 가져간 일) 중 가장 급한 것을 가져간다. 다른 세션이 이미 가져갔으면 거부. 결과의 model 로 실행할 모델을 정한다 — 비어 있으면 tier 를 판단해 task_edit 로 남길 것.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in claimArgs) (*mcp.CallToolResult, claimOut, error) {
		defer s.begin()()
		agent, err := s.callerAgent(req, in.Agent)
		if err != nil {
			return nil, claimOut{}, err
		}
		session := in.Session
		if session == "" {
			session = s.session
		}
		res, err := s.svc.Claim(service.ClaimInput{Ref: in.Ref, Agent: agent, Session: session})
		if err != nil {
			return nil, claimOut{}, err
		}
		if err := s.finish(); err != nil {
			return nil, claimOut{}, err
		}
		return nil, claimOut{mutateOut: toMutateOut(res, s.svc.Today()), Model: s.svc.ModelFor(res.Task)}, nil
	})

	mcp.AddTool(s.mcp, &mcp.Tool{
		Name:        "task_release",
		Description: "가져간 일을 놓는다 (진행중이면 대기중으로). 이 세션이 가져간 것만 놓을 수 있고, 죽은 세션의 claim 은 사용자가 확인한 뒤 force 로.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in releaseArgs) (*mcp.CallToolResult, mutateOut, error) {
		defer s.begin()()
		by := ""
		if !in.Force {
			agent, err := s.callerAgent(req, in.Agent)
			if err != nil {
				return nil, mutateOut{}, err
			}
			session := in.Session
			if session == "" {
				session = s.session
			}
			if by, err = domain.ClaimRef(agent, session); err != nil {
				return nil, mutateOut{}, err
			}
		}
		res, err := s.svc.Release(in.Ref, by, in.Force)
		if err != nil {
			return nil, mutateOut{}, err
		}
		if err := s.finish(); err != nil {
			return nil, mutateOut{}, err
		}
		return nil, toMutateOut(res, s.svc.Today()), nil
	})

	mcp.AddTool(s.mcp, &mcp.Tool{
		Name:        "task_note",
		Description: "태스크 본문에 시각이 붙은 메모 한 줄 추가. agent 작업에는 실제 수행 단계·검증 결과·계획 변경을 기록한다.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in noteArgs) (*mcp.CallToolResult, mutateOut, error) {
		defer s.begin()()
		task, err := s.svc.Load(in.Ref)
		if err != nil {
			return nil, mutateOut{}, err
		}
		var issues []style.Issue
		if task.Executor.Effective() == domain.ExecutorHuman {
			issues = style.CheckNoteLine(in.Text)
		}
		if err := style.Err(issues); err != nil {
			return nil, mutateOut{}, err
		}
		res, err := s.svc.AddNote(in.Ref, in.Text)
		if err != nil {
			return nil, mutateOut{}, err
		}
		if err := s.finish(); err != nil {
			return nil, mutateOut{}, err
		}
		return nil, withWarnings(toMutateOut(res, s.svc.Today()), issues), nil
	})

	mcp.AddTool(s.mcp, &mcp.Tool{
		Name:        "project_create",
		Description: "프로젝트 생성 (projects/<slug>/project.md). 태스크의 project 필드는 파일 없이도 쓸 수 있지만, 상태·마감을 달려면 파일이 있어야 한다.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in projectCreateArgs) (*mcp.CallToolResult, projectOut, error) {
		defer s.begin()()
		p, err := s.svc.CreateProject(in.Slug, in.Name)
		if err != nil {
			return nil, projectOut{}, err
		}
		if err := s.finish(); err != nil {
			return nil, projectOut{}, err
		}
		return nil, toProjectOut(p), nil
	})

	mcp.AddTool(s.mcp, &mcp.Tool{
		Name:        "project_set",
		Description: "프로젝트 메타 수정 (이름·상태·담당·마감). 마감을 넣으면 project_status 가 진행률과 함께 남은 날짜를 계산한다.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in projectSetArgs) (*mcp.CallToolResult, projectOut, error) {
		defer s.begin()()
		ei := service.ProjectEditInput{Name: in.Name, Status: in.Status, Owner: in.Owner}
		if in.Due != nil {
			d, err := parseDate(*in.Due, s.svc.Today())
			if err != nil {
				return nil, projectOut{}, err
			}
			ei.Due = &d
		}
		p, err := s.svc.EditProject(in.Slug, ei)
		if err != nil {
			return nil, projectOut{}, err
		}
		if err := s.finish(); err != nil {
			return nil, projectOut{}, err
		}
		return nil, toProjectOut(p), nil
	})

	mcp.AddTool(s.mcp, &mcp.Tool{
		Name:        "archive",
		Description: "오래된 완료·취소 태스크를 archive/ 로 이동. 인덱스가 가벼워지는 대신 그 태스크들은 조회 대상에서 빠지므로, 먼저 dry_run 으로 확인할 것.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in archiveArgs) (*mcp.CallToolResult, archiveOut, error) {
		defer s.begin()()
		today := s.svc.Today()
		cutoff, err := parseDate(in.Before, today)
		if err != nil {
			return nil, archiveOut{}, err
		}
		if cutoff.IsZero() {
			days := in.Days
			if days <= 0 {
				days = 30
			}
			cutoff = today.AddDays(-days)
		}
		rep, err := s.svc.Archive(cutoff, in.DryRun)
		if err != nil {
			return nil, archiveOut{}, err
		}
		if err := s.finish(); err != nil {
			return nil, archiveOut{}, err
		}
		out := archiveOut{Cutoff: cutoff.String(), DryRun: rep.DryRun}
		for _, m := range rep.Moved {
			out.Moved = append(out.Moved, toTaskJSON(m.Task, today))
		}
		return nil, out, nil
	})

	mcp.AddTool(s.mcp, &mcp.Tool{
		Name:        "rollover",
		Description: "지난 날짜의 미완료(대기중·진행중) 항목을 오늘로 이월하고 이월 횟수를 올림. 보류는 남의 응답 대기이므로 제외.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, _ any) (*mcp.CallToolResult, rolloverOut, error) {
		defer s.begin()()
		rep, err := s.svc.Rollover()
		if err != nil {
			return nil, rolloverOut{}, err
		}
		if err := s.finish(); err != nil {
			return nil, rolloverOut{}, err
		}
		out := rolloverOut{Warning: rep.StaleWarning()}
		today := s.svc.Today()
		for _, r := range rep.Rolled {
			out.Rolled = append(out.Rolled, toTaskJSON(r.Task, today))
		}
		return nil, out, nil
	})
}

func toProjectOut(p *domain.Project) projectOut {
	return projectOut{
		Slug: p.Slug, Name: p.Display(), Status: p.Status,
		Owner: p.Owner, Due: p.Due.String(), Path: p.Path,
	}
}

func toMutateOut(res *service.Result, today domain.Date) mutateOut {
	out := mutateOut{
		Task:     toTaskJSON(res.Task, today),
		Preview:  res.Task.Preview(today),
		Warnings: res.Warnings,
	}
	for _, u := range res.Unblocked {
		out.Unblocked = append(out.Unblocked, toTaskJSON(u.Task, today))
	}
	if res.Next != nil {
		n := toTaskJSON(res.Next, today)
		out.Next = &n
	}
	return out
}

// callerAgent is the explicit agent argument, or else the agent the MCP client
// identified itself as. Guessing from anything vaguer would let a session
// claim work under another agent's name.
func (s *Server) callerAgent(req *mcp.CallToolRequest, explicit string) (domain.Agent, error) {
	if explicit != "" {
		return domain.ParseAgent(explicit)
	}
	if req != nil && req.Session != nil {
		if p := req.Session.InitializeParams(); p != nil && p.ClientInfo != nil {
			name := strings.ToLower(p.ClientInfo.Name)
			for _, a := range s.svc.Cfg.Agents.Allowed {
				if strings.Contains(name, string(a)) {
					return a, nil
				}
			}
		}
	}
	return "", fmt.Errorf("agent 를 지정하세요 (클라이언트 이름으로 판단할 수 없음)")
}
