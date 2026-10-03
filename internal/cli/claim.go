package cli

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"github.com/spf13/cobra"

	"task-planner/internal/domain"
	"task-planner/internal/service"
)

func newClaimCmd() *cobra.Command {
	var agent, session string
	cmd := &cobra.Command{
		Use:   "claim [태스크] --agent <agent>",
		Short: "agent 실행이 일을 가져감 (진행중으로)",
		Long: `한 실행이 일을 가져가 claimed_by 에 기록하고 진행중으로 바꾼다.
태스크를 생략하면 그 agent 몫(agent 가 같거나 auto) 중 가장 급한 일을 고른다.
다른 실행이 이미 가져간 일은 거부한다.

  tp claim --agent codex --session build-42
  tp claim 291 --agent claude`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withService(func(svc *service.Service) error {
				a, err := domain.ParseAgent(agent)
				if err != nil {
					return err
				}
				if session == "" {
					session = "cli-" + randomHex()
				}
				in := service.ClaimInput{Agent: a, Session: session}
				if len(args) == 1 {
					in.Ref = args[0]
				}
				res, err := svc.Claim(in)
				if err != nil {
					return err
				}
				t := res.Task
				out := cmd.OutOrStdout()
				fmt.Fprintf(out, "가져감 %s  %s\n  claimed_by: %s\n", t.ShortID(), t.Title, t.ClaimedBy)
				if m := svc.ModelFor(t); m != "" {
					fmt.Fprintf(out, "  모델: %s (%s)\n", m, t.Tier)
				} else {
					fmt.Fprintln(out, "  모델: tier 미정 — 판단해 `tp set --tier` 로 남길 것")
				}
				for _, w := range res.Warnings {
					fmt.Fprintln(cmd.ErrOrStderr(), "  주의: "+w)
				}
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&agent, "agent", "", "가져가는 agent (필수)")
	cmd.Flags().StringVar(&session, "session", "", "claimed_by 에 남길 세션 식별자 (생략 시 cli-<무작위>)")
	_ = cmd.MarkFlagRequired("agent")
	return cmd
}

func newReleaseCmd() *cobra.Command {
	var agent, session string
	var force bool
	cmd := &cobra.Command{
		Use:   "release <태스크>",
		Short: "가져간 일을 놓음 (진행중이면 대기중으로)",
		Long: `가져간 실행만 놓을 수 있다. 끝난 세션이 남긴 claim 은 확인한 뒤 --force.

  tp release 291 --agent codex --session build-42
  tp release 291 --force`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withService(func(svc *service.Service) error {
				by := ""
				if !force {
					a, err := domain.ParseAgent(agent)
					if err != nil {
						return err
					}
					if by, err = domain.ClaimRef(a, session); err != nil {
						return fmt.Errorf("%w — --agent·--session 을 주거나 --force", err)
					}
				}
				res, err := svc.Release(args[0], by, force)
				if err != nil {
					return err
				}
				t := res.Task
				fmt.Fprintf(cmd.OutOrStdout(), "%s %s  %s  (놓음)\n", t.Status.Glyph(), t.ShortID(), t.Title)
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&agent, "agent", "", "가져간 agent")
	cmd.Flags().StringVar(&session, "session", "", "가져간 세션 식별자")
	cmd.Flags().BoolVar(&force, "force", false, "다른 실행의 claim 도 해제")
	return cmd
}

func randomHex() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "0"
	}
	return hex.EncodeToString(b)
}
