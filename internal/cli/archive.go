package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"task-planner/internal/domain"
	"task-planner/internal/service"
)

// defaultArchiveAge is how long finished work stays in tasks/. A month is long
// enough that a mistaken completion is still where you left it, and short
// enough that the directory does not become a year's worth of noise.
const defaultArchiveAge = 30

func newArchiveCmd() *cobra.Command {
	var (
		before string
		days   int
		dryRun bool
	)
	cmd := &cobra.Command{
		Use:   "archive",
		Short: "오래된 완료·취소 항목을 archive/ 로 이동",
		Long: `완료·취소된 지 오래된 태스크를 archive/<연도>-Q<분기>/ 로 옮긴다.

인덱스는 tasks/ 아래를 전부 훑으므로, 끝난 일이 쌓이면 모든 명령이 조금씩
느려진다. 옮겨도 markdown 은 그대로이고 원하면 되돌릴 수 있다.

  tp archive --dry-run          # 무엇이 옮겨질지만 확인
  tp archive --days 90          # 90일 이전 완료분
  tp archive --before 2026-01-01`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return withService(func(svc *service.Service) error {
				cutoff, err := archiveCutoff(svc, cmd, before, days)
				if err != nil {
					return err
				}
				rep, err := svc.Archive(cutoff, dryRun)
				if err != nil {
					return err
				}
				out := cmd.OutOrStdout()
				if rep.Empty() {
					fmt.Fprintf(out, "%s 이전에 끝난 항목이 없습니다.\n", cutoff)
					return nil
				}
				verb := "이동"
				if dryRun {
					verb = "이동 예정"
				}
				fmt.Fprintf(out, "%s 이전 완료분 %d건 %s\n", cutoff, len(rep.Moved), verb)
				for _, m := range rep.Moved {
					fmt.Fprintf(out, "  %s %s  %s\n", m.Task.Status.Glyph(), m.Task.ShortID(), m.Task.Title)
					fmt.Fprintf(out, "      → %s\n", m.To)
				}
				if dryRun {
					fmt.Fprintln(out, "\n--dry-run 이므로 파일은 그대로입니다.")
				}
				return nil
			})
		},
	}
	f := cmd.Flags()
	f.StringVar(&before, "before", "", "이 날짜 이전 완료분 (YYYY-MM-DD | -3m)")
	f.IntVar(&days, "days", defaultArchiveAge, "며칠 이전 완료분을 옮길지")
	f.BoolVar(&dryRun, "dry-run", false, "옮기지 않고 목록만 출력")
	return cmd
}

func archiveCutoff(svc *service.Service, cmd *cobra.Command, before string, days int) (domain.Date, error) {
	if cmd.Flags().Changed("before") {
		d, err := parseDateFlag(svc, before)
		if err != nil {
			return domain.Date{}, err
		}
		if d.IsZero() {
			return domain.Date{}, fmt.Errorf("--before 에 날짜를 지정하세요")
		}
		return d, nil
	}
	if days < 0 {
		return domain.Date{}, fmt.Errorf("--days 는 0 이상이어야 함")
	}
	return svc.Today().AddDays(-days), nil
}
