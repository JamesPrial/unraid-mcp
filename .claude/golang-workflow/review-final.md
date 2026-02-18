# Final Code Review: GraphQL Integration into unraid-mcp

**Reviewer:** Go Code Reviewer (automated)
**Date:** 2026-02-18
**Scope:** All new GraphQL implementation files, domain packages, tests, and main.go wiring

---

## VERDICT: REQUEST_CHANGES

Two issues must be fixed before merging. One is a potential data mismatch bug, the other is a missing test coverage gap for a security-critical function. Everything else is high quality.

---

## Critical Issues (must fix)

### 1. UPS GraphQL query field name vs response struct mismatch

**File:** `/Users/jamesprial/code/unraid-mcp/internal/ups/manager.go`, line 35

The GraphQL query sends:

```go
const upsQuery = `query { ups { id name model status battery { charge runtime } power { inputVoltage outputVoltage load } } }`
```

But the response struct expects the JSON key `"upsDevices"`:

```go
type upsResponse struct {
    UPS []UPSDevice `json:"upsDevices"`
}
```

The query field is `ups`, which means the Unraid GraphQL API will return a response envelope where the key is `ups`, not `upsDevices`. For example: `{"ups": [...]}`. The `json:"upsDevices"` tag will never match that key, so `resp.UPS` will always be nil, and `GetDevices` will always return an empty slice regardless of how many UPS devices exist.

The tests pass because every test mock returns JSON with `"upsDevices"` as the key -- which matches the struct tag but does not match what the real API would return given the query field name `ups`.

**Fix:** Either change the JSON tag to `json:"ups"` to match the query, or change the query to use whatever field name the Unraid API actually returns. The test mock responses must also be updated to match the real API shape.

### 2. No tests for `validateID` injection prevention or invalid filter types in notifications manager

**File:** `/Users/jamesprial/code/unraid-mcp/internal/notifications/manager.go`, lines 46 and 70-75

The `validateID` function (line 70) and the `validFilterTypes` check (line 46) are security-critical guards against GraphQL injection. Neither has direct unit test coverage in `/Users/jamesprial/code/unraid-mcp/internal/notifications/manager_test.go`:

- No test calls `List` with an invalid filter type (e.g., `"INVALID"`) to verify the error path.
- No test calls `Archive`, `Unarchive`, or `Delete` with an ID containing `"`, `'`, or `\` to verify that `validateID` rejects them.
- No test verifies that an empty ID is handled (it currently would pass `validateID` but produce a broken query string).

Since these are the primary defense against query injection in the notification mutations, they need explicit test coverage.

**Fix:** Add table-driven tests for:
- `List` with invalid filter types (expect error with descriptive message)
- `Archive`/`Unarchive`/`Delete` with IDs containing `"`, `'`, `\` (expect error from `validateID`)
- Optionally, `Archive`/`Delete` with empty string IDs (to verify behavior)

---

## Moderate Issues (strongly recommended)

### 3. Nil client guard missing on all constructors

**Files:**
- `/Users/jamesprial/code/unraid-mcp/internal/notifications/manager.go`, line 31
- `/Users/jamesprial/code/unraid-mcp/internal/array/manager.go`, line 24
- `/Users/jamesprial/code/unraid-mcp/internal/shares/manager.go`, line 19
- `/Users/jamesprial/code/unraid-mcp/internal/ups/manager.go`, line 23

All four `NewGraphQL*` constructors accept a `graphql.Client` interface parameter but do not validate that it is non-nil. A nil client would cause a nil pointer dereference on the first `Execute` call. The `graphql.NewHTTPClient` constructor validates its config fields; these constructors should follow the same defensive pattern.

This is not a blocking issue since `main.go` always creates the client before constructing managers, but it would be an easy nil-pointer crash if the wiring ever changes.

**Recommendation:** Add a nil check at the top of each constructor:

```go
func NewGraphQLNotificationManager(client graphql.Client) *GraphQLNotificationManager {
    if client == nil {
        panic("notifications: nil graphql client")
    }
    // ...
}
```

Or return an error. Either approach is acceptable.

### 4. `validParityActions` map is referenced from both manager.go and tools.go

**File:** `/Users/jamesprial/code/unraid-mcp/internal/array/tools.go`, line 122

The `parityCheck` tool handler validates the action against `validParityActions` (defined in `manager.go`, line 47) before dispatching. The manager's `ParityCheck` method also validates the same set. This dual validation is correct for defense-in-depth, but the two locations could drift if a new action is added to one but not the other. Consider extracting `ValidParityActions` as an exported function or using the manager as the single source of truth.

Not blocking, but worth noting for maintainability.

---

## Positive Observations

### Architecture and Design

- **Clean interface-based design.** Every domain package defines an interface (`NotificationManager`, `ArrayManager`, `ShareManager`, `UPSMonitor`) with a single concrete GraphQL-backed implementation. This makes testing trivial and future transport swaps painless.

