# Explorer Findings: Unraid MCP Refactoring Analysis

Generated: 2026-02-17
Analyzed by: Code Explorer Agent

---

## Overview

This document captures all findings required for the 5-stage refactoring plan of the
`github.com/jamesprial/unraid-mcp` codebase. The MCP server exposes 31 tools (16 Docker,
12 VM, 3 system health) over Streamable HTTP. The module path is
`github.com/jamesprial/unraid-mcp`.

---

## File Inventory (Absolute Paths)

```
/Users/jamesprial/code/unraid-mcp/
  cmd/server/main.go
  internal/
    auth/
      middleware.go
      middleware_test.go
    config/
      config.go
      config_test.go
    docker/
      manager.go
      manager_test.go        ← contains MockDockerManager
      tools.go
      types.go
    safety/
      audit.go
      audit_test.go
      confirm.go
      confirm_test.go
      filter.go
      filter_test.go
    system/
      health.go
      health_test.go
      tools.go
      types.go
    tools/
      registration.go
    vm/
      manager.go             ← build tag: libvirt
      manager_stub.go        ← build tag: !libvirt
      manager_test.go        ← contains MockVMManager
      tools.go
      types.go
  testdata/
    config/invalid.yaml, valid.yaml
    emhttp/disks.ini, var.ini
    proc/meminfo, stat
    sys/hwmon/hwmon0, hwmon1
```

---

## Stage 1: Duplicated Tool Helpers

### Pattern Summary

Three packages define functionally identical sets of helper functions with package-specific
prefixes. The bodies are byte-for-byte identical in logic.

### Exact Duplicates Across Packages

#### JSONResult pattern

| Package  | Function name         | File                                    | Lines  |
|----------|-----------------------|-----------------------------------------|--------|
| docker   | `dockerToolJSONResult`| `/internal/docker/tools.go`             | 50–56  |
| vm       | `vmJSONResult`        | `/internal/vm/tools.go`                 | 46–52  |
| system   | `sysJSONResult`       | `/internal/system/tools.go`             | 31–37  |

All three bodies:
```go
data, err := json.MarshalIndent(v, "", "  ")
if err != nil {
    return mcp.NewToolResultText(fmt.Sprintf("error marshaling result: %v", err))
}
return mcp.NewToolResultText(string(data))
```

#### ErrorResult pattern

| Package  | Function name      | File                            | Lines  |
|----------|--------------------|---------------------------------|--------|
| docker   | `dockerToolError`  | `/internal/docker/tools.go`     | 59–61  |
| vm       | `vmErrorResult`    | `/internal/vm/tools.go`         | 55–57  |

Both bodies:
```go
return mcp.NewToolResultText(fmt.Sprintf("error: %s", msg))
```

NOTE: `system/tools.go` does NOT use a named `sysError` helper. Instead, the three system
tool handlers inline the error return directly:
```go
return mcp.NewToolResultText(fmt.Sprintf("error: %s", err.Error())), nil
```
at lines 69, 91, 113. These three inline sites should also be unified.

#### LogAudit pattern

| Package  | Function name         | File                            | Lines  |
|----------|-----------------------|---------------------------------|--------|
| docker   | `dockerToolLogAudit`  | `/internal/docker/tools.go`     | 65–76  |
| vm       | `vmLogAudit`          | `/internal/vm/tools.go`         | 60–71  |
| system   | `sysLogAudit`         | `/internal/system/tools.go`     | 40–51  |

All three bodies:
```go
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
```

Signature differences:
- docker: `(audit *safety.AuditLogger, toolName string, params map[string]any, result string, start time.Time)`
- vm:     `(audit *safety.AuditLogger, tool string, params map[string]any, result string, start time.Time)`
- system: `(audit *safety.AuditLogger, tool string, params map[string]any, result string, start time.Time)`

The `toolName` vs `tool` parameter name difference is cosmetic only; the bodies are identical.

#### ConfirmPrompt pattern

| Package  | Function name              | File                        | Lines  |
|----------|----------------------------|-----------------------------|--------|
| docker   | `dockerToolConfirmPrompt`  | `/internal/docker/tools.go` | 80–86  |
| vm       | `vmConfirmationPrompt`     | `/internal/vm/tools.go`     | 74–80  |

NOTE: `system/tools.go` has NO ConfirmPrompt because system tools are read-only.

Both bodies:
```go
token := confirm.RequestConfirmation(tool, resource, description)
return mcp.NewToolResultText(fmt.Sprintf(
    "Confirmation required for %s on %q.\n\n%s\n\nTo proceed, call %s again with confirmation_token=%q.",
    tool, resource, description, tool, token,
))
```

Parameter name differences cosmetic only (`toolName` vs `tool`).

### Proposed New File: `/internal/tools/helpers.go`

Imports needed: `encoding/json`, `fmt`, `time`, `github.com/jamesprial/unraid-mcp/internal/safety`,
`github.com/mark3labs/mcp-go/mcp`.

IMPORTANT: The `internal/tools` package currently only imports `mcp-go/mcp` and `mcp-go/server`.
Adding `safety` would add a new dependency on the `internal/safety` package. Verify no cycle:
- `tools` -> `safety`: OK (safety imports nothing internal)
- `docker`, `vm`, `system` currently import both `tools` and `safety`, so this is safe.

### All Call Sites for Each Helper

#### `dockerToolJSONResult` call sites (in `/internal/docker/tools.go`)
- Line 120: `return dockerToolJSONResult(filtered), nil` (toolDockerList)
- Line 152: `return dockerToolJSONResult(detail), nil` (toolDockerInspect)
- Line 220: `return dockerToolJSONResult(stats), nil` (toolDockerStats)
- Line 504: `return dockerToolJSONResult(networks), nil` (toolDockerNetworkList)
- Line 535: `return dockerToolJSONResult(detail), nil` (toolDockerNetworkInspect)

