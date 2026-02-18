# Explorer Findings: unraid-mcp Codebase Analysis

Generated: 2026-02-18  
Analyst: Code Explorer Agent  
Repository: `/Users/jamesprial/code/unraid-mcp`

---

## 1. Project Structure

### Module Path

```
github.com/jamesprial/unraid-mcp
```

Source: `/Users/jamesprial/code/unraid-mcp/go.mod`, line 1.

### Top-Level Layout

```
unraid-mcp/
├── cmd/
│   └── server/
│       └── main.go               # Binary entry point
├── internal/
│   ├── auth/                     # HTTP auth middleware
│   ├── config/                   # YAML config loading
│   ├── docker/                   # Docker manager + MCP tools
│   ├── safety/                   # Confirmation, audit, filter
│   ├── system/                   # System health monitor + tools
│   ├── tools/                    # Shared registration helpers
│   └── vm/                       # VM manager (libvirt) + MCP tools
├── testdata/
│   ├── config/                   # valid.yaml, invalid.yaml
│   ├── emhttp/                   # disks.ini, var.ini fixtures
│   ├── proc/                     # meminfo, stat fixtures
│   └── sys/hwmon/                # hwmon sensor fixtures
├── config.example.yaml
├── docker-compose.yml
├── Dockerfile
├── go.mod
└── go.sum
```

### Packages Under `internal/`

| Package | Files | Purpose |
|---|---|---|
| `auth` | `middleware.go`, `middleware_test.go` | Bearer token HTTP middleware |
| `config` | `config.go`, `config_test.go`, `helpers_test.go` | YAML config struct, load/defaults/env overrides |
| `docker` | `types.go`, `manager.go`, `container_tools.go`, `network_tools.go`, `tools.go` | Docker API client + MCP tool registrations |
| `safety` | `audit.go`, `confirm.go`, `filter.go` + tests | AuditLogger, ConfirmationTracker, Filter |
| `system` | `types.go`, `health.go`, `tools.go`, `health_test.go` | System health monitor, proc/sys/emhttp readers |
| `tools` | `registration.go`, `helpers.go`, `helpers_test.go` | Shared MCP registration types and result helpers |
| `vm` | `types.go`, `manager.go`, `manager_stub.go`, `tools.go` + tests | Libvirt VM manager (build-tagged) + MCP tools |

---

## 2. Tool Registration Pattern

### Files

- `/Users/jamesprial/code/unraid-mcp/internal/tools/registration.go`
- `/Users/jamesprial/code/unraid-mcp/internal/tools/helpers.go`

### `Registration` Struct

```go
// internal/tools/registration.go

type Registration struct {
    Tool    mcp.Tool
    Handler server.ToolHandlerFunc
}
```

Pairs a `mcp.Tool` definition (name, description, parameter schema) with its `server.ToolHandlerFunc` callback.

### `RegisterAll()`

```go
func RegisterAll(s *server.MCPServer, registrations []Registration) {
    for _, r := range registrations {
        s.AddTool(r.Tool, r.Handler)
    }
}
```

Iterates over a `[]Registration` slice and calls `s.AddTool()` for each. Called once from `main.go` after accumulating all domain registrations into a single slice.

### `JSONResult()`

```go
func JSONResult(v any) *mcp.CallToolResult
```

Marshals any value to indented JSON (2-space indent) and wraps it in `mcp.NewToolResultText()`. On marshal error, returns an error-text result instead of `nil`. Never panics, never returns `nil`.

### `ErrorResult()`

```go
func ErrorResult(msg string) *mcp.CallToolResult
```

Returns `mcp.NewToolResultText(fmt.Sprintf("error: %s", msg))`. Always prefixes with `"error: "`.

### `LogAudit()`

```go
func LogAudit(
    audit  *safety.AuditLogger,
    toolName string,
    params   map[string]any,
    result   string,
    start    time.Time,
)
```

Writes a JSON audit entry. Silently no-ops when `audit == nil` — callers do not need nil checks before calling.

### `ConfirmPrompt()`

```go
func ConfirmPrompt(
    confirm     *safety.ConfirmationTracker,
    toolName    string,
    resource    string,
    description string,
) *mcp.CallToolResult
```

Calls `confirm.RequestConfirmation()`, returns a text result instructing the caller to re-invoke the tool with the returned `confirmation_token`. Format:

