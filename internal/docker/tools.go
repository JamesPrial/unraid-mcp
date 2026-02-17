// Package docker provides Docker container and network management for the unraid-mcp server.
package docker

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

// DockerTools returns a slice of tool registrations for all Docker MCP tools.
// Each tool is wired to the provided DockerManager, safety Filter,
// ConfirmationTracker, and AuditLogger.
func DockerTools(
	mgr DockerManager,
	filter *safety.Filter,
	confirm *safety.ConfirmationTracker,
	audit *safety.AuditLogger,
) []tools.Registration {
	return []tools.Registration{
		toolDockerList(mgr, filter, audit),
		toolDockerInspect(mgr, filter, audit),
		toolDockerLogs(mgr, filter, audit),
		toolDockerStats(mgr, filter, audit),
		toolDockerStart(mgr, filter, audit),
		toolDockerStop(mgr, filter, confirm, audit),
		toolDockerRestart(mgr, filter, confirm, audit),
		toolDockerRemove(mgr, filter, confirm, audit),
		toolDockerCreate(mgr, filter, confirm, audit),
		toolDockerPull(mgr, audit),
		toolDockerNetworkList(mgr, audit),
		toolDockerNetworkInspect(mgr, filter, audit),
		toolDockerNetworkCreate(mgr, filter, confirm, audit),
		toolDockerNetworkRemove(mgr, filter, confirm, audit),
		toolDockerNetworkConnect(mgr, filter, audit),
		toolDockerNetworkDisconnect(mgr, filter, audit),
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// dockerToolJSONResult marshals v to indented JSON and returns an mcp.CallToolResult.
func dockerToolJSONResult(v any) *mcp.CallToolResult {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf("error marshaling result: %v", err))
	}
	return mcp.NewToolResultText(string(data))
}

// dockerToolError returns an mcp.CallToolResult that describes an error condition.
func dockerToolError(msg string) *mcp.CallToolResult {
	return mcp.NewToolResultText(fmt.Sprintf("error: %s", msg))
}

// dockerToolLogAudit logs a tool invocation to the audit logger, silently
// ignoring a nil logger.
func dockerToolLogAudit(audit *safety.AuditLogger, toolName string, params map[string]any, result string, start time.Time) {
	if audit == nil {
		return
	}
	_ = audit.Log(safety.AuditEntry{
		Timestamp: start,
		Tool:      toolName,
		Params:    params,
		Result:    result,
		Duration:  time.Since(start),
	})
}

// dockerToolConfirmPrompt returns a prompt asking the caller to confirm a
// destructive action by re-invoking with the returned token.
func dockerToolConfirmPrompt(confirm *safety.ConfirmationTracker, toolName, resource, description string) *mcp.CallToolResult {
	token := confirm.RequestConfirmation(toolName, resource, description)
	return mcp.NewToolResultText(fmt.Sprintf(
		"Confirmation required for %s on %q.\n\n%s\n\nTo proceed, call %s again with confirmation_token=%q.",
		toolName, resource, description, toolName, token,
	))
}

// ---------------------------------------------------------------------------
// Container tools
// ---------------------------------------------------------------------------

func toolDockerList(mgr DockerManager, filter *safety.Filter, audit *safety.AuditLogger) tools.Registration {
	tool := mcp.NewTool("docker_list",
		mcp.WithDescription("List Docker containers."),
		mcp.WithBoolean("all",
			mcp.Description("Include stopped containers (default: false)"),
		),
	)

	handler := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		start := time.Now()
		all := req.GetBool("all", false)
		params := map[string]any{"all": all}

		containers, err := mgr.ListContainers(ctx, all)
		if err != nil {
			dockerToolLogAudit(audit, "docker_list", params, "error: "+err.Error(), start)
			return dockerToolError(err.Error()), nil
		}

		// Apply filter to the result set.
		filtered := make([]Container, 0, len(containers))
		for _, c := range containers {
			if filter.IsAllowed(c.Name) {
				filtered = append(filtered, c)
			}
		}

		dockerToolLogAudit(audit, "docker_list", params, "ok", start)
		return dockerToolJSONResult(filtered), nil
	}

	return tools.Registration{Tool: tool, Handler: server.ToolHandlerFunc(handler)}
}

