// Package vm provides virtual machine management for Unraid systems via libvirt.
package vm

import (
	"context"
	"fmt"
	"time"

	"github.com/jamesprial/unraid-mcp/internal/safety"
	"github.com/jamesprial/unraid-mcp/internal/tools"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// DestructiveTools lists VM tool names that require confirmation before execution.
var DestructiveTools = []string{
	"vm_stop",
	"vm_force_stop",
	"vm_restart",
	"vm_create",
	"vm_delete",
}

// VMTools returns a slice of tool registrations for all VM MCP tools.
// Each tool is wired to the provided VMManager, safety Filter,
// ConfirmationTracker, and AuditLogger.
func VMTools(
	mgr VMManager,
	filter *safety.Filter,
	confirm *safety.ConfirmationTracker,
	audit *safety.AuditLogger,
) []tools.Registration {
	return []tools.Registration{
		vmList(mgr, audit),
		vmInspect(mgr, filter, audit),
		vmStart(mgr, filter, audit),
		vmStop(mgr, filter, confirm, audit),
		vmForceStop(mgr, filter, confirm, audit),
		vmPause(mgr, filter, audit),
		vmResume(mgr, filter, audit),
		vmRestart(mgr, filter, confirm, audit),
		vmCreate(mgr, confirm, audit),
		vmDelete(mgr, filter, confirm, audit),
		vmSnapshotList(mgr, filter, audit),
		vmSnapshotCreate(mgr, filter, audit),
	}
}

// ---------------------------------------------------------------------------
// VM tools
// ---------------------------------------------------------------------------

func vmList(mgr VMManager, audit *safety.AuditLogger) tools.Registration {
	tool := mcp.NewTool("vm_list",
		mcp.WithDescription("List all virtual machines."),
	)

	handler := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		start := time.Now()
		params := map[string]any{}

		vms, err := mgr.ListVMs(ctx)
		if err != nil {
			tools.LogAudit(audit, "vm_list", params, "error: "+err.Error(), start)
			return tools.ErrorResult(err.Error()), nil
		}

		tools.LogAudit(audit, "vm_list", params, "ok", start)
		return tools.JSONResult(vms), nil
	}

	return tools.Registration{Tool: tool, Handler: server.ToolHandlerFunc(handler)}
}

func vmInspect(mgr VMManager, filter *safety.Filter, audit *safety.AuditLogger) tools.Registration {
	tool := mcp.NewTool("vm_inspect",
		mcp.WithDescription("Inspect a virtual machine and return its full details."),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("VM name"),
		),
	)

	handler := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		start := time.Now()
		name := req.GetString("name", "")
		params := map[string]any{"name": name}

		if !filter.IsAllowed(name) {
			tools.LogAudit(audit, "vm_inspect", params, "denied", start)
			return tools.ErrorResult(fmt.Sprintf("access to VM %q is not allowed", name)), nil
		}

		detail, err := mgr.InspectVM(ctx, name)
		if err != nil {
			tools.LogAudit(audit, "vm_inspect", params, "error: "+err.Error(), start)
			return tools.ErrorResult(err.Error()), nil
		}

		tools.LogAudit(audit, "vm_inspect", params, "ok", start)
		return tools.JSONResult(detail), nil
	}

	return tools.Registration{Tool: tool, Handler: server.ToolHandlerFunc(handler)}
}

func vmStart(mgr VMManager, filter *safety.Filter, audit *safety.AuditLogger) tools.Registration {
	tool := mcp.NewTool("vm_start",
		mcp.WithDescription("Start a stopped virtual machine."),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("VM name"),
		),
	)

	handler := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		start := time.Now()
		name := req.GetString("name", "")
		params := map[string]any{"name": name}

		if !filter.IsAllowed(name) {
			tools.LogAudit(audit, "vm_start", params, "denied", start)
			return tools.ErrorResult(fmt.Sprintf("access to VM %q is not allowed", name)), nil
		}

		if err := mgr.StartVM(ctx, name); err != nil {
			tools.LogAudit(audit, "vm_start", params, "error: "+err.Error(), start)
			return tools.ErrorResult(err.Error()), nil
		}

		tools.LogAudit(audit, "vm_start", params, "ok", start)
		return mcp.NewToolResultText(fmt.Sprintf("VM %q started successfully", name)), nil
	}

	return tools.Registration{Tool: tool, Handler: server.ToolHandlerFunc(handler)}
}

