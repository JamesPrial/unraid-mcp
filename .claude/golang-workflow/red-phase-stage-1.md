# TDD Red Phase Report - Stage 1 (Foundation)

**Date:** 2026-02-17
**Verdict:** RED_VERIFIED

---

## Summary

All three packages (`internal/config`, `internal/auth`, `internal/safety`) have test files written but zero implementation files. The test suite fails to compile, confirming the tests are meaningful and exercise real, not-yet-implemented behavior.

- **Tests that exist:** 30 (across 5 test files)
- **Tests that fail:** All 30 - packages cannot compile (build failed)
- **Root cause:** No implementation files exist; all types and functions referenced by tests are undefined

---

## Command 1: `go build ./...`

```
EXIT CODE: 0
```

`go build` succeeds trivially because there are no non-test `.go` source files to compile. The build tool has nothing to process in the production code tree.

---

## Command 2: `go test -v ./internal/config/ ./internal/auth/ ./internal/safety/`

```
# github.com/jamesprial/unraid-mcp/internal/config [github.com/jamesprial/unraid-mcp/internal/config.test]
internal/config/config_test.go:38:39: undefined: Config
internal/config/config_test.go:47:38: undefined: Config
internal/config/config_test.go:117:38: undefined: Config
internal/config/config_test.go:132:38: undefined: Config
internal/config/config_test.go:146:38: undefined: Config
internal/config/config_test.go:170:16: undefined: LoadConfig
internal/config/config_test.go:195:36: undefined: Config
internal/config/config_test.go:199:38: undefined: Config
internal/config/config_test.go:208:38: undefined: Config
internal/config/config_test.go:217:38: undefined: Config
internal/config/config_test.go:217:38: too many errors
# github.com/jamesprial/unraid-mcp/internal/safety [github.com/jamesprial/unraid-mcp/internal/safety.test]
internal/safety/audit_test.go:14:12: undefined: AuditEntry
internal/safety/audit_test.go:20:11: undefined: AuditEntry
internal/safety/audit_test.go:37:11: undefined: AuditEntry
internal/safety/audit_test.go:54:11: undefined: AuditEntry
internal/safety/audit_test.go:74:14: undefined: NewAuditLogger
internal/safety/audit_test.go:101:12: undefined: NewAuditLogger
internal/safety/audit_test.go:103:11: undefined: AuditEntry
internal/safety/audit_test.go:157:12: undefined: NewAuditLogger
internal/safety/audit_test.go:159:15: undefined: AuditEntry
internal/safety/audit_test.go:210:12: undefined: NewAuditLogger
internal/safety/audit_test.go:210:12: too many errors
# github.com/jamesprial/unraid-mcp/internal/auth [github.com/jamesprial/unraid-mcp/internal/auth.test]
internal/auth/middleware_test.go:108:18: undefined: NewAuthMiddleware
internal/auth/middleware_test.go:136:16: undefined: NewAuthMiddleware
internal/auth/middleware_test.go:163:16: undefined: NewAuthMiddleware
FAIL    github.com/jamesprial/unraid-mcp/internal/config [build failed]
FAIL    github.com/jamesprial/unraid-mcp/internal/auth [build failed]
FAIL    github.com/jamesprial/unraid-mcp/internal/safety [build failed]
FAIL
EXIT CODE: 1
```

---

## Failing Tests and What They Expect

### Package: `internal/config`

All tests fail because `Config` (struct), `LoadConfig` (function), and `DefaultConfig` (function) are undefined.