func toolDockerInspect(mgr DockerManager, filter *safety.Filter, audit *safety.AuditLogger) tools.Registration {
	tool := mcp.NewTool("docker_inspect",
		mcp.WithDescription("Inspect a Docker container and return its full details."),
		mcp.WithString("id",
			mcp.Required(),
			mcp.Description("Container ID or name"),
		),
	)

	handler := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		start := time.Now()
		id := req.GetString("id", "")
		params := map[string]any{"id": id}

		if !filter.IsAllowed(id) {
			dockerToolLogAudit(audit, "docker_inspect", params, "denied", start)
			return dockerToolError(fmt.Sprintf("access to container %q is not allowed", id)), nil
		}

		detail, err := mgr.InspectContainer(ctx, id)
		if err != nil {
			dockerToolLogAudit(audit, "docker_inspect", params, "error: "+err.Error(), start)
			return dockerToolError(err.Error()), nil
		}

		dockerToolLogAudit(audit, "docker_inspect", params, "ok", start)
		return dockerToolJSONResult(detail), nil
	}

	return tools.Registration{Tool: tool, Handler: server.ToolHandlerFunc(handler)}
}

func toolDockerLogs(mgr DockerManager, filter *safety.Filter, audit *safety.AuditLogger) tools.Registration {
	tool := mcp.NewTool("docker_logs",
		mcp.WithDescription("Fetch logs from a Docker container."),
		mcp.WithString("id",
			mcp.Required(),
			mcp.Description("Container ID or name"),
		),
		mcp.WithNumber("tail",
			mcp.Description("Number of lines to return from the end of the logs (0 = all)"),
		),
	)

	handler := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		start := time.Now()
		id := req.GetString("id", "")
		tail := req.GetInt("tail", 0)
		params := map[string]any{"id": id, "tail": tail}

		if !filter.IsAllowed(id) {
			dockerToolLogAudit(audit, "docker_logs", params, "denied", start)
			return dockerToolError(fmt.Sprintf("access to container %q is not allowed", id)), nil
		}

		logs, err := mgr.GetLogs(ctx, id, tail)
		if err != nil {
			dockerToolLogAudit(audit, "docker_logs", params, "error: "+err.Error(), start)
			return dockerToolError(err.Error()), nil
		}

		dockerToolLogAudit(audit, "docker_logs", params, "ok", start)
		return mcp.NewToolResultText(logs), nil
	}

	return tools.Registration{Tool: tool, Handler: server.ToolHandlerFunc(handler)}
}

func toolDockerStats(mgr DockerManager, filter *safety.Filter, audit *safety.AuditLogger) tools.Registration {
	tool := mcp.NewTool("docker_stats",
		mcp.WithDescription("Get resource usage statistics for a Docker container."),
		mcp.WithString("id",
			mcp.Required(),
			mcp.Description("Container ID or name"),
		),
	)

	handler := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		start := time.Now()
		id := req.GetString("id", "")
		params := map[string]any{"id": id}

		if !filter.IsAllowed(id) {
			dockerToolLogAudit(audit, "docker_stats", params, "denied", start)
			return dockerToolError(fmt.Sprintf("access to container %q is not allowed", id)), nil
		}

		stats, err := mgr.GetStats(ctx, id)
		if err != nil {
			dockerToolLogAudit(audit, "docker_stats", params, "error: "+err.Error(), start)
			return dockerToolError(err.Error()), nil
		}

		dockerToolLogAudit(audit, "docker_stats", params, "ok", start)
		return dockerToolJSONResult(stats), nil
	}

	return tools.Registration{Tool: tool, Handler: server.ToolHandlerFunc(handler)}
}

func toolDockerStart(mgr DockerManager, filter *safety.Filter, audit *safety.AuditLogger) tools.Registration {
	tool := mcp.NewTool("docker_start",
		mcp.WithDescription("Start a stopped Docker container."),
		mcp.WithString("id",
			mcp.Required(),
			mcp.Description("Container ID or name"),
		),
	)

	handler := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		start := time.Now()
		id := req.GetString("id", "")
		params := map[string]any{"id": id}

		if !filter.IsAllowed(id) {
			dockerToolLogAudit(audit, "docker_start", params, "denied", start)
			return dockerToolError(fmt.Sprintf("access to container %q is not allowed", id)), nil
		}

		if err := mgr.StartContainer(ctx, id); err != nil {
			dockerToolLogAudit(audit, "docker_start", params, "error: "+err.Error(), start)
			return dockerToolError(err.Error()), nil
		}

		dockerToolLogAudit(audit, "docker_start", params, "ok", start)
		return mcp.NewToolResultText(fmt.Sprintf("container %q started successfully", id)), nil
	}

	return tools.Registration{Tool: tool, Handler: server.ToolHandlerFunc(handler)}
}

