package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"task-planner/internal/config"
	"task-planner/internal/domain"
	"task-planner/internal/query"
	"task-planner/internal/service"
)

func newInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "vault 디렉토리와 기본 설정을 생성",
		RunE: func(cmd *cobra.Command, args []string) error {
			vault, err := config.ResolveVault(flagVault)
			if err != nil {
				return err
			}
			cfg := config.Default(vault)
			svc, err := service.Init(cfg)
			if err != nil {
				return err
			}
			defer svc.Close()
			fmt.Fprintf(cmd.OutOrStdout(), "vault 생성: %s\n설정 파일: %s\n",
				vault, vault+"/config.yaml")
			return nil
		},
	}
}

func newAddCmd() *cobra.Command {
	var (
		project, priority, sched, due, estimate, note, recur string
		tags, links                                          []string
		start                                                bool
	)
	cmd := &cobra.Command{
		Use:   "add <제목...>",
		Short: "태스크 추가 (빠른 캡처)",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withService(func(svc *service.Service) error {
				in := service.AddInput{
					Title:   strings.Join(args, " "),
					Project: project,
					Tags:    tags,
					Links:   links,
					Note:    note,
					Recur:   recur,
				}
				var err error
				if in.Priority, err = domain.ParsePriority(priority); err != nil {
					return err
				}
				if in.Scheduled, err = parseDateFlag(svc, sched); err != nil {
					return err
				}
				if in.Due, err = parseDateFlag(svc, due); err != nil {
					return err
				}
				if in.Estimate, err = domain.ParseDuration(estimate); err != nil {
					return err
				}
				if start {
					in.Status = domain.StatusDoing
				}
				res, err := svc.AddWithResult(in)
				if err != nil {
					return err
				}
				t := res.Task
				fmt.Fprintf(cmd.OutOrStdout(), "추가됨 %s  %s\n  %s\n", t.ShortID(), t.Title, t.Path)
				for _, w := range res.Warnings {
					fmt.Fprintln(cmd.ErrOrStderr(), "  주의: "+w)
				}
				return nil
			})
		},
	}
	f := cmd.Flags()
	f.StringVarP(&project, "project", "p", "", "프로젝트 slug")
	f.StringVar(&priority, "priority", "", "우선순위 P0~P3")
	f.StringVarP(&sched, "scheduled", "s", "", "착수 예정일 (YYYY-MM-DD | today | tomorrow | +3d | mon)")
	f.StringVarP(&due, "due", "d", "", "마감일 (동일 형식)")
	f.StringVarP(&estimate, "estimate", "e", "", "예상 소요 (30m, 2h)")
	f.StringSliceVarP(&tags, "tag", "t", nil, "태그 (반복 지정 가능)")
	f.StringSliceVarP(&links, "link", "l", nil, "외부 링크 (jira:ABC-123 등)")
	f.StringVarP(&note, "note", "n", "", "메모 본문")
	f.StringVar(&recur, "recur", "", "반복 규칙 (daily, weekly, every monday ...)")
	f.BoolVar(&start, "start", false, "추가와 동시에 진행중으로")
	return cmd
}

func newTodayCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "today",
		Short: "오늘 해야 할 일",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withService(func(svc *service.Service) error {
				today := svc.Today()
				fmt.Fprintf(cmd.OutOrStdout(), "%s (%s)%s\n", today, today.WeekdayKO(),
					loadSuffix(svc.DayLoad(today)))
				renderList(cmd.OutOrStdout(), svc.TodayList(), today, svc.Cfg.DueSoonDays)
				return nil
			})
		},
	}
}

func newWeekCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "week",
		Short: "이번 주 진행해야 할 일",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withService(func(svc *service.Service) error {
				today := svc.Today()
				start := today.WeekStart()
				fmt.Fprintf(cmd.OutOrStdout(), "%s  (%s ~ %s)\n",
					today.WeekLabel(), start, start.AddDays(6))
				renderWeekLoad(cmd.OutOrStdout(), svc.WeekLoad(today), today)
				renderList(cmd.OutOrStdout(), svc.WeekList(today), today, svc.Cfg.DueSoonDays)
				return nil
			})
		},
	}
}