| Test | Undefined Symbols | Expected Behavior |
|------|-------------------|-------------------|
| `Test_LoadConfig_Cases/valid_config_loads_all_fields` | `Config`, `LoadConfig` | Reads `testdata/config/valid.yaml`; populates `Config.Server.Port=9090`, `Config.Server.AuthToken`, allowlists/denylists for Docker and VMs, custom Paths, Audit settings |
| `Test_LoadConfig_Cases/missing_file_returns_error` | `Config`, `LoadConfig` | Returns `(nil, err)` containing "no such file" when path does not exist |
| `Test_LoadConfig_Cases/invalid_YAML_returns_unmarshal_error` | `Config`, `LoadConfig` | Returns `(nil, err)` containing "unmarshal" for malformed YAML |
| `Test_LoadConfig_Cases/empty_file_returns_config_with_zero_values` | `Config`, `LoadConfig` | Returns non-nil `Config` with all zero values for empty YAML |
| `Test_DefaultConfig_Values/port_is_8080` | `Config`, `DefaultConfig` | `Config.Server.Port == 8080` |
| `Test_DefaultConfig_Values/audit_enabled_is_true` | `Config`, `DefaultConfig` | `Config.Audit.Enabled == true` |
| `Test_DefaultConfig_Values/audit_log_path_is_/config/audit.log` | `Config`, `DefaultConfig` | `Config.Audit.LogPath == "/config/audit.log"` |
| `Test_DefaultConfig_Values/docker_socket_path` | `Config`, `DefaultConfig` | `Config.Paths.DockerSocket == "/var/run/docker.sock"` |
| `Test_DefaultConfig_Values/libvirt_socket_path` | `Config`, `DefaultConfig` | `Config.Paths.LibvirtSocket == "/var/run/libvirt/libvirt-sock"` |
| `Test_DefaultConfig_Values/emhttp_path` | `Config`, `DefaultConfig` | `Config.Paths.Emhttp == "/host/emhttp"` |
| `Test_DefaultConfig_Values/proc_path` | `Config`, `DefaultConfig` | `Config.Paths.Proc == "/host/proc"` |
| `Test_DefaultConfig_Values/sys_path` | `Config`, `DefaultConfig` | `Config.Paths.Sys == "/host/sys"` |
| `Test_DefaultConfig_ReturnsNewInstance` | `Config`, `DefaultConfig` | Each call to `DefaultConfig()` returns a distinct pointer |

**Minimum API surface required:**
```go
// /Users/jamesprial/code/unraid-mcp/internal/config/config.go
type Config struct {
    Server ServerConfig
    Safety SafetyConfig
    Paths  PathsConfig
    Audit  AuditConfig
}
type ServerConfig struct { Port int; AuthToken string }
type SafetyConfig  struct { Docker ListConfig; VMs ListConfig }
type ListConfig    struct { Allowlist []string; Denylist []string }
type PathsConfig   struct { Emhttp, Proc, Sys, DockerSocket, LibvirtSocket string }
type AuditConfig   struct { Enabled bool; LogPath string; MaxSizeMB int }

func LoadConfig(path string) (*Config, error)
func DefaultConfig() *Config
```

---

### Package: `internal/auth`

All tests fail because `NewAuthMiddleware` is undefined.

| Test | Undefined Symbols | Expected Behavior |
|------|-------------------|-------------------|
| `Test_NewAuthMiddleware_Cases/valid_token_passes_through` | `NewAuthMiddleware` | `Bearer <token>` matching config token -> HTTP 200 |
| `Test_NewAuthMiddleware_Cases/missing_header_returns_401` | `NewAuthMiddleware` | No `Authorization` header -> HTTP 401 |
| `Test_NewAuthMiddleware_Cases/wrong_token_returns_401` | `NewAuthMiddleware` | Wrong token value -> HTTP 401 |
| `Test_NewAuthMiddleware_Cases/malformed_header_returns_401` | `NewAuthMiddleware` | Non-`Bearer` scheme -> HTTP 401 |
| `Test_NewAuthMiddleware_Cases/empty_token_config_disables_auth_-_no_header` | `NewAuthMiddleware` | Empty config token disables auth; any request passes -> HTTP 200 |
| `Test_NewAuthMiddleware_Cases/empty_token_config_disables_auth_-_any_header` | `NewAuthMiddleware` | Empty config token disables auth -> HTTP 200 regardless of header |
| `Test_NewAuthMiddleware_Cases/Bearer_with_extra_spaces_returns_401` | `NewAuthMiddleware` | `"Bearer  token"` (double space) -> HTTP 401 |
| `Test_NewAuthMiddleware_Cases/empty_Authorization_header_returns_401` | `NewAuthMiddleware` | Empty string header value -> HTTP 401 |
| `Test_NewAuthMiddleware_Cases/Bearer_prefix_with_no_token_returns_401` | `NewAuthMiddleware` | `"Bearer "` (trailing space only) -> HTTP 401 |
| `Test_NewAuthMiddleware_Cases/only_Bearer_word_returns_401` | `NewAuthMiddleware` | `"Bearer"` with no space or token -> HTTP 401 |
| `Test_NewAuthMiddleware_Cases/case_sensitive_Bearer_prefix` | `NewAuthMiddleware` | `"bearer token"` (lowercase b) -> HTTP 401 |
| `Test_NewAuthMiddleware_PassesRequestToNext` | `NewAuthMiddleware` | On auth success, next handler is called |
| `Test_NewAuthMiddleware_BlocksRequestFromNext` | `NewAuthMiddleware` | On auth failure, next handler is NOT called |