```
Confirmation required for <toolName> on "<resource>".

<description>

To proceed, call <toolName> again with confirmation_token="<token>".
```

---

## 3. Safety Layer

### Files

- `/Users/jamesprial/code/unraid-mcp/internal/safety/confirm.go`
- `/Users/jamesprial/code/unraid-mcp/internal/safety/audit.go`
- `/Users/jamesprial/code/unraid-mcp/internal/safety/filter.go`

### `ConfirmationTracker` API

```go
type ConfirmationTracker struct {
    destructive map[string]struct{}
    mu          sync.Mutex
    tokens      map[string]*pendingConfirmation
}

// Constructor
func NewConfirmationTracker(destructiveTools []string) *ConfirmationTracker

// Query
func (ct *ConfirmationTracker) NeedsConfirmation(tool string) bool

// Two-phase confirmation
func (ct *ConfirmationTracker) RequestConfirmation(tool, resourceName, description string) string // returns token
func (ct *ConfirmationTracker) Confirm(token string) bool  // consumes token; false if empty/expired/unknown
```

Token TTL: **5 minutes**. Tokens are **single-use** — `Confirm()` deletes the token on first successful use. Expired tokens are swept lazily on each `RequestConfirmation()` call. Tokens are 16 random bytes hex-encoded (32 chars). Thread-safe via `sync.Mutex`.

Usage pattern in tool handlers:

```go
if !confirm.Confirm(token) {
    return tools.ConfirmPrompt(confirm, toolName, resource, desc), nil
}
// proceed with destructive action
```

### `AuditLogger` API

```go
type AuditEntry struct {
    Timestamp time.Time      `json:"timestamp"`
    Tool      string         `json:"tool"`
    Params    map[string]any `json:"params"`
    Result    string         `json:"result"`
    Duration  time.Duration  `json:"duration_ns"`
}

type AuditLogger struct {
    mu sync.Mutex
    w  io.Writer
}

func NewAuditLogger(w io.Writer) *AuditLogger  // returns nil if w is nil
func (l *AuditLogger) Log(entry AuditEntry) error
```

Writes newline-delimited JSON. Thread-safe via `sync.Mutex`. `Log()` is nil-receiver safe — returns `ErrNilWriter` rather than panicking when called on a nil `*AuditLogger`. The `tools.LogAudit()` wrapper further protects callers from nil checks.

### `Filter` API

```go
type Filter struct {
    allowlist []string
    denylist  []string
}

func NewFilter(allowlist, denylist []string) *Filter
func (f *Filter) IsAllowed(name string) bool
```

Rules (in priority order):
1. Denylist wins — if name matches any denylist glob, denied.
2. Empty allowlist — everything not denied is allowed.
3. Non-empty allowlist — name must match at least one allowlist glob to be allowed.

Uses `filepath.Match` for glob patterns. Malformed patterns are treated as non-matching.

---

## 4. Domain Package Pattern

All three domain packages (`docker/`, `system/`, `vm/`) follow the same four-file pattern:

### Pattern Overview

```
internal/<domain>/
├── types.go      — data types + interface definition(s)
├── manager.go    — concrete implementation of the interface
├── tools.go      — exported XxxTools() factory + DestructiveTools list
└── [tool_funcs]  — unexported per-tool constructor functions (may be split across files)
```

### `types.go` Pattern

Defines plain Go structs for domain entities, then declares the interface at the bottom:

```go
// docker/types.go example

type Container struct { ... }
type ContainerDetail struct { Container; ... }  // embedding for "detail" variant
type ContainerManager interface {
    ListContainers(ctx context.Context, all bool) ([]Container, error)
    InspectContainer(ctx context.Context, id string) (*ContainerDetail, error)
    // ... other methods
}
```

Interfaces use `context.Context` as first parameter throughout. Error-returning mutating methods return `error` only; queries return `(*Detail, error)`.

### `manager.go` Pattern

Concrete struct with a constructor and method implementations. Includes a compile-time interface check at the bottom:

```go
var _ DockerManager = (*DockerClientManager)(nil)
```

### `tools.go` Pattern

