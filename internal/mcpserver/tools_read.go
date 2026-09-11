package mcpserver

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"task-planner/internal/query"
)

type taskListOut struct {
	Date  string     `json:"date"`
	Tasks []taskJSON `json:"tasks"`
}

type weekArgs struct {
	WeekOf string `json:"week_of,omitempty" jsonschema:"기준 날짜(YYYY-MM-DD). 그 날짜가 포함된 주를 조회. 생략 시 이번 주"`
}

type queryArgs struct {
	Query string `json:"query" jsonschema:"필터 질의. 예: status:doing project:infra due<7d is:overdue -status:done 자유어. 필드: status project tag priority due scheduled rollover is id"`
}

type refArgs struct {
	Ref string `json:"ref" jsonschema:"태스크 지정: 전체 id(T-20260912-0001), 짧은 번호(#1 또는 1), 제목 부분일치"`
}

type taskDetailOut struct {
	Task taskJSON `json:"task"`
	Note string   `json:"note,omitempty"`
	Log  []string `json:"log,omitempty"`
	Path string   `json:"path"`
}

type summaryOut struct {
	Date          string `json:"date"`
	DueToday      int    `json:"due_today"`
	Overdue       int    `json:"overdue"`
	Doing         int    `json:"doing"`
	WIPLimit      int    `json:"wip_limit"`
	Blocked       int    `json:"blocked"`
	BlockedMaxDay int    `json:"blocked_max_days"`
	Carried       int    `json:"carried"`
}

type projectRow struct {
	Project string `json:"project"`
	Open    int    `json:"open"`
	Doing   int    `json:"doing"`
	Blocked int    `json:"blocked"`
	Done    int    `json:"done"`
	Overdue int    `json:"overdue"`
}

type timeArgs struct {
	Period string `json:"period,omitempty" jsonschema:"집계 기간: week(이번 주) 또는 all(전체, 기본)"`
}

type timeRow struct {
	Project  string  `json:"project"`
	Tasks    int     `json:"tasks"`
	Estimate string  `json:"estimate,omitempty"`
	Actual   string  `json:"actual,omitempty"`
	Ratio    float64 `json:"ratio,omitempty"`
}

type reportArgs struct {
	WeekOf string `json:"week_of,omitempty" jsonschema:"기준 날짜(YYYY-MM-DD). 생략 시 이번 주"`
	Save   bool   `json:"save,omitempty" jsonschema:"true 면 vault 의 reports/ 에 파일로도 저장"`
}

type reportOut struct {
	Week     string `json:"week"`
	Markdown string `json:"markdown"`
	Path     string `json:"path,omitempty"`
}

