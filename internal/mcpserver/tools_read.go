package mcpserver

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"task-planner/internal/query"
	"task-planner/internal/service"
)

type taskListOut struct {
	Date  string     `json:"date"`
	Tasks []taskJSON `json:"tasks"`
}

type vaultInfoOut struct {
	Path string `json:"path"`
	Mode string `json:"mode" jsonschema:"shared (한 vault에서 executor 필드로 구분)"`
	// Agents is the table a session needs to turn a task's agent·tier into a
	// model without reading config.yaml itself.
	Agents       []string                     `json:"agents" jsonschema:"태스크에 지정할 수 있는 agent (auto 는 이 중 누구든)"`
	AgentsSource string                       `json:"agents_source" jsonschema:"agents 를 정한 곳: env($TP_AGENTS, 이 장비) | config(vault config.yaml) | detected(PATH 의 CLI) | default"`
	Models       map[string]map[string]string `json:"models" jsonschema:"agent별 tier(fast|standard|deep) → 모델"`
	Session      string                       `json:"session" jsonschema:"task_claim 이 session 생략 시 쓰는 이 연결의 식별자"`
}

type weekArgs struct {
	WeekOf string `json:"week_of,omitempty" jsonschema:"기준 날짜(YYYY-MM-DD). 그 날짜가 포함된 주를 조회. 생략 시 이번 주"`
}

type queryArgs struct {
	Query string `json:"query" jsonschema:"필터 질의. 예: executor:agent status:doing project:infra. 필드: executor agent tier pick status project tag priority due scheduled rollover is id body. 자기 몫 찾기: pick:claude"`
}

type refArgs struct {
	Ref string `json:"ref" jsonschema:"태스크 지정: 전체 id(T-20260912-0001), 짧은 번호(#1 또는 1), 제목 부분일치"`
}

type taskDetailOut struct {
	Task  taskJSON `json:"task"`
	Model string   `json:"model,omitempty" jsonschema:"agent·tier 로 정해진 모델. 둘 중 하나가 열려 있으면 비어 있음"`
	Note  string   `json:"note,omitempty"`
	Log   []string `json:"log,omitempty"`
	Path  string   `json:"path"`
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
	Name    string `json:"name,omitempty"`
	Status  string `json:"status,omitempty" jsonschema:"active|paused|done. 비어 있으면 project.md 가 없는 슬러그"`
	Due     string `json:"due,omitempty" jsonschema:"프로젝트 마감일(마일스톤)"`
	Open    int    `json:"open"`
	Doing   int    `json:"doing"`
	Blocked int    `json:"blocked"`
	Done    int    `json:"done"`
	Overdue int    `json:"overdue"`
	// Progress excludes cancelled work on both sides: abandoning tasks is not
	// progress, and counting it as such would let a project reach 100% by
	// giving up.
	Progress float64 `json:"progress" jsonschema:"완료/(완료+열림). 취소는 제외"`
	Remain   string  `json:"remain_estimate,omitempty" jsonschema:"열린 태스크의 예상 소요 합 (추정치가 있는 것만)"`
}

type projectRowsOut struct {
	Rows []projectRow `json:"rows"`
}

type nextArgs struct {
	Limit int `json:"limit,omitempty" jsonschema:"추천 개수 (기본 3, 0 이면 전부)"`
}

type nextOut struct {
	Task   taskJSON `json:"task"`
	Reason string   `json:"reason" jsonschema:"왜 이게 먼저인지 (마감·우선순위·후행 대기 등)"`
}

type nextRowsOut struct {
	Rows []nextOut `json:"rows"`
}

type loadArgs struct {
	Date string `json:"date,omitempty" jsonschema:"기준 날짜. 생략 시 오늘"`
	Week bool   `json:"week,omitempty" jsonschema:"true 면 그 날짜가 포함된 주의 7일치를 반환"`
}

type loadOut struct {
	Date      string `json:"date"`
	Weekday   string `json:"weekday"`
	Planned   string `json:"planned" jsonschema:"그 날 기간이 걸친 열린 태스크들의 예상 소요 합. 여러 날짜리는 기간으로 나눈 몫만 계산"`
	Limit     string `json:"limit,omitempty" jsonschema:"config 의 daily_capacity"`
	Tasks     int    `json:"tasks"`
	Estimated int    `json:"estimated" jsonschema:"그중 예상 소요가 적힌 건수. planned 는 이 비율만큼만 신뢰할 수 있음"`
	Over      bool   `json:"over,omitempty" jsonschema:"true 면 과다 배정"`
}