- **Consistent layered structure.** Each package follows the same three-file pattern: `types.go` (interface + data types), `manager.go` (implementation), `tools.go` (MCP tool registration). This is easy to navigate and extend.

- **Compile-time interface checks** (`var _ Interface = (*ConcreteType)(nil)`) are present in every implementation file. Good practice.

- **Dependency injection throughout.** All managers accept `graphql.Client` interfaces. All tool functions accept manager interfaces. The `main.go` wiring composes everything cleanly.

### Error Handling

- **Consistent error wrapping with %w.** All manager-layer errors use `fmt.Errorf("context: %w", err)`, preserving the error chain. Example from `/Users/jamesprial/code/unraid-mcp/internal/graphql/client.go`, line 94: `fmt.Errorf("graphql: marshal request: %w", err)`.

- **Tool handlers never return Go errors.** Every handler returns `(result, nil)` and surfaces errors via `tools.ErrorResult()`. This matches the MCP convention correctly and is tested exhaustively.

- **Specific HTTP status handling.** The GraphQL client distinguishes 401 (auth failure) from other non-2xx codes with distinct error messages at lines 110-114 of `client.go`.

### Security

- **GraphQL injection prevention via allowlists.** The notifications manager uses `validFilterTypes` (line 15 of manager.go) to reject arbitrary filter strings before they are interpolated into queries. Parity actions use `validParityActions`. This is the correct approach for enum-like parameters.

- **`validateID` character blocklist.** The notification ID validator (line 70 of `manager.go`) rejects `"`, `'`, and `\` characters to prevent string escape attacks in interpolated GraphQL queries.

- **Confirmation tokens for destructive operations.** All mutating array tools and the `notifications_manage` tool require confirmation tokens. The `DestructiveTools` slices are correctly wired to `ConfirmationTracker` instances in `main.go`.

- **API key not logged.** The audit logger captures tool name, params, and result, but the API key is never included in audit entries.

### main.go Wiring

- **Graceful degradation.** GraphQL tools are only registered when `cfg.GraphQL.URL != ""` (line 107). If `NewHTTPClient` fails, a warning is logged and the server continues without GraphQL tools. This matches the existing pattern for VM tools (line 77).

- **Single shared confirmation tracker for all GraphQL destructive tools.** Lines 113-116 correctly aggregate destructive tool names from both `notifications.DestructiveTools` and `array.DestructiveTools` into one `gqlConfirm` tracker. This is appropriate because they share the same trust domain.

- **Clean shutdown.** Signal handling, context timeout, and deferred file close are all present.

### Test Quality

- **Extensive table-driven tests.** The `client_test.go` file alone has table tests for URL normalization, constructor validation, HTTP status codes, and GraphQL error handling. Domain manager tests follow the same pattern.

- **Mock isolation.** Each test package defines its own mock types that implement the relevant interface, avoiding cross-package test dependencies.

- **Confirmation flow tested end-to-end.** Tests in `array/manager_test.go` exercise the full two-step confirmation flow: first call gets a token, second call uses it.

- **Concurrent request test.** `Test_Execute_ConcurrentRequests` in `client_test.go` verifies the HTTP client is safe for concurrent use with 10 goroutines.

- **Benchmarks included.** Every package includes benchmarks for hot paths (handler execution, manager operations).

### Code Organization

- **Consistent doc comments on all exported items.** Every exported type, function, constant, and variable has a Go doc comment.

- **Idiomatic naming.** Types follow Go conventions: `GraphQLNotificationManager`, not `NotificationManagerGraphQL`. Tool names use snake_case as MCP convention requires.

- **No unused imports or variables** detected across all files.

---

## Checklist Summary

| Check | Status |
|---|---|
| All exported items documented | PASS |
| Error handling follows %w pattern | PASS |
| Nil safety guards on constructors | WARN (Issue #3) |
| Table tests structured correctly | PASS |
| Error paths have test coverage | FAIL (Issue #2 -- validateID/filterType) |
| Naming conventions followed | PASS |
| No logic errors | FAIL (Issue #1 -- UPS query/response mismatch) |
| Confirmation for destructive ops | PASS |
| GraphQL injection prevention | PASS (design), FAIL (test coverage) |
| main.go wiring correct | PASS |
| Audit logging consistent | PASS |
| Resource cleanup uses defer | PASS |

---

## Required Actions Before Merge

1. **Fix UPS query/response field name mismatch** -- Change `json:"upsDevices"` to match the actual GraphQL query field `ups`, or vice versa. Update test mocks to match the real API response shape.

2. **Add tests for `validateID` and invalid filter types** -- These are security-critical code paths that currently have zero direct test coverage.

After these two items are resolved, this integration is ready for Wave 4 verification.