**Minimum API surface required:**
```go
// /Users/jamesprial/code/unraid-mcp/internal/auth/middleware.go
func NewAuthMiddleware(token string) func(http.Handler) http.Handler
```

---

### Package: `internal/safety`

All tests fail because `NewFilter`, `Filter`, `NewConfirmationTracker`, `ConfirmationTracker`, `NewAuditLogger`, `AuditLogger`, and `AuditEntry` are all undefined.

**filter_test.go** (`NewFilter` / `Filter.IsAllowed`):

| Test | Expected Behavior |
|------|-------------------|
| `Test_Filter_IsAllowed_Cases/empty_lists_allow_everything` | Empty allowlist + denylist -> `IsAllowed` returns `true` |
| `Test_Filter_IsAllowed_Cases/nil_lists_allow_everything` | Nil slices -> `IsAllowed` returns `true` |
| `Test_Filter_IsAllowed_Cases/in_allowlist_is_allowed` | Resource in allowlist -> `true` |
| `Test_Filter_IsAllowed_Cases/not_in_allowlist_is_denied` | Non-empty allowlist, resource absent -> `false` |
| `Test_Filter_IsAllowed_Cases/in_denylist_is_denied` | Resource in denylist -> `false` |
| `Test_Filter_IsAllowed_Cases/denylist_wins_over_allowlist` | Resource in both lists -> denylist wins -> `false` |
| `Test_Filter_IsAllowed_Cases/glob_pattern_in_denylist_matches` | `"*backup*"` glob denylist matches `"nightly-backup-db"` -> `false` |
| `Test_Filter_IsAllowed_Cases/glob_pattern_in_allowlist_matches` | `"plex*"` glob allowlist matches `"plex-media"` -> `true` |
| `Test_Filter_IsAllowed_Cases/glob_pattern_no_match_in_allowlist` | `"plex*"` does not match `"sonarr"` -> `false` |
| `Test_Filter_IsAllowed_Cases/glob_denylist_takes_priority_over_glob_allowlist` | `"*backup*"` denylist wins over `"*media*"` allowlist -> `false` |
| `Test_Filter_IsAllowed_Cases/exact_match_in_denylist_with_glob_allowlist` | `"*"` allowlist + `"dangerous"` denylist + resource `"dangerous"` -> `false` |
| `Test_Filter_IsAllowed_Cases/wildcard_allowlist_allows_non-denied` | `"*"` allowlist + `"dangerous"` denylist + resource `"safe-service"` -> `true` |
| `Test_Filter_IsAllowed_Cases/empty_resource_name_with_empty_lists` | Empty resource, empty lists -> `true` |
| `Test_Filter_IsAllowed_Cases/empty_resource_name_not_in_allowlist` | Empty resource, non-empty allowlist -> `false` |
| `Test_NewFilter_ReturnsNonNil/*` | `NewFilter` never returns nil |

**confirm_test.go** (`NewConfirmationTracker` / `ConfirmationTracker`):

