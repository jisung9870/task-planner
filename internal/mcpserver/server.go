// Package mcpserver exposes the task vault over the Model Context Protocol.
//
// It is an adapter like cli and tui: every tool goes through internal/service,
// so an agent gets the same domain rules a human does - id allocation, the
// blocked-needs-a-reason gate, transition logs, recurrence, auto-unblocking.
// An agent editing markdown by hand would have to reimplement all of that.
//
// Deliberately absent: task deletion. Cancelling is the recorded way to stop
// work; removing a file outright stays a human decision made in the CLI.
package mcpserver

import (
	"context"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"task-planner/internal/service"
)

// Server wraps the MCP server around a service handle.
type Server struct {
	svc *service.Service
	mcp *mcp.Server

	// mu serialises tool calls. The MCP session is long-lived and the SDK may
	// dispatch concurrently; Edit's load-modify-save is not safe to interleave.
	mu sync.Mutex
}

// New builds the server and registers the tool surface.
func New(svc *service.Service, version string) *Server {
	s := &Server{svc: svc}
	s.mcp = mcp.NewServer(&mcp.Implementation{
		Name:    "task-planner",
		Title:   "task-planner (markdown 업무 관리)",
		Version: version,
	}, nil)
	s.registerReadTools()
	s.registerWriteTools()
	return s
}

// Run serves on stdio until the client hangs up. Nothing in this process may
// write to stdout except the protocol itself - logs belong on stderr.
func (s *Server) Run(ctx context.Context) error {
	return s.mcp.Run(ctx, &mcp.StdioTransport{})
}

// Connect attaches an arbitrary transport (tests use the in-memory pair).
func (s *Server) Connect(ctx context.Context, t mcp.Transport) (*mcp.ServerSession, error) {
	return s.mcp.Connect(ctx, t, nil)
}

// begin locks the server and refreshes the index. Each Claude Code session
// runs its own tp mcp process, and the TUI or editor may be writing the same
// vault - syncing per call is what keeps answers current across processes.
func (s *Server) begin() func() {
	s.mu.Lock()
	s.svc.Sync()
	return s.mu.Unlock
}

// finish flushes the index and commits when git auto-commit is on. The MCP
// process lives for a whole editor session, so waiting for Close() would batch
// hours of changes into one commit; per-mutation matches the CLI's semantics.
func (s *Server) finish() error {
	if _, err := s.svc.CommitPending(); err != nil {
		return err
	}
	return nil
}
