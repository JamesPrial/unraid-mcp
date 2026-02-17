// Package docker provides Docker container and network management for the unraid-mcp server.
package docker

import (
	"github.com/jamesprial/unraid-mcp/internal/safety"
	"github.com/jamesprial/unraid-mcp/internal/tools"
)

// DestructiveTools lists Docker tool names that require confirmation before execution.
var DestructiveTools = []string{
	"docker_stop",
	"docker_restart",
	"docker_remove",
	"docker_create",
	"docker_network_remove",
}

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
