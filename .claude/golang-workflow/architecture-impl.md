# Architecture Implementation Design

## Stage 1: Extract Shared Tool Helpers to `internal/tools/helpers.go`

New file `internal/tools/helpers.go` with 4 exports:
- `JSONResult(v any) *mcp.CallToolResult` — marshal to indented JSON
- `ErrorResult(msg string) *mcp.CallToolResult` — format "error: " + msg
- `LogAudit(audit *safety.AuditLogger, toolName string, params map[string]any, result string, start time.Time)` — nil-safe audit log
- `ConfirmPrompt(confirm *safety.ConfirmationTracker, toolName, resource, description string) *mcp.CallToolResult` — issue confirmation

Remove from docker/tools.go: dockerToolJSONResult, dockerToolError, dockerToolLogAudit, dockerToolConfirmPrompt
Remove from vm/tools.go: vmJSONResult, vmErrorResult, vmLogAudit, vmConfirmationPrompt
Remove from system/tools.go: sysJSONResult, sysLogAudit
Replace 3 inline fmt.Sprintf("error: %s",...) in system/tools.go with tools.ErrorResult

New import: internal/tools -> internal/safety (no cycle)

## Stage 2: Co-locate Destructive Tool Lists

Add to docker/tools.go: `var DestructiveTools = []string{"docker_stop", "docker_restart", "docker_remove", "docker_create", "docker_network_remove"}`
Add to vm/tools.go: `var DestructiveTools = []string{"vm_stop", "vm_force_stop", "vm_restart", "vm_create", "vm_delete"}`
Update main.go to use docker.DestructiveTools and vm.DestructiveTools

## Stage 3a: Split docker/tools.go

- tools.go: DockerTools() factory + DestructiveTools var + imports
- container_tools.go: 10 container tool handler functions
- network_tools.go: 6 network tool handler functions
Split point: line 484 "// Network tools" comment

## Stage 3b: Unify checkError in docker/manager.go

Remove checkError (line 90). Keep checkErrorFromBody (rename to checkAPIError).
Update 6 call sites that use checkError to use readBody() + checkAPIError() pattern.
Call sites: ListContainers, InspectContainer, GetLogs, GetStats, ListNetworks, InspectNetwork.

## Stage 3c: Extract sentinel error in vm/manager_stub.go

Add: `var ErrLibvirtNotCompiled = errors.New("libvirt support not compiled: rebuild with -tags libvirt")`
Replace 12 identical fmt.Errorf calls with ErrLibvirtNotCompiled.
NewLibvirtVMManager wraps with %w: `fmt.Errorf("%w and ensure ... (socket: %s)", ErrLibvirtNotCompiled, socketPath)`

## Stage 4: Extract main.go Helpers to internal/config

Add to internal/config/config.go:
- `ApplyEnvOverrides(cfg *Config)` — reads UNRAID_MCP_AUTH_TOKEN env
- `EnsureAuthToken(cfg *Config) (string, error)` — generates token if empty
- `GenerateRandomToken() (string, error)` — 32-char hex random token

Update main.go: call config.ApplyEnvOverrides, config.EnsureAuthToken, remove 3 private functions.
EnsureAuthToken returns (string, error) instead of logging directly.

## Stage 5: Split DockerManager Interface

In types.go:
```
ContainerManager interface { 10 methods: List, Inspect, Start, Stop, Restart, Remove, Create, Pull, GetLogs, GetStats }
NetworkManager interface { 6 methods: ListNetworks, InspectNetwork, CreateNetwork, RemoveNetwork, ConnectNetwork, DisconnectNetwork }
DockerManager interface { ContainerManager; NetworkManager }
```

Add compile-time checks in manager.go and manager_test.go.
Fully backward-compatible — DockerManager retains all 16 methods via embedding.