#### `dockerToolError` call sites (in `/internal/docker/tools.go`)
- Line 108: `return dockerToolError(err.Error()), nil` (toolDockerList)
- Line 142: `return dockerToolError(fmt.Sprintf("access to container %q is not allowed", id)), nil`
- Line 148: `return dockerToolError(err.Error()), nil`
- Line 179: `return dockerToolError(fmt.Sprintf("access to container %q is not allowed", id)), nil`
- Line 184: `return dockerToolError(err.Error()), nil`
- Line 209: `return dockerToolError(fmt.Sprintf("access to container %q is not allowed", id)), nil`
- Line 215: `return dockerToolError(err.Error()), nil`
- Line 243: `return dockerToolError(fmt.Sprintf("access to container %q is not allowed", id)), nil`
- Line 247: `return dockerToolError(err.Error()), nil`
- Line 286: `return dockerToolError(fmt.Sprintf("access to container %q is not allowed", id)), nil`
- Line 295: `return dockerToolError(err.Error()), nil`
- Line 334: `return dockerToolError(fmt.Sprintf("access to container %q is not allowed", id)), nil`
- Line 390: `return dockerToolError(err.Error()), nil`
- Line 433: `return dockerToolError(fmt.Sprintf("creation of container %q is not allowed", name)), nil`
- Line 448: `return dockerToolError(err.Error()), nil`
- Line 474: `return dockerToolError(err.Error()), nil`
- Line 524: `return dockerToolError(fmt.Sprintf("access to network %q is not allowed", id)), nil`
- Line 530: `return dockerToolError(err.Error()), nil`
- Line 571: `return dockerToolError(fmt.Sprintf("creation of network %q is not allowed", name)), nil`
- Line 589: `return dockerToolError(err.Error()), nil`
- Line 619: `return dockerToolError(fmt.Sprintf("access to network %q is not allowed", id)), nil`
- Line 630: `return dockerToolError(err.Error()), nil`
- Line 662: `return dockerToolError(fmt.Sprintf("access to network %q is not allowed", networkID)), nil`
- Line 664: `return dockerToolError(fmt.Sprintf("access to container %q is not allowed", containerID)), nil`
- Line 670: `return dockerToolError(err.Error()), nil`
- Line 701: `return dockerToolError(fmt.Sprintf("access to network %q is not allowed", networkID)), nil`
- Line 703: `return dockerToolError(fmt.Sprintf("access to container %q is not allowed", containerID)), nil`
- Line 710: `return dockerToolError(err.Error()), nil`

#### `dockerToolLogAudit` call sites (in `/internal/docker/tools.go`)
- Line 107: `dockerToolLogAudit(audit, "docker_list", params, "error: "+err.Error(), start)`
- Line 119: `dockerToolLogAudit(audit, "docker_list", params, "ok", start)`
- Line 141: `dockerToolLogAudit(audit, "docker_inspect", params, "denied", start)`
- Line 147: `dockerToolLogAudit(audit, "docker_inspect", params, "error: "+err.Error(), start)`
- Line 151: `dockerToolLogAudit(audit, "docker_inspect", params, "ok", start)`
- Line 178: `dockerToolLogAudit(audit, "docker_logs", params, "denied", start)`
- Line 183: `dockerToolLogAudit(audit, "docker_logs", params, "error: "+err.Error(), start)`
- Line 187: `dockerToolLogAudit(audit, "docker_logs", params, "ok", start)`
- Line 208: `dockerToolLogAudit(audit, "docker_stats", params, "denied", start)`
- Line 214: `dockerToolLogAudit(audit, "docker_stats", params, "error: "+err.Error(), start)`
- Line 219: `dockerToolLogAudit(audit, "docker_stats", params, "ok", start)`
- Line 242: `dockerToolLogAudit(audit, "docker_start", params, "denied", start)`
- Line 246: `dockerToolLogAudit(audit, "docker_start", params, "error: "+err.Error(), start)`
- Line 250: `dockerToolLogAudit(audit, "docker_start", params, "ok", start)`
- Line 285: `dockerToolLogAudit(audit, toolName, params, "denied", start)`
- Line 295: `dockerToolLogAudit(audit, toolName, params, "error: "+err.Error(), start)`  [stop]
- Line 299: `dockerToolLogAudit(audit, toolName, params, "ok", start)` [stop]
- Line 334: `dockerToolLogAudit(audit, toolName, params, "denied", start)` [restart]
- Line 344: `dockerToolLogAudit(audit, toolName, params, "error: "+err.Error(), start)` [restart]
- Line 348: `dockerToolLogAudit(audit, toolName, params, "ok", start)` [restart]
- Line 380: `dockerToolLogAudit(audit, toolName, params, "denied", start)` [remove]
- Line 390: `dockerToolLogAudit(audit, toolName, params, "error: "+err.Error(), start)` [remove]
- Line 394: `dockerToolLogAudit(audit, toolName, params, "ok", start)` [remove]
- Line 431: `dockerToolLogAudit(audit, toolName, params, "denied", start)` [create]
- Line 447: `dockerToolLogAudit(audit, toolName, params, "error: "+err.Error(), start)` [create]
- Line 451: `dockerToolLogAudit(audit, toolName, params, "ok: "+containerID, start)` [create]
- Line 473: `dockerToolLogAudit(audit, "docker_pull", params, "error: "+err.Error(), start)`
- Line 477: `dockerToolLogAudit(audit, "docker_pull", params, "ok", start)`
- Line 499: `dockerToolLogAudit(audit, "docker_network_list", params, "error: "+err.Error(), start)`
- Line 503: `dockerToolLogAudit(audit, "docker_network_list", params, "ok", start)`
- Line 524: `dockerToolLogAudit(audit, "docker_network_inspect", params, "denied", start)`
- Line 530: `dockerToolLogAudit(audit, "docker_network_inspect", params, "error: "+err.Error(), start)`
- Line 535: `dockerToolLogAudit(audit, "docker_network_inspect", params, "ok", start)`
- Line 571: `dockerToolLogAudit(audit, toolName, params, "denied", start)` [net_create]
- Line 589: `dockerToolLogAudit(audit, toolName, params, "error: "+err.Error(), start)` [net_create]
- Line 592: `dockerToolLogAudit(audit, toolName, params, "ok: "+networkID, start)` [net_create]
- Line 619: `dockerToolLogAudit(audit, toolName, params, "denied", start)` [net_remove]
- Line 630: `dockerToolLogAudit(audit, toolName, params, "error: "+err.Error(), start)` [net_remove]
- Line 634: `dockerToolLogAudit(audit, toolName, params, "ok", start)` [net_remove]
- Line 662: `dockerToolLogAudit(audit, "docker_network_connect", params, "denied", start)`
- Line 664: `dockerToolLogAudit(audit, "docker_network_connect", params, "denied", start)`
- Line 670: `dockerToolLogAudit(audit, "docker_network_connect", params, "error: "+err.Error(), start)`
- Line 674: `dockerToolLogAudit(audit, "docker_network_connect", params, "ok", start)`
- Line 701: `dockerToolLogAudit(audit, "docker_network_disconnect", params, "denied", start)`
- Line 703: `dockerToolLogAudit(audit, "docker_network_disconnect", params, "denied", start)`
- Line 710: `dockerToolLogAudit(audit, "docker_network_disconnect", params, "error: "+err.Error(), start)`
- Line 714: `dockerToolLogAudit(audit, "docker_network_disconnect", params, "ok", start)`

