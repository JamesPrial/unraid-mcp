// Package array provides Unraid array management via the GraphQL API.
package array

import (
	"context"
	"fmt"
	"time"

	"github.com/jamesprial/unraid-mcp/internal/safety"
	"github.com/jamesprial/unraid-mcp/internal/tools"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// DestructiveTools lists the array tool names that require explicit
// confirmation before execution.
var DestructiveTools = []string{
	"array_start",
	"array_stop",
	"parity_check",
}

// ArrayTools returns a slice of tool registrations for all Unraid array
// management MCP tools. All three tools are destructive and require a
// confirmation token.
func ArrayTools(mgr ArrayManager, confirm *safety.ConfirmationTracker, audit *safety.AuditLogger) []tools.Registration {
	return []tools.Registration{
		arrayStart(mgr, confirm, audit),
		arrayStop(mgr, confirm, audit),
		parityCheck(mgr, confirm, audit),
	}
}

// arrayStart constructs the array_start tool Registration.
func arrayStart(mgr ArrayManager, confirm *safety.ConfirmationTracker, audit *safety.AuditLogger) tools.Registration {
	const toolName = "array_start"

	tool := mcp.NewTool(toolName,
		mcp.WithDescription("Start the Unraid array. Requires confirmation."),
		mcp.WithString("confirmation_token",
			mcp.Description("Confirmation token returned by a prior call to this tool."),
		),
	)

	handler := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		start := time.Now()
		token := req.GetString("confirmation_token", "")
		params := map[string]any{}

		if !confirm.Confirm(token) {
			return tools.ConfirmPrompt(confirm, toolName, "array", "Start the Unraid array"), nil
		}

		if err := mgr.Start(ctx); err != nil {
			tools.LogAudit(audit, toolName, params, "error: "+err.Error(), start)
			return tools.ErrorResult(err.Error()), nil
		}

		tools.LogAudit(audit, toolName, params, "ok", start)
		return mcp.NewToolResultText("Array start command issued successfully."), nil
	}

	return tools.Registration{Tool: tool, Handler: server.ToolHandlerFunc(handler)}
}

// arrayStop constructs the array_stop tool Registration.
func arrayStop(mgr ArrayManager, confirm *safety.ConfirmationTracker, audit *safety.AuditLogger) tools.Registration {
	const toolName = "array_stop"

	tool := mcp.NewTool(toolName,
		mcp.WithDescription("Stop the Unraid array. Requires confirmation."),
		mcp.WithString("confirmation_token",
			mcp.Description("Confirmation token returned by a prior call to this tool."),
		),
	)

	handler := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		start := time.Now()
		token := req.GetString("confirmation_token", "")
		params := map[string]any{}

		if !confirm.Confirm(token) {
			return tools.ConfirmPrompt(confirm, toolName, "array", "Stop the Unraid array"), nil
		}

		if err := mgr.Stop(ctx); err != nil {
			tools.LogAudit(audit, toolName, params, "error: "+err.Error(), start)
			return tools.ErrorResult(err.Error()), nil
		}

		tools.LogAudit(audit, toolName, params, "ok", start)
		return mcp.NewToolResultText("Array stop command issued successfully."), nil
	}

	return tools.Registration{Tool: tool, Handler: server.ToolHandlerFunc(handler)}
}

// parityCheck constructs the parity_check tool Registration.
// The action parameter is validated BEFORE checking the confirmation token so
// that invalid actions return an error immediately without requiring a token.
func parityCheck(mgr ArrayManager, confirm *safety.ConfirmationTracker, audit *safety.AuditLogger) tools.Registration {
	const toolName = "parity_check"

	tool := mcp.NewTool(toolName,
		mcp.WithDescription("Control a parity check on the Unraid array. Requires confirmation."),
		mcp.WithString("action",
			mcp.Required(),
			mcp.Description("Parity check action: start, start_correct, pause, resume, or cancel."),
		),
		mcp.WithString("confirmation_token",
			mcp.Description("Confirmation token returned by a prior call to this tool."),
		),
	)

	handler := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		start := time.Now()
		action := req.GetString("action", "")
		token := req.GetString("confirmation_token", "")
		params := map[string]any{"action": action}

		// Validate the action BEFORE checking the confirmation token.
		if !IsValidParityAction(action) {
			errMsg := fmt.Sprintf("invalid parity check action: %q (valid: start, start_correct, pause, resume, cancel)", action)
			tools.LogAudit(audit, toolName, params, "error: "+errMsg, start)
			return tools.ErrorResult(errMsg), nil
		}

		if !confirm.Confirm(token) {
			return tools.ConfirmPrompt(confirm, toolName, "array", fmt.Sprintf("Parity check: %s", action)), nil
		}

		msg, err := mgr.ParityCheck(ctx, action)
		if err != nil {
			tools.LogAudit(audit, toolName, params, "error: "+err.Error(), start)
			return tools.ErrorResult(err.Error()), nil
		}

		tools.LogAudit(audit, toolName, params, "ok", start)
		return mcp.NewToolResultText(msg), nil
	}

	return tools.Registration{Tool: tool, Handler: server.ToolHandlerFunc(handler)}
}
