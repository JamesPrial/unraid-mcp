// Package shares provides Unraid share listing via the GraphQL API.
package shares

import (
	"context"
	"time"

	"github.com/jamesprial/unraid-mcp/internal/safety"
	"github.com/jamesprial/unraid-mcp/internal/tools"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// ShareTools returns the slice of MCP tool registrations for the shares package.
// All share tools are read-only and require no confirmation.
func ShareTools(mgr ShareManager, audit *safety.AuditLogger) []tools.Registration {
	return []tools.Registration{
		sharesListTool(mgr, audit),
	}
}

// sharesListTool registers the shares_list MCP tool.
func sharesListTool(mgr ShareManager, audit *safety.AuditLogger) tools.Registration {
	tool := mcp.NewTool("shares_list",
		mcp.WithDescription("List all Unraid shares, including name, size, used space, free space, and comment."),
	)

	handler := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		start := time.Now()
		params := map[string]any{}

		shares, err := mgr.List(ctx)
		if err != nil {
			tools.LogAudit(audit, "shares_list", params, "error: "+err.Error(), start)
			return tools.ErrorResult(err.Error()), nil
		}

		tools.LogAudit(audit, "shares_list", params, "ok", start)
		return tools.JSONResult(shares), nil
	}

	return tools.Registration{Tool: tool, Handler: server.ToolHandlerFunc(handler)}
}