| Test | Expected Behavior |
|------|-------------------|
| `Test_ConfirmationTracker_NeedsConfirmation_Cases/destructive_tool_needs_confirmation` | Tools in destructive list -> `NeedsConfirmation` returns `true` |
| `Test_ConfirmationTracker_NeedsConfirmation_Cases/non-destructive_tool_does_not_need_confirmation` | Tool not in list -> `false` |
| `Test_ConfirmationTracker_NeedsConfirmation_Cases/empty_tool_name_does_not_need_confirmation` | Empty string -> `false` |
| `Test_ConfirmationTracker_NeedsConfirmation_EmptyDestructiveList` | Empty list -> nothing needs confirmation |
| `Test_ConfirmationTracker_NeedsConfirmation_NilDestructiveList` | Nil list -> nothing needs confirmation |
| `Test_ConfirmationTracker_RequestAndConfirm` | `RequestConfirmation` returns non-empty token; `Confirm(token)` returns `true` |
| `Test_ConfirmationTracker_InvalidToken` | Unknown token -> `Confirm` returns `false` |
| `Test_ConfirmationTracker_EmptyToken` | Empty token -> `Confirm` returns `false` |
| `Test_ConfirmationTracker_TokenSingleUse` | Token consumed on first `Confirm`; second call returns `false` |
| `Test_ConfirmationTracker_MultipleTokensIndependent` | Multiple tokens are independent; each is single-use |
| `Test_ConfirmationTracker_RequestConfirmation_ReturnsNonEmptyToken/*` | `RequestConfirmation` always returns non-empty string |
| `Test_ConfirmationTracker_TokenExpiry` | Token valid immediately after creation |
| `Test_ConfirmationTracker_TokenExpiry_Simulation` | Token valid after 10ms sleep (well within 5-min window) |
| `Test_NewConfirmationTracker_ReturnsNonNil/*` | `NewConfirmationTracker` never returns nil |

**audit_test.go** (`NewAuditLogger` / `AuditLogger` / `AuditEntry`):

| Test | Expected Behavior |
|------|-------------------|
| `Test_AuditLogger_Log_Cases/valid_entry_is_written_successfully` | `Log(entry)` returns nil error; writes non-empty output |
| `Test_AuditLogger_Log_Cases/entry_with_nil_params` | Nil `Params` field is handled without error |
| `Test_AuditLogger_Log_Cases/entry_with_empty_tool_name` | Empty tool name logs without error |
| `Test_AuditLogger_Log_Format_JSON` | Output is valid JSON with `"tool"` and `"result"` fields |
| `Test_AuditLogger_Log_MultipleEntries` | Three `Log` calls produce exactly 3 newline-delimited JSON lines |
| `Test_AuditLogger_NilWriter` | `NewAuditLogger(nil)` either returns nil or `Log` returns an error (no panic) |
| `Test_NewAuditLogger_NonNilWriter` | `NewAuditLogger(&buf)` returns non-nil logger |

**Minimum API surface required:**
```go
// /Users/jamesprial/code/unraid-mcp/internal/safety/filter.go
type Filter struct { ... }
func NewFilter(allowlist, denylist []string) *Filter
func (f *Filter) IsAllowed(resource string) bool

// /Users/jamesprial/code/unraid-mcp/internal/safety/confirm.go
type ConfirmationTracker struct { ... }
func NewConfirmationTracker(destructiveTools []string) *ConfirmationTracker
func (ct *ConfirmationTracker) NeedsConfirmation(tool string) bool
func (ct *ConfirmationTracker) RequestConfirmation(tool, resourceName, description string) string
func (ct *ConfirmationTracker) Confirm(token string) bool

// /Users/jamesprial/code/unraid-mcp/internal/safety/audit.go
type AuditEntry struct {
    Timestamp time.Time
    Tool      string
    Params    map[string]any
    Result    string
    Duration  time.Duration
}
type AuditLogger struct { ... }
func NewAuditLogger(w io.Writer) *AuditLogger
func (al *AuditLogger) Log(entry AuditEntry) error
```

---

## Conclusion

The Red Phase verification confirms:

1. `go build ./...` exits 0 (no production source files to build - trivially passes).
2. `go test -v ./internal/config/ ./internal/auth/ ./internal/safety/` exits 1 with `[build failed]` for all three packages.
3. The failures are all `undefined: <symbol>` compile errors - every symbol the tests reference must be created from scratch.
4. The tests are not tautological; they exercise concrete behavior (HTTP status codes, YAML parsing, glob matching, token lifecycles, JSON log format) that does not exist yet.

**The implementation phase may proceed. All 30 tests across 5 files are verified as meaningful.**
