# Design Review (Round 2): Stage 2 - GraphQL Escape Hatch Tool

**Files reviewed:**
- `/Users/jamesprial/code/unraid-mcp/internal/graphql/tools.go` (82 lines)
- `/Users/jamesprial/code/unraid-mcp/internal/graphql/tools_test.go` (688 lines)
- `/Users/jamesprial/code/unraid-mcp/internal/graphql/types.go` (context)
- `/Users/jamesprial/code/unraid-mcp/internal/graphql/client.go` (context)
- `/Users/jamesprial/code/unraid-mcp/internal/docker/container_tools.go` (pattern reference)
- `/Users/jamesprial/code/unraid-mcp/internal/docker/tools.go` (pattern reference)
- `/Users/jamesprial/code/unraid-mcp/internal/tools/registration.go` (pattern reference)
- `/Users/jamesprial/code/unraid-mcp/internal/tools/helpers.go` (pattern reference)
- `/Users/jamesprial/code/unraid-mcp/cmd/server/main.go` (integration context)

---

## Verdict: REQUEST_CHANGES

Two issues from the prior review (review-stage-2.md) remain unresolved, and one additional issue was identified. The implementation code (tools.go) is well-designed and ready; only the test file needs corrections.

---

## Unresolved Issues from Prior Review

### 1. Dead `wantResultErr` and `wantErrNil` fields in table test struct (still present)

**File:** `/Users/jamesprial/code/unraid-mcp/internal/graphql/tools_test.go`, lines 209-210

The table test struct declares two boolean fields that are populated in every test case but never referenced in the assertion loop (lines 301-335):

```go
// Line 209-210
wantErrNil      bool // second return value from handler should always be nil
wantResultErr   bool // result text should contain "error"
```

`wantErrNil` is set to `true` in all six cases (lines 222, 241, 256, 270, 283, 295). `wantResultErr` is set to `true` in two cases (lines 284, 296). Neither field appears in any assertion -- confirmed by searching for `tt.wantResultErr` and `tt.wantErrNil` which yield zero matches in the assertion loop.

The `err != nil` check at line 318 runs unconditionally (not gated by `tt.wantErrNil`), so the handler-returns-nil-error invariant is effectively tested. However, dead struct fields in table tests are a maintenance hazard -- future readers will assume they are being asserted.

**Required fix (choose one):**
- **(A)** Wire `wantResultErr` into the assertion loop and remove `wantErrNil` (since the invariant is always checked):
  ```go
  if tt.wantResultErr && !strings.Contains(text, "error") {
      t.Errorf("result text = %q, expected it to contain 'error'", text)
  }
  ```
- **(B)** Remove both dead fields entirely if the existing `wantContains` checks are considered sufficient.

### 2. Missing test for invalid JSON returned by `client.Execute` (still missing)

**File:** `/Users/jamesprial/code/unraid-mcp/internal/graphql/tools_test.go`

In `tools.go` lines 71-75, the handler has a third error path where `json.Unmarshal(data, &parsed)` fails because `client.Execute` returned syntactically invalid JSON bytes:

```go
// tools.go lines 71-75
var parsed any
if err := json.Unmarshal(data, &parsed); err != nil {
    tools.LogAudit(audit, toolNameGraphQLQuery, params, "error: "+err.Error(), start)
    return tools.ErrorResult(err.Error()), nil
}
```

No test case exercises this path. The three tested error paths are:
1. Invalid variables JSON (line 55)
2. `client.Execute` returns an error (line 64)
3. **(untested)** `client.Execute` returns non-JSON bytes

**Required fix:** Add a test case to `Test_GraphQLQueryHandler_Cases`:

```go
{
    name: "client returns invalid JSON bytes produces error result",
    args: map[string]any{
        "query": "{ info }",
    },
    executeFunc: func(ctx context.Context, query string, variables map[string]any) ([]byte, error) {
        return []byte("not valid json"), nil
    },
    wantErrNil:    true,
    wantResultErr: true,
    wantContains:  "invalid character",
},
```

---

## New Issue

### 3. Empty query string is not validated

**File:** `/Users/jamesprial/code/unraid-mcp/internal/graphql/tools.go`, line 44

When `query` is an empty string (either because the caller omitted it despite `Required()`, or because the MCP framework delivers an empty string), it is passed directly to `client.Execute` without validation:

```go
query := req.GetString("query", "")
```