#### `dockerToolConfirmPrompt` call sites (in `/internal/docker/tools.go`)
- Line 291: `return dockerToolConfirmPrompt(confirm, toolName, id, desc), nil` (toolDockerStop)
- Line 340: `return dockerToolConfirmPrompt(confirm, toolName, id, desc), nil` (toolDockerRestart)
- Line 386: `return dockerToolConfirmPrompt(confirm, toolName, id, desc), nil` (toolDockerRemove)
- Line 437: `return dockerToolConfirmPrompt(confirm, toolName, resourceName, desc), nil` (toolDockerCreate)
- Line 577: `return dockerToolConfirmPrompt(confirm, toolName, name, desc), nil` (toolDockerNetworkCreate)
- Line 625: `return dockerToolConfirmPrompt(confirm, toolName, id, desc), nil` (toolDockerNetworkRemove)

#### `vmJSONResult` call sites (in `/internal/vm/tools.go`)
- Line 102: `return vmJSONResult(vms), nil` (vmList)
- Line 134: `return vmJSONResult(detail), nil` (vmInspect)
- Line 464: `return vmJSONResult(snapshots), nil` (vmSnapshotList)

#### `vmErrorResult` call sites (in `/internal/vm/tools.go`)
- Line 98: `return vmErrorResult(err.Error()), nil`
- Line 124: `return vmErrorResult(fmt.Sprintf("access to VM %q is not allowed", name)), nil`
- Line 129: `return vmErrorResult(err.Error()), nil`
- Line 155: `return vmErrorResult(fmt.Sprintf("access to VM %q is not allowed", name)), nil`
- Line 160: `return vmErrorResult(err.Error()), nil`
- Line 192: `return vmErrorResult(fmt.Sprintf("access to VM %q is not allowed", name)), nil`
- Line 201: `return vmErrorResult(err.Error()), nil`
- Line 234: `return vmErrorResult(fmt.Sprintf("access to VM %q is not allowed", name)), nil`
- Line 244: `return vmErrorResult(err.Error()), nil`
- Line 270: `return vmErrorResult(fmt.Sprintf("access to VM %q is not allowed", name)), nil`
- Line 274: `return vmErrorResult(err.Error()), nil`
- Line 303: `return vmErrorResult(fmt.Sprintf("access to VM %q is not allowed", name)), nil`
- Line 307: `return vmErrorResult(err.Error()), nil`
- Line 338: `return vmErrorResult(fmt.Sprintf("access to VM %q is not allowed", name)), nil`
- Line 348: `return vmErrorResult(err.Error()), nil`
- Line 385: `return vmErrorResult(err.Error()), nil`
- Line 416: `return vmErrorResult(fmt.Sprintf("access to VM %q is not allowed", name)), nil`
- Line 427: `return vmErrorResult(err.Error()), nil`
- Line 453: `return vmErrorResult(fmt.Sprintf("access to VM %q is not allowed", name)), nil`
- Line 459: `return vmErrorResult(err.Error()), nil`
- Line 489: `return vmErrorResult(fmt.Sprintf("access to VM %q is not allowed", name)), nil`
- Line 494: `return vmErrorResult(err.Error()), nil`

