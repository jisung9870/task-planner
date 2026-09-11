package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"task-planner/internal/editor"
	"task-planner/internal/service"
)

func newEditCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "edit <태스크>",
		Short: "$EDITOR 로 태스크 파일 열기",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withService(func(svc *service.Service) error {
				t, err := svc.Resolve(args[0])
				if err != nil {
					return err
				}
				ed, err := editor.Command(svc.Cfg, t.Path)
				if err != nil {
					return err
				}
				ed.Stdin, ed.Stdout, ed.Stderr = os.Stdin, os.Stdout, os.Stderr
				if err := ed.Run(); err != nil {
					return fmt.Errorf("편집기 종료 오류: %w", err)
				}
				// The file may now be invalid markdown; reporting that here is
				// better than letting the next command fail obscurely.
				reloaded, err := svc.Load(t.ID)
				if err != nil {
					return fmt.Errorf("편집 후 파일을 읽을 수 없음: %w", err)
				}
				if err := svc.Refresh(reloaded); err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s %s  %s\n",
					reloaded.Status.Glyph(), reloaded.ShortID(), reloaded.Title)
				return nil
			})
		},
	}
}
