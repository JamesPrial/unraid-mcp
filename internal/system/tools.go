// Package system provides system health monitoring for an Unraid server.
package system

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jamesprial/unraid-mcp/internal/safety"
	"github.com/jamesprial/unraid-mcp/internal/tools"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// SystemTools returns a slice of tool registrations for all system health MCP
// tools. These tools are all read-only and require no confirmation.
func SystemTools(mon SystemMonitor, audit *safety.AuditLogger) []tools.Registration {
	return []tools.Registration{
		systemOverview(mon, audit),
		systemArrayStatus(mon, audit),
		systemDisks(mon, audit),
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// sysJSONResult marshals v to indented JSON and returns an mcp.CallToolResult.
func sysJSONResult(v any) *mcp.CallToolResult {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf("error marshaling result: %v", err))
	}
	return mcp.NewToolResultText(string(data))
}

// sysLogAudit logs a tool invocation to the audit logger if non-nil.
func sysLogAudit(audit *safety.AuditLogger, tool string, params map[string]any, result string, start time.Time) {
	if audit == nil {
		return
	}
	_ = audit.Log(safety.AuditEntry{
		Timestamp: start,
		Tool:      tool,
		Params:    params,
		Result:    result,
		Duration:  time.Since(start),
	})
}

// ---------------------------------------------------------------------------
// System tools
// ---------------------------------------------------------------------------

func systemOverview(mon SystemMonitor, audit *safety.AuditLogger) tools.Registration {
	tool := mcp.NewTool("system_overview",
		mcp.WithDescription("Get a snapshot of overall system health: CPU usage, memory usage, and hardware temperatures."),
	)

	handler := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		start := time.Now()
		params := map[string]any{}

		overview, err := mon.GetOverview(ctx)
		if err != nil {
			sysLogAudit(audit, "system_overview", params, "error: "+err.Error(), start)
			return mcp.NewToolResultText(fmt.Sprintf("error: %s", err.Error())), nil
		}

		sysLogAudit(audit, "system_overview", params, "ok", start)
		return sysJSONResult(overview), nil
	}

	return tools.Registration{Tool: tool, Handler: server.ToolHandlerFunc(handler)}
}

func systemArrayStatus(mon SystemMonitor, audit *safety.AuditLogger) tools.Registration {
	tool := mcp.NewTool("system_array_status",
		mcp.WithDescription("Get the current state of the Unraid storage array, including disk count, protection status, and any sync progress."),
	)

	handler := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		start := time.Now()
		params := map[string]any{}

		status, err := mon.GetArrayStatus(ctx)
		if err != nil {
			sysLogAudit(audit, "system_array_status", params, "error: "+err.Error(), start)
			return mcp.NewToolResultText(fmt.Sprintf("error: %s", err.Error())), nil
		}

		sysLogAudit(audit, "system_array_status", params, "ok", start)
		return sysJSONResult(status), nil
	}

	return tools.Registration{Tool: tool, Handler: server.ToolHandlerFunc(handler)}
}

func systemDisks(mon SystemMonitor, audit *safety.AuditLogger) tools.Registration {
	tool := mcp.NewTool("system_disks",
		mcp.WithDescription("Get per-disk details for every disk known to the Unraid array, including device name, temperature, status, and filesystem usage."),
	)

	handler := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		start := time.Now()
		params := map[string]any{}

		disks, err := mon.GetDiskInfo(ctx)
		if err != nil {
			sysLogAudit(audit, "system_disks", params, "error: "+err.Error(), start)
			return mcp.NewToolResultText(fmt.Sprintf("error: %s", err.Error())), nil
		}

		sysLogAudit(audit, "system_disks", params, "ok", start)
		return sysJSONResult(disks), nil
	}

	return tools.Registration{Tool: tool, Handler: server.ToolHandlerFunc(handler)}
}
