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
			tools.LogAudit(audit, "docker_list", params, "error: "+err.Error(), start)
			return tools.ErrorResult(err.Error()), nil
		}

		// Apply filter to the result set.
		filtered := make([]Container, 0, len(containers))
		for _, c := range containers {
			if filter.IsAllowed(c.Name) {
				filtered = append(filtered, c)
			}
		}

		tools.LogAudit(audit, "docker_list", params, "ok", start)
		return tools.JSONResult(filtered), nil
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
			tools.LogAudit(audit, "docker_inspect", params, "denied", start)
			return tools.ErrorResult(fmt.Sprintf("access to container %q is not allowed", id)), nil
		}

		detail, err := mgr.InspectContainer(ctx, id)
		if err != nil {
			tools.LogAudit(audit, "docker_inspect", params, "error: "+err.Error(), start)
			return tools.ErrorResult(err.Error()), nil
		}

		tools.LogAudit(audit, "docker_inspect", params, "ok", start)
		return tools.JSONResult(detail), nil
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
			tools.LogAudit(audit, "docker_logs", params, "denied", start)
			return tools.ErrorResult(fmt.Sprintf("access to container %q is not allowed", id)), nil
		}

		logs, err := mgr.GetLogs(ctx, id, tail)
		if err != nil {
			tools.LogAudit(audit, "docker_logs", params, "error: "+err.Error(), start)
			return tools.ErrorResult(err.Error()), nil
		}

		tools.LogAudit(audit, "docker_logs", params, "ok", start)
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
			tools.LogAudit(audit, "docker_stats", params, "denied", start)
			return tools.ErrorResult(fmt.Sprintf("access to container %q is not allowed", id)), nil
		}

		stats, err := mgr.GetStats(ctx, id)
		if err != nil {
			tools.LogAudit(audit, "docker_stats", params, "error: "+err.Error(), start)
			return tools.ErrorResult(err.Error()), nil
		}

		tools.LogAudit(audit, "docker_stats", params, "ok", start)
		return tools.JSONResult(stats), nil
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
			tools.LogAudit(audit, "docker_start", params, "denied", start)
			return tools.ErrorResult(fmt.Sprintf("access to container %q is not allowed", id)), nil
		}

		if err := mgr.StartContainer(ctx, id); err != nil {
			tools.LogAudit(audit, "docker_start", params, "error: "+err.Error(), start)
			return tools.ErrorResult(err.Error()), nil
		}

		tools.LogAudit(audit, "docker_start", params, "ok", start)
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
			tools.LogAudit(audit, toolName, params, "denied", start)
			return tools.ErrorResult(fmt.Sprintf("access to container %q is not allowed", id)), nil
		}

		if !confirm.Confirm(token) {
			desc := fmt.Sprintf("This will stop container %q with a %d second timeout.", id, timeout)
			return tools.ConfirmPrompt(confirm, toolName, id, desc), nil
		}

		if err := mgr.StopContainer(ctx, id, timeout); err != nil {
			tools.LogAudit(audit, toolName, params, "error: "+err.Error(), start)
			return tools.ErrorResult(err.Error()), nil
		}

		tools.LogAudit(audit, toolName, params, "ok", start)
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
			tools.LogAudit(audit, toolName, params, "denied", start)
			return tools.ErrorResult(fmt.Sprintf("access to container %q is not allowed", id)), nil
		}

		if !confirm.Confirm(token) {
			desc := fmt.Sprintf("This will restart container %q with a %d second timeout.", id, timeout)
			return tools.ConfirmPrompt(confirm, toolName, id, desc), nil
		}

		if err := mgr.RestartContainer(ctx, id, timeout); err != nil {
			tools.LogAudit(audit, toolName, params, "error: "+err.Error(), start)
			return tools.ErrorResult(err.Error()), nil
		}

		tools.LogAudit(audit, toolName, params, "ok", start)
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
			tools.LogAudit(audit, toolName, params, "denied", start)
			return tools.ErrorResult(fmt.Sprintf("access to container %q is not allowed", id)), nil
		}

		if !confirm.Confirm(token) {
			desc := fmt.Sprintf("This will permanently remove container %q (force=%v).", id, force)
			return tools.ConfirmPrompt(confirm, toolName, id, desc), nil
		}

		if err := mgr.RemoveContainer(ctx, id, force); err != nil {
			tools.LogAudit(audit, toolName, params, "error: "+err.Error(), start)
			return tools.ErrorResult(err.Error()), nil
		}

		tools.LogAudit(audit, toolName, params, "ok", start)
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
			tools.LogAudit(audit, toolName, params, "denied", start)
			return tools.ErrorResult(fmt.Sprintf("creation of container %q is not allowed", name)), nil
		}

		if !confirm.Confirm(token) {
			desc := fmt.Sprintf("This will create a new container from image %q with name %q.", image, name)
			return tools.ConfirmPrompt(confirm, toolName, resourceName, desc), nil
		}

		cfg := ContainerCreateConfig{
			Name:  name,
			Image: image,
		}

		containerID, err := mgr.CreateContainer(ctx, cfg)
		if err != nil {
			tools.LogAudit(audit, toolName, params, "error: "+err.Error(), start)
			return tools.ErrorResult(err.Error()), nil
		}

		tools.LogAudit(audit, toolName, params, "ok: "+containerID, start)
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
			tools.LogAudit(audit, "docker_pull", params, "error: "+err.Error(), start)
			return tools.ErrorResult(err.Error()), nil
		}

		tools.LogAudit(audit, "docker_pull", params, "ok", start)
		return mcp.NewToolResultText(fmt.Sprintf("image %q pulled successfully", image)), nil
	}

	return tools.Registration{Tool: tool, Handler: server.ToolHandlerFunc(handler)}
}