```go
// Exported list of tool names requiring confirmation
var DestructiveTools = []string{"tool_stop", "tool_remove", ...}

// Exported factory that returns all registrations for this domain
func DomainTools(
    mgr    DomainManager,
    filter *safety.Filter,
    confirm *safety.ConfirmationTracker,
    audit  *safety.AuditLogger,
) []tools.Registration {
    return []tools.Registration{
        toolDomainList(mgr, audit),
        toolDomainInspect(mgr, filter, audit),
        toolDomainStop(mgr, filter, confirm, audit),  // destructive
        ...
    }
}
```

Read-only tools receive `(mgr, audit)` or `(mgr, filter, audit)`.  
Destructive tools receive `(mgr, filter, confirm, audit)`.  
`system.SystemTools` skips `filter` and `confirm` entirely — it is read-only.

### Per-Tool Constructor Pattern

Each tool is a package-level unexported function returning `tools.Registration`:

```go
func toolDomainAction(
    mgr    DomainManager,
    filter *safety.Filter,
    confirm *safety.ConfirmationTracker,
    audit  *safety.AuditLogger,
) tools.Registration {
    const toolName = "domain_action"

    tool := mcp.NewTool(toolName,
        mcp.WithDescription("..."),
        mcp.WithString("param", mcp.Required(), mcp.Description("...")),
        mcp.WithString("confirmation_token",
            mcp.Description("Confirmation token returned by a prior call to this tool"),
        ),
    )

    handler := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
        start := time.Now()
        param := req.GetString("param", "")
        token := req.GetString("confirmation_token", "")
        params := map[string]any{"param": param}

        // 1. Filter check (access control)
        if !filter.IsAllowed(param) {
            tools.LogAudit(audit, toolName, params, "denied", start)
            return tools.ErrorResult(fmt.Sprintf("access to %q is not allowed", param)), nil
        }

        // 2. Confirmation check (destructive only)
        if !confirm.Confirm(token) {
            desc := fmt.Sprintf("This will <action> %q.", param)
            return tools.ConfirmPrompt(confirm, toolName, param, desc), nil
        }

        // 3. Perform action
        if err := mgr.DoAction(ctx, param); err != nil {
            tools.LogAudit(audit, toolName, params, "error: "+err.Error(), start)
            return tools.ErrorResult(err.Error()), nil
        }

        // 4. Audit and return
        tools.LogAudit(audit, toolName, params, "ok", start)
        return mcp.NewToolResultText(fmt.Sprintf("%q action succeeded", param)), nil
    }

    return tools.Registration{Tool: tool, Handler: server.ToolHandlerFunc(handler)}
}
```

Note: Tool handlers always return `(result, nil)` — errors are encoded into the result text, not returned as Go errors. The second return value of `ToolHandlerFunc` is reserved for transport-level errors only.

---

## 5. `main.go` Wiring

File: `/Users/jamesprial/code/unraid-mcp/cmd/server/main.go`

### Initialization Sequence

```go
func main() {
    // 1. Load config (YAML file → env overrides → defaults fallback)
    cfg := loadConfig()
    config.ApplyEnvOverrides(cfg)

    // 2. Auth token (generate if empty)
    config.EnsureAuthToken(cfg)

    // 3. Audit logger (open file if enabled; nil if disabled/error)
    var auditLogger *safety.AuditLogger
    if cfg.Audit.Enabled {
        f, err := os.OpenFile(cfg.Audit.LogPath, ...)
        auditLogger = safety.NewAuditLogger(f)
    }

    // 4. Safety components (one Filter + one ConfirmationTracker per domain)
    dockerFilter := safety.NewFilter(cfg.Safety.Docker.Allowlist, cfg.Safety.Docker.Denylist)
    vmFilter     := safety.NewFilter(cfg.Safety.VMs.Allowlist,    cfg.Safety.VMs.Denylist)
    dockerConfirm := safety.NewConfirmationTracker(docker.DestructiveTools)
    vmConfirm     := safety.NewConfirmationTracker(vm.DestructiveTools)

    // 5. Resource managers
    dockerMgr, _ := docker.NewDockerClientManager(cfg.Paths.DockerSocket)

    // 6. VM manager: conditional registration (graceful degradation)
    var vmMgr vm.VMManager
    if rawVMMgr, vmErr := vm.NewLibvirtVMManager(cfg.Paths.LibvirtSocket); vmErr != nil {
        log.Printf("warning: VM manager unavailable — VM tools will not be registered")
    } else {
        vmMgr = rawVMMgr
    }

    systemMon := system.NewFileSystemMonitor(cfg.Paths.Proc, cfg.Paths.Sys, cfg.Paths.Emhttp)

    // 7. MCP server
    mcpServer := server.NewMCPServer("unraid-mcp", "1.0.0", server.WithToolCapabilities(false))

    // 8. Tool accumulation + conditional VM registration
    var registrations []tools.Registration
    registrations = append(registrations, docker.DockerTools(dockerMgr, dockerFilter, dockerConfirm, auditLogger)...)
    if vmMgr != nil {
        registrations = append(registrations, vm.VMTools(vmMgr, vmFilter, vmConfirm, auditLogger)...)
    }
    registrations = append(registrations, system.SystemTools(systemMon, auditLogger)...)

    // 9. Bulk registration
    tools.RegisterAll(mcpServer, registrations)

    // 10. HTTP server + auth middleware + graceful shutdown
    httpHandler    := server.NewStreamableHTTPServer(mcpServer)
    authMiddleware := auth.NewAuthMiddleware(cfg.Server.AuthToken)
    wrappedHandler := authMiddleware(httpHandler)
    // ... ListenAndServe + SIGTERM handler
}
```

