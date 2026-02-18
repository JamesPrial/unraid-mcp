# Code Review: Stage 1 - GraphQL HTTP Client

**Reviewer:** Go Reviewer Agent
**Date:** 2026-02-18
**Files reviewed:**
- `/Users/jamesprial/code/unraid-mcp/internal/graphql/types.go`
- `/Users/jamesprial/code/unraid-mcp/internal/graphql/client.go`
- `/Users/jamesprial/code/unraid-mcp/internal/graphql/client_test.go`

---

## Verdict: REQUEST_CHANGES

Two issues must be addressed before merge. One is a bug, the other is a design gap that will surface immediately when callers attempt `errors.Is`/`errors.As` on GraphQL-level errors.

---

## Issues Requiring Changes

### 1. [BUG] `normalizeURL` does not handle URLs that already end with `/graphql/` (trailing slash after suffix)

**File:** `/Users/jamesprial/code/unraid-mcp/internal/graphql/client.go`, lines 52-58

```go
func normalizeURL(rawURL string) string {
	u := strings.TrimRight(rawURL, "/")
	if !strings.HasSuffix(u, "/graphql") {
		u += "/graphql"
	}
	return u
}
```

`strings.TrimRight(rawURL, "/")` trims individual `/` characters from the right. This is correct for trailing slashes. However, the function silently accepts completely malformed URLs (e.g. `"not-a-url"` becomes `"not-a-url/graphql"`). While the downstream `http.NewRequestWithContext` will catch truly invalid URLs, consider documenting that `normalizeURL` does not validate the URL scheme or host. This is a minor documentation gap rather than a bug.

More importantly, there is **no test for `normalizeURL`** as a standalone unit. Given that it is an unexported helper, testing it through `NewHTTPClient` is acceptable, but the existing `NewHTTPClient` tests do not assert on the resulting `graphqlURL` field. You should either:
- Add a table-driven test for `normalizeURL` directly (even though it is unexported, it is in the same package as tests), or
- Add assertions in `Test_NewHTTPClient_Cases` that verify the constructed client's URL ends with `/graphql`.

**Severity:** Medium -- silent misconfiguration risk.

### 2. [DESIGN] GraphQL errors are not wrappable or inspectable via `errors.Is`/`errors.As`

**File:** `/Users/jamesprial/code/unraid-mcp/internal/graphql/client.go`, lines 122-128

```go
if len(gqlResp.Errors) > 0 {
    msgs := make([]string, len(gqlResp.Errors))
    for i, e := range gqlResp.Errors {
        msgs[i] = e.Message
    }
    return nil, fmt.Errorf("graphql: %s", strings.Join(msgs, "; "))
}
```

The existing Docker client in this codebase uses plain `fmt.Errorf` for API errors too, so this is at least consistent. However, for the GraphQL client specifically, callers will likely need to distinguish between:
- Network/transport errors (wrappable, already using `%w`)
- Authentication errors (HTTP 401)
- GraphQL-level errors (query validation failures, resolver errors)

Currently, the only way to distinguish GraphQL-level errors is string matching. Consider defining a typed error:

```go
// Error represents one or more errors returned in a GraphQL response.
type Error struct {
    Messages []string
}

func (e *Error) Error() string {
    return "graphql: " + strings.Join(e.Messages, "; ")
}
```

This allows callers to use `errors.As(err, &graphqlErr)` to programmatically inspect GraphQL errors without fragile string matching. The `GraphQLError` struct in `types.go` already exists but is only used for JSON deserialization -- it is not used as a Go error type.

**Severity:** Medium -- this is a design decision that becomes harder to change after other stages depend on the error types. Better to address now.

---

## Items Reviewed and Approved

### Interface Design

The `Client` interface in `/Users/jamesprial/code/unraid-mcp/internal/graphql/types.go` (lines 13-15) is excellent:

```go
type Client interface {
    Execute(ctx context.Context, query string, variables map[string]any) ([]byte, error)
}
```

