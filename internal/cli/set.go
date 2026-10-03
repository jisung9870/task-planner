package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"task-planner/internal/domain"
	"task-planner/internal/service"
)

// clearValue is what a user types to empty a field. "none" matches the date
// flags; "-" matches the TUI prompts. Both exist because both get typed.
func clearValue(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "-", "none", "clear", "해제":
		return ""
	}
	return strings.TrimSpace(v)
}

func newSetCmd() *cobra.Command {
	var (
		title, project, executor, agent, tier, priority, sched, due, span, estimate, recur string
		tags, links                                                                        []string
	)
	cmd := &cobra.Command{
		Use:   "set <태스크> [플래그]",
		Short: "필드 수정 (지정한 것만 바뀜)",
		Long: `필드 수정. 지정하지 않은 필드는 그대로 둔다.

  tp set 12 --due +3d --priority P1
  tp set 12 --span 09-15~09-19        # 진행 기간 (scheduled~due)
  tp set 12 --project infra --tag ops --tag infra
  tp set 12 --due none                # 해제 (none 또는 -)
  tp set 12 --agent codex --tier deep # 실행 agent·모델 등급

파일을 직접 고쳐도 되지만 이 명령은 변경 내역을 태스크 로그에 남긴다.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withService(func(svc *service.Service) error {
				f := cmd.Flags()
				in := service.EditInput{}
				if f.Changed("title") {
					v := strings.TrimSpace(title)
					if v == "" {
						return fmt.Errorf("제목은 비울 수 없음")
					}
					in.Title = &v
				}
				if f.Changed("project") {
					v := clearValue(project)
					in.Project = &v
				}
				if f.Changed("executor") {
					e, err := domain.ParseExecutor(executor)
					if err != nil {
						return err
					}
					in.Executor = &e
				}
				if f.Changed("agent") {
					a, err := domain.ParseAgent(clearValue(agent))
					if err != nil {
						return err
					}
					in.Agent = &a
				}
				if f.Changed("tier") {
					tr, err := domain.ParseTier(clearValue(tier))
					if err != nil {
						return err
					}
					in.Tier = &tr
				}
				if f.Changed("priority") {
					p, err := domain.ParsePriority(clearValue(priority))
					if err != nil {
						return err
					}
					in.Priority = &p
				}
				if f.Changed("estimate") {
					e, err := domain.ParseDuration(clearValue(estimate))
					if err != nil {
						return err
					}
					in.Estimate = &e
				}
				if f.Changed("recur") {
					v := clearValue(recur)
					in.Recur = &v
				}
				if f.Changed("tag") {
					in.Tags = &tags
				}
				if f.Changed("link") {
					in.Links = &links
				}
				if f.Changed("scheduled") {
					d, err := parseDateFlag(svc, sched)
					if err != nil {
						return err
					}
					in.Scheduled = &d
				}
				if f.Changed("due") {
					d, err := parseDateFlag(svc, due)
					if err != nil {
						return err
					}
					in.Due = &d
				}

				// --span writes the same two dates; EditWithSpan refuses the
				// combination with --scheduled/--due and saves the rest with it.
				var period *domain.Span
				if f.Changed("span") {
					sp, err := svc.ParseSpan(span)
					if err != nil {
						return err
					}
					period = &sp
				}
				var res *service.Result
				if period != nil || in.Any() {
					var err error
					if res, err = svc.EditWithSpan(args[0], in, period); err != nil {
						return err
					}
				}
				if res == nil {
					return fmt.Errorf("바꿀 필드를 하나 이상 지정하세요 (tp set --help)")
				}
				printSet(cmd, res)
				return nil
			})
		},
	}
	f := cmd.Flags()
	f.StringVar(&title, "title", "", "제목")
	f.StringVarP(&project, "project", "p", "", "프로젝트 slug (none 이면 해제)")
	f.StringVar(&executor, "executor", "", "실행 주체 human|agent")
	f.StringVar(&agent, "agent", "", "실행할 agent 또는 auto (none 이면 해제)")
	f.StringVar(&tier, "tier", "", "fast|standard|deep (none 이면 해제)")
	f.StringVar(&priority, "priority", "", "우선순위 P0~P3 (none 이면 해제)")
	f.StringVarP(&sched, "scheduled", "s", "", "착수 예정일 (YYYY-MM-DD | today | +3d | mon | none)")
	f.StringVarP(&due, "due", "d", "", "마감일 (동일 형식)")
	f.StringVar(&span, "span", "", "진행 기간 (09-15~09-19 | today~+4d | none)")
	f.StringVarP(&estimate, "estimate", "e", "", "예상 소요 (30m, 2h)")
	f.StringSliceVarP(&tags, "tag", "t", nil, "태그 (전체 교체)")
	f.StringSliceVarP(&links, "link", "l", nil, "링크 (전체 교체)")
	f.StringVar(&recur, "recur", "", "반복 규칙 (none 이면 중단)")
	return cmd
}

func printSet(cmd *cobra.Command, res *service.Result) {
	t := res.Task
	fmt.Fprintf(cmd.OutOrStdout(), "%s %s  %s\n", t.Status.Glyph(), t.ShortID(), t.Title)
	fmt.Fprintf(cmd.OutOrStdout(), "  %s\n", strings.Join(setSummary(t), "  ·  "))
}

func setSummary(t *domain.Task) []string {
	var parts []string
	if t.Project != "" {
		parts = append(parts, "프로젝트 "+t.Project)
	}
	if t.Priority != "" {
		parts = append(parts, string(t.Priority))
	}
	if t.Agent != "" || t.Tier != "" || t.ClaimedBy != "" {
		parts = append(parts, "실행 "+t.AgentBadge())
	}
	if t.HasSpan() {
		parts = append(parts, "기간 "+t.SpanLabel())
	} else {
		if !t.Scheduled.IsZero() {
			parts = append(parts, "예정 "+t.Scheduled.String())
		}
		if !t.Due.IsZero() {
			parts = append(parts, "마감 "+t.Due.String())
		}
	}
	if !t.Estimate.IsZero() {
		parts = append(parts, "예상 "+t.Estimate.String())
	}
	if t.Recur != "" {
		parts = append(parts, "반복 "+t.Recur)
	}
	if len(t.Tags) > 0 {
		parts = append(parts, "#"+strings.Join(t.Tags, " #"))
	}
	if len(parts) == 0 {
		parts = append(parts, "(설정된 필드 없음)")
	}
	return parts
}

func newNoteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "note <태스크> <내용...>",
		Short: "태스크에 한 줄 메모 추가 (타임스탬프 포함)",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withService(func(svc *service.Service) error {
				res, err := svc.AddNote(args[0], strings.Join(args[1:], " "))
				if err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "메모 추가 %s  %s\n", res.Task.ShortID(), res.Task.Title)
				return nil
			})
		},
	}
}