### Conditional VM Registration Pattern

```go
var vmMgr vm.VMManager   // nil by default (interface)
if rawVMMgr, vmErr := vm.NewLibvirtVMManager(...); vmErr != nil {
    log.Printf("warning: ...")
} else {
    vmMgr = rawVMMgr
}

// Later:
if vmMgr != nil {
    registrations = append(registrations, vm.VMTools(vmMgr, ...)...)
}
```

VM tools are simply omitted from the server when libvirt is unavailable. No stub tools or error responses are registered in its place.

---

## 6. Config — `GraphQLConfig`

File: `/Users/jamesprial/code/unraid-mcp/internal/config/config.go`

```go
type GraphQLConfig struct {
    URL     string `yaml:"url"`
    APIKey  string `yaml:"api_key"`
    Timeout int    `yaml:"timeout"`  // seconds
}

type Config struct {
    Server  ServerConfig  `yaml:"server"`
    Safety  SafetyConfig  `yaml:"safety"`
    Paths   PathsConfig   `yaml:"paths"`
    Audit   AuditConfig   `yaml:"audit"`
    GraphQL GraphQLConfig `yaml:"graphql"`  // already present in the struct
}
```

Default values (from `DefaultConfig()`):

```go
GraphQL: GraphQLConfig{
    URL:     "http://localhost/graphql",
    Timeout: 30,
    // APIKey: "" (empty by default)
},
```

Environment variable overrides (from `ApplyEnvOverrides()`):

| Env Var | Config Field |
|---|---|
| `UNRAID_GRAPHQL_URL` | `cfg.GraphQL.URL` |
| `UNRAID_GRAPHQL_API_KEY` | `cfg.GraphQL.APIKey` |

The `GraphQL` field is already wired in the `Config` struct. No additional struct changes are needed to use it — only a client implementation that reads `cfg.GraphQL`.

---

## 7. mcp-go Library Usage

### Import Paths

```go
import (
    "github.com/mark3labs/mcp-go/mcp"
    "github.com/mark3labs/mcp-go/server"
)
```

### `mcp.NewTool()` — Tool Definition

```go
tool := mcp.NewTool("tool_name",
    mcp.WithDescription("Human-readable description."),
)
```

### `mcp.WithString()` — String Parameter

```go
mcp.WithString("param_name",
    mcp.Required(),                   // marks parameter as required
    mcp.Description("What it does"),  // inline description
)
```

### `mcp.WithNumber()` — Numeric Parameter

```go
mcp.WithNumber("timeout",
    mcp.Description("Seconds to wait (default: 10)"),
)
```

Note: The handler reads numeric params via `req.GetInt("timeout", 10)` — not `GetFloat`. There is no `mcp.WithInt()`.

### `mcp.WithBoolean()` — Boolean Parameter

```go
mcp.WithBoolean("all",
    mcp.Description("Include stopped containers (default: false)"),
)
```

Read in handler via `req.GetBool("all", false)`.

### Reading Parameters in Handlers

