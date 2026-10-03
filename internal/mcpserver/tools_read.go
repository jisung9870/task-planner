package mcpserver

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"task-planner/internal/domain"
	"task-planner/internal/query"
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
	// Started/Finished/Elapsed are the 걸린 기간 read from the log: the first
	// time work started and the last completion. Empty when not recorded.
	Started  string `json:"started,omitempty" jsonschema:"처음 진행중이 된 시각 (착수 소급 포함)"`
	Finished string `json:"finished,omitempty" jsonschema:"마지막 완료 시각"`
	Elapsed  string `json:"elapsed,omitempty" jsonschema:"걸린 기간 = 완료 − 착수 (달력 기준, 중단 포함). 진행중이면 지금까지, 멈춘 채 완료 전이면 비어 있음"`
}

type summaryOut struct {
	Date          string `json:"date"`
	Doing         int    `json:"doing"`
	WIPLimit      int    `json:"wip_limit"`
	Blocked       int    `json:"blocked"`
	BlockedMaxDay int    `json:"blocked_max_days"`
	Stale         int    `json:"stale" jsonschema:"꺼낸 뒤 stale_days 동안 착수하지 않았거나 그만큼 진행중인 열린 일"`
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
	Stale   int    `json:"stale" jsonschema:"꺼낸 뒤 오래 착수하지 않았거나 오래 진행중인 열린 일"`
	// Progress excludes cancelled work on both sides: abandoning tasks is not
	// progress, and counting it as such would let a project reach 100% by
	// giving up.
	Progress float64 `json:"progress" jsonschema:"완료/(완료+열림). 취소는 제외"`
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

type timeArgs struct {
	Period string `json:"period,omitempty" jsonschema:"집계 기간: week(이번 주) 또는 all(전체, 기본). 그 밖의 값은 오류. 실소요는 분 단위로 합산"`
}

type timeRow struct {
	Project string `json:"project"`
	Tasks   int    `json:"tasks"`
	Actual  string `json:"actual,omitempty" jsonschema:"작업 시간 합 (세션마다 session_cap 상한)"`
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
		out := taskDetailOut{
			Task:  toTaskJSON(t, s.svc.Today()),
			Model: s.svc.ModelFor(t),
			Note:  t.Note(),
			Log:   t.LogLines(),
			Path:  t.Path,
		}
		now := s.svc.Now()
		h := t.WorkHistory(now.Location())
		if start, end, done := h.Lead(); !start.IsZero() {
			out.Started = start.Format("2006-01-02 15:04")
			switch {
			case done:
				out.Finished = end.Format("2006-01-02 15:04")
				out.Elapsed = domain.SpanText(end.Sub(start))
			case h.Running():
				out.Elapsed = domain.SpanText(now.Sub(start))
			}
		}
		return nil, out, nil
	})

	mcp.AddTool(s.mcp, &mcp.Tool{
		Name:        "summary",
		Description: "브리핑 요약: 진행중(WIP)/보류(최장 경과일)/오래 멈춘 일 건수.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, _ any) (*mcp.CallToolResult, summaryOut, error) {
		defer s.begin()()
		sum := s.svc.Summarize()
		return nil, summaryOut{
			Date:          s.svc.Today().String(),
			Doing:         sum.Doing,
			WIPLimit:      sum.WIPLimit,
			Blocked:       sum.Blocked,
			BlockedMaxDay: sum.BlockedMaxDay,
			Stale:         sum.Stale,
		}, nil
	})

	mcp.AddTool(s.mcp, &mcp.Tool{
		Name:        "project_status",
		Description: "프로젝트별 진행 현황: 건수 집계 + 진행률 + 오래 멈춘 일 + 프로젝트 마감(마일스톤). 태스크가 아직 없는 프로젝트도 포함됨.",
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
				Done: r.Done, Stale: r.Stale,
				Progress: r.Progress(),
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
		Name:        "time_summary",
		Description: "프로젝트별 작업 시간(actual) 집계. 한 태스크가 언제 시작해 얼마나 걸렸는지는 task_get 의 started·finished·elapsed.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in timeArgs) (*mcp.CallToolResult, timeRowsOut, error) {
		defer s.begin()()
		// Same periods as `tp time`: no bounds for all, Monday~Sunday for week.
		// Anything else is an error - a "month" silently read as everything
		// answers a question nobody asked.
		var from, to domain.Date
		switch in.Period {
		case "", "all":
		case "week":
			from = s.svc.Today().WeekStart()
			to = from.AddDays(6)
		default:
			return nil, timeRowsOut{}, fmt.Errorf("period 는 week 또는 all 이어야 함: %q", in.Period)
		}
		rows := s.svc.TimeSummary(from, to)
		out := make([]timeRow, len(rows))
		for i, r := range rows {
			out[i] = timeRow{
				Project: query.ProjectLabel(r.Slug), Tasks: r.Tasks,
				Actual: r.Actual.String(),
			}
		}
		return nil, timeRowsOut{Rows: out}, nil
	})

	mcp.AddTool(s.mcp, &mcp.Tool{
		Name:        "report_week",
		Description: "주간 리포트 markdown 생성 (완료·진행중은 착수·걸린 기간·작업 시간과 함께, 보류/오래 멈춤/다음 주 꺼낼 일, 프로젝트별 그룹). 주간보고 초안의 출발점.",
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
