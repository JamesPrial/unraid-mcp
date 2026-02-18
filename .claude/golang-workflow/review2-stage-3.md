# Design Review: Stage 3 -- 4 Domain Packages

**Reviewer:** Go Reviewer (Opus 4.6)
**Date:** 2026-02-18
**Packages:** `notifications`, `array`, `shares`, `ups`

---

## Verdict: REQUEST_CHANGES

The overall design is solid and follows the established codebase patterns well. The `types.go -> manager.go -> tools.go` file layout is consistent, the interface-driven design enables clean testability, and the confirmation flow for destructive operations is correctly implemented. However, there are several issues that should be addressed before merge.

---

## Critical Issues

### 1. GraphQL Injection Vulnerability in Notifications Manager

**File:** `/Users/jamesprial/code/unraid-mcp/internal/notifications/manager.go`, lines 38-41 and 59, 69, 79

The `List` method uses `fmt.Sprintf` to interpolate user-supplied `filterType` directly into the GraphQL query string:

```go
query := fmt.Sprintf(
    `{ notifications { list(filter: { type: %s, limit: %d }) { ... } } }`,
    filterType, limit,
)
```

The `filterType` parameter comes from user input (via the tool handler's `req.GetString("filter_type", "UNREAD")`). While the value defaults to `"UNREAD"`, a caller could pass arbitrary strings that would be injected directly into the query. The same pattern is used for `Archive`, `Unarchive`, and `Delete` mutations where `id` is interpolated with `%s` inside double quotes:

```go
mutation := fmt.Sprintf(`mutation { notifications { archive(id: "%s") } }`, id)
```

An `id` value containing `"` could break out of the string literal.

**Recommended fix:** Either validate `filterType` against an allowlist before query construction (similar to how `validParityActions` works in the array package), or use GraphQL variables via the `client.Execute(ctx, query, variables)` mechanism. For the `id` parameter, validate it does not contain special characters, or pass it as a GraphQL variable.

### 2. Inconsistent JSON Response Envelope Structures

**File:** `/Users/jamesprial/code/unraid-mcp/internal/shares/manager.go`, lines 24-28
**File:** `/Users/jamesprial/code/unraid-mcp/internal/ups/manager.go`, lines 28-32
**File:** `/Users/jamesprial/code/unraid-mcp/internal/notifications/manager.go`, lines 29-32

The shares and UPS packages expect the GraphQL response to have a `data` wrapper:

```go
// shares
type sharesResponse struct {
    Data struct {
        Shares []Share `json:"shares"`
    } `json:"data"`
}

// ups
type upsResponse struct {
    Data struct {
        UPS []UPSDevice `json:"ups"`
    } `json:"data"`
}
```

But the notifications package does NOT have a `data` wrapper:

```go
type listResponse struct {
    Notifications struct {
        List []Notification `json:"list"`
    } `json:"notifications"`
}
```

This means either the `graphql.Client.Execute` method returns the raw GraphQL response (including `data` key) or it returns just the `data` contents. It cannot be both. One of these envelope structures is wrong and will cause silent unmarshalling failures (all fields will be zero-valued).

**Recommended fix:** Determine what `graphql.Client.Execute` actually returns and make all response structs consistent. If `Execute` returns the entire `{"data": {...}}` envelope, the notifications response struct needs a `Data` wrapper. If `Execute` strips the `data` key, the shares and UPS structs should remove their `Data` wrapper.

---

## Design Issues (Should Fix)

### 3. Missing Package Doc Comment on `array/types.go`

**File:** `/Users/jamesprial/code/unraid-mcp/internal/array/types.go`

The `types.go` file is missing a package doc comment. Compare with the other packages:
- `notifications/types.go` has `// Package notifications provides...`
- `shares/types.go` has no package comment (also missing)
- `ups/types.go` has no package comment (also missing)

While Go only requires one package comment per package (and `manager.go` provides it in each case), it is cleaner to have the comment on `types.go` since it is the first file alphabetically and the canonical place for types. At minimum, `array/types.go` and `shares/types.go` should be consistent with the others -- either all have it or none do.

### 4. Naming Inconsistency: Interface Name for UPS

**File:** `/Users/jamesprial/code/unraid-mcp/internal/ups/types.go`, line 29

The UPS interface is named `UPSMonitor`, while the other packages follow a `<Domain>Manager` convention:
- `notifications` -> `NotificationManager`
- `array` -> `ArrayManager`
- `shares` -> `ShareManager`
- `ups` -> `UPSMonitor` (inconsistent)

Since the UPS package is read-only monitoring, "Monitor" is semantically correct. However, this breaks the naming convention. The docker and VM packages use `DockerManager` and `VMManager` even though they also have read-only list/inspect operations.

**Recommendation:** This is a borderline issue. "Monitor" is arguably more accurate for a read-only interface. Document the rationale if keeping it, or rename to `UPSManager` for consistency.

### 5. Missing `DestructiveTools` Variable in Read-Only Packages

**File:** `/Users/jamesprial/code/unraid-mcp/internal/shares/tools.go`
**File:** `/Users/jamesprial/code/unraid-mcp/internal/ups/tools.go`

The `shares` and `ups` packages do not export a `DestructiveTools` variable. The `notifications` and `array` packages both export one. For packages that have no destructive tools, exporting an empty slice would be more consistent and allow the registration layer to uniformly collect destructive tool names:

```go
// DestructiveTools is empty -- all share tools are read-only.
var DestructiveTools []string
```

This is a minor consistency point, but it simplifies the main registration code since it can iterate all packages uniformly.

### 6. Inconsistent Tool Function Naming Conventions

Across the four packages, the internal tool constructor functions use inconsistent naming:

- `notifications`: `toolNotificationsList`, `toolNotificationsManage` (prefix: `tool`)
- `array`: `arrayStart`, `arrayStop`, `parityCheck` (prefix: domain name)
- `shares`: `sharesListTool` (suffix: `Tool`)
- `ups`: `upsStatus` (prefix: domain name)

Compare with reference patterns:
- `graphql/tools.go`: `toolGraphQLQuery` (prefix: `tool`)
- `docker/container_tools.go`: `toolDockerList`, `toolDockerStop` (prefix: `tool`)
- `vm/tools.go`: `vmList`, `vmStop` (prefix: domain name)

The codebase itself has two conventions (`tool` prefix vs domain prefix). Since these are unexported functions, this is not a public API concern, but consistency within the new packages would improve readability. Pick one convention and use it for all four packages.

---

## Minor Observations (Non-blocking)

### 7. `ShareTools` Signature Does Not Accept `*safety.ConfirmationTracker`

**File:** `/Users/jamesprial/code/unraid-mcp/internal/shares/tools.go`, line 16

```go
func ShareTools(mgr ShareManager, audit *safety.AuditLogger) []tools.Registration {
```

This is correct because shares are read-only. However, `UPSTools` has the same signature. This is fine -- just noting for consistency awareness. If future UPS tools need confirmation, the signature will need to change.

### 8. Duplicate Test Helpers Across Packages

The `newCallToolRequest`, `extractResultText`, and similar test helpers are duplicated across all four test files. This is acceptable for now since Go test helpers are typically package-local, but if more domain packages are added, consider a shared `internal/testutil` package.

### 9. Notification `formatNotification` Uses Redundant Importance Display

**File:** `/Users/jamesprial/code/unraid-mcp/internal/notifications/tools.go`, line 49

```go
return fmt.Sprintf("%s [%s] %s -- %s\n  Subject: %s\n  ID: %s\n  Timestamp: %s",
    importanceMarker(n.Importance),  // e.g., "[WARNING]"
    n.Importance,                     // e.g., "warning"
    ...
```

This produces output like `[WARNING] [warning] Title -- Description`, showing the importance twice. Consider removing one of them.

### 10. Test Quality Assessment

The tests are thorough and well-structured:
- Table-driven tests with clear case names
- Both manager-layer and tool-handler-layer tests
- Mock at both levels (GraphQL client mock for manager tests, manager mock for tool tests)
- Confirmation flow round-trip tests
- Error propagation tests
- Handler nil-error contract verification
- Benchmarks included
- Compile-time interface checks

The test coverage appears comprehensive for the happy path, error paths, validation edge cases, and the confirmation token lifecycle.

---

## Summary of Required Changes

| # | Severity | Description |
|---|----------|-------------|
| 1 | Critical | GraphQL injection: `filterType` and `id` interpolated unsafely into query strings in notifications manager |
| 2 | Critical | Inconsistent JSON response envelope (`data` wrapper present in shares/ups but absent in notifications) -- one approach is wrong |
| 3 | Minor | Missing/inconsistent package doc comments on `types.go` files |
| 4 | Minor | `UPSMonitor` vs `<Domain>Manager` naming convention |
| 5 | Minor | Missing `DestructiveTools` variable in read-only packages |
| 6 | Minor | Inconsistent internal tool function naming across packages |

Items 1 and 2 should be addressed before merge. Items 3-6 can be addressed in a follow-up if preferred, but would be ideal to fix now.