```go
handler := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
    name   := req.GetString("name", "")       // string with default
    timeout := req.GetInt("timeout", 10)       // int with default
    all    := req.GetBool("all", false)        // bool with default
    ...
}
```

### Returning Results

```go
// Success with JSON
return tools.JSONResult(someStruct), nil

// Success with plain text
return mcp.NewToolResultText("operation succeeded"), nil

// Error (tool-level, not transport)
return tools.ErrorResult(err.Error()), nil

// Confirmation prompt
return tools.ConfirmPrompt(confirm, toolName, resource, desc), nil
```

### Handler Registration

```go
return tools.Registration{
    Tool:    tool,
    Handler: server.ToolHandlerFunc(handler),
}
```

---

## 8. Test Patterns

### File Locations

| Package | Test Files |
|---|---|
| `internal/docker` | `manager_test.go` (mock + table tests), `interface_test.go` (compile-time + reflection), `destructive_test.go` (DestructiveTools), `network_tools.go` (implicit) |
| `internal/vm` | `manager_test.go`, `manager_stub.go` (build-tagged stub), `destructive_test.go`, `stub_error_test.go` |
| `internal/tools` | `helpers_test.go` (external package `tools_test`) |
| `internal/system` | `health_test.go` |
| `internal/config` | `config_test.go`, `helpers_test.go` |
| `internal/safety` | `audit_test.go`, `confirm_test.go`, `filter_test.go` |

### Pattern 1 — Table-Driven Tests with `validate` Functions

The dominant pattern. Each test case carries an optional `validate func(t *testing.T, ...)` for assertions beyond simple error checking:

```go
tests := []struct {
    name        string
    id          string
    wantErr     bool
    errContains string
    validate    func(t *testing.T, detail *ContainerDetail)
}{
    {
        name:    "inspect existing container returns detail",
        id:      "abc123",
        wantErr: false,
        validate: func(t *testing.T, detail *ContainerDetail) {
            t.Helper()
            if detail.ID != "abc123" { t.Errorf(...) }
        },
    },
    {
        name:        "inspect nonexistent returns error",
        id:          "nonexistent",
        wantErr:     true,
        errContains: "not found",
    },
}

for _, tt := range tests {
    t.Run(tt.name, func(t *testing.T) {
        detail, err := mgr.InspectContainer(ctx, tt.id)
        if tt.wantErr {
            if err == nil { t.Fatal("expected error, got nil") }
            if tt.errContains != "" && !strings.Contains(...) { t.Errorf(...) }
            return
        }
        if err != nil { t.Fatalf("unexpected error: %v", err) }
        if tt.validate != nil { tt.validate(t, detail) }
    })
}
```

Error strings are tested with `strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tt.errContains))` — case-insensitive substring match.

### Pattern 2 — In-Package Mock Managers

Mocks live in the same package as the interface (not `_test` suffix), allowing access to unexported types. Example: `docker/manager_test.go` defines `MockDockerManager` in `package docker`.

Mock structure:
- In-memory maps keyed by ID
- `sync.RWMutex` for concurrent safety
- `t.Helper()` factories like `newPopulatedMock(t *testing.T)` to seed state
- `SetXxx()` and `AddXxx()` helper methods for test setup
- Context cancellation checked at entry of every method via `checkCtx(ctx)`

```go
type MockDockerManager struct {
    mu         sync.RWMutex
    containers map[string]*ContainerDetail
    networks   map[string]*NetworkDetail
    logs       map[string]string
    stats      map[string]*ContainerStats
    idCounter  int
    networkLinks map[string]map[string]struct{}
}

func NewMockDockerManager() *MockDockerManager { ... }
func (m *MockDockerManager) AddContainer(detail *ContainerDetail) { ... }
func (m *MockDockerManager) SetLogs(containerID, logs string) { ... }
```

### Pattern 3 — Compile-Time Interface Checks

Two locations:

**In production code** (`manager.go`):
```go
var _ DockerManager = (*DockerClientManager)(nil)
```

**In test code** (`interface_test.go`):
```go
var _ ContainerManager = (*DockerClientManager)(nil)
var _ NetworkManager   = (*DockerClientManager)(nil)
var _ DockerManager    = (*DockerClientManager)(nil)
var _ ContainerManager = (*MockDockerManager)(nil)
var _ NetworkManager   = (*MockDockerManager)(nil)
var _ DockerManager    = (*MockDockerManager)(nil)
```