#### `vmLogAudit` call sites (in `/internal/vm/tools.go`)
- Line 97: `vmLogAudit(audit, "vm_list", params, "error: "+err.Error(), start)`
- Line 101: `vmLogAudit(audit, "vm_list", params, "ok", start)`
- Line 123: `vmLogAudit(audit, "vm_inspect", params, "denied", start)`
- Line 128: `vmLogAudit(audit, "vm_inspect", params, "error: "+err.Error(), start)`
- Line 133: `vmLogAudit(audit, "vm_inspect", params, "ok", start)`
- Line 154: `vmLogAudit(audit, "vm_start", params, "denied", start)`
- Line 159: `vmLogAudit(audit, "vm_start", params, "error: "+err.Error(), start)`
- Line 163: `vmLogAudit(audit, "vm_start", params, "ok", start)`
- Line 191: `vmLogAudit(audit, toolName, params, "denied", start)` [stop]
- Line 200: `vmLogAudit(audit, toolName, params, "error: "+err.Error(), start)` [stop]
- Line 204: `vmLogAudit(audit, toolName, params, "ok", start)` [stop]
- Line 233: `vmLogAudit(audit, toolName, params, "denied", start)` [force_stop]
- Line 243: `vmLogAudit(audit, toolName, params, "error: "+err.Error(), start)` [force_stop]
- Line 247: `vmLogAudit(audit, toolName, params, "ok", start)` [force_stop]
- Line 269: `vmLogAudit(audit, "vm_pause", params, "denied", start)`
- Line 273: `vmLogAudit(audit, "vm_pause", params, "error: "+err.Error(), start)`
- Line 278: `vmLogAudit(audit, "vm_pause", params, "ok", start)`
- Line 302: `vmLogAudit(audit, "vm_resume", params, "denied", start)`
- Line 306: `vmLogAudit(audit, "vm_resume", params, "error: "+err.Error(), start)`
- Line 311: `vmLogAudit(audit, "vm_resume", params, "ok", start)`
- Line 337: `vmLogAudit(audit, toolName, params, "denied", start)` [restart]
- Line 347: `vmLogAudit(audit, toolName, params, "error: "+err.Error(), start)` [restart]
- Line 351: `vmLogAudit(audit, toolName, params, "ok", start)` [restart]
- Line 384: `vmLogAudit(audit, toolName, params, "error: "+err.Error(), start)` [create]
- Line 389: `vmLogAudit(audit, toolName, params, "ok", start)` [create]
- Line 415: `vmLogAudit(audit, toolName, params, "denied", start)` [delete]
- Line 426: `vmLogAudit(audit, toolName, params, "error: "+err.Error(), start)` [delete]
- Line 431: `vmLogAudit(audit, toolName, params, "ok", start)` [delete]
- Line 452: `vmLogAudit(audit, "vm_snapshot_list", params, "denied", start)`
- Line 458: `vmLogAudit(audit, "vm_snapshot_list", params, "error: "+err.Error(), start)`
- Line 463: `vmLogAudit(audit, "vm_snapshot_list", params, "ok", start)`
- Line 489: `vmLogAudit(audit, "vm_snapshot_create", params, "denied", start)`
- Line 494: `vmLogAudit(audit, "vm_snapshot_create", params, "error: "+err.Error(), start)`
- Line 499: `vmLogAudit(audit, "vm_snapshot_create", params, "ok", start)`

#### `vmConfirmationPrompt` call sites (in `/internal/vm/tools.go`)
- Line 198: `return vmConfirmationPrompt(confirm, toolName, name, desc), nil` (vmStop)
- Line 240: `return vmConfirmationPrompt(confirm, toolName, name, desc), nil` (vmForceStop)
- Line 344: `return vmConfirmationPrompt(confirm, toolName, name, desc), nil` (vmRestart)
- Line 381: `return vmConfirmationPrompt(confirm, toolName, "new-vm", desc), nil` (vmCreate)
- Line 422: `return vmConfirmationPrompt(confirm, toolName, name, desc), nil` (vmDelete)

#### `sysJSONResult` call sites (in `/internal/system/tools.go`)
- Line 73: `return sysJSONResult(overview), nil`
- Line 95: `return sysJSONResult(status), nil`
- Line 118: `return sysJSONResult(disks), nil`

#### `sysLogAudit` call sites (in `/internal/system/tools.go`)
- Line 68: `sysLogAudit(audit, "system_overview", params, "error: "+err.Error(), start)`
- Line 72: `sysLogAudit(audit, "system_overview", params, "ok", start)`
- Line 90: `sysLogAudit(audit, "system_array_status", params, "error: "+err.Error(), start)`
- Line 94: `sysLogAudit(audit, "system_array_status", params, "ok", start)`
- Line 112: `sysLogAudit(audit, "system_disks", params, "error: "+err.Error(), start)`
- Line 116: `sysLogAudit(audit, "system_disks", params, "ok", start)`

---

## Stage 2: Destructive Tool Lists

### Location in main.go

File: `/Users/jamesprial/code/unraid-mcp/cmd/server/main.go`
Lines 55–68:

```go
destructiveDockerTools := []string{
    "docker_stop",
    "docker_restart",
    "docker_remove",
    "docker_create",
    "docker_network_remove",
}
destructiveVMTools := []string{
    "vm_stop",
    "vm_force_stop",
    "vm_restart",
    "vm_create",
    "vm_delete",
}
```

### How They Are Used in main.go

Lines 70–71:
```go
dockerConfirm := safety.NewConfirmationTracker(destructiveDockerTools)
vmConfirm := safety.NewConfirmationTracker(destructiveVMTools)
```

Both `dockerConfirm` and `vmConfirm` are then passed to the tools factory functions on
lines 103 and 106:
```go
registrations = append(registrations, docker.DockerTools(dockerMgr, dockerFilter, dockerConfirm, auditLogger)...)
registrations = append(registrations, vm.VMTools(vmMgr, vmFilter, vmConfirm, auditLogger)...)
```

### Proposed Move

Move `destructiveDockerTools` to a `var` or exported function in
`/Users/jamesprial/code/unraid-mcp/internal/docker/tools.go` (e.g., `DestructiveDockerTools() []string`
or `var DestructiveDockerTools = []string{...}`).

Move `destructiveVMTools` similarly to
`/Users/jamesprial/code/unraid-mcp/internal/vm/tools.go`.

Then `main.go` would call:
```go
dockerConfirm := safety.NewConfirmationTracker(docker.DestructiveDockerTools)
vmConfirm := safety.NewConfirmationTracker(vm.DestructiveVMTools)
```

