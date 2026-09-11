package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"task-planner/internal/gitsync"
	"task-planner/internal/service"
)

func newGitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "git",
		Short: "vault 의 git 저장소 관리",
		Long: "vault 를 git 저장소로 두면 이력·백업·기기 간 동기화가 한 번에 해결된다.\n" +
			"config.yaml 의 git.auto_commit 을 켜면 세션마다 자동으로 커밋한다.",
	}

	initCmd := &cobra.Command{
		Use:   "init",
		Short: "vault 를 git 저장소로 만들고 .index/ 를 무시 목록에 추가",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withService(func(svc *service.Service) error {
				if err := svc.GitInit(); err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "git 저장소 준비됨: %s\n", svc.Cfg.Vault)
				if !svc.Cfg.Git.AutoCommit {
					fmt.Fprintln(cmd.OutOrStdout(),
						"자동 커밋을 켜려면 config.yaml 에서 git.auto_commit: true 로 설정하세요")
				}
				return nil
			})
		},
	}

	var message string
	syncCmd := &cobra.Command{
		Use:   "sync",
		Short: "지금 커밋 (설정과 무관하게 즉시 실행)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withService(func(svc *service.Service) error {
				committed, err := svc.GitCommitNow(message)
				if err != nil {
					return err
				}
				if !committed {
					fmt.Fprintln(cmd.OutOrStdout(), "변경 없음")
					return nil
				}
				fmt.Fprintln(cmd.OutOrStdout(), "커밋 완료")
				return nil
			})
		},
	}
	syncCmd.Flags().StringVarP(&message, "message", "m", "", "커밋 메시지")

	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "자동 커밋 설정 상태",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withService(func(svc *service.Service) error {
				fmt.Fprintf(cmd.OutOrStdout(),
					"vault        %s\ngit 설치     %v\n저장소       %v\n자동 커밋    %v\n자동 push    %v (%s)\n",
					svc.Cfg.Vault, gitsync.Available(), gitsync.IsRepo(svc.Cfg.Vault),
					svc.Cfg.Git.AutoCommit, svc.Cfg.Git.AutoPush, svc.Cfg.Git.Remote)
				return nil
			})
		},
	}

	cmd.AddCommand(initCmd, syncCmd, statusCmd)
	return cmd
}