func toolDockerStop(mgr DockerManager, filter *safety.Filter, confirm *safety.ConfirmationTracker, audit *safety.AuditLogger) tools.Registration {
	const toolName = "docker_stop"

	tool := mcp.NewTool(toolName,
		mcp.WithDescription("Stop a running Docker container. Requires confirmation."),
		mcp.WithString("id",
			mcp.Required(),
			mcp.Description("Container ID or name"),
		),
		mcp.WithNumber("timeout",
			mcp.Description("Seconds to wait before forcibly killing the container (default: 10)"),
		),
		mcp.WithString("confirmation_token",
			mcp.Description("Confirmation token returned by a prior call to this tool"),
		),
	)

	handler := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		start := time.Now()
		id := req.GetString("id", "")
		timeout := req.GetInt("timeout", 10)
		if timeout == 0 {
			timeout = 10
		}
		token := req.GetString("confirmation_token", "")
		params := map[string]any{"id": id, "timeout": timeout}

		if !filter.IsAllowed(id) {
			dockerToolLogAudit(audit, toolName, params, "denied", start)
			return dockerToolError(fmt.Sprintf("access to container %q is not allowed", id)), nil
		}

		if !confirm.Confirm(token) {
			desc := fmt.Sprintf("This will stop container %q with a %d second timeout.", id, timeout)
			return dockerToolConfirmPrompt(confirm, toolName, id, desc), nil
		}

		if err := mgr.StopContainer(ctx, id, timeout); err != nil {
			dockerToolLogAudit(audit, toolName, params, "error: "+err.Error(), start)
			return dockerToolError(err.Error()), nil
		}

		dockerToolLogAudit(audit, toolName, params, "ok", start)
		return mcp.NewToolResultText(fmt.Sprintf("container %q stopped successfully", id)), nil
	}

	return tools.Registration{Tool: tool, Handler: server.ToolHandlerFunc(handler)}
}

func toolDockerRestart(mgr DockerManager, filter *safety.Filter, confirm *safety.ConfirmationTracker, audit *safety.AuditLogger) tools.Registration {
	const toolName = "docker_restart"

	tool := mcp.NewTool(toolName,
		mcp.WithDescription("Restart a Docker container. Requires confirmation."),
		mcp.WithString("id",
			mcp.Required(),
			mcp.Description("Container ID or name"),
		),
		mcp.WithNumber("timeout",
			mcp.Description("Seconds to wait before forcibly killing the container (default: 10)"),
		),
		mcp.WithString("confirmation_token",
			mcp.Description("Confirmation token returned by a prior call to this tool"),
		),
	)

	handler := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		start := time.Now()
		id := req.GetString("id", "")
		timeout := req.GetInt("timeout", 10)
		if timeout == 0 {
			timeout = 10
		}
		token := req.GetString("confirmation_token", "")
		params := map[string]any{"id": id, "timeout": timeout}

		if !filter.IsAllowed(id) {
			dockerToolLogAudit(audit, toolName, params, "denied", start)
			return dockerToolError(fmt.Sprintf("access to container %q is not allowed", id)), nil
		}

		if !confirm.Confirm(token) {
			desc := fmt.Sprintf("This will restart container %q with a %d second timeout.", id, timeout)
			return dockerToolConfirmPrompt(confirm, toolName, id, desc), nil
		}

		if err := mgr.RestartContainer(ctx, id, timeout); err != nil {
			dockerToolLogAudit(audit, toolName, params, "error: "+err.Error(), start)
			return dockerToolError(err.Error()), nil
		}

		dockerToolLogAudit(audit, toolName, params, "ok", start)
		return mcp.NewToolResultText(fmt.Sprintf("container %q restarted successfully", id)), nil
	}

	return tools.Registration{Tool: tool, Handler: server.ToolHandlerFunc(handler)}
}