The tool names match exactly what is used inside each package's handler closures (as the
`const toolName` values), ensuring they stay co-located with the tools that use them.

Cross-reference: The tool names in the lists match the `const toolName` values declared
inside each tool function:
- `"docker_stop"` → `const toolName = "docker_stop"` at line 258
- `"docker_restart"` → `const toolName = "docker_restart"` at line 307
- `"docker_remove"` → `const toolName = "docker_remove"` at line 356
- `"docker_create"` → `const toolName = "docker_create"` at line 402
- `"docker_network_remove"` → `const toolName = "docker_network_remove"` at line 600
- `"vm_stop"` → `const toolName = "vm_stop"` at line 172
- `"vm_force_stop"` → `const toolName = "vm_force_stop"` at line 214
- `"vm_restart"` → `const toolName = "vm_restart"` at line 318
- `"vm_create"` → `const toolName = "vm_create"` at line 360
- `"vm_delete"` → `const toolName = "vm_delete"` at line 397

---

## Stage 3: File Organization

### docker/tools.go Container vs Network Split

The file `/Users/jamesprial/code/unraid-mcp/internal/docker/tools.go` has a natural
split point already marked by a comment:

```
Line 1–43:    Package declaration, imports, DockerTools() factory function
Line 45–87:   // --- Helpers --- (4 private helper functions)
Line 88–482:  // --- Container tools --- (10 tool constructors)
Line 484–719: // --- Network tools --- (6 tool constructors)
```

Exact split:

**container_tools.go** (package docker) should contain:
- The `DockerTools()` factory function (or a subset of it)
- The helpers section (or move to helpers.go per Stage 1)
- `toolDockerList` (line 92)
- `toolDockerInspect` (line 126)
- `toolDockerLogs` (line 158)
- `toolDockerStats` (line 194)
- `toolDockerStart` (line 226)
- `toolDockerStop` (line 257)
- `toolDockerRestart` (line 306)
- `toolDockerRemove` (line 355)
- `toolDockerCreate` (line 401)
- `toolDockerPull` (line 458)

**network_tools.go** (package docker) should contain:
- `toolDockerNetworkList` (line 488)
- `toolDockerNetworkInspect` (line 510)
- `toolDockerNetworkCreate` (line 542)
- `toolDockerNetworkRemove` (line 599)
- `toolDockerNetworkConnect` (line 641)
- `toolDockerNetworkDisconnect` (line 681)

The `DockerTools()` factory function calls both container and network tools, so it should
remain in `container_tools.go` (or a separate `tools.go` that aggregates both). The
existing separator comment `// ---------------------------------------------------------------------------\n// Network tools` at line 484 is the clean split boundary.

### checkError vs checkErrorFromBody in docker/manager.go

File: `/Users/jamesprial/code/unraid-mcp/internal/docker/manager.go`

**`checkError`** (lines 90–108): Takes `*http.Response` + `notFoundMsg string`. Calls
`readBody(resp)` internally. Used when the body has NOT yet been read.

**`checkErrorFromBody`** (lines 447–463): Takes `statusCode int`, `body []byte`,
`notFoundMsg string`. Used after the body has already been read via `readBody(resp)`.

Both have identical error extraction logic. The distinction is purely technical: some
operations need to read the body separately before error-checking (e.g., to then decode
JSON from it).

#### `checkError` call sites (in `/internal/docker/manager.go`)
- Line 137: `ListContainers` — `if err := checkError(resp, "container not found"); err != nil`
- Line 209: `InspectContainer` — `if err := checkError(resp, fmt.Sprintf("container not found: %s", id)); err != nil`
  NOTE: There is also a manual StatusNotFound check at line 206 before `checkError`.
- Line 505: `GetLogs` — `if err := checkError(resp, fmt.Sprintf("container not found: %s", id)); err != nil`
- Line 574: `GetStats` — `if err := checkError(resp, fmt.Sprintf("container not found: %s", id)); err != nil`
- Line 636: `ListNetworks` — `if err := checkError(resp, "network not found"); err != nil`
- Line 688: `InspectNetwork` — `if err := checkError(resp, fmt.Sprintf("network not found: %s", id)); err != nil`
  NOTE: Manual StatusNotFound check at line 685 before `checkError`.

#### `checkErrorFromBody` call sites (in `/internal/docker/manager.go`)
- Line 278: `StartContainer` — `if err := checkErrorFromBody(resp.StatusCode, body, ...); err != nil`
- Line 303: `StopContainer` — `if err := checkErrorFromBody(resp.StatusCode, body, ...); err != nil`
- Line 326: `RestartContainer` — `if err := checkErrorFromBody(resp.StatusCode, body, ...); err != nil`
- Line 354: `RemoveContainer` — `if err := checkErrorFromBody(resp.StatusCode, body, ...); err != nil`
- Line 429: `CreateContainer` — `if err := checkErrorFromBody(resp.StatusCode, body, ...); err != nil`
- Line 772: `CreateNetwork` — `if err := checkErrorFromBody(resp.StatusCode, body, ...); err != nil`
- Line 804: `RemoveNetwork` — `if err := checkErrorFromBody(resp.StatusCode, body, ...); err != nil`
- Line 837: `ConnectNetwork` — `if err := checkErrorFromBody(resp.StatusCode, body, ...); err != nil`
- Line 871: `DisconnectNetwork` — `if err := checkErrorFromBody(resp.StatusCode, body, ...); err != nil`

**Unification approach**: `checkError` can be implemented by reading body and delegating
to `checkErrorFromBody`:
```go
func checkError(resp *http.Response, notFoundMsg string) error {
    body, _ := readBody(resp)
    return checkErrorFromBody(resp.StatusCode, body, notFoundMsg)
}
```
This eliminates the code duplication. Note the existing `checkError` also calls `readBody`
internally at line 94, so the behavior is already equivalent.

