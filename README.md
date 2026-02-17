# unraid-mcp

An MCP (Model Context Protocol) server that gives AI assistants controlled access to manage Docker containers, virtual machines, and system health on an Unraid server.

Runs as a Docker container on Unraid. Connects to Claude Code (or any MCP client) over Streamable HTTP with Bearer token authentication.

## Features

**31 MCP tools across three domains:**

- **Docker (16 tools)** -- list, inspect, start, stop, restart, remove, create containers; pull images; view logs and stats; list, inspect, create, remove, connect, disconnect networks
- **Virtual Machines (12 tools)** -- list, inspect, start, stop, force stop, pause, resume, restart, create, delete VMs; list and create snapshots (via libvirt)
- **System Health (3 tools)** -- CPU/memory/temperature overview, Unraid array status, per-disk info

**Safety guardrails:**

- **Authentication** -- Bearer token required on all requests
- **Confirmation prompts** -- destructive operations (stop, remove, delete) require a two-step confirmation flow with single-use, time-limited tokens
- **Allowlist/denylist** -- glob-pattern filtering on container and VM names; the server's own container is always protected
- **Audit logging** -- every tool invocation logged as newline-delimited JSON with timestamp, parameters, result, and duration

## Quick Start

### 1. Deploy on Unraid

```bash
git clone https://github.com/jamesprial/unraid-mcp.git
cd unraid-mcp

# Set your auth token (or leave empty to auto-generate one)
export UNRAID_MCP_AUTH_TOKEN="your-secret-token"

# Build and run
docker compose up -d
```

The server starts on port 8080. If no auth token is set, one is generated and printed to the container logs:

```bash
docker logs unraid-mcp
```

### 2. Configure Claude Code

Add to your `.mcp.json`:

```json
{
  "mcpServers": {
    "unraid": {
      "type": "streamable-http",
      "url": "http://<unraid-ip>:8080/mcp",
      "headers": {
        "Authorization": "Bearer your-secret-token"
      }
    }
  }
}
```

### 3. Use it

Ask Claude to manage your Unraid server:

- "List all running Docker containers"
- "Stop the plex container"
- "What's the array status?"
- "Show me disk temperatures"
- "Start the Windows 10 VM"

## Configuration

Copy and edit the example config:

```bash
cp config.example.yaml config/config.yaml
```

```yaml
server:
  port: 8080
  auth_token: ""          # set here or via UNRAID_MCP_AUTH_TOKEN env var

safety:
  docker:
    allowlist: []          # empty = all containers allowed
    denylist:
      - "unraid-mcp"      # prevent self-management
      - "*backup*"         # glob patterns supported
  vms:
    allowlist: []
    denylist: []

paths:
  emhttp: "/host/emhttp"
  proc: "/host/proc"
  sys: "/host/sys"
  docker_socket: "/var/run/docker.sock"
  libvirt_socket: "/var/run/libvirt/libvirt-sock"

audit:
  enabled: true
  log_path: "/config/audit.log"
  max_size_mb: 50
```

### Environment Variables

| Variable | Description |
|----------|-------------|
| `UNRAID_MCP_AUTH_TOKEN` | Bearer token (overrides config file) |
| `UNRAID_MCP_CONFIG_PATH` | Config file path (default: `/config/config.yaml`) |

## Container Mounts

| Host Path | Container Path | Mode | Purpose |
|-----------|---------------|------|---------|
| `/var/run/docker.sock` | `/var/run/docker.sock` | rw | Docker API |
| `/var/run/libvirt/libvirt-sock` | `/var/run/libvirt/libvirt-sock` | rw | VM management via libvirt |
| `/var/local/emhttp` | `/host/emhttp` | ro | Unraid array and disk state |
| `/proc` | `/host/proc` | ro | CPU and memory stats |
| `/sys` | `/host/sys` | ro | Hardware temperatures |
| `./config` | `/config` | rw | Config file and audit log |

## Safety Model

### Confirmation Flow

Destructive tools (`docker_stop`, `docker_remove`, `vm_delete`, etc.) use a two-step confirmation:

1. First call returns a description of the action and a `confirmation_token`
2. Second call with the token executes the operation
3. Tokens are single-use and expire after 5 minutes

### Allowlist/Denylist

Filter containers and VMs by name using glob patterns. Denylist always takes priority. The MCP server's own container (`unraid-mcp`) is always implicitly denied.

### Audit Log

All operations are recorded to `/config/audit.log` as newline-delimited JSON:

```json
{"timestamp":"2025-01-15T10:30:00Z","tool":"docker_stop","params":{"id":"plex"},"result":"ok","duration_ns":150000000}
```

## Development

```bash
# Run tests
go test ./...

# Run with race detector
go test -race ./...

# Build without libvirt (VM tools return stub errors)
go build ./...

# Build with libvirt support
go build -tags libvirt ./...
```

## License

MIT