func toolDockerRemove(mgr DockerManager, filter *safety.Filter, confirm *safety.ConfirmationTracker, audit *safety.AuditLogger) tools.Registration {
	const toolName = "docker_remove"

	tool := mcp.NewTool(toolName,
		mcp.WithDescription("Remove a Docker container. Requires confirmation."),
		mcp.WithString("id",
			mcp.Required(),
			mcp.Description("Container ID or name"),
		),
		mcp.WithBoolean("force",
			mcp.Description("Force removal of a running container (default: false)"),
		),
		mcp.WithString("confirmation_token",
			mcp.Description("Confirmation token returned by a prior call to this tool"),
		),
	)

	handler := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		start := time.Now()
		id := req.GetString("id", "")
		force := req.GetBool("force", false)
		token := req.GetString("confirmation_token", "")
		params := map[string]any{"id": id, "force": force}

		if !filter.IsAllowed(id) {
			dockerToolLogAudit(audit, toolName, params, "denied", start)
			return dockerToolError(fmt.Sprintf("access to container %q is not allowed", id)), nil
		}

		if !confirm.Confirm(token) {
			desc := fmt.Sprintf("This will permanently remove container %q (force=%v).", id, force)
			return dockerToolConfirmPrompt(confirm, toolName, id, desc), nil
		}

		if err := mgr.RemoveContainer(ctx, id, force); err != nil {
			dockerToolLogAudit(audit, toolName, params, "error: "+err.Error(), start)
			return dockerToolError(err.Error()), nil
		}

		dockerToolLogAudit(audit, toolName, params, "ok", start)
		return mcp.NewToolResultText(fmt.Sprintf("container %q removed successfully", id)), nil
	}

	return tools.Registration{Tool: tool, Handler: server.ToolHandlerFunc(handler)}
}

func toolDockerCreate(mgr DockerManager, filter *safety.Filter, confirm *safety.ConfirmationTracker, audit *safety.AuditLogger) tools.Registration {
	const toolName = "docker_create"

	tool := mcp.NewTool(toolName,
		mcp.WithDescription("Create a new Docker container. Requires confirmation."),
		mcp.WithString("name",
			mcp.Description("Container name"),
		),
		mcp.WithString("image",
			mcp.Required(),
			mcp.Description("Image name and optional tag (e.g. nginx:latest)"),
		),
		mcp.WithString("confirmation_token",
			mcp.Description("Confirmation token returned by a prior call to this tool"),
		),
	)

	handler := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		start := time.Now()
		name := req.GetString("name", "")
		image := req.GetString("image", "")
		token := req.GetString("confirmation_token", "")
		params := map[string]any{"name": name, "image": image}

		resourceName := name
		if resourceName == "" {
			resourceName = image
		}

		if name != "" && !filter.IsAllowed(name) {
			dockerToolLogAudit(audit, toolName, params, "denied", start)
			return dockerToolError(fmt.Sprintf("creation of container %q is not allowed", name)), nil
		}

		if !confirm.Confirm(token) {
			desc := fmt.Sprintf("This will create a new container from image %q with name %q.", image, name)
			return dockerToolConfirmPrompt(confirm, toolName, resourceName, desc), nil
		}

		cfg := ContainerCreateConfig{
			Name:  name,
			Image: image,
		}

		containerID, err := mgr.CreateContainer(ctx, cfg)
		if err != nil {
			dockerToolLogAudit(audit, toolName, params, "error: "+err.Error(), start)
			return dockerToolError(err.Error()), nil
		}

		dockerToolLogAudit(audit, toolName, params, "ok: "+containerID, start)
		return mcp.NewToolResultText(fmt.Sprintf("container created with ID %q", containerID)), nil
	}

	return tools.Registration{Tool: tool, Handler: server.ToolHandlerFunc(handler)}
}

func toolDockerPull(mgr DockerManager, audit *safety.AuditLogger) tools.Registration {
	tool := mcp.NewTool("docker_pull",
		mcp.WithDescription("Pull a Docker image from a registry."),
		mcp.WithString("image",
			mcp.Required(),
			mcp.Description("Image name and optional tag (e.g. nginx:latest)"),
		),
	)

	handler := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		start := time.Now()
		image := req.GetString("image", "")
		params := map[string]any{"image": image}

		if err := mgr.PullImage(ctx, image); err != nil {
			dockerToolLogAudit(audit, "docker_pull", params, "error: "+err.Error(), start)
			return dockerToolError(err.Error()), nil
		}

		dockerToolLogAudit(audit, "docker_pull", params, "ok", start)
		return mcp.NewToolResultText(fmt.Sprintf("image %q pulled successfully", image)), nil
	}

	return tools.Registration{Tool: tool, Handler: server.ToolHandlerFunc(handler)}
}

// ---------------------------------------------------------------------------
// Network tools
// ---------------------------------------------------------------------------

