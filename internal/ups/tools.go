// Package ups provides UPS (Uninterruptible Power Supply) monitoring for
// Unraid systems via GraphQL.
package ups

import (
	"context"
	"time"

	"github.com/jamesprial/unraid-mcp/internal/safety"
	"github.com/jamesprial/unraid-mcp/internal/tools"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

const toolNameUPSStatus = "ups_status"

// UPSTools returns a slice of tool registrations for UPS monitoring.
// All tools are read-only; no destructive operations are provided.
func UPSTools(mon UPSMonitor, audit *safety.AuditLogger) []tools.Registration {
	return []tools.Registration{
		upsStatus(mon, audit),
	}
}

// upsStatus constructs the ups_status Registration.
func upsStatus(mon UPSMonitor, audit *safety.AuditLogger) tools.Registration {
	tool := mcp.NewTool(toolNameUPSStatus,
		mcp.WithDescription("List all UPS devices and their current status, battery levels, and power information."),
	)

	handler := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		start := time.Now()
		params := map[string]any{}

		devices, err := mon.GetDevices(ctx)
		if err != nil {
			tools.LogAudit(audit, toolNameUPSStatus, params, "error: "+err.Error(), start)
			return tools.ErrorResult(err.Error()), nil
		}

		tools.LogAudit(audit, toolNameUPSStatus, params, "ok", start)
		return tools.JSONResult(devices), nil
	}

	return tools.Registration{Tool: tool, Handler: server.ToolHandlerFunc(handler)}
}
