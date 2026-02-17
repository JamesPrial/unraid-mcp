# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Test Commands

```bash
go build ./...                          # Build all packages (default: libvirt stub)
go build -tags libvirt ./...            # Build with real libvirt support
go test ./...                           # Run all tests
go test -race ./...                     # Run with race detector
go test -v ./internal/safety/           # Run one package verbose
go test -v -run Test_Filter ./internal/safety/  # Run single test
go vet ./...                            # Static analysis
CGO_ENABLED=0 go build -o unraid-mcp ./cmd/server  # Production binary
docker compose up --build               # Build and run in Docker
```

## Architecture

MCP server exposing 31 tools (16 Docker, 12 VM, 3 system health) over Streamable HTTP transport with Bearer token auth.

### Request Flow

```
Client → HTTP (Bearer auth middleware) → MCP Server → Tool Handler
  → Safety Layer (filter → confirm → audit) → Resource Manager → External System
```

### Key Patterns

**Manager interfaces** (`internal/{docker,vm,system}/types.go`): Each subsystem defines an interface (`DockerManager`, `VMManager`, `SystemMonitor`) with a real implementation and a mock in tests. Tools depend on the interface, not the concrete type.

**Tool registration** (`internal/tools/registration.go`): Each package exports a factory function (`DockerTools()`, `VMTools()`, `SystemTools()`) that returns `[]tools.Registration` pairs. `main.go` collects all registrations and calls `tools.RegisterAll()` to wire them into the MCP server.

**Safety wrapping**: Every tool handler follows the same pattern — filter check → confirmation check (destructive ops) → manager call → audit log. Destructive tools use single-use, 5-minute-TTL confirmation tokens.

**Build tags for libvirt**: `internal/vm/manager.go` uses `//go:build libvirt`, `manager_stub.go` uses `//go:build !libvirt`. When libvirt is unavailable, `main.go` gracefully skips VM tool registration. Tests always work without the tag since they use `MockVMManager`.

### Docker Manager Implementation

`DockerClientManager` in `internal/docker/manager.go` communicates with the Docker daemon via raw HTTP over a Unix socket (`/var/run/docker.sock`). It does **not** use the Docker SDK — it uses `net/http` with a custom `DialContext` targeting API v1.41.

### System Health

`FileSystemMonitor` reads directly from host filesystem paths (`/proc/stat`, `/proc/meminfo`, `/sys/hwmon/*/temp*_input`, `/var/local/emhttp/*.ini`). Constructor takes configurable paths so tests use `testdata/` fixtures.

## Testing Conventions

- Table-driven tests with `t.Run()` subtests
- Mocks defined in `_test.go` files (e.g., `MockDockerManager`, `MockVMManager`)
- System health tests use fixture files in `testdata/` (emhttp ini files, proc, sys)
- Auth tests use `httptest.NewRequest/NewRecorder`
- Docker/VM mock tests validate interface contracts (state transitions, not-found errors, context cancellation)

## Configuration

Config loaded from `/config/config.yaml` (override with `UNRAID_MCP_CONFIG_PATH`). Auth token overridden by `UNRAID_MCP_AUTH_TOKEN` env var. Auto-generated if empty. See `config.example.yaml` for structure.

## Docker Deployment

Container requires these host mounts:
- `/var/run/docker.sock` — Docker API
- `/var/run/libvirt/libvirt-sock` — libvirt API
- `/var/local/emhttp` (ro) — Unraid array/disk state
- `/proc` (ro), `/sys` (ro) — system health
- `./config` — persistent config and audit log
