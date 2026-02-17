// Package docker provides Docker container and network management for the unraid-mcp server.
package docker

import (
	"context"
	"fmt"
	"time"

	"github.com/jamesprial/unraid-mcp/internal/safety"
	"github.com/jamesprial/unraid-mcp/internal/tools"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func toolDockerNetworkList(mgr DockerManager, audit *safety.AuditLogger) tools.Registration {
	tool := mcp.NewTool("docker_network_list",
		mcp.WithDescription("List all Docker networks."),
	)

	handler := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		start := time.Now()
		params := map[string]any{}

		networks, err := mgr.ListNetworks(ctx)
		if err != nil {
			tools.LogAudit(audit, "docker_network_list", params, "error: "+err.Error(), start)
			return tools.ErrorResult(err.Error()), nil
		}

		tools.LogAudit(audit, "docker_network_list", params, "ok", start)
		return tools.JSONResult(networks), nil
	}

	return tools.Registration{Tool: tool, Handler: server.ToolHandlerFunc(handler)}
}

func toolDockerNetworkInspect(mgr DockerManager, filter *safety.Filter, audit *safety.AuditLogger) tools.Registration {
	tool := mcp.NewTool("docker_network_inspect",
		mcp.WithDescription("Inspect a Docker network and return its full details."),
		mcp.WithString("id",
			mcp.Required(),
			mcp.Description("Network ID or name"),
		),
	)

	handler := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		start := time.Now()
		id := req.GetString("id", "")
		params := map[string]any{"id": id}

		if !filter.IsAllowed(id) {
			tools.LogAudit(audit, "docker_network_inspect", params, "denied", start)
			return tools.ErrorResult(fmt.Sprintf("access to network %q is not allowed", id)), nil
		}

		detail, err := mgr.InspectNetwork(ctx, id)
		if err != nil {
			tools.LogAudit(audit, "docker_network_inspect", params, "error: "+err.Error(), start)
			return tools.ErrorResult(err.Error()), nil
		}

		tools.LogAudit(audit, "docker_network_inspect", params, "ok", start)
		return tools.JSONResult(detail), nil
	}

	return tools.Registration{Tool: tool, Handler: server.ToolHandlerFunc(handler)}
}

func toolDockerNetworkCreate(mgr DockerManager, filter *safety.Filter, confirm *safety.ConfirmationTracker, audit *safety.AuditLogger) tools.Registration {
	const toolName = "docker_network_create"

	tool := mcp.NewTool(toolName,
		mcp.WithDescription("Create a new Docker network. Requires confirmation."),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("Network name"),
		),
		mcp.WithString("driver",
			mcp.Description("Network driver (e.g. bridge, overlay)"),
		),
		mcp.WithString("subnet",
			mcp.Description("Subnet in CIDR notation (e.g. 192.168.100.0/24)"),
		),
		mcp.WithString("confirmation_token",
			mcp.Description("Confirmation token returned by a prior call to this tool"),
		),
	)

	handler := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		start := time.Now()
		name := req.GetString("name", "")
		driver := req.GetString("driver", "")
		subnet := req.GetString("subnet", "")
		token := req.GetString("confirmation_token", "")
		params := map[string]any{"name": name, "driver": driver, "subnet": subnet}

		if !filter.IsAllowed(name) {
			tools.LogAudit(audit, toolName, params, "denied", start)
			return tools.ErrorResult(fmt.Sprintf("creation of network %q is not allowed", name)), nil
		}

		if !confirm.Confirm(token) {
			desc := fmt.Sprintf("This will create a new Docker network %q (driver=%q, subnet=%q).", name, driver, subnet)
			return tools.ConfirmPrompt(confirm, toolName, name, desc), nil
		}

		cfg := NetworkCreateConfig{
			Name:   name,
			Driver: driver,
			Subnet: subnet,
		}

		networkID, err := mgr.CreateNetwork(ctx, cfg)
		if err != nil {
			tools.LogAudit(audit, toolName, params, "error: "+err.Error(), start)
			return tools.ErrorResult(err.Error()), nil
		}

		tools.LogAudit(audit, toolName, params, "ok: "+networkID, start)
		return mcp.NewToolResultText(fmt.Sprintf("network %q created with ID %q", name, networkID)), nil
	}

	return tools.Registration{Tool: tool, Handler: server.ToolHandlerFunc(handler)}
}

