package cli

import (
	"github.com/spf13/cobra"

	"task-planner/internal/mcpserver"
	"task-planner/internal/service"
)

func newMCPCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "mcp",
		Short: "MCP 서버 모드 (stdio)",
		Long: `stdio 로 Model Context Protocol 서버를 돌린다. 데몬이 아니다 —
MCP 클라이언트(Claude Code 등)가 이 프로세스를 자식으로 띄우고 stdin/stdout 으로
통신하며, 세션이 끝나면 함께 종료된다.

Claude Code 등록:
  claude mcp add task-planner -- tp mcp
  claude mcp add -s user task-planner -- tp mcp   # 모든 프로젝트에서

제공 도구: vault_info, task_today, task_week, task_query, task_get, summary, project_status,
time_summary, report_week (읽기) / task_add, task_status, task_skip, task_edit,
rollover (쓰기). 삭제는 제공하지 않는다 — 접을 일은 cancelled 로 처리하고,
파일 삭제는 사람이 CLI 에서 한다.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return withService(func(svc *service.Service) error {
				// stdout is the protocol channel; this handler must never print
				// to it. cobra errors go to stderr via SilenceUsage in root.
				return mcpserver.New(svc, VersionString()).Run(cmd.Context())
			})
		},
	}
}
