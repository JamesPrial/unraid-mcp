# Code Review: Stage 2 - GraphQL Escape Hatch Tool

**Files reviewed:**
- `/Users/jamesprial/code/unraid-mcp/internal/graphql/tools.go`
- `/Users/jamesprial/code/unraid-mcp/internal/graphql/tools_test.go`
- `/Users/jamesprial/code/unraid-mcp/internal/graphql/types.go` (context)
- `/Users/jamesprial/code/unraid-mcp/internal/graphql/client.go` (context)
- `/Users/jamesprial/code/unraid-mcp/internal/docker/container_tools.go` (pattern reference)
- `/Users/jamesprial/code/unraid-mcp/internal/tools/helpers.go` (helper reference)

---

## Verdict: REQUEST_CHANGES

Two concrete issues must be addressed. The code is otherwise well-structured and closely follows established patterns.

---

## Issues Requiring Changes

### 1. `wantResultErr` field is declared but never asserted (Bug in tests)

**File:** `/Users/jamesprial/code/unraid-mcp/internal/graphql/tools_test.go`, lines 210, 284, 296, and the assertion loop starting at line 301.

The table test struct declares `wantResultErr bool` with the comment "result text should contain 'error'", and two test cases set it to `true`:

```go
wantResultErr   bool // result text should contain "error"
```

However, the test execution loop at lines 301-335 never checks `tt.wantResultErr`. The field is dead code. This means the table-driven tests for the "invalid variables JSON" and "client returns error" cases are not actually asserting that the result is an error result -- they only check `wantContains`, which happens to overlap but does not validate the error semantics explicitly.

**Fix:** Add an assertion in the test loop:

```go
if tt.wantResultErr {
    if !strings.Contains(text, "error") {
        t.Errorf("result text = %q, expected it to contain 'error'", text)
    }
}
```

Or remove the field if the `wantContains` checks are considered sufficient. Either way, dead struct fields in table tests are a code smell and a maintenance hazard.

### 2. Missing test for invalid JSON returned by `client.Execute` (Gap in error path coverage)

**File:** `/Users/jamesprial/code/unraid-mcp/internal/graphql/tools_test.go`

In `tools.go` at lines 71-75, there is a third error path where `client.Execute` returns bytes that are not valid JSON:

```go
var parsed any
if err := json.Unmarshal(data, &parsed); err != nil {
    tools.LogAudit(audit, toolNameGraphQLQuery, params, "error: "+err.Error(), start)
    return tools.ErrorResult(err.Error()), nil
}
```

No test case covers this path. The table test and standalone tests only cover:
- Successful JSON responses
- `client.Execute` returning an error
- Invalid variables JSON

A test case like the following should be added to `Test_GraphQLQueryHandler_Cases`:

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

## Positive Observations

### Pattern Compliance (tools.go)

The implementation follows the established pattern from `container_tools.go` precisely:

- **Function signature:** `GraphQLTools(client Client, audit *safety.AuditLogger) []tools.Registration` matches the convention of returning `[]tools.Registration`.
- **Tool construction:** Uses `mcp.NewTool` with `mcp.WithDescription`, `mcp.WithString`, `mcp.Required()`, and `mcp.Description()` -- identical to docker tools.
- **Handler closure pattern:** `func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error)` with `start := time.Now()` at the top -- matches exactly.
- **Return convention:** Always returns `(result, nil)`, never a Go error. Verified across all three error paths (lines 58, 66, 74) and the success path (line 78).
- **Registration assembly:** `tools.Registration{Tool: tool, Handler: server.ToolHandlerFunc(handler)}` -- identical to docker tools.

### Error Handling (tools.go)

- All three error paths (invalid variables JSON, client execution error, unmarshal error) correctly use `tools.ErrorResult()` and return `nil` as the second value.
- All three error paths have audit logging with `"error: "` prefix before the error message.
- The success path logs with `"ok"` status.
- The `params` map is captured before any error can occur, ensuring audit entries always have complete parameters.

### Audit Logging (tools.go)

- Audit logging on all four code paths (3 error + 1 success) -- complete coverage.
- Uses `tools.LogAudit()` which handles nil audit logger gracefully (verified in `helpers.go` line 29).
- The `toolNameGraphQLQuery` constant avoids string duplication -- good practice, consistent with `docker_stop`/`docker_restart` using `const toolName`.

### Nil Safety (tools.go)

- `tools.LogAudit` already guards against nil audit logger.
- `parsedVars` starts as `nil` and is only set if `variablesStr != ""`, so empty/absent variables correctly pass `nil` to `client.Execute`.

### Documentation (tools.go)

- Package-level doc comment present.
- `GraphQLTools` (exported) has a proper doc comment.
- `toolGraphQLQuery` (unexported) has a doc comment -- exceeds the minimum requirement.

### Test Quality (tools_test.go)

- **Mock implementation:** `mockClient` with `executeFunc` field allows per-test behavior injection. Compile-time interface satisfaction check at line 29 is good practice.
- **Test helpers:** `newCallToolRequest`, `extractResultText`, and `newTestAuditLogger` are well-documented with `t.Helper()` calls.
- **Table-driven test:** `Test_GraphQLQueryHandler_Cases` covers happy path, valid variables, empty variables, absent variables, invalid variables, and client error cases.
- **Standalone tests:** Targeted tests for nil audit logger (no-panic), variable forwarding, query forwarding, pretty JSON output, error prefix, and invalid variables short-circuit provide good focused coverage.
- **Benchmarks:** Two benchmarks (happy path and with-variables) are a nice addition.
- **Test naming:** Follows `Test_TypeOrFunc_Description` convention consistently.

### Variables JSON Parsing (tools.go)

- Empty string and absent key both correctly result in `nil` variables (lines 54-60).
- Invalid JSON is caught before `client.Execute` is called (confirmed by `Test_GraphQLQueryHandler_InvalidVariablesDoesNotCallClient`).
- Complex nested JSON (arrays, objects) is tested.

---

## Summary

The implementation is clean, well-documented, and follows established project patterns faithfully. The two issues are:

1. **Dead `wantResultErr` field in table test** -- declared, populated, but never asserted. Must be wired into assertions or removed.
2. **Missing test for invalid JSON from `client.Execute`** -- the third error path (json.Unmarshal failure at line 72 of tools.go) has no test coverage.

Both are straightforward fixes.
