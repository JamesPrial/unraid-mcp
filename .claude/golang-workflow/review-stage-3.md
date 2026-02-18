# Code Review: Stage 3 -- 4 Domain Packages

**Reviewer:** Go Reviewer Agent
**Date:** 2026-02-18
**Verdict:** REQUEST_CHANGES

---

## Summary

Four domain packages (notifications, array, shares, ups) were reviewed. The code is well-structured, follows consistent patterns across all packages, and demonstrates good separation of concerns between manager (business logic / GraphQL interaction) and tools (MCP handler registration). The test suites are thorough with good edge case coverage. However, there are two issues that should be fixed before merge -- one is a correctness bug in the shares response parsing, and the other is a missing package-level doc comment on three types.go files.

---

## CRITICAL: Response Parsing Bug in `shares/manager.go`

**File:** `/Users/jamesprial/code/unraid-mcp/internal/shares/manager.go`, lines 24-28 and 40-41

The `sharesResponse` struct wraps the response in a `Data` field:

```go
type sharesResponse struct {
    Data struct {
        Shares []Share `json:"shares"`
    } `json:"data"`
}
```

However, the `graphql.Client.Execute()` method (defined in `/Users/jamesprial/code/unraid-mcp/internal/graphql/client.go`, line 130) already strips the `"data"` envelope -- it returns `[]byte(gqlResp.Data)`, which is the raw JSON of just the data field contents.

This means that when `Execute()` returns, the bytes represent `{"shares": [...]}` rather than `{"data": {"shares": [...]}}`.

The `sharesResponse` struct expects a `"data"` wrapper that will never be present in the bytes returned by `Execute()`. As a result, `resp.Data.Shares` will always be `nil` (because JSON unmarshalling will silently ignore the top-level `"shares"` key that does not match the `"data"` key), and the method will always return an empty slice.

**The same bug exists in `ups/manager.go`** (lines 28-32), where `upsResponse` also wraps in a `Data` field.

**Contrast with `notifications/manager.go`** (lines 28-32) which correctly does NOT have the extra `Data` wrapper:

```go
type listResponse struct {
    Notifications struct {
        List []Notification `json:"list"`
    } `json:"notifications"`
}
```

This is correct because `Execute()` already strips the `"data"` envelope.

### Fix Required

In `/Users/jamesprial/code/unraid-mcp/internal/shares/manager.go`, change:

```go
type sharesResponse struct {
    Data struct {
        Shares []Share `json:"shares"`
    } `json:"data"`
}
```

to:

```go
type sharesResponse struct {
    Shares []Share `json:"shares"`
}
```

And update line 45 from `resp.Data.Shares` to `resp.Shares`, and line 46-47 similarly.

In `/Users/jamesprial/code/unraid-mcp/internal/ups/manager.go`, change:

```go
type upsResponse struct {
    Data struct {
        UPS []UPSDevice `json:"ups"`
    } `json:"data"`
}
```

to:

```go
type upsResponse struct {
    UPS []UPSDevice `json:"ups"`
}
```

And update line 52 from `resp.Data.UPS` to `resp.UPS`.

**Note:** The tests for both shares and ups currently pass because the mock test data includes the `"data"` wrapper, matching the (incorrect) struct. When the structs are fixed, the test mock responses must also be updated to omit the `"data"` wrapper to match what `graphql.Client.Execute()` actually returns.

---

## Issue 2: Missing Package Doc Comments on types.go Files

**Files:**
- `/Users/jamesprial/code/unraid-mcp/internal/array/types.go` (line 1) -- has `package array` but no `// Package array ...` doc comment
- `/Users/jamesprial/code/unraid-mcp/internal/shares/types.go` (line 1) -- has `package shares` but no doc comment
- `/Users/jamesprial/code/unraid-mcp/internal/ups/types.go` (line 1) -- has `package ups` but no doc comment