func newListCmd() *cobra.Command {
	var (
		project string
		all     bool
	)
	cmd := &cobra.Command{
		Use:     "list [질의]",
		Aliases: []string{"ls", "q"},
		Short:   "태스크 목록 / 검색",
		Long: `태스크 목록. 인자를 주면 질의로 해석한다.

  tp list status:doing project:infra
  tp list due<7d -status:done
  tp list is:overdue
  tp list is:carried rollover>2
  tp list 파이프라인            # 제목·프로젝트·태그 부분일치
  tp list body:타임아웃         # 메모·로그 본문까지 검색 (파일을 읽는다)

  필드   status project tag priority due scheduled rollover is id body
  연산   : 같음   < <= > >= 비교 (날짜·숫자)
  날짜   2026-09-15  today  tomorrow  +7d  2w  1m  none  any
  is     open closed overdue duesoon blocked carried unscheduled recurring
  부정   앞에 - 또는 ! 를 붙인다 (-status:done, !is:carried)

플래그는 질의보다 앞에 온다: tp list -p infra due<7d`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return withService(func(svc *service.Service) error {
				today := svc.Today()
				var ts []*domain.Task
				switch {
				case project != "":
					ts = svc.ProjectList(project, all)
				case all:
					ts = svc.All()
					domain.SortDefault(ts, today)
				default:
					ts = svc.OpenList()
				}
				if expr := strings.Join(args, " "); strings.TrimSpace(expr) != "" {
					// An explicit query overrides the default "open only" scope:
					// `tp list status:done` must be able to find completed work.
					if !all && project == "" {
						ts = svc.All()
					}
					f, err := svc.Filter(expr)
					if err != nil {
						return err
					}
					ts = svc.ApplyFilter(f, ts)
				}
				renderList(cmd.OutOrStdout(), ts, today, svc.Cfg.DueSoonDays)
				return nil
			})
		},
	}
	cmd.Flags().StringVarP(&project, "project", "p", "", "프로젝트로 한정")
	cmd.Flags().BoolVarP(&all, "all", "a", false, "완료·취소 포함")
	// Stop flag parsing at the first positional argument so a negated term like
	// `-status:done` reaches the query parser instead of pflag.
	cmd.Flags().SetInterspersed(false)
	return cmd
}

func newShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <태스크>",
		Short: "태스크 상세",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withService(func(svc *service.Service) error {
				t, err := svc.Load(args[0])
				if err != nil {
					return err
				}
				raw, err := os.ReadFile(t.Path)
				if err != nil {
					return err
				}
				out := cmd.OutOrStdout()
				fmt.Fprintf(out, "%s\n\n%s", t.Path, raw)
				printDeps(out, svc, t)
				return nil
			})
		},
	}
}

// newStatusCmds builds the one-shot transition commands. They share a body
// because the only difference is the target state.
func newStatusCmds() []*cobra.Command {
	simple := []struct {
		use, short string
		to         domain.Status
	}{
		{"start <태스크>", "진행중으로 전환", domain.StatusDoing},
		{"done <태스크>", "완료 처리", domain.StatusDone},
		{"cancel <태스크>", "취소 처리", domain.StatusCancelled},
		{"reopen <태스크>", "대기중으로 되돌림", domain.StatusTodo},
	}
	cmds := make([]*cobra.Command, 0, len(simple)+1)
	for _, sc := range simple {
		to := sc.to
		cmds = append(cmds, &cobra.Command{
			Use:   sc.use,
			Short: sc.short,
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				return withService(func(svc *service.Service) error {
					res, err := svc.SetStatus(args[0], to, nil)
					if err != nil {
						return err
					}
					printResult(cmd, res)
					return nil
				})
			},
		})
	}

	skip := &cobra.Command{
		Use:   "skip <태스크>",
		Short: "반복 태스크의 이번 회차를 건너뛰고 다음 회차 생성",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withService(func(svc *service.Service) error {
				res, err := svc.Skip(args[0])
				if err != nil {
					return err
				}
				printResult(cmd, res)
				return nil
			})
		},
	}
	cmds = append(cmds, skip)

	var by []string
	block := &cobra.Command{
		Use:   "block <태스크> [사유]",
		Short: "보류 처리 (사유 또는 --by 필수)",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withService(func(svc *service.Service) error {
				res, err := svc.Block(args[0], strings.Join(args[1:], " "), by)
				if err != nil {
					return err
				}
				printResult(cmd, res)
				return nil
			})
		},
	}
	block.Flags().StringSliceVar(&by, "by", nil, "선행 태스크 id (완료되면 해제 대상)")
	return append(cmds, block)
}