Also uses `reflect.TypeOf((*Interface)(nil)).Elem()` to verify method counts and names at runtime in tests, catching interface drift.

### Pattern 4 — External Package Tests for Exported APIs

`internal/tools/helpers_test.go` uses `package tools_test` (external) to test only the exported surface. It defines a `resultText(t, result)` helper to extract `mcp.TextContent` from a `*mcp.CallToolResult`:

```go
func resultText(t *testing.T, result *mcp.CallToolResult) string {
    t.Helper()
    tc, ok := result.Content[0].(mcp.TextContent)
    if !ok { t.Fatalf(...) }
    return tc.Text
}
```

### Pattern 5 — Context Cancellation Tests

All mock methods check context at entry. Tests use an already-cancelled context to verify propagation:

```go
ctx, cancel := context.WithCancel(context.Background())
cancel()  // cancel immediately

err := m.SomeMethod(ctx, ...)
if err != context.Canceled { t.Errorf(...) }
```

### Pattern 6 — Build Tag Stubs

The `vm/manager_stub.go` uses `//go:build !libvirt` to provide a stub `LibvirtVMManager` when the `libvirt` build tag is absent (default for tests). The real implementation in `manager.go` uses `//go:build libvirt`. This allows `go test ./...` to run without libvirt installed.

### Pattern 7 — testdata Fixtures

Config tests use `testdata/config/valid.yaml` and `testdata/config/invalid.yaml`. System health tests use `testdata/proc/`, `testdata/sys/`, and `testdata/emhttp/` fixtures. Paths are resolved with:

```go
filepath.Abs(filepath.Join("..", "..", "testdata", "config"))
```

### Pattern 8 — Benchmarks

Every package with significant hot paths includes `Benchmark_` functions alongside tests in the same `_test.go` files. Pattern:

```go
func Benchmark_ListContainers_All(b *testing.B) {
    m := NewMockDockerManager()
    // seed data
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        _, _ = m.ListContainers(ctx, true)
    }
}
```

---

## 9. `go.mod` — All Dependencies

File: `/Users/jamesprial/code/unraid-mcp/go.mod`

```
module github.com/jamesprial/unraid-mcp

go 1.24.0
```

### Direct Dependencies

| Module | Version | Purpose |
|---|---|---|
| `github.com/digitalocean/go-libvirt` | `v0.0.0-20260127224054-f7013236e99a` | Libvirt VM management (build-tagged) |
| `github.com/mark3labs/mcp-go` | `v0.44.0` | MCP server framework |
| `gopkg.in/yaml.v3` | `v3.0.1` | YAML config parsing |

### Indirect Dependencies

| Module | Version | Role |
|---|---|---|
| `github.com/bahlo/generic-list-go` | `v0.2.0` | mcp-go internal |
| `github.com/buger/jsonparser` | `v1.1.1` | mcp-go internal |
| `github.com/google/uuid` | `v1.6.0` | mcp-go (tool IDs) |
| `github.com/invopop/jsonschema` | `v0.13.0` | mcp-go (schema generation) |
| `github.com/mailru/easyjson` | `v0.7.7` | mcp-go internal |
| `github.com/spf13/cast` | `v1.7.1` | mcp-go (type casting in GetString/GetInt) |
| `github.com/wk8/go-ordered-map/v2` | `v2.1.8` | mcp-go internal |
| `github.com/yosida95/uritemplate/v3` | `v3.0.2` | mcp-go internal |
| `golang.org/x/crypto` | `v0.47.0` | go-libvirt (SSH transport) |
| `golang.org/x/sys` | `v0.40.0` | go-libvirt / system calls |

---

## 10. Key Integration Points for New GraphQL Tools

Based on the above findings, here is the exact integration contract for adding a new `internal/graphql/` package with GraphQL-backed MCP tools:

### 10.1 Accessing Config

```go
// cfg.GraphQL is already populated after loadConfig() + ApplyEnvOverrides()
cfg.GraphQL.URL      // string — e.g. "http://192.168.1.1/graphql"
cfg.GraphQL.APIKey   // string — x-api-key header value
cfg.GraphQL.Timeout  // int    — seconds; convert with time.Duration(cfg.GraphQL.Timeout) * time.Second
```