func vmStop(mgr VMManager, filter *safety.Filter, confirm *safety.ConfirmationTracker, audit *safety.AuditLogger) tools.Registration {
	const toolName = "vm_stop"

	tool := mcp.NewTool(toolName,
		mcp.WithDescription("Gracefully stop a running virtual machine via ACPI. Requires confirmation."),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("VM name"),
		),
		mcp.WithString("confirmation_token",
			mcp.Description("Confirmation token returned by a prior call to this tool"),
		),
	)

	handler := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		start := time.Now()
		name := req.GetString("name", "")
		token := req.GetString("confirmation_token", "")
		params := map[string]any{"name": name}

		if !filter.IsAllowed(name) {
			tools.LogAudit(audit, toolName, params, "denied", start)
			return tools.ErrorResult(fmt.Sprintf("access to VM %q is not allowed", name)), nil
		}

		if !confirm.Confirm(token) {
			desc := fmt.Sprintf("This will gracefully shut down VM %q via ACPI.", name)
			return tools.ConfirmPrompt(confirm, toolName, name, desc), nil
		}

		if err := mgr.StopVM(ctx, name); err != nil {
			tools.LogAudit(audit, toolName, params, "error: "+err.Error(), start)
			return tools.ErrorResult(err.Error()), nil
		}

		tools.LogAudit(audit, toolName, params, "ok", start)
		return mcp.NewToolResultText(fmt.Sprintf("VM %q stopped successfully", name)), nil
	}

	return tools.Registration{Tool: tool, Handler: server.ToolHandlerFunc(handler)}
}

func vmForceStop(mgr VMManager, filter *safety.Filter, confirm *safety.ConfirmationTracker, audit *safety.AuditLogger) tools.Registration {
	const toolName = "vm_force_stop"

	tool := mcp.NewTool(toolName,
		mcp.WithDescription("Forcibly destroy a virtual machine (equivalent to a power cut). Requires confirmation."),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("VM name"),
		),
		mcp.WithString("confirmation_token",
			mcp.Description("Confirmation token returned by a prior call to this tool"),
		),
	)

	handler := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		start := time.Now()
		name := req.GetString("name", "")
		token := req.GetString("confirmation_token", "")
		params := map[string]any{"name": name}

		if !filter.IsAllowed(name) {
			tools.LogAudit(audit, toolName, params, "denied", start)
			return tools.ErrorResult(fmt.Sprintf("access to VM %q is not allowed", name)), nil
		}

		if !confirm.Confirm(token) {
			desc := fmt.Sprintf("This will FORCIBLY destroy VM %q immediately (like pulling the power cord). Data loss may occur.", name)
			return tools.ConfirmPrompt(confirm, toolName, name, desc), nil
		}

		if err := mgr.ForceStopVM(ctx, name); err != nil {
			tools.LogAudit(audit, toolName, params, "error: "+err.Error(), start)
			return tools.ErrorResult(err.Error()), nil
		}

		tools.LogAudit(audit, toolName, params, "ok", start)
		return mcp.NewToolResultText(fmt.Sprintf("VM %q force-stopped successfully", name)), nil
	}

	return tools.Registration{Tool: tool, Handler: server.ToolHandlerFunc(handler)}
}

func vmPause(mgr VMManager, filter *safety.Filter, audit *safety.AuditLogger) tools.Registration {
	tool := mcp.NewTool("vm_pause",
		mcp.WithDescription("Pause (suspend) a running virtual machine."),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("VM name"),
		),
	)

	handler := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		start := time.Now()
		name := req.GetString("name", "")
		params := map[string]any{"name": name}

		if !filter.IsAllowed(name) {
			tools.LogAudit(audit, "vm_pause", params, "denied", start)
			return tools.ErrorResult(fmt.Sprintf("access to VM %q is not allowed", name)), nil
		}

		if err := mgr.PauseVM(ctx, name); err != nil {
			tools.LogAudit(audit, "vm_pause", params, "error: "+err.Error(), start)
			return tools.ErrorResult(err.Error()), nil
		}

		tools.LogAudit(audit, "vm_pause", params, "ok", start)
		return mcp.NewToolResultText(fmt.Sprintf("VM %q paused successfully", name)), nil
	}

	return tools.Registration{Tool: tool, Handler: server.ToolHandlerFunc(handler)}
}