func toolDockerNetworkRemove(mgr DockerManager, filter *safety.Filter, confirm *safety.ConfirmationTracker, audit *safety.AuditLogger) tools.Registration {
	const toolName = "docker_network_remove"

	tool := mcp.NewTool(toolName,
		mcp.WithDescription("Remove a Docker network. Requires confirmation."),
		mcp.WithString("id",
			mcp.Required(),
			mcp.Description("Network ID or name"),
		),
		mcp.WithString("confirmation_token",
			mcp.Description("Confirmation token returned by a prior call to this tool"),
		),
	)

	handler := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		start := time.Now()
		id := req.GetString("id", "")
		token := req.GetString("confirmation_token", "")
		params := map[string]any{"id": id}

		if !filter.IsAllowed(id) {
			tools.LogAudit(audit, toolName, params, "denied", start)
			return tools.ErrorResult(fmt.Sprintf("access to network %q is not allowed", id)), nil
		}

		if !confirm.Confirm(token) {
			desc := fmt.Sprintf("This will permanently remove network %q.", id)
			return tools.ConfirmPrompt(confirm, toolName, id, desc), nil
		}

		if err := mgr.RemoveNetwork(ctx, id); err != nil {
			tools.LogAudit(audit, toolName, params, "error: "+err.Error(), start)
			return tools.ErrorResult(err.Error()), nil
		}

		tools.LogAudit(audit, toolName, params, "ok", start)
		return mcp.NewToolResultText(fmt.Sprintf("network %q removed successfully", id)), nil
	}

	return tools.Registration{Tool: tool, Handler: server.ToolHandlerFunc(handler)}
}

func toolDockerNetworkConnect(mgr DockerManager, filter *safety.Filter, audit *safety.AuditLogger) tools.Registration {
	tool := mcp.NewTool("docker_network_connect",
		mcp.WithDescription("Connect a container to a Docker network."),
		mcp.WithString("network_id",
			mcp.Required(),
			mcp.Description("Network ID or name"),
		),
		mcp.WithString("container_id",
			mcp.Required(),
			mcp.Description("Container ID or name"),
		),
	)

	handler := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		start := time.Now()
		networkID := req.GetString("network_id", "")
		containerID := req.GetString("container_id", "")
		params := map[string]any{"network_id": networkID, "container_id": containerID}

		if !filter.IsAllowed(networkID) {
			tools.LogAudit(audit, "docker_network_connect", params, "denied", start)
			return tools.ErrorResult(fmt.Sprintf("access to network %q is not allowed", networkID)), nil
		}
		if !filter.IsAllowed(containerID) {
			tools.LogAudit(audit, "docker_network_connect", params, "denied", start)
			return tools.ErrorResult(fmt.Sprintf("access to container %q is not allowed", containerID)), nil
		}

		if err := mgr.ConnectNetwork(ctx, networkID, containerID); err != nil {
			tools.LogAudit(audit, "docker_network_connect", params, "error: "+err.Error(), start)
			return tools.ErrorResult(err.Error()), nil
		}

		tools.LogAudit(audit, "docker_network_connect", params, "ok", start)
		return mcp.NewToolResultText(fmt.Sprintf("container %q connected to network %q", containerID, networkID)), nil
	}

	return tools.Registration{Tool: tool, Handler: server.ToolHandlerFunc(handler)}
}

func toolDockerNetworkDisconnect(mgr DockerManager, filter *safety.Filter, audit *safety.AuditLogger) tools.Registration {
	tool := mcp.NewTool("docker_network_disconnect",
		mcp.WithDescription("Disconnect a container from a Docker network."),
		mcp.WithString("network_id",
			mcp.Required(),
			mcp.Description("Network ID or name"),
		),
		mcp.WithString("container_id",
			mcp.Required(),
			mcp.Description("Container ID or name"),
		),
	)

	handler := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		start := time.Now()
		networkID := req.GetString("network_id", "")
		containerID := req.GetString("container_id", "")
		params := map[string]any{"network_id": networkID, "container_id": containerID}

		if !filter.IsAllowed(networkID) {
			tools.LogAudit(audit, "docker_network_disconnect", params, "denied", start)
			return tools.ErrorResult(fmt.Sprintf("access to network %q is not allowed", networkID)), nil
		}
		if !filter.IsAllowed(containerID) {
			tools.LogAudit(audit, "docker_network_disconnect", params, "denied", start)
			return tools.ErrorResult(fmt.Sprintf("access to container %q is not allowed", containerID)), nil
		}

		if err := mgr.DisconnectNetwork(ctx, networkID, containerID); err != nil {
			tools.LogAudit(audit, "docker_network_disconnect", params, "error: "+err.Error(), start)
			return tools.ErrorResult(err.Error()), nil
		}

		tools.LogAudit(audit, "docker_network_disconnect", params, "ok", start)
		return mcp.NewToolResultText(fmt.Sprintf("container %q disconnected from network %q", containerID, networkID)), nil
	}

	return tools.Registration{Tool: tool, Handler: server.ToolHandlerFunc(handler)}
}