// printDeps renders both directions of the dependency edge. Seeing what a task
// blocks is what turns a finished item into a prompt to move the next one.
func printDeps(w io.Writer, svc *service.Service, t *domain.Task) {
	known, missing := svc.Blockers(t)
	if len(known)+len(missing) > 0 {
		fmt.Fprintln(w, "\n선행 (이 태스크가 기다리는 것)")
		for _, d := range known {
			fmt.Fprintf(w, "  %s %s  %s\n", d.Status.Glyph(), d.ShortID(), d.Title)
		}
		for _, id := range missing {
			fmt.Fprintf(w, "  ? %s  (인덱스에 없음)\n", id)
		}
	}
	if blocking := svc.Blocking(t.ID); len(blocking) > 0 {
		fmt.Fprintln(w, "\n후행 (이 태스크를 기다리는 것)")
		for _, d := range blocking {
			fmt.Fprintf(w, "  %s %s  %s\n", d.Status.Glyph(), d.ShortID(), d.Title)
		}
	}
}

func newProjectsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "projects",
		Aliases: []string{"proj"},
		Short:   "프로젝트별 진행 현황",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withService(func(svc *service.Service) error {
				rows, err := svc.ProjectRows()
				if err != nil {
					return err
				}
				if len(rows) == 0 {
					fmt.Fprintln(cmd.OutOrStdout(), "  (없음)  `tp projects new <slug>` 로 추가")
					return nil
				}
				out := cmd.OutOrStdout()
				today := svc.Today()
				fmt.Fprintf(out, "%s %s %s %s %s %s %s %s\n",
					pad("프로젝트", 18), pad("상태", 8), pad("진행률", 14),
					padLeft("열림", 5), padLeft("진행중", 6), padLeft("완료", 5),
					padLeft("남은예상", 9), "마감")
				for _, r := range rows {
					fmt.Fprintf(out, "%s %s %s %s %s %s %s %s\n",
						pad(query.ProjectLabel(r.Slug), 18), pad(projectStatusLabel(r), 8),
						pad(progressBar(r.Progress(), 5), 14),
						padLeft(fmt.Sprint(r.Open), 5), padLeft(fmt.Sprint(r.Doing), 6),
						padLeft(fmt.Sprint(r.Done), 5), padLeft(r.Remain.String(), 9),
						projectDueLabel(r, today))
				}
				return nil
			})
		},
	}
	cmd.AddCommand(newProjectNewCmd(), newProjectSetCmd())
	return cmd
}

// progressBar renders done-vs-open as a fixed-width bar. The number alone gets
// skimmed past; the bar is what makes a stalled project visible in a list.
func progressBar(ratio float64, width int) string {
	filled := int(ratio*float64(width) + 0.5)
	if filled > width {
		filled = width
	}
	return strings.Repeat("▰", filled) + strings.Repeat("▱", width-filled) +
		fmt.Sprintf(" %3.0f%%", ratio*100)
}

// projectDueLabel shows the milestone as remaining days, which is the form the
// answer is needed in ("2주 남았다"), with the date for anything further out.
func projectDueLabel(r service.ProjectRow, today domain.Date) string {
	if r.Due.IsZero() {
		return ""
	}
	d := r.Due.DaysUntil(today)
	switch {
	case d < 0:
		return fmt.Sprintf("%s (%d일 초과)", r.Due, -d)
	case d == 0:
		return fmt.Sprintf("%s (오늘)", r.Due)
	case d <= 30:
		return fmt.Sprintf("%s (D-%d)", r.Due, d)
	}
	return r.Due.String()
}