func vmResume(mgr VMManager, filter *safety.Filter, audit *safety.AuditLogger) tools.Registration {
	tool := mcp.NewTool("vm_resume",
		mcp.WithDescription("Resume a paused virtual machine."),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("VM name"),
		),
	)

	handler := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		start := time.Now()
		name := req.GetString("name", "")
		params := map[string]any{"name": name}

		if !filter.IsAllowed(name) {
			tools.LogAudit(audit, "vm_resume", params, "denied", start)
			return tools.ErrorResult(fmt.Sprintf("access to VM %q is not allowed", name)), nil
		}

		if err := mgr.ResumeVM(ctx, name); err != nil {
			tools.LogAudit(audit, "vm_resume", params, "error: "+err.Error(), start)
			return tools.ErrorResult(err.Error()), nil
		}

		tools.LogAudit(audit, "vm_resume", params, "ok", start)
		return mcp.NewToolResultText(fmt.Sprintf("VM %q resumed successfully", name)), nil
	}

	return tools.Registration{Tool: tool, Handler: server.ToolHandlerFunc(handler)}
}

func vmRestart(mgr VMManager, filter *safety.Filter, confirm *safety.ConfirmationTracker, audit *safety.AuditLogger) tools.Registration {
	const toolName = "vm_restart"

	tool := mcp.NewTool(toolName,
		mcp.WithDescription("Restart a virtual machine. Requires confirmation."),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("VM name"),
		),
		mcp.WithString("confirmation_token",
			mcp.Description("Confirmation token returned by a prior call to this tool"),
		),
	)

	handler := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		start := time.Now()
		name := req.GetString("name", "")
		token := req.GetString("confirmation_token", "")
		params := map[string]any{"name": name}

		if !filter.IsAllowed(name) {
			tools.LogAudit(audit, toolName, params, "denied", start)
			return tools.ErrorResult(fmt.Sprintf("access to VM %q is not allowed", name)), nil
		}

		if !confirm.Confirm(token) {
			desc := fmt.Sprintf("This will restart VM %q.", name)
			return tools.ConfirmPrompt(confirm, toolName, name, desc), nil
		}

		if err := mgr.RestartVM(ctx, name); err != nil {
			tools.LogAudit(audit, toolName, params, "error: "+err.Error(), start)
			return tools.ErrorResult(err.Error()), nil
		}

		tools.LogAudit(audit, toolName, params, "ok", start)
		return mcp.NewToolResultText(fmt.Sprintf("VM %q restarted successfully", name)), nil
	}

	return tools.Registration{Tool: tool, Handler: server.ToolHandlerFunc(handler)}
}

func vmCreate(mgr VMManager, confirm *safety.ConfirmationTracker, audit *safety.AuditLogger) tools.Registration {
	const toolName = "vm_create"

	tool := mcp.NewTool(toolName,
		mcp.WithDescription("Create a new virtual machine from an XML configuration. Requires confirmation."),
		mcp.WithString("xml_config",
			mcp.Required(),
			mcp.Description("Libvirt XML domain configuration"),
		),
		mcp.WithString("confirmation_token",
			mcp.Description("Confirmation token returned by a prior call to this tool"),
		),
	)

	handler := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		start := time.Now()
		xmlConfig := req.GetString("xml_config", "")
		token := req.GetString("confirmation_token", "")
		params := map[string]any{"xml_config_length": len(xmlConfig)}

		if !confirm.Confirm(token) {
			desc := "This will define a new virtual machine from the provided XML configuration."
			return tools.ConfirmPrompt(confirm, toolName, "new-vm", desc), nil
		}

		if err := mgr.CreateVM(ctx, xmlConfig); err != nil {
			tools.LogAudit(audit, toolName, params, "error: "+err.Error(), start)
			return tools.ErrorResult(err.Error()), nil
		}

		tools.LogAudit(audit, toolName, params, "ok", start)
		return mcp.NewToolResultText("virtual machine created successfully"), nil
	}

	return tools.Registration{Tool: tool, Handler: server.ToolHandlerFunc(handler)}
}

