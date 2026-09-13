package mcpserver

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"task-planner/internal/style"
)

// registerResources exposes the writing conventions over the protocol.
//
// The tool descriptions carry one line and a pointer here rather than the
// whole rulebook: a long description is read once and then pushed out by the
// conversation, while this is fetched at the moment it is needed - which, in
// practice, is when a write was refused and the error named this URI.
func (s *Server) registerResources() {
	s.mcp.AddResource(&mcp.Resource{
		URI:         style.ConventionsURI,
		Name:        "태스크 작성 규약",
		Description: "제목·메모·보류 사유를 사람이 읽을 수 있게 쓰는 법. 나쁨/중간/좋음 예시 포함",
		MIMEType:    "text/markdown",
	}, func(context.Context, *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{
			URI:      style.ConventionsURI,
			MIMEType: "text/markdown",
			Text:     style.Conventions,
		}}}, nil
	})
}