type loadRowsOut struct {
	Rows []loadOut `json:"rows"`
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

type timeRowsOut struct {
	Rows []timeRow `json:"rows"`
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
		Name:        "vault_info",
		Description: "연결된 공유 vault 경로를 확인. 에이전트 수행 작업은 task_add executor=agent로 기록한다.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, _ any) (*mcp.CallToolResult, vaultInfoOut, error) {
		ag := s.svc.Cfg.Agents
		out := vaultInfoOut{Path: s.svc.Cfg.Vault, Mode: "shared", Session: s.session,
			AgentsSource: string(ag.Source), Models: map[string]map[string]string{}}
		for _, a := range ag.Allowed {
			out.Agents = append(out.Agents, string(a))
		}
		for a, tiers := range ag.Models {
			m := map[string]string{}
			for tier, model := range tiers {
				m[string(tier)] = model
			}
			out.Models[string(a)] = m
		}
		return nil, out, nil
	})
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
		Description: "질의식으로 태스크 검색. 조건은 AND 로 결합, 앞에 - 를 붙이면 부정. 날짜 값: YYYY-MM-DD, today, +7d, 2w, none, any. body:<문자열> 은 메모·로그 본문까지 찾는다 (파일을 읽으므로 다른 조건과 함께 쓸 것).",
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
			Task:  toTaskJSON(t, s.svc.Today()),
			Model: s.svc.ModelFor(t),
			Note:  t.Note(),
			Log:   t.LogLines(),
			Path:  t.Path,
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
		Description: "프로젝트별 진행 현황: 건수 집계 + 진행률 + 남은 예상 시간 + 프로젝트 마감(마일스톤). 태스크가 아직 없는 프로젝트도 포함됨.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, _ any) (*mcp.CallToolResult, projectRowsOut, error) {
		defer s.begin()()
		list, err := s.svc.ProjectRows()
		if err != nil {
			return nil, projectRowsOut{}, err
		}
		rows := make([]projectRow, len(list))
		for i, r := range list {
			rows[i] = projectRow{
				Project: query.ProjectLabel(r.Slug), Name: r.Name,
				Status: r.Status, Due: r.Due.String(),
				Open: r.Open, Doing: r.Doing, Blocked: r.Blocked,
				Done: r.Done, Overdue: r.Overdue,
				Progress: r.Progress(), Remain: r.Remain.String(),
			}
		}
		return nil, projectRowsOut{Rows: rows}, nil
	})

	mcp.AddTool(s.mcp, &mcp.Tool{
		Name:        "task_next",
		Description: "지금 바로 할 수 있는 일 추천. 선행이 남지 않은 열린 태스크를 급한 순으로, 각각 그 이유와 함께 반환. 보류는 제외 (선행이 끝나면 자동으로 대기중이 되므로, 아직 보류면 이 도구가 모르는 것을 기다리는 중).",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in nextArgs) (*mcp.CallToolResult, nextRowsOut, error) {
		defer s.begin()()
		limit := in.Limit
		if limit == 0 {
			limit = 3
		}
		if limit < 0 {
			limit = 0
		}
		today := s.svc.Today()
		sugg := s.svc.NextUp(limit)
		out := make([]nextOut, len(sugg))
		for i, sg := range sugg {
			out[i] = nextOut{Task: toTaskJSON(sg.Task, today), Reason: sg.Reason}
		}
		return nil, nextRowsOut{Rows: out}, nil
	})

	mcp.AddTool(s.mcp, &mcp.Tool{
		Name:        "day_load",
		Description: "하루(또는 한 주)에 얼마나 잡혀 있는지. 계획을 세우기 전에 이걸로 빈 날을 찾을 것 — 예상 소요가 없는 태스크는 계산에 못 들어가므로 estimated/tasks 비율을 함께 볼 것.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in loadArgs) (*mcp.CallToolResult, loadRowsOut, error) {
		defer s.begin()()
		today := s.svc.Today()
		ref, err := parseDate(in.Date, today)
		if err != nil {
			return nil, loadRowsOut{}, err
		}
		if ref.IsZero() {
			ref = today
		}
		loads := []service.DayLoad{s.svc.DayLoad(ref)}
		if in.Week {
			loads = s.svc.WeekLoad(ref)
		}
		out := make([]loadOut, len(loads))
		for i, l := range loads {
			out[i] = loadOut{
				Date: l.Date.String(), Weekday: l.Date.WeekdayKO(),
				Planned: l.Planned.String(), Limit: l.Limit.String(),
				Tasks: l.Tasks, Estimated: l.Estimated, Over: l.Over(),
			}
		}
		return nil, loadRowsOut{Rows: out}, nil
	})

	mcp.AddTool(s.mcp, &mcp.Tool{
		Name:        "time_summary",
		Description: "프로젝트별 예상(estimate) 대비 실소요(actual) 집계. ratio > 1 이면 예상이 낙관적이었다는 뜻.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in timeArgs) (*mcp.CallToolResult, timeRowsOut, error) {
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
		return nil, timeRowsOut{Rows: out}, nil
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
