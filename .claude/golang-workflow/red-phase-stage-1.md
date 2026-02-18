# TDD Red Phase Report — Stage 1: GraphQL Client

**Date:** 2026-02-18
**Stage:** 1 — GraphQL Client (`internal/graphql`)
**Verdict:** RED_VERIFIED

---

## Summary

- **Verdict:** RED_VERIFIED
- **Build result:** Test binary FAILS to compile (expected — `HTTPClient` and `NewHTTPClient` are not yet defined)
- **Test result:** Cannot run (build failure blocks execution)
- **Missing symbols:** `HTTPClient` (struct), `NewHTTPClient` (constructor function)
- **Existing stubs:** `types.go` defines `GraphQLError` and the `Client` interface — these compile cleanly

The tests reference `HTTPClient` and `NewHTTPClient` which do not exist in any `.go` file in the package. This is the correct RED state for TDD. The tests cannot pass without a real implementation, which confirms they are meaningful.

---

## Command Output

### go build ./internal/graphql/...

```
EXIT_CODE: 0
```

The non-test package source (`types.go` only) compiles cleanly. The failure surfaces only when the test binary is compiled, because the test file is what references the missing symbols.

### go test -v ./internal/graphql/...

```
# github.com/jamesprial/unraid-mcp/internal/graphql [github.com/jamesprial/unraid-mcp/internal/graphql.test]
internal/graphql/client_test.go:22:18: undefined: HTTPClient
internal/graphql/client_test.go:115:19: undefined: NewHTTPClient
internal/graphql/client_test.go:153:17: undefined: NewHTTPClient
internal/graphql/client_test.go:202:17: undefined: NewHTTPClient
internal/graphql/client_test.go:250:17: undefined: NewHTTPClient
internal/graphql/client_test.go:294:17: undefined: NewHTTPClient
internal/graphql/client_test.go:332:17: undefined: NewHTTPClient
internal/graphql/client_test.go:362:17: undefined: NewHTTPClient
internal/graphql/client_test.go:384:17: undefined: NewHTTPClient
internal/graphql/client_test.go:411:17: undefined: NewHTTPClient
internal/graphql/client_test.go:411:17: too many errors
FAIL	github.com/jamesprial/unraid-mcp/internal/graphql [build failed]
FAIL
EXIT_CODE: 1
```

---

## Failing Symbols (What the Tests Expect)

| Symbol | Kind | Location | What it expects |
|--------|------|----------|-----------------|
| `HTTPClient` | struct | `client_test.go:22` | Satisfies `Client` interface (`var _ Client = (*HTTPClient)(nil)`) |
| `NewHTTPClient` | func | `client_test.go:115` (and 9+ other lines) | `func NewHTTPClient(cfg config.GraphQLConfig) (*HTTPClient, error)` |

---

## Tests That Will Fail (Once Build is Fixed)

The following test functions exist in `client_test.go` and will remain failing until implementation is complete:

### Constructor Tests
- `Test_NewHTTPClient_Cases` — 6 subtests covering: valid config, URL without `/graphql` suffix, empty URL returns error, zero timeout uses default, negative timeout uses default, empty API key succeeds at construction time

### Happy Path Tests
- `Test_Execute_HappyPath` — `Execute` returns data bytes containing expected fields from a 200 response
- `Test_Execute_QueryWithVariables` — request body contains `query` and `variables` fields correctly
- `Test_Execute_NilVariables` — nil variables omitted or null in request body
- `Test_Execute_ConcurrentRequests` — 10 concurrent goroutines all succeed, server receives 10 requests
- `Test_Execute_RequestMethod` — HTTP method is POST

### Header Tests
- `Test_Execute_APIKeyHeader` — `x-api-key` header is set to the configured key; `Content-Type` is `application/json`
- `Test_Execute_EmptyAPIKey_ReturnsError` — returns error containing `"api key is not configured"` without contacting server

### HTTP Error Tests
- `Test_Execute_HTTP401` — returns error containing `"authentication failed"`
- `Test_Execute_HTTP500` — returns error containing `"unexpected HTTP status 500"`
- `Test_Execute_HTTPStatusCodes` — table-driven: 200 OK, 401, 403, 500, 502, 503; each maps to correct error text

### GraphQL Error Tests
- `Test_Execute_GraphQLSingleError` — error contains `"field not found"` from `errors[0].message`
- `Test_Execute_GraphQLMultipleErrors` — error contains both `"first error"` and `"second error"` joined by `"; "`

### Context Tests
- `Test_Execute_ContextCancelled` — error references `"canceled"` when context cancelled before send
- `Test_Execute_ContextDeadlineExceeded` — error references `"deadline"`, `"timeout"`, or `"canceled"` when deadline expired

### Malformed Response Tests
- `Test_Execute_MalformedJSONResponse` — error contains `"decode response"` for non-JSON body

### Connection Tests
- `Test_Execute_ConnectionRefused` — error contains `"request failed"` for closed port

### Type Tests (Compile to Pass)
- `Test_GraphQLError_JSONUnmarshal` — `GraphQLError.Message` unmarshals correctly (3 subtests)
- `Test_GraphQLError_JSONMarshal` — `GraphQLError` round-trips through JSON
- `Test_GraphQLError_ZeroValue` — zero value has empty `Message`
- `Test_Client_InterfaceHasExecuteMethod` — nil `Client` interface value is nil

### Benchmarks
- `Benchmark_Execute_HappyPath`
- `Benchmark_Execute_WithVariables`
- `Benchmark_NewHTTPClient`

---

## What Must Be Implemented

A file `internal/graphql/client.go` must define:

```go
// HTTPClient is an HTTP-based GraphQL client.
type HTTPClient struct {
    // cfg    config.GraphQLConfig
    // httpCl *http.Client
}

// NewHTTPClient constructs an HTTPClient from the given config.
// Returns an error if cfg.URL is empty.
// Zero or negative Timeout is replaced with a default (e.g., 30s).
func NewHTTPClient(cfg config.GraphQLConfig) (*HTTPClient, error) { ... }

// Execute sends a GraphQL query and returns the raw `data` field bytes.
// Errors:
//   - empty APIKey → "api key is not configured"
//   - connection failure → "request failed: ..."
//   - HTTP 401 → "authentication failed"
//   - HTTP non-200 → "unexpected HTTP status NNN"
//   - JSON decode failure → "decode response: ..."
//   - GraphQL errors → messages joined by "; "
//   - context cancel/deadline → propagated from http.Client
func (c *HTTPClient) Execute(ctx context.Context, query string, variables map[string]any) ([]byte, error) { ... }
```

---

## Files

| File | Status |
|------|--------|
| `/Users/jamesprial/code/unraid-mcp/internal/graphql/types.go` | EXISTS — `GraphQLError`, `Client` interface defined |
| `/Users/jamesprial/code/unraid-mcp/internal/graphql/client_test.go` | EXISTS — 901 lines, all tests defined |
| `/Users/jamesprial/code/unraid-mcp/internal/graphql/client.go` | MISSING — must be created in GREEN phase |