### vm/manager_stub.go Repeated Error Message

File: `/Users/jamesprial/code/unraid-mcp/internal/vm/manager_stub.go`

The message `"libvirt support not compiled: rebuild with -tags libvirt"` appears **12 times**
(once per method stub body, lines 42, 46, 51, 56, 61, 66, 71, 76, 81, 86, 91, 96).

The constructor (`NewLibvirtVMManager`) has a slightly longer variant with socket path info
(line 30–34).

Proposed sentinel error:
```go
// errStubNotCompiled is returned by all methods in the libvirt stub.
var errStubNotCompiled = errors.New("libvirt support not compiled: rebuild with -tags libvirt")
```

Then each method becomes:
```go
func (m *LibvirtVMManager) ListVMs(_ context.Context) ([]VM, error) {
    return nil, errStubNotCompiled
}
```

This requires adding `"errors"` to the import block (currently only `"context"` and `"fmt"`).

---

## Stage 4: main.go Helpers to Extract

### Candidates for Extraction to `/internal/config/config.go`

File: `/Users/jamesprial/code/unraid-mcp/cmd/server/main.go`

#### `applyEnvOverrides` (lines 169–173)

```go
func applyEnvOverrides(cfg *config.Config) {
    if token := os.Getenv("UNRAID_MCP_AUTH_TOKEN"); token != "" {
        cfg.Server.AuthToken = token
    }
}
```