This is a minor design concern rather than a bug, since `Required()` in the MCP schema should prevent the tool from being called without a query. However, defensive validation is a pattern seen elsewhere -- for example, the Docker tools always receive required parameters and still operate on them without extra guards, so this is **consistent** with the codebase pattern.

**Severity:** Observation only. No change required. If the team prefers defensive validation, a guard like `if query == "" { return tools.ErrorResult("query parameter is required"), nil }` could be added, but this is optional.

---

## Design Assessment (Implementation: tools.go)

The implementation is clean, concise (82 lines), and well-designed as an escape hatch. The following aspects are strong:

### Package Organization

The `internal/graphql` package cleanly separates concerns across three files:
- `types.go` -- interface definition and shared types
- `client.go` -- HTTP transport implementation
- `tools.go` -- MCP tool registration

This mirrors the established pattern in `internal/docker` (types.go / manager.go / tools.go / container_tools.go). The package is self-contained and has appropriate dependencies.

### API Surface

The exported API is minimal and correct:
- `GraphQLTools(client Client, audit *safety.AuditLogger) []tools.Registration` -- the factory function
- `Client` interface and `HTTPClient` struct from Stage 1

The factory function signature follows the convention from `DockerTools()`, `VMTools()`, and `SystemTools()`. Notably, the GraphQL escape hatch intentionally omits `*safety.Filter` and `*safety.ConfirmationTracker` parameters. This is a reasonable design choice: the escape hatch is meant to allow arbitrary queries, so filtering by resource name does not apply, and mutations through this tool are the caller's responsibility. This is documented in the tool description ("Use when direct API access is needed beyond the provided tools.").

### Handler Pattern Compliance

The handler follows the established pattern exactly:

| Pattern Element | Docker Tools | GraphQL Tool |
|----------------|-------------|--------------|
| `start := time.Now()` at top | Yes | Yes |
| `params` map captured before logic | Yes | Yes |
| Error paths return `(tools.ErrorResult(...), nil)` | Yes | Yes |
| Success path returns `(tools.JSONResult(...), nil)` | Yes (for JSON) | Yes |
| Audit logged on every path | Yes | Yes (all 4 paths) |
| Tool name as constant | `const toolName` | `const toolNameGraphQLQuery` |
| Registration via `tools.Registration{Tool, Handler}` | Yes | Yes |

### Variables-as-String Design

Accepting variables as a JSON string (`mcp.WithString("variables", ...)`) rather than as a nested object is the correct design choice for an MCP tool. MCP tool parameters are flat key-value pairs with typed schemas -- passing a pre-serialized JSON string avoids schema complexity and allows arbitrary variable shapes. The handler parses it internally before forwarding to `client.Execute`. This is well-implemented.

### Integration Readiness

`main.go` does not yet wire up `graphql.GraphQLTools(...)`. This is expected for a staged implementation. The wiring will require:
1. Creating an `HTTPClient` from config
2. Appending `graphql.GraphQLTools(client, auditLogger)...` to the registrations slice

This is straightforward and consistent with the existing registration pattern.

---

## Test Assessment (tools_test.go)

### Strengths

- **Mock design:** `mockClient` with injectable `executeFunc` is clean. The compile-time interface check (`var _ Client = (*mockClient)(nil)`) is good practice.
- **Test helpers:** `newCallToolRequest`, `extractResultText`, and `newTestAuditLogger` all use `t.Helper()` correctly and have clear documentation.
- **Table-driven test:** `Test_GraphQLQueryHandler_Cases` covers 6 scenarios with good edge case coverage (empty variables, absent variables, invalid variables JSON).
- **Focused standalone tests:** Individual tests for nil-audit-logger safety, variable forwarding, query forwarding, pretty JSON output, error prefix formatting, and invalid-variables short-circuit provide targeted coverage of specific concerns.
- **Benchmarks:** Two benchmarks for happy path and with-variables are a welcome addition.
- **Naming:** All test names follow `Test_TypeOrFunc_Description` convention consistently.

### Issues (listed above)

1. Dead struct fields in the table test (`wantResultErr`, `wantErrNil`)
2. Missing test for `json.Unmarshal(data, &parsed)` error path

---

## Summary

The implementation file (`tools.go`) is well-designed, follows codebase conventions precisely, and is ready as-is. The test file has two outstanding issues carried from the prior review:

1. **Dead `wantResultErr`/`wantErrNil` fields** in the table test struct -- must be wired into assertions or removed.
2. **Missing test for invalid JSON response from client** -- the `json.Unmarshal` error path at line 72 of tools.go has no test coverage.

Both are straightforward fixes confined to `tools_test.go`.