func vmDelete(mgr VMManager, filter *safety.Filter, confirm *safety.ConfirmationTracker, audit *safety.AuditLogger) tools.Registration {
	const toolName = "vm_delete"

	tool := mcp.NewTool(toolName,
		mcp.WithDescription("Delete (undefine) a virtual machine. Requires confirmation."),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("VM name"),
		),
		mcp.WithString("confirmation_token",
			mcp.Description("Confirmation token returned by a prior call to this tool"),
		),
	)

	handler := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		start := time.Now()
		name := req.GetString("name", "")
		token := req.GetString("confirmation_token", "")
		params := map[string]any{"name": name}

		if !filter.IsAllowed(name) {
			tools.LogAudit(audit, toolName, params, "denied", start)
			return tools.ErrorResult(fmt.Sprintf("access to VM %q is not allowed", name)), nil
		}

		if !confirm.Confirm(token) {
			desc := fmt.Sprintf("This will permanently delete (undefine) VM %q. The disk images are NOT automatically deleted.", name)
			return tools.ConfirmPrompt(confirm, toolName, name, desc), nil
		}

		if err := mgr.DeleteVM(ctx, name); err != nil {
			tools.LogAudit(audit, toolName, params, "error: "+err.Error(), start)
			return tools.ErrorResult(err.Error()), nil
		}

		tools.LogAudit(audit, toolName, params, "ok", start)
		return mcp.NewToolResultText(fmt.Sprintf("VM %q deleted successfully", name)), nil
	}

	return tools.Registration{Tool: tool, Handler: server.ToolHandlerFunc(handler)}
}

func vmSnapshotList(mgr VMManager, filter *safety.Filter, audit *safety.AuditLogger) tools.Registration {
	tool := mcp.NewTool("vm_snapshot_list",
		mcp.WithDescription("List all snapshots for a virtual machine."),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("VM name"),
		),
	)

	handler := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		start := time.Now()
		name := req.GetString("name", "")
		params := map[string]any{"name": name}

		if !filter.IsAllowed(name) {
			tools.LogAudit(audit, "vm_snapshot_list", params, "denied", start)
			return tools.ErrorResult(fmt.Sprintf("access to VM %q is not allowed", name)), nil
		}

		snapshots, err := mgr.ListSnapshots(ctx, name)
		if err != nil {
			tools.LogAudit(audit, "vm_snapshot_list", params, "error: "+err.Error(), start)
			return tools.ErrorResult(err.Error()), nil
		}

		tools.LogAudit(audit, "vm_snapshot_list", params, "ok", start)
		return tools.JSONResult(snapshots), nil
	}

	return tools.Registration{Tool: tool, Handler: server.ToolHandlerFunc(handler)}
}

func vmSnapshotCreate(mgr VMManager, filter *safety.Filter, audit *safety.AuditLogger) tools.Registration {
	tool := mcp.NewTool("vm_snapshot_create",
		mcp.WithDescription("Create a snapshot of a virtual machine."),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("VM name"),
		),
		mcp.WithString("snapshot_name",
			mcp.Required(),
			mcp.Description("Name for the new snapshot"),
		),
	)

	handler := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		start := time.Now()
		name := req.GetString("name", "")
		snapName := req.GetString("snapshot_name", "")
		params := map[string]any{"name": name, "snapshot_name": snapName}

		if !filter.IsAllowed(name) {
			tools.LogAudit(audit, "vm_snapshot_create", params, "denied", start)
			return tools.ErrorResult(fmt.Sprintf("access to VM %q is not allowed", name)), nil
		}

		if err := mgr.CreateSnapshot(ctx, name, snapName); err != nil {
			tools.LogAudit(audit, "vm_snapshot_create", params, "error: "+err.Error(), start)
			return tools.ErrorResult(err.Error()), nil
		}

		tools.LogAudit(audit, "vm_snapshot_create", params, "ok", start)
		return mcp.NewToolResultText(fmt.Sprintf("snapshot %q created for VM %q", snapName, name)), nil
	}

	return tools.Registration{Tool: tool, Handler: server.ToolHandlerFunc(handler)}
}