// projectStatusLabel distinguishes a project with a file from a slug that only
// exists on tasks - the latter has no status to report.
func projectStatusLabel(r service.ProjectRow) string {
	if !r.Defined {
		return "-"
	}
	if r.Status == "" {
		return "active"
	}
	return r.Status
}

func newProjectNewCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "new <slug> [이름...]",
		Aliases: []string{"add"},
		Short:   "프로젝트 생성 (projects/<slug>/project.md)",
		Args:    cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withService(func(svc *service.Service) error {
				p, err := svc.CreateProject(args[0], strings.Join(args[1:], " "))
				if err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "프로젝트 생성 %s  %s\n  %s\n", p.Slug, p.Display(), p.Path)
				return nil
			})
		},
	}
}

func newProjectSetCmd() *cobra.Command {
	var name, status, owner, due string
	cmd := &cobra.Command{
		Use:   "set <slug> [플래그]",
		Short: "프로젝트 메타 수정 (상태·마감·담당)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withService(func(svc *service.Service) error {
				f := cmd.Flags()
				in := service.ProjectEditInput{}
				if f.Changed("name") {
					v := strings.TrimSpace(name)
					in.Name = &v
				}
				if f.Changed("status") {
					v := strings.ToLower(strings.TrimSpace(status))
					in.Status = &v
				}
				if f.Changed("owner") {
					v := clearValue(owner)
					in.Owner = &v
				}
				if f.Changed("due") {
					d, err := parseDateFlag(svc, due)
					if err != nil {
						return err
					}
					in.Due = &d
				}
				p, err := svc.EditProject(args[0], in)
				if err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s  %s  상태 %s", p.Slug, p.Display(), p.Status)
				if !p.Due.IsZero() {
					fmt.Fprintf(cmd.OutOrStdout(), "  마감 %s", p.Due)
				}
				fmt.Fprintln(cmd.OutOrStdout())
				return nil
			})
		},
	}
	f := cmd.Flags()
	f.StringVar(&name, "name", "", "표시 이름")
	f.StringVar(&status, "status", "", "active | paused | done")
	f.StringVar(&owner, "owner", "", "담당 (none 이면 해제)")
	f.StringVar(&due, "due", "", "프로젝트 마감일 (YYYY-MM-DD | +2w | none)")
	return cmd
}

func newIndexCmd() *cobra.Command {
	var rebuild bool
	cmd := &cobra.Command{
		Use:   "index",
		Short: "인덱스 상태 확인 / 재생성",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withService(func(svc *service.Service) error {
				st := svc.IndexStats()
				if rebuild {
					var err error
					if st, err = svc.Rebuild(); err != nil {
						return err
					}
				}
				fmt.Fprintf(cmd.OutOrStdout(),
					"vault     %s\n총 태스크 %d (열림 %d)\n재파싱    %d개\n제거      %d개\n소요      %s\n빌드시각  %s\n",
					svc.Cfg.Vault, st.Total, st.Open, st.Scanned, st.Removed,
					st.Duration.Round(1e6), st.BuiltAt.Format("2006-01-02 15:04:05"))
				return nil
			})
		},
	}
	cmd.Flags().BoolVar(&rebuild, "rebuild", false, "인덱스를 버리고 전체 재생성")
	return cmd
}

func printResult(cmd *cobra.Command, res *service.Result) {
	fmt.Fprintf(cmd.OutOrStdout(), "%s %s  %s → %s\n",
		res.Task.Status.Glyph(), res.Task.ShortID(), res.Task.Title, res.Task.Status.Label())
	if res.Next != nil {
		fmt.Fprintf(cmd.OutOrStdout(), "  ↻ 다음 회차 %s  %s (%s)\n",
			res.Next.ShortID(), res.Next.Title, res.Next.Scheduled)
	}
	for _, u := range res.Unblocked {
		fmt.Fprintf(cmd.OutOrStdout(), "  ↑ %s %s  보류 해제\n", u.Task.ShortID(), u.Task.Title)
	}
	for _, w := range res.Warnings {
		if strings.Contains(w, "보류 해제") {
			continue // already printed above
		}
		fmt.Fprintln(cmd.ErrOrStderr(), "  주의: "+w)
	}
}
