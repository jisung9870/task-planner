package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"task-planner/internal/service"
)

func newNextCmd() *cobra.Command {
	var limit int
	cmd := &cobra.Command{
		Use:   "next",
		Short: "지금 바로 할 수 있는 일 추천",
		Long: `막힌 것이 없는 열린 태스크를 급한 순으로 보여준다.

보류는 제외된다 — 선행이 끝나면 자동으로 대기중으로 돌아오므로, 아직 보류인
항목은 이 도구가 모르는 무언가를 기다리는 중이다.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return withService(func(svc *service.Service) error {
				sugg := svc.NextUp(limit)
				out := cmd.OutOrStdout()
				if len(sugg) == 0 {
					fmt.Fprintln(out, "  지금 바로 할 수 있는 일이 없습니다. (전부 완료이거나 보류 중)")
					return nil
				}
				for i, sg := range sugg {
					t := sg.Task
					fmt.Fprintf(out, "%d. %s %s  %s\n", i+1, t.Status.Glyph(), t.ShortID(), t.Title)
					meta := ""
					if t.Project != "" {
						meta = t.Project + "  ·  "
					}
					fmt.Fprintf(out, "     %s%s\n", meta, sg.Reason)
				}
				return nil
			})
		},
	}
	cmd.Flags().IntVarP(&limit, "number", "n", 3, "보여줄 개수 (0 이면 전부)")
	return cmd
}