No struct changes needed. The `GraphQLConfig` struct and its YAML/env binding are complete.

### 10.2 Domain Package Template

New package `internal/graphql/` should follow:

```
internal/graphql/
├── types.go      — GraphQL response structs + manager interface
├── client.go     — HTTP client implementation (httpx equivalent in Go = net/http + httpx pattern)
├── tools.go      — GraphQLTools() factory + DestructiveTools list (if any)
└── [query_tools].go — per-tool constructor functions
```

### 10.3 Tool Constructor Signature

Read-only GraphQL tool (no filter, no confirm needed unless scoping by resource name):

```go
func graphqlQueryTool(mgr GraphQLManager, audit *safety.AuditLogger) tools.Registration
```

Mutating GraphQL tool:

```go
func graphqlMutationTool(
    mgr     GraphQLManager,
    filter  *safety.Filter,
    confirm *safety.ConfirmationTracker,
    audit   *safety.AuditLogger,
) tools.Registration
```

### 10.4 Wiring in main.go

```go
// After building graphqlMgr from cfg.GraphQL:
registrations = append(registrations,
    graphql.GraphQLTools(graphqlMgr, graphqlFilter, graphqlConfirm, auditLogger)...,
)
```

### 10.5 Handler Boilerplate

```go
handler := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
    start  := time.Now()
    param  := req.GetString("param", "")
    params := map[string]any{"param": param}

    result, err := mgr.Query(ctx, param)
    if err != nil {
        tools.LogAudit(audit, toolName, params, "error: "+err.Error(), start)
        return tools.ErrorResult(err.Error()), nil
    }

    tools.LogAudit(audit, toolName, params, "ok", start)
    return tools.JSONResult(result), nil
}
```

### 10.6 mcp-go Parameter Option Functions

Confirmed available option functions (all in `github.com/mark3labs/mcp-go/mcp`):

- `mcp.WithDescription(string)` — tool description
- `mcp.WithString(name string, opts ...ParameterOption)` — string parameter
- `mcp.WithNumber(name string, opts ...ParameterOption)` — numeric parameter (read with `GetInt`)
- `mcp.WithBoolean(name string, opts ...ParameterOption)` — boolean parameter (read with `GetBool`)
- `mcp.Required()` — ParameterOption marking field as required
- `mcp.Description(string)` — ParameterOption for parameter description
- `mcp.NewToolResultText(string)` — construct a text result

---

## Architectural Diagram

```
cmd/server/main.go
    │
    ├── config.LoadConfig() + ApplyEnvOverrides()
    │       └── config.GraphQLConfig{URL, APIKey, Timeout}
    │
    ├── safety.NewAuditLogger(file) → *AuditLogger (nil if disabled)
    ├── safety.NewFilter(allow, deny) → *Filter  [per domain]
    ├── safety.NewConfirmationTracker(tools) → *ConfirmationTracker  [per domain]
    │
    ├── docker.NewDockerClientManager(socket) → DockerManager
    ├── vm.NewLibvirtVMManager(socket) → VMManager  (conditional, nil on error)
    ├── system.NewFileSystemMonitor(proc, sys, emhttp) → SystemMonitor
    │
    ├── server.NewMCPServer(...)
    │
    ├── []tools.Registration accumulation:
    │       ├── docker.DockerTools(mgr, filter, confirm, audit)
    │       ├── vm.VMTools(mgr, filter, confirm, audit)    [if vmMgr != nil]
    │       └── system.SystemTools(mon, audit)
    │
    ├── tools.RegisterAll(mcpServer, registrations)
    │
    └── server.NewStreamableHTTPServer(mcpServer)
            └── auth.NewAuthMiddleware(token)(httpHandler)
```

```
tools.Registration
    ├── Tool:    mcp.NewTool(name, WithDescription, WithString, WithNumber, WithBoolean)
    └── Handler: server.ToolHandlerFunc(func(ctx, req) (*mcp.CallToolResult, error))
                    ├── req.GetString / GetInt / GetBool
                    ├── filter.IsAllowed(resource)
                    ├── confirm.Confirm(token) || ConfirmPrompt(...)
                    ├── mgr.Method(ctx, ...) → result, err
                    ├── tools.LogAudit(audit, name, params, result, start)
                    └── tools.JSONResult / ErrorResult / mcp.NewToolResultText
```
