// Package notifications provides notification management for Unraid systems
// via the Unraid GraphQL API.
package notifications

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jamesprial/unraid-mcp/internal/safety"
	"github.com/jamesprial/unraid-mcp/internal/tools"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// DestructiveTools lists notification tool names that require confirmation
// before execution.
var DestructiveTools = []string{"notifications_manage"}

// NotificationTools returns a slice of tool registrations for notification
// management. It exposes notifications_list (read-only) and
// notifications_manage (with confirmation for destructive actions).
func NotificationTools(mgr NotificationManager, confirm *safety.ConfirmationTracker, audit *safety.AuditLogger) []tools.Registration {
	return []tools.Registration{
		toolNotificationsList(mgr, audit),
		toolNotificationsManage(mgr, confirm, audit),
	}
}

// importanceMarker returns a short prefix marker for a notification importance level.
func importanceMarker(importance string) string {
	switch strings.ToLower(importance) {
	case "alert":
		return "[ALERT]"
	case "warning":
		return "[WARNING]"
	default:
		return "[INFO]"
	}
}

// formatNotification renders a single notification as a human-readable string.
func formatNotification(n Notification) string {
	ts := "N/A"
	if n.Timestamp != nil {
		ts = *n.Timestamp
	}
	return fmt.Sprintf("%s [%s] %s — %s\n  Subject: %s\n  ID: %s\n  Timestamp: %s",
		importanceMarker(n.Importance),
		n.Importance,
		n.Title,
		n.Description,
		n.Subject,
		n.ID,
		ts,
	)
}

// toolNotificationsList constructs the notifications_list Registration.
func toolNotificationsList(mgr NotificationManager, audit *safety.AuditLogger) tools.Registration {
	const toolName = "notifications_list"

	tool := mcp.NewTool(toolName,
		mcp.WithDescription("List Unraid notifications. Supports filtering by type (UNREAD, ARCHIVE, ALL) and a result limit."),
		mcp.WithString("filter_type",
			mcp.Description("Filter type: UNREAD (default), ARCHIVE, or ALL"),
		),
		mcp.WithNumber("limit",
			mcp.Description("Maximum number of notifications to return (default: 20)"),
		),
	)

	handler := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		start := time.Now()

		filterType := req.GetString("filter_type", "UNREAD")
		limit := req.GetInt("limit", 20)

		params := map[string]any{
			"filter_type": filterType,
			"limit":       limit,
		}

		notifs, err := mgr.List(ctx, filterType, limit)
		if err != nil {
			tools.LogAudit(audit, toolName, params, "error: "+err.Error(), start)
			return tools.ErrorResult(err.Error()), nil
		}

		if len(notifs) == 0 {
			tools.LogAudit(audit, toolName, params, "ok: empty", start)
			return mcp.NewToolResultText("No notifications found."), nil
		}

		var sb strings.Builder
		for i, n := range notifs {
			if i > 0 {
				sb.WriteString("\n\n")
			}
			sb.WriteString(formatNotification(n))
		}

		tools.LogAudit(audit, toolName, params, "ok", start)
		return mcp.NewToolResultText(sb.String()), nil
	}

	return tools.Registration{Tool: tool, Handler: server.ToolHandlerFunc(handler)}
}

// singleItemActions is the set of actions that require a notification id.
var singleItemActions = map[string]struct{}{
	"archive":   {},
	"unarchive": {},
	"delete":    {},
}

// destructiveActions is the set of actions that require a confirmation token.
var destructiveActions = map[string]struct{}{
	"delete":     {},
	"delete_all": {},
}

// toolNotificationsManage constructs the notifications_manage Registration.
func toolNotificationsManage(mgr NotificationManager, confirm *safety.ConfirmationTracker, audit *safety.AuditLogger) tools.Registration {
	const toolName = "notifications_manage"

	tool := mcp.NewTool(toolName,
		mcp.WithDescription("Manage Unraid notifications. Supports archive, unarchive, delete, archive_all, and delete_all actions. Destructive actions (delete, delete_all) require a confirmation token."),
		mcp.WithString("action",
			mcp.Required(),
			mcp.Description("Action to perform: archive, unarchive, delete, archive_all, delete_all"),
		),
		mcp.WithString("id",
			mcp.Description("Notification ID (required for archive, unarchive, delete)"),
		),
		mcp.WithString("confirmation_token",
			mcp.Description("Confirmation token returned by a prior call for destructive actions"),
		),
	)

	handler := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		start := time.Now()

		action := req.GetString("action", "")
		id := req.GetString("id", "")
		token := req.GetString("confirmation_token", "")

		params := map[string]any{
			"action": action,
			"id":     id,
		}

		// Validate action.
		validActions := map[string]struct{}{
			"archive":     {},
			"unarchive":   {},
			"delete":      {},
			"archive_all": {},
			"delete_all":  {},
		}
		if _, ok := validActions[action]; !ok {
			msg := fmt.Sprintf("unknown action %q: valid actions are archive, unarchive, delete, archive_all, delete_all", action)
			tools.LogAudit(audit, toolName, params, "error: "+msg, start)
			return tools.ErrorResult(msg), nil
		}

		// Validate id is present for single-item actions.
		if _, needsID := singleItemActions[action]; needsID && id == "" {
			msg := fmt.Sprintf("action %q requires an id parameter", action)
			tools.LogAudit(audit, toolName, params, "error: "+msg, start)
			return tools.ErrorResult(msg), nil
		}

		// Check confirmation for destructive actions.
		if _, isDestructive := destructiveActions[action]; isDestructive {
			if !confirm.Confirm(token) {
				resource := action
				if id != "" {
					resource = fmt.Sprintf("%s (id=%s)", action, id)
				}
				desc := fmt.Sprintf("This will permanently %s. This cannot be undone.", action)
				return tools.ConfirmPrompt(confirm, toolName, resource, desc), nil
			}
		}

		// Dispatch action.
		var err error
		switch action {
		case "archive":
			err = mgr.Archive(ctx, id)
		case "unarchive":
			err = mgr.Unarchive(ctx, id)
		case "delete":
			err = mgr.Delete(ctx, id)
		case "archive_all":
			err = mgr.ArchiveAll(ctx)
		case "delete_all":
			err = mgr.DeleteAll(ctx)
		}

		if err != nil {
			tools.LogAudit(audit, toolName, params, "error: "+err.Error(), start)
			return tools.ErrorResult(err.Error()), nil
		}

		var successMsg string
		switch action {
		case "archive":
			successMsg = fmt.Sprintf("notification %q archived successfully", id)
		case "unarchive":
			successMsg = fmt.Sprintf("notification %q unarchived successfully", id)
		case "delete":
			successMsg = fmt.Sprintf("notification %q deleted successfully", id)
		case "archive_all":
			successMsg = "all notifications archived successfully"
		case "delete_all":
			successMsg = "all notifications deleted successfully"
		}

		tools.LogAudit(audit, toolName, params, "ok", start)
		return mcp.NewToolResultText(successMsg), nil
	}

	return tools.Registration{Tool: tool, Handler: server.ToolHandlerFunc(handler)}
}
