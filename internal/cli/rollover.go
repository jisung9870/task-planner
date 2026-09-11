package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"task-planner/internal/service"
)

func newRolloverCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rollover",
		Short: "지난 날짜의 미완료 항목을 오늘로 이월",
		Long: "어제까지 예정이었으나 끝나지 않은 대기중·진행중 항목을 오늘로 옮기고\n" +
			"이월 횟수를 센다. 보류 항목은 남의 응답을 기다리는 중이므로 제외한다.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withService(func(svc *service.Service) error {
				rep, err := svc.Rollover()
				if err != nil {
					return err
				}
				out := cmd.OutOrStdout()
				if rep.Empty() {
					fmt.Fprintln(out, "이월할 항목 없음")
					return nil
				}
				fmt.Fprintf(out, "%d건 이월\n", len(rep.Rolled))
				for _, r := range rep.Rolled {
					mark := ""
					if r.Count >= svc.Cfg.RolloverWarnAt {
						mark = fmt.Sprintf("  ← %d회째, 쪼개거나 버릴 때", r.Count)
					}
					fmt.Fprintf(out, "  %s %s  (%s → 오늘)%s\n",
						r.Task.ShortID(), r.Task.Title, r.From, mark)
				}
				return nil
			})
		},
	}
}