The corresponding `manager.go` files in each package DO have proper package-level doc comments, so this is a minor consistency gap. Only one file in a package needs the package doc comment for `go doc` to pick it up, so the manager.go files cover it. However, for consistency and to follow the pattern set by `notifications/types.go` (which has `// Package notifications provides ...`), these should be added. This is low severity.

---

## Positive Observations

### Cross-Package Consistency
- All four packages follow the same architecture: `types.go` (interface + domain types), `manager.go` (GraphQL implementation), `tools.go` (MCP tool handlers).
- Compile-time interface checks (`var _ Interface = (*Impl)(nil)`) are present in all packages.
- Error wrapping consistently uses `%w` format verb throughout.
- All handlers follow the `(result, nil)` contract -- they never return a Go error.

### Error Handling
- Error messages are consistently prefixed with the operation context (e.g., `"notifications list: %w"`, `"array start: %w"`, `"shares: execute query: %w"`).
- The `tools.LogAudit` helper correctly handles nil audit loggers (line 28-30 in helpers.go).
- All tool handlers log both success and error paths to the audit logger.

### Nil Safety
- `Notification.Timestamp` is correctly `*string` and checked for nil in `formatNotification()` (line 46-48 in notifications/tools.go).
- UPS types use pointer types for optional fields (`*float64`, `*int`) throughout Battery and PowerInfo.
- The `mockGraphQLClient.Execute` in shares and ups tests does NOT guard against nil `executeFunc` (unlike notifications and array which do). This is acceptable because the test mocks always configure the function, but it is worth noting the inconsistency.

### Confirmation Flow
- Array tools correctly require confirmation for all three tools (start, stop, parity_check).
- Notifications correctly requires confirmation only for destructive actions (delete, delete_all) but not for archive/unarchive.
- The `parity_check` tool correctly validates the action BEFORE checking the confirmation token (lines 121-126 in array/tools.go), avoiding unnecessary confirmation prompts for invalid input.

### Test Quality
- Tests are thorough with table-driven patterns for the core cases.
- Edge cases are well covered: cancelled contexts, empty results, nil optional fields, invalid actions, missing required parameters.
- The confirmation flow is tested end-to-end (request token, then use token).
- Single-use token behavior is verified.
- Benchmarks are included in all four test files.

### GraphQL Queries
- Notifications uses `fmt.Sprintf` for parameterized queries (filter type and limit), which is appropriate for enum-like values.
- Array mutations are static strings, which is correct for parameterless operations.
- Parity check correctly maps `"start"` to `correct: false` and `"start_correct"` to `correct: true`.

### API Design
- Clean separation between read-only tools (no confirmation needed) and destructive tools (confirmation required).
- The `DestructiveTools` variable is exported from each package for use by the server registration code.
- Shares and UPS packages correctly have no `DestructiveTools` since they are read-only.
- The `ShareTools` function signature takes only `(mgr, audit)` while `ArrayTools` and `NotificationTools` additionally take `confirm`, correctly reflecting the destructive/read-only distinction.

---

## Action Items

| # | Severity | Package | File | Issue |
|---|----------|---------|------|-------|
| 1 | **Critical** | shares | manager.go:24-28 | `sharesResponse` struct has incorrect `Data` wrapper that does not match `Execute()` return format |
| 2 | **Critical** | ups | manager.go:28-32 | `upsResponse` struct has incorrect `Data` wrapper that does not match `Execute()` return format |
| 3 | **Critical** | shares | manager_test.go | Test mock responses include `"data"` wrapper; must be updated after fixing #1 |
| 4 | **Critical** | ups | manager_test.go | Test mock responses include `"data"` wrapper; must be updated after fixing #2 |
| 5 | Low | array | types.go:1 | Missing package doc comment |
| 6 | Low | shares | types.go:1 | Missing package doc comment |
| 7 | Low | ups | types.go:1 | Missing package doc comment |

Items 1-4 are blocking. Items 5-7 are recommended but not blocking.
