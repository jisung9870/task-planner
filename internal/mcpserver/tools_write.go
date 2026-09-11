package mcpserver

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"task-planner/internal/domain"
	"task-planner/internal/service"
)

type addArgs struct {
	Title     string   `json:"title" jsonschema:"태스크 제목 (필수)"`
	Project   string   `json:"project,omitempty" jsonschema:"프로젝트 slug"`
	Priority  string   `json:"priority,omitempty" jsonschema:"우선순위 P0~P3"`
	Scheduled string   `json:"scheduled,omitempty" jsonschema:"착수 예정일: YYYY-MM-DD | today | tomorrow"`
	Due       string   `json:"due,omitempty" jsonschema:"마감일: YYYY-MM-DD | today | tomorrow"`
	Estimate  string   `json:"estimate,omitempty" jsonschema:"예상 소요 (30m, 2h, 1h30m)"`
	Tags      []string `json:"tags,omitempty"`
	Links     []string `json:"links,omitempty" jsonschema:"외부 링크 (jira:ABC-123 등)"`
	Note      string   `json:"note,omitempty" jsonschema:"메모 본문 (markdown)"`
	Recur     string   `json:"recur,omitempty" jsonschema:"반복 규칙: daily|weekly|monthly|weekdays|every N days|every monday|monthly on 15"`
	Start     bool     `json:"start,omitempty" jsonschema:"true 면 추가와 동시에 진행중으로 (타이머 시작)"`
}

type mutateOut struct {
	Task      taskJSON   `json:"task"`
	Warnings  []string   `json:"warnings,omitempty"`
	Unblocked []taskJSON `json:"unblocked,omitempty" jsonschema:"이 변경으로 보류가 자동 해제된 태스크"`
	Next      *taskJSON  `json:"next_occurrence,omitempty" jsonschema:"반복 태스크 완료로 생성된 다음 회차"`
}

type statusArgs struct {
	Ref       string   `json:"ref" jsonschema:"태스크 지정: id, 짧은 번호, 제목 부분일치"`
	Status    string   `json:"status" jsonschema:"todo|doing|blocked|done|cancelled. doing 은 타이머 시작, done 은 반복이면 다음 회차 생성과 후행 보류 해제"`
	Reason    string   `json:"reason,omitempty" jsonschema:"보류(blocked) 사유. blocked 는 reason 또는 blocked_by 없이는 거부됨"`
	BlockedBy []string `json:"blocked_by,omitempty" jsonschema:"선행 태스크 참조 목록. 선행이 전부 끝나면 자동으로 대기중 복귀"`
}

type editArgs struct {
	Ref       string    `json:"ref" jsonschema:"태스크 지정"`
	Title     *string   `json:"title,omitempty"`
	Project   *string   `json:"project,omitempty" jsonschema:"빈 문자열이면 프로젝트 해제"`
	Priority  *string   `json:"priority,omitempty" jsonschema:"P0~P3, 빈 문자열이면 해제"`
	Scheduled *string   `json:"scheduled,omitempty" jsonschema:"YYYY-MM-DD | today | tomorrow | none(해제)"`
	Due       *string   `json:"due,omitempty" jsonschema:"YYYY-MM-DD | today | tomorrow | none(해제)"`
	Estimate  *string   `json:"estimate,omitempty" jsonschema:"30m, 2h 등. 빈 문자열이면 해제"`
	Tags      *[]string `json:"tags,omitempty" jsonschema:"전체 교체"`
	Recur     *string   `json:"recur,omitempty" jsonschema:"반복 규칙, 빈 문자열이면 반복 중단"`
}

type rolloverOut struct {
	Rolled  []taskJSON `json:"rolled"`
	Warning string     `json:"warning,omitempty"`
}

func (s *Server) registerWriteTools() {
	mcp.AddTool(s.mcp, &mcp.Tool{
		Name:        "task_add",
		Description: "태스크 추가. id 채번·생성 로그·WIP 경고가 자동 처리됨. 파일을 직접 만들지 말고 이 도구를 쓸 것.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in addArgs) (*mcp.CallToolResult, mutateOut, error) {
		defer s.begin()()
		today := s.svc.Today()
		ai := service.AddInput{
			Title: in.Title, Project: in.Project,
			Tags: in.Tags, Links: in.Links, Note: in.Note, Recur: in.Recur,
		}
		var err error
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
		return nil, toMutateOut(res, today), nil
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
		var res *service.Result
		if st == domain.StatusBlocked {
			res, err = s.svc.Block(in.Ref, in.Reason, in.BlockedBy)
		} else {
			res, err = s.svc.SetStatus(in.Ref, st, nil)
		}
		if err != nil {
			return nil, mutateOut{}, err
		}
		if err := s.finish(); err != nil {
			return nil, mutateOut{}, err
		}
		return nil, toMutateOut(res, s.svc.Today()), nil
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
		Description: "필드 수정 (부분 갱신 — 지정한 필드만 바뀜). 변경 내역이 태스크 로그에 남음.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in editArgs) (*mcp.CallToolResult, mutateOut, error) {
		defer s.begin()()
		today := s.svc.Today()
		ei := service.EditInput{Title: in.Title, Project: in.Project, Tags: in.Tags, Recur: in.Recur}
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
		res, err := s.svc.Edit(in.Ref, ei)
		if err != nil {
			return nil, mutateOut{}, err
		}
		if err := s.finish(); err != nil {
			return nil, mutateOut{}, err
		}
		return nil, toMutateOut(res, today), nil
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

func toMutateOut(res *service.Result, today domain.Date) mutateOut {
	out := mutateOut{Task: toTaskJSON(res.Task, today), Warnings: res.Warnings}
	for _, u := range res.Unblocked {
		out.Unblocked = append(out.Unblocked, toTaskJSON(u.Task, today))
	}
	if res.Next != nil {
		n := toTaskJSON(res.Next, today)
		out.Next = &n
	}
	return out
}