- Single-method interface follows Go's small-interface principle.
- `context.Context` is the first parameter per Go convention.
- `map[string]any` for variables is appropriate for GraphQL.
- Returns `[]byte` (raw JSON) -- the right level of abstraction for a transport layer; callers handle deserialization.
- Consistent with the project's `DockerManager` interface pattern in `/Users/jamesprial/code/unraid-mcp/internal/docker/types.go`.

### Error Handling Patterns

All errors are correctly prefixed with `"graphql:"`, matching the project convention (`"docker:"` prefix in `/Users/jamesprial/code/unraid-mcp/internal/docker/manager.go`). Wrapping with `%w` is used for all I/O and marshaling errors (lines 94, 99, 106, 119). The HTTP 401 path correctly returns a distinct error message (line 111).

### Nil Safety

- `NewHTTPClient` validates `cfg.URL` is non-empty before use (line 32).
- `Execute` checks `c.apiKey` before making any network call (line 83).
- `resp.Body.Close()` is deferred immediately after a successful `Do` call (line 108).
- The `variables` parameter correctly accepts nil -- the `json:"omitempty"` tag on `graphqlRequest.Variables` handles this gracefully.

### Context Propagation

`http.NewRequestWithContext` is used (line 97), ensuring the context flows through to the HTTP transport. This correctly enables cancellation and deadline propagation, as verified by the test suite.

### Documentation

All exported items have godoc comments:
- Package comment on both files (lines 1-3 in each).
- `GraphQLError` struct (line 7 of types.go).
- `Client` interface (line 12 of types.go).
- `HTTPClient` struct (lines 19-21 of client.go).
- `NewHTTPClient` constructor (lines 27-30 of client.go) -- includes behavior documentation for zero/negative timeout and empty API key.
- `Execute` method (lines 72-81 of client.go) -- comprehensive error condition list.
- Unexported helpers (`normalizeURL`, `graphqlRequest`, `graphqlResponse`) also have comments, which is good practice.

### Timeout Handling

The default timeout of 30 seconds (line 17) matches the Docker client's timeout in `/Users/jamesprial/code/unraid-mcp/internal/docker/manager.go` (line 44). The timeout conversion from `int` seconds (from `config.GraphQLConfig.Timeout`) to `time.Duration` is correct (line 36). The zero/negative check (line 37) correctly falls back to the default.

### HTTP Status Code Handling

The 401-specific check (line 110) before the generic non-2xx check (line 113) is correct ordering. The error messages are distinct and informative.

### Test Quality

The test file is well-structured and thorough:

- **Compile-time interface check** (line 22): `var _ Client = (*HTTPClient)(nil)` -- correct pattern.
- **Table-driven tests** for `NewHTTPClient` (lines 49-136) and HTTP status codes (lines 667-751) follow Go conventions.
- **Test helpers** use `t.Helper()` (line 31) correctly.
- **`httptest.NewServer`** is used correctly throughout with `defer srv.Close()`.
- **Context cancellation tests** (lines 462-518) properly verify context propagation.
- **Concurrent request test** (lines 577-630) validates thread safety with proper synchronization.
- **Request body verification** (lines 181-276) confirms the wire format.
- **Benchmarks** (lines 834-900) are a nice addition.
- **Connection refused test** (lines 551-571) uses a creative pattern of starting/closing a server to guarantee a free port.

The test naming uses `Test_FunctionName_Scenario` convention consistently. All error path tests verify both that an error is returned and that the error message contains expected content.

### Consistency with Codebase

The GraphQL client follows the same structural patterns as the Docker client:
- Constructor takes a config struct, returns `(*ConcreteType, error)`.
- Compile-time interface satisfaction check at bottom of implementation file.
- Error prefix matches package name.
- `http.Client` is stored as a private field, not exposed.

---

## Summary

Two changes are required:

1. **Add test coverage for `normalizeURL`** -- either as a standalone table test or by asserting the resulting URL in the `NewHTTPClient` test cases. This ensures URL normalization edge cases (trailing slashes, URLs already ending in `/graphql`, etc.) are verified.

2. **Define a typed `Error` for GraphQL-level errors** that implements the `error` interface, so callers can use `errors.As` to distinguish GraphQL response errors from transport errors. This should be done now before other stages build on this package.

Everything else is clean, idiomatic, well-documented, and consistent with the existing codebase patterns.