func toolDockerNetworkList(mgr DockerManager, audit *safety.AuditLogger) tools.Registration {
	tool := mcp.NewTool("docker_network_list",
		mcp.WithDescription("List all Docker networks."),
	)

	handler := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		start := time.Now()
		params := map[string]any{}

		networks, err := mgr.ListNetworks(ctx)
		if err != nil {
			dockerToolLogAudit(audit, "docker_network_list", params, "error: "+err.Error(), start)
			return dockerToolError(err.Error()), nil
		}

		dockerToolLogAudit(audit, "docker_network_list", params, "ok", start)
		return dockerToolJSONResult(networks), nil
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
			dockerToolLogAudit(audit, "docker_network_inspect", params, "denied", start)
			return dockerToolError(fmt.Sprintf("access to network %q is not allowed", id)), nil
		}

		detail, err := mgr.InspectNetwork(ctx, id)
		if err != nil {
			dockerToolLogAudit(audit, "docker_network_inspect", params, "error: "+err.Error(), start)
			return dockerToolError(err.Error()), nil
		}

		dockerToolLogAudit(audit, "docker_network_inspect", params, "ok", start)
		return dockerToolJSONResult(detail), nil
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
			dockerToolLogAudit(audit, toolName, params, "denied", start)
			return dockerToolError(fmt.Sprintf("creation of network %q is not allowed", name)), nil
		}

		if !confirm.Confirm(token) {
			desc := fmt.Sprintf("This will create a new Docker network %q (driver=%q, subnet=%q).", name, driver, subnet)
			return dockerToolConfirmPrompt(confirm, toolName, name, desc), nil
		}

		cfg := NetworkCreateConfig{
			Name:   name,
			Driver: driver,
			Subnet: subnet,
		}

		networkID, err := mgr.CreateNetwork(ctx, cfg)
		if err != nil {
			dockerToolLogAudit(audit, toolName, params, "error: "+err.Error(), start)
			return dockerToolError(err.Error()), nil
		}

		dockerToolLogAudit(audit, toolName, params, "ok: "+networkID, start)
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
			dockerToolLogAudit(audit, toolName, params, "denied", start)
			return dockerToolError(fmt.Sprintf("access to network %q is not allowed", id)), nil
		}

		if !confirm.Confirm(token) {
			desc := fmt.Sprintf("This will permanently remove network %q.", id)
			return dockerToolConfirmPrompt(confirm, toolName, id, desc), nil
		}

		if err := mgr.RemoveNetwork(ctx, id); err != nil {
			dockerToolLogAudit(audit, toolName, params, "error: "+err.Error(), start)
			return dockerToolError(err.Error()), nil
		}

		dockerToolLogAudit(audit, toolName, params, "ok", start)
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
			dockerToolLogAudit(audit, "docker_network_connect", params, "denied", start)
			return dockerToolError(fmt.Sprintf("access to network %q is not allowed", networkID)), nil
		}
		if !filter.IsAllowed(containerID) {
			dockerToolLogAudit(audit, "docker_network_connect", params, "denied", start)
			return dockerToolError(fmt.Sprintf("access to container %q is not allowed", containerID)), nil
		}

		if err := mgr.ConnectNetwork(ctx, networkID, containerID); err != nil {
			dockerToolLogAudit(audit, "docker_network_connect", params, "error: "+err.Error(), start)
			return dockerToolError(err.Error()), nil
		}

		dockerToolLogAudit(audit, "docker_network_connect", params, "ok", start)
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
			dockerToolLogAudit(audit, "docker_network_disconnect", params, "denied", start)
			return dockerToolError(fmt.Sprintf("access to network %q is not allowed", networkID)), nil
		}
		if !filter.IsAllowed(containerID) {
			dockerToolLogAudit(audit, "docker_network_disconnect", params, "denied", start)
			return dockerToolError(fmt.Sprintf("access to container %q is not allowed", containerID)), nil
		}

		if err := mgr.DisconnectNetwork(ctx, networkID, containerID); err != nil {
			dockerToolLogAudit(audit, "docker_network_disconnect", params, "error: "+err.Error(), start)
			return dockerToolError(err.Error()), nil
		}

		dockerToolLogAudit(audit, "docker_network_disconnect", params, "ok", start)
		return mcp.NewToolResultText(fmt.Sprintf("container %q disconnected from network %q", containerID, networkID)), nil
	}

	return tools.Registration{Tool: tool, Handler: server.ToolHandlerFunc(handler)}
}