func (s *Server) registerReadTools() {
	mcp.AddTool(s.mcp, &mcp.Tool{
		Name:        "task_today",
		Description: "오늘 해야 할 일. 진행중이거나, 오늘까지 예정(scheduled)이거나, 오늘까지 마감(due)인 태스크와 오늘 완료분.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, _ any) (*mcp.CallToolResult, taskListOut, error) {
		defer s.begin()()
		today := s.svc.Today()
		return nil, taskListOut{Date: today.String(), Tasks: toTaskList(s.svc.TodayList(), today)}, nil
	})

	mcp.AddTool(s.mcp, &mcp.Tool{
		Name:        "task_week",
		Description: "해당 주에 진행해야 할 일 (예정·마감이 그 주에 있거나, 진행중이거나, 마감 초과).",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in weekArgs) (*mcp.CallToolResult, taskListOut, error) {
		defer s.begin()()
		today := s.svc.Today()
		ref, err := parseDate(in.WeekOf, today)
		if err != nil {
			return nil, taskListOut{}, err
		}
		if ref.IsZero() {
			ref = today
		}
		return nil, taskListOut{Date: ref.WeekLabel(), Tasks: toTaskList(s.svc.WeekList(ref), today)}, nil
	})

	mcp.AddTool(s.mcp, &mcp.Tool{
		Name:        "task_query",
		Description: "질의식으로 태스크 검색. 조건은 AND 로 결합, 앞에 - 를 붙이면 부정. 날짜 값: YYYY-MM-DD, today, +7d, 2w, none, any.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in queryArgs) (*mcp.CallToolResult, taskListOut, error) {
		defer s.begin()()
		ts, err := s.svc.Query(in.Query)
		if err != nil {
			return nil, taskListOut{}, err
		}
		today := s.svc.Today()
		return nil, taskListOut{Date: today.String(), Tasks: toTaskList(ts, today)}, nil
	})

	mcp.AddTool(s.mcp, &mcp.Tool{
		Name:        "task_get",
		Description: "태스크 상세: 전체 필드 + 메모(Note) + 상태 전이 이력(Log).",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in refArgs) (*mcp.CallToolResult, taskDetailOut, error) {
		defer s.begin()()
		t, err := s.svc.Load(in.Ref)
		if err != nil {
			return nil, taskDetailOut{}, err
		}
		return nil, taskDetailOut{
			Task: toTaskJSON(t, s.svc.Today()),
			Note: t.Note(),
			Log:  t.LogLines(),
			Path: t.Path,
		}, nil
	})

	mcp.AddTool(s.mcp, &mcp.Tool{
		Name:        "summary",
		Description: "브리핑 요약: 오늘 마감/마감 초과/진행중(WIP)/보류(최장 경과일)/이월 건수.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, _ any) (*mcp.CallToolResult, summaryOut, error) {
		defer s.begin()()
		sum := s.svc.Summarize()
		return nil, summaryOut{
			Date:          s.svc.Today().String(),
			DueToday:      sum.DueToday,
			Overdue:       sum.Overdue,
			Doing:         sum.Doing,
			WIPLimit:      sum.WIPLimit,
			Blocked:       sum.Blocked,
			BlockedMaxDay: sum.BlockedMaxDay,
			Carried:       sum.Carried,
		}, nil
	})

	mcp.AddTool(s.mcp, &mcp.Tool{
		Name:        "project_status",
		Description: "프로젝트별 진행 현황 집계 (열림/진행중/보류/완료/마감초과).",
	}, func(ctx context.Context, req *mcp.CallToolRequest, _ any) (*mcp.CallToolResult, []projectRow, error) {
		defer s.begin()()
		counts := s.svc.ProjectCounts()
		rows := make([]projectRow, len(counts))
		for i, c := range counts {
			rows[i] = projectRow{
				Project: query.ProjectLabel(c.Slug),
				Open:    c.Open, Doing: c.Doing, Blocked: c.Blocked,
				Done: c.Done, Overdue: c.Overdue,
			}
		}
		return nil, rows, nil
	})

	mcp.AddTool(s.mcp, &mcp.Tool{
		Name:        "time_summary",
		Description: "프로젝트별 예상(estimate) 대비 실소요(actual) 집계. ratio > 1 이면 예상이 낙관적이었다는 뜻.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in timeArgs) (*mcp.CallToolResult, []timeRow, error) {
		defer s.begin()()
		var from, to = s.svc.Today(), s.svc.Today()
		if in.Period == "week" {
			from = from.WeekStart()
			to = from.AddDays(6)
		} else {
			from, to = from.AddDays(-365*10), to.AddDays(1)
		}
		rows := s.svc.TimeSummary(from, to)
		out := make([]timeRow, len(rows))
		for i, r := range rows {
			out[i] = timeRow{
				Project: query.ProjectLabel(r.Slug), Tasks: r.Tasks,
				Estimate: r.Estimate.String(), Actual: r.Actual.String(), Ratio: r.Ratio(),
			}
		}
		return nil, out, nil
	})

	mcp.AddTool(s.mcp, &mcp.Tool{
		Name:        "report_week",
		Description: "주간 리포트 markdown 생성 (완료/진행중/보류/이월/다음 주 예정, 프로젝트별 그룹). 주간보고 초안의 출발점.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in reportArgs) (*mcp.CallToolResult, reportOut, error) {
		defer s.begin()()
		today := s.svc.Today()
		ref, err := parseDate(in.WeekOf, today)
		if err != nil {
			return nil, reportOut{}, err
		}
		if ref.IsZero() {
			ref = today
		}
		out := reportOut{Week: ref.WeekLabel(), Markdown: s.svc.WeekReport(ref)}
		if in.Save {
			if out.Path, err = s.svc.WriteWeekReport(ref); err != nil {
				return nil, reportOut{}, err
			}
		}
		return nil, out, nil
	})
}
