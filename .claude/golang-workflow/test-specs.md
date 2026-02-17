# Test Specifications

## Stage 1: `internal/tools/helpers.go`

### `tools.JSONResult`

| Scenario | Input | Expected Output | Error |
|---|---|---|---|
| Simple struct | A struct with string and int fields | CallToolResult text containing valid indented JSON | None |
| Nil input | `nil` | CallToolResult text containing `"null"` | None |
| Empty map | Empty `map[string]any{}` | CallToolResult text containing `"{}"` | None |
| Channel (unmarshalable) | A channel value | CallToolResult text containing `"error marshaling result:"` | None |

### `tools.ErrorResult`

| Scenario | Input | Expected Output | Error |
|---|---|---|---|
| Simple message | `"container not found"` | CallToolResult text equal to `"error: container not found"` | None |
| Empty message | `""` | CallToolResult text equal to `"error: "` | None |

### `tools.LogAudit`

| Scenario | Input | Expected Output | Error |
|---|---|---|---|
| Nil audit logger | `nil` logger | No panic, returns immediately | None |
| Valid logger with buffer | Non-nil logger backed by bytes.Buffer | Buffer contains JSON with "tool", "params", "result", "timestamp", "duration_ns" | None |
| Duration positive | Start time in past | "duration_ns" > 0 | None |
| Params preserved | `{"id": "abc", "force": true}` | Written JSON params contains both k/v pairs | None |

### `tools.ConfirmPrompt`

| Scenario | Input | Expected Output | Error |
|---|---|---|---|
| Standard prompt | tool="docker_stop", resource="my-container", desc="stop it" | Text contains "Confirmation required for docker_stop", "my-container", "stop it", and confirmation_token= | None |
| Token unique per call | Two calls same args | Different confirmation_token values | None |
| Token is consumable | Extract token, call confirm.Confirm() | Returns true exactly once | None |

## Stage 2: Destructive Tool Lists

### `docker.DestructiveTools`

| Scenario | Expected |
|---|---|
| Contains expected names | "docker_stop", "docker_restart", "docker_remove", "docker_create", "docker_network_remove" |
| Length | 5 |

### `vm.DestructiveTools`

| Scenario | Expected |
|---|---|
| Contains expected names | "vm_stop", "vm_force_stop", "vm_restart", "vm_create", "vm_delete" |
| Length | 5 |

## Stage 3c: `vm.ErrLibvirtNotCompiled`

| Scenario | Expected |
|---|---|
| Is an error | Satisfies error interface |
| Message content | Contains "libvirt support not compiled" |
| Constructor wraps sentinel | `errors.Is(err, ErrLibvirtNotCompiled)` is true |
| All stub methods return sentinel | Each returned error satisfies `errors.Is(err, ErrLibvirtNotCompiled)` |

## Stage 4: `internal/config` Helpers

### `config.ApplyEnvOverrides`

| Scenario | Input | Expected Output | Error |
|---|---|---|---|
| Token env var set | Empty AuthToken, env UNRAID_MCP_AUTH_TOKEN=my-token | cfg.Server.AuthToken == "my-token" | None |
| Token env overrides existing | AuthToken="old", env=new | cfg.Server.AuthToken == "new" | None |
| Token env not set | AuthToken="existing", env unset | cfg.Server.AuthToken == "existing" | None |
| Empty env does not override | AuthToken="existing", env="" | cfg.Server.AuthToken == "existing" | None |

### `config.EnsureAuthToken`

| Scenario | Input | Expected Output | Error |
|---|---|---|---|
| Token already set | AuthToken="pre-set" | Returns ("pre-set", nil), config unchanged | None |
| Token empty, gen succeeds | AuthToken="" | Returns (32-char hex, nil), cfg updated | None |

### `config.GenerateRandomToken`

| Scenario | Expected |
|---|---|
| Returns 32-char hex | len==32, all chars in [0-9a-f] |
| Unique per call | Two calls produce different values |
| Concurrent safety | 100 goroutines all succeed, all unique |

## Stage 5: Interface Segregation

### Compile-time checks

| Assertion | Compiles? |
|---|---|
| `var _ ContainerManager = (*DockerClientManager)(nil)` | Yes |
| `var _ NetworkManager = (*DockerClientManager)(nil)` | Yes |
| `var _ DockerManager = (*DockerClientManager)(nil)` | Yes |
| `var _ ContainerManager = (*MockDockerManager)(nil)` | Yes |
| `var _ NetworkManager = (*MockDockerManager)(nil)` | Yes |

### Structural

| Scenario | Expected |
|---|---|
| DockerManager method count == ContainerManager + NetworkManager | True |
| No method name collisions between ContainerManager and NetworkManager | True |
