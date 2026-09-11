package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"task-planner/internal/domain"
	"task-planner/internal/service"
)

func newReportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "report",
		Short: "리포트 생성",
	}
	cmd.AddCommand(newReportWeekCmd())
	return cmd
}

func newReportWeekCmd() *cobra.Command {
	var (
		toStdout bool
		weekOf   string
	)
	c := &cobra.Command{
		Use:   "week",
		Short: "주간 리포트 생성 (reports/YYYY-Www.md)",
		Long: "완료·진행중·보류·이월·다음 주 예정을 프로젝트별로 묶어 markdown 으로 만든다.\n" +
			"주간보고 초안의 출발점이며, 그대로 제출하는 문서가 아니다.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withService(func(svc *service.Service) error {
				ref := domain.Date{}
				if weekOf != "" {
					var err error
					if ref, err = parseDateFlag(svc, weekOf); err != nil {
						return err
					}
				}
				if toStdout {
					fmt.Fprint(cmd.OutOrStdout(), svc.WeekReport(ref))
					return nil
				}
				path, err := svc.WriteWeekReport(ref)
				if err != nil {
					return err
				}
				fmt.Fprintln(cmd.OutOrStdout(), path)
				return nil
			})
		},
	}
	c.Flags().BoolVar(&toStdout, "stdout", false, "파일로 쓰지 않고 표준출력으로")
	c.Flags().StringVar(&weekOf, "week-of", "", "해당 날짜가 포함된 주 (기본: 이번 주, 예: -7d)")
	return c
}