- Takes `*config.Config`, reads env var `UNRAID_MCP_AUTH_TOKEN`
- Currently only overrides `AuthToken`; designed to be extended
- Moving to `config.go` gives it access to the `Config` type without import
- In `config.go`, this would be a method or free function in the `config` package
- Requires `os` import (already not in `config.go`'s current imports)

#### `ensureAuthToken` (lines 177–190)

```go
func ensureAuthToken(cfg *config.Config) {
    if cfg.Server.AuthToken != "" {
        return
    }
    token, err := generateRandomToken()
    if err != nil {
        log.Printf("warning: could not generate auth token: %v — running without authentication", err)
        return
    }
    cfg.Server.AuthToken = token
    log.Printf("generated auth token (set UNRAID_MCP_AUTH_TOKEN to persist): %s", token)
}
```

- Depends on `generateRandomToken()` (below)
- Uses `log.Printf` — moving to config package adds `log` dependency to `config` package
- Alternatively, return `(string, error)` and let `main.go` handle logging

#### `generateRandomToken` (lines 194–200)

```go
func generateRandomToken() (string, error) {
    b := make([]byte, 16)
    if _, err := rand.Read(b); err != nil {
        return "", fmt.Errorf("rand.Read: %w", err)
    }
    return hex.EncodeToString(b), nil
}
```

- Pure function: no dependencies on any internal packages
- Imports needed: `crypto/rand`, `encoding/hex`, `fmt`
- Could be exported as `config.GenerateRandomToken()`

#### `loadConfig` (lines 152–166)

```go
func loadConfig() *config.Config {
    path := os.Getenv("UNRAID_MCP_CONFIG_PATH")
    if path == "" {
        path = defaultConfigPath
    }
    cfg, err := config.LoadConfig(path)
    if err != nil {
        log.Printf("could not load config from %q (%v), using defaults", path, err)
        return config.DefaultConfig()
    }
    log.Printf("loaded config from %q", path)
    return cfg
}
```

NOTE: `loadConfig` is a wrapper that could also move to `config.go`, but it uses `log`
and the sentinel `defaultConfigPath = "/config/config.yaml"`. Its logic can be merged into
an exported `config.LoadConfigWithDefaults(path string) *Config` function that does NOT log.
The logging would remain in `main.go`.

### Current Imports in `/internal/config/config.go`

```go
import (
    "fmt"
    "os"
    "gopkg.in/yaml.v3"
)
```

Adding `generateRandomToken` requires: `crypto/rand`, `encoding/hex`
Adding `applyEnvOverrides` requires: (already has `os`)
Adding `ensureAuthToken` requires: `log` (or remove logging from the function)

---

## Stage 5: DockerManager Interface Split

### Current Interface Definition

File: `/Users/jamesprial/code/unraid-mcp/internal/docker/types.go`, lines 91–108

```go
type DockerManager interface {
    // Container operations (10 methods):
    ListContainers(ctx context.Context, all bool) ([]Container, error)
    InspectContainer(ctx context.Context, id string) (*ContainerDetail, error)
    StartContainer(ctx context.Context, id string) error
    StopContainer(ctx context.Context, id string, timeout int) error
    RestartContainer(ctx context.Context, id string, timeout int) error
    RemoveContainer(ctx context.Context, id string, force bool) error
    CreateContainer(ctx context.Context, config ContainerCreateConfig) (string, error)
    PullImage(ctx context.Context, image string) error
    GetLogs(ctx context.Context, id string, tail int) (string, error)
    GetStats(ctx context.Context, id string) (*ContainerStats, error)
    // Network operations (6 methods):
    ListNetworks(ctx context.Context) ([]Network, error)
    InspectNetwork(ctx context.Context, id string) (*NetworkDetail, error)
    CreateNetwork(ctx context.Context, config NetworkCreateConfig) (string, error)
    RemoveNetwork(ctx context.Context, id string) error
    ConnectNetwork(ctx context.Context, networkID, containerID string) error
    DisconnectNetwork(ctx context.Context, networkID, containerID string) error
}
```

### Proposed Split

```go
// ContainerManager handles container lifecycle and inspection.
type ContainerManager interface {
    ListContainers(ctx context.Context, all bool) ([]Container, error)
    InspectContainer(ctx context.Context, id string) (*ContainerDetail, error)
    StartContainer(ctx context.Context, id string) error
    StopContainer(ctx context.Context, id string, timeout int) error
    RestartContainer(ctx context.Context, id string, timeout int) error
    RemoveContainer(ctx context.Context, id string, force bool) error
    CreateContainer(ctx context.Context, config ContainerCreateConfig) (string, error)
    PullImage(ctx context.Context, image string) error
    GetLogs(ctx context.Context, id string, tail int) (string, error)
    GetStats(ctx context.Context, id string) (*ContainerStats, error)
}

// NetworkManager handles Docker network lifecycle and container-network connections.
type NetworkManager interface {
    ListNetworks(ctx context.Context) ([]Network, error)
    InspectNetwork(ctx context.Context, id string) (*NetworkDetail, error)
    CreateNetwork(ctx context.Context, config NetworkCreateConfig) (string, error)
    RemoveNetwork(ctx context.Context, id string) error
    ConnectNetwork(ctx context.Context, networkID, containerID string) error
    DisconnectNetwork(ctx context.Context, networkID, containerID string) error
}

// DockerManager is the full interface combining ContainerManager and NetworkManager.
// Used by the DockerTools factory and the DockerClientManager implementation.
type DockerManager interface {
    ContainerManager
    NetworkManager
}
```

### All Implementations of DockerManager

#### Real Implementation

- **Type**: `DockerClientManager`
- **File**: `/Users/jamesprial/code/unraid-mcp/internal/docker/manager.go`
- **Compliance check**: Line 882: `var _ DockerManager = (*DockerClientManager)(nil)`
- **All 16 methods implemented**: Yes (verified by reading manager.go)

#### Mock Implementation

- **Type**: `MockDockerManager`
- **File**: `/Users/jamesprial/code/unraid-mcp/internal/docker/manager_test.go`
- **Package**: `package docker` (internal, white-box test)
- **Compliance check**: Line 383: `var _ DockerManager = (*MockDockerManager)(nil)`
- **All 16 methods implemented**: Yes (verified by reading manager_test.go)

### Call Site Analysis for DockerManager

The `DockerManager` interface is used in:

1. **`DockerTools` factory function signature** (`/internal/docker/tools.go`, line 20):
   `mgr DockerManager` — all 16 tool functions receive the full interface

2. **Individual tool functions** receive `mgr DockerManager`:
   - Container tools: `toolDockerList`, `toolDockerInspect`, `toolDockerLogs`, `toolDockerStats`,
     `toolDockerStart`, `toolDockerStop`, `toolDockerRestart`, `toolDockerRemove`,
     `toolDockerCreate`, `toolDockerPull` — use only `ContainerManager` methods
   - Network tools: `toolDockerNetworkList`, `toolDockerNetworkInspect`, `toolDockerNetworkCreate`,
     `toolDockerNetworkRemove`, `toolDockerNetworkConnect`, `toolDockerNetworkDisconnect` —
     use only `NetworkManager` methods

3. **`main.go`** (`/cmd/server/main.go`, line 74): Creates `dockerMgr` as `*DockerClientManager`
   and passes it to `docker.DockerTools(...)` which accepts `DockerManager`.

After the split, individual tool functions could accept `ContainerManager` or `NetworkManager`,
but `DockerTools` would continue to accept the composed `DockerManager` and pass sub-portions
to each tool factory.

---

## Existing Imports Per File (Import Cycle Safety)

### `/internal/tools/registration.go`
```go
import (
    "github.com/mark3labs/mcp-go/mcp"
    "github.com/mark3labs/mcp-go/server"
)
```
Currently imports NO internal packages. Adding `safety` for helpers is safe.

### `/internal/docker/tools.go`
```go
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
```
After Stage 1: removes `encoding/json` (if helpers moved), adds `tools` helper call.

### `/internal/vm/tools.go`
```go
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
```
Same as docker/tools.go.

### `/internal/system/tools.go`
```go
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
```

### `/internal/docker/manager.go`
```go
import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "io"
    "net"
    "net/http"
    "strconv"
    "strings"
    "time"
)
```
No internal imports. Fully self-contained.

### `/internal/vm/manager_stub.go`
```go
import (
    "context"
    "fmt"
)
```
After Stage 3: add `"errors"` for sentinel error var.

### `/internal/vm/manager.go` (build tag: libvirt)
```go
import (
    "context"
    "encoding/xml"
    "fmt"
    "net"
    "strings"
    "time"
    "github.com/digitalocean/go-libvirt"
)
```

### `/internal/config/config.go`
```go
import (
    "fmt"
    "os"
    "gopkg.in/yaml.v3"
)
```
After Stage 4: add `"crypto/rand"`, `"encoding/hex"`.

### `/cmd/server/main.go`
```go
import (
    "context"
    "crypto/rand"
    "encoding/hex"
    "fmt"
    "log"
    "net/http"
    "os"
    "os/signal"
    "syscall"
    "time"
    "github.com/jamesprial/unraid-mcp/internal/auth"
    "github.com/jamesprial/unraid-mcp/internal/config"
    "github.com/jamesprial/unraid-mcp/internal/docker"
    "github.com/jamesprial/unraid-mcp/internal/safety"
    "github.com/jamesprial/unraid-mcp/internal/system"
    "github.com/jamesprial/unraid-mcp/internal/tools"
    "github.com/jamesprial/unraid-mcp/internal/vm"
    "github.com/mark3labs/mcp-go/server"
)
```
After Stage 4: removes `"crypto/rand"`, `"encoding/hex"` (if generateRandomToken moves).

### `/internal/safety/audit.go`
```go
import (
    "encoding/json"
    "errors"
    "io"
    "sync"
    "time"
)
```

### `/internal/safety/confirm.go`
```go
import (
    "crypto/rand"
    "encoding/hex"
    "sync"
    "time"
)
```

### `/internal/tools/registration.go`
```go
import (
    "github.com/mark3labs/mcp-go/mcp"
    "github.com/mark3labs/mcp-go/server"
)
```

---

## Test File Locations and Patterns

### Test Files

| File | Package | Mock Type | Pattern |
|------|---------|-----------|---------|
| `/internal/docker/manager_test.go` | `package docker` | `MockDockerManager` | Table-driven, lifecycle, concurrent, benchmarks |
| `/internal/vm/manager_test.go` | `package vm` | `MockVMManager` | Table-driven, state transitions, concurrent |
| `/internal/system/health_test.go` | `package system` | Uses `FileSystemMonitor` with testdata fixtures | Table-driven |
| `/internal/config/config_test.go` | `package config` | None (real `LoadConfig`) | Table-driven with temp files |
| `/internal/auth/middleware_test.go` | `package auth` | Uses `httptest` | Table-driven |
| `/internal/safety/audit_test.go` | `package safety` | None | Table-driven |
| `/internal/safety/confirm_test.go` | `package safety` | None | Table-driven |
| `/internal/safety/filter_test.go` | `package safety` | None | Table-driven |

### Key Testing Observations

1. **Mocks in `_test.go` files**: `MockDockerManager` lives in `manager_test.go` in
   `package docker` (white-box), not in a separate `_mock` file. Same for `MockVMManager`.

2. **Interface compliance checks**: Both mocks have compile-time assertions:
   - `var _ DockerManager = (*MockDockerManager)(nil)` (line 383, manager_test.go)
   - `var _ VMManager = (*MockVMManager)(nil)` (line 303, manager_test.go)

3. **Testdata fixtures**: System health tests use
   `/testdata/proc/stat`, `/testdata/proc/meminfo`, `/testdata/sys/hwmon/`,
   `/testdata/emhttp/var.ini`, `/testdata/emhttp/disks.ini`

4. **After Stage 5 (interface split)**: `MockDockerManager` implements all 16 methods and
   will automatically satisfy `ContainerManager`, `NetworkManager`, and `DockerManager` via
   the embedded interface approach.

---

## Import Cycle Risk Assessment

The proposed `internal/tools/helpers.go` would create a new dependency:
`tools` → `safety` (for `*safety.AuditLogger` and `safety.AuditEntry`)

Current dependency graph relevant to helpers extraction:
```
main
  → docker → tools (Registration type)
  → docker → safety (Filter, ConfirmationTracker, AuditLogger)
  → vm → tools
  → vm → safety
  → system → tools
  → system → safety
  → tools (RegisterAll)
  → safety (NewFilter, NewConfirmationTracker, NewAuditLogger)

tools → mcp-go/mcp, mcp-go/server (currently)
safety → (stdlib only)
```

After helpers extraction:
```
tools → safety  (NEW — but safety has no upstream internal deps, so NO CYCLE)
```

This is safe. No cycles are introduced by any of the 5 stages.

---

## go.mod Module Information

Module: `github.com/jamesprial/unraid-mcp`
Go version: `1.24.0`

Direct dependencies:
- `github.com/digitalocean/go-libvirt v0.0.0-20260127224054-f7013236e99a`
- `github.com/mark3labs/mcp-go v0.44.0`
- `gopkg.in/yaml.v3 v3.0.1`

---

## Summary of Changes Required Per Stage

### Stage 1: Extract shared tool helpers

**New file**: `/Users/jamesprial/code/unraid-mcp/internal/tools/helpers.go`

Functions to define (canonical names):
- `JSONResult(v any) *mcp.CallToolResult`
- `ErrorResult(msg string) *mcp.CallToolResult`
- `LogAudit(audit *safety.AuditLogger, toolName string, params map[string]any, result string, start time.Time)`
- `ConfirmPrompt(confirm *safety.ConfirmationTracker, toolName, resource, description string) *mcp.CallToolResult`

Files to update (remove local helpers, call `tools.JSONResult` etc.):
- `/internal/docker/tools.go` — remove 4 helper functions (~40 lines), update ~40+ call sites
- `/internal/vm/tools.go` — remove 4 helper functions (~40 lines), update ~30+ call sites
- `/internal/system/tools.go` — remove 2 helper functions + inline error sites (~25 lines), update ~10 call sites

### Stage 2: Move destructive tool lists

**Files to update**:
- `/internal/docker/tools.go` — add exported `var DestructiveDockerTools = []string{...}`
- `/internal/vm/tools.go` — add exported `var DestructiveVMTools = []string{...}`
- `/cmd/server/main.go` — replace local slices with `docker.DestructiveDockerTools`, `vm.DestructiveVMTools`

### Stage 3: File organization

**New files**:
- `/internal/docker/container_tools.go` (split from tools.go, container section)
- `/internal/docker/network_tools.go` (split from tools.go, network section)

**Files to update**:
- `/internal/docker/manager.go` — unify `checkError` to call `checkErrorFromBody`
- `/internal/vm/manager_stub.go` — extract sentinel `errStubNotCompiled`, add `"errors"` import

### Stage 4: Extract main.go helpers

**Files to update**:
- `/internal/config/config.go` — add `GenerateRandomToken`, `ApplyEnvOverrides`, optionally `EnsureAuthToken`
- `/cmd/server/main.go` — call config package functions, remove local function definitions

### Stage 5: Split DockerManager interface

**Files to update**:
- `/internal/docker/types.go` — replace `DockerManager` with `ContainerManager`, `NetworkManager`, embedded `DockerManager`
- `/internal/docker/tools.go` or split files — optionally narrow parameter types in tool functions
- `/internal/docker/manager_test.go` — mock automatically satisfies all three; compliance check updates

