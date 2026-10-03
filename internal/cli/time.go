package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"task-planner/internal/domain"
	"task-planner/internal/query"
	"task-planner/internal/service"
)

func newTimeCmd() *cobra.Command {
	var (
		week  bool
		since string
		until string
	)
	cmd := &cobra.Command{
		Use:   "time",
		Short: "예상 대비 실소요 집계",
		Long: "진행중 전환 시점부터 벗어날 때까지를 누적한 실소요를 프로젝트별로 모은다.\n" +
			"비율이 1보다 크면 예상이 낙관적이었다는 뜻이고, 그 감각을 교정하는 게 목적이다.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withService(func(svc *service.Service) error {
				var from, to domain.Date
				var err error
				if week {
					from = svc.Today().WeekStart()
					to = from.AddDays(6)
				}
				if since != "" {
					if from, err = parseDateFlag(svc, since); err != nil {
						return err
					}
				}
				if until != "" {
					if to, err = parseDateFlag(svc, until); err != nil {
						return err
					}
				}
				out := cmd.OutOrStdout()
				if running := svc.Running(); running != nil {
					fmt.Fprintf(out, "진행중  %s %s  %s 경과\n\n",
						running.ShortID(), running.Title,
						running.ElapsedLabel(svc.Now(), svc.Cfg.SessionCap))
				}
				rows := svc.TimeSummary(from, to)
				if len(rows) == 0 {
					fmt.Fprintln(out, "집계할 기록 없음")
					return nil
				}
				period := "전체"
				if !from.IsZero() || !to.IsZero() {
					period = fmt.Sprintf("%s ~ %s", from, to)
				}
				fmt.Fprintf(out, "기간: %s\n\n", period)
				fmt.Fprintf(out, "%s %5s %9s %9s %7s\n", pad("프로젝트", 24), "건", "예상", "실소요", "비율")
				for _, r := range rows {
					printTimeRow(cmd, r)
				}
				fmt.Fprintln(out)
				printTimeRow(cmd, query.TotalTime(rows))
				return nil
			})
		},
	}
	cmd.Flags().BoolVar(&week, "week", false, "이번 주만")
	cmd.Flags().StringVar(&since, "since", "", "시작일 (예: -30d)")
	cmd.Flags().StringVar(&until, "until", "", "종료일")
	return cmd
}

func printTimeRow(cmd *cobra.Command, r query.ProjectTime) {
	ratio := "-"
	if v := r.Ratio(); v > 0 {
		ratio = fmt.Sprintf("%.2f", v)
	}
	est := r.Estimate.String()
	if est == "" {
		est = "-"
	}
	act := r.Actual.String()
	if act == "" {
		act = "-"
	}
	fmt.Fprintf(cmd.OutOrStdout(), "%s %5d %9s %9s %7s\n",
		pad(query.ProjectLabel(r.Slug), 24), r.Tasks, est, act, ratio)
}
