// Package cli is the command-line adapter. Like every other adapter it talks
// only to internal/service.
package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"task-planner/internal/config"
	"task-planner/internal/service"
)

var (
	flagVault string
	// Version is overridden at build time via -ldflags.
	Version = "dev"
)

// Execute runs the root command.
func Execute() int {
	root := newRoot()
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "오류:", err)
		return 1
	}
	return 0
}

func newRoot() *cobra.Command {
	root := &cobra.Command{
		Use:   "tp",
		Short: "markdown 기반 개인 업무 관리",
		Long: "tp — markdown 파일이 원천인 개인 업무 관리 도구.\n" +
			"인자 없이 실행하면 TUI 가 열립니다.",
		Version:       Version,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTUI(cmd)
		},
	}
	root.PersistentFlags().StringVar(&flagVault, "vault", "",
		"vault 경로 (기본: $"+config.EnvVault+" 또는 ~/"+config.DefaultVaultDir+")")

	root.AddCommand(
		newInitCmd(),
		newAddCmd(),
		newTodayCmd(),
		newWeekCmd(),
		newListCmd(),
		newShowCmd(),
		newEditCmd(),
		newRolloverCmd(),
		newReportCmd(),
		newGitCmd(),
		newProjectsCmd(),
		newIndexCmd(),
	)
	root.AddCommand(newStatusCmds()...)
	return root
}

// open resolves the vault and returns a ready service. Every command that
// touches data starts here.
func open() (*service.Service, error) {
	vault, err := config.ResolveVault(flagVault)
	if err != nil {
		return nil, err
	}
	cfg, err := config.Load(vault)
	if err != nil {
		return nil, err
	}
	return service.Open(cfg)
}

// withService runs fn against an open service and always flushes the index.
func withService(fn func(*service.Service) error) error {
	svc, err := open()
	if err != nil {
		return err
	}
	defer svc.Close()
	return fn(svc)
}
