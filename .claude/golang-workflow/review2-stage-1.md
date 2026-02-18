# Design Review: Stage 1 -- GraphQL HTTP Client

**Files reviewed:**
- `/Users/jamesprial/code/unraid-mcp/internal/graphql/types.go`
- `/Users/jamesprial/code/unraid-mcp/internal/graphql/client.go`
- `/Users/jamesprial/code/unraid-mcp/internal/graphql/client_test.go`

**Reference files:**
- `/Users/jamesprial/code/unraid-mcp/internal/docker/types.go`
- `/Users/jamesprial/code/unraid-mcp/internal/docker/manager.go`
- `/Users/jamesprial/code/unraid-mcp/internal/system/types.go`
- `/Users/jamesprial/code/unraid-mcp/internal/system/health.go`
- `/Users/jamesprial/code/unraid-mcp/internal/config/config.go`

---

## Verdict: APPROVE

The GraphQL HTTP client is well-designed, consistent with existing codebase patterns, and ready to merge. The issues identified below are minor and do not block merging; they are suggestions for consideration in future iterations.

---

## Detailed Findings

### Package Organization

The split between `types.go` and `client.go` is clean and follows the established codebase convention. The `docker` package uses the same pattern: `types.go` holds the interface and domain types while `manager.go` holds the concrete implementation. The `graphql` package mirrors this exactly with `Client` interface + `GraphQLError` in `types.go` and `HTTPClient` + `NewHTTPClient` + `Execute` in `client.go`. This is logical and consistent.

### Interface Design

The `Client` interface is appropriately minimal with a single method:

```go
type Client interface {
    Execute(ctx context.Context, query string, variables map[string]any) ([]byte, error)
}
```

This follows Go's preference for small interfaces. It accepts `context.Context` as the first parameter, follows the `(result, error)` return convention, and uses `[]byte` for the raw JSON data field -- which is the right abstraction level for a transport-layer client, leaving deserialization to callers. The `map[string]any` for variables is the idiomatic choice for GraphQL variable maps.

The compile-time interface satisfaction check at line 22 of the test file (`var _ Client = (*HTTPClient)(nil)`) follows the same pattern used in the docker package at line 865 of `manager.go`.

### Exported API Surface

The exported surface is clean and minimal:
- `Client` (interface) -- the abstraction consumers depend on
- `HTTPClient` (struct) -- the concrete implementation
- `NewHTTPClient` (constructor) -- the only way to create an `HTTPClient`
- `GraphQLError` (struct) -- needed for callers who want to inspect error structure

Internal types (`graphqlRequest`, `graphqlResponse`) and the helper function (`normalizeURL`) are correctly unexported. No unnecessary exports were found.

### Naming Conventions

Names follow Go standards:
- `HTTPClient` correctly capitalizes the HTTP acronym (not `HttpClient`)
- `NewHTTPClient` follows the `New<Type>` constructor convention
- `graphqlURL` field uses lowercase `graphql` prefix with uppercase `URL` acronym
- `apiKey` is properly lowercased as an unexported field
- The `x-api-key` header name matches the Unraid API convention

### Constructor Pattern Consistency

`NewHTTPClient(cfg config.GraphQLConfig) (*HTTPClient, error)` is consistent with the codebase. Comparing with existing constructors:

| Constructor | Signature |
|---|---|
| `NewDockerClientManager` | `(socketPath string) (*DockerClientManager, error)` |
| `NewLibvirtVMManager` | `(socketPath string) (*LibvirtVMManager, error)` |
| `NewFileSystemMonitor` | `(procPath, sysPath, emhttpPath string) *FileSystemMonitor` |
| **`NewHTTPClient`** | `(cfg config.GraphQLConfig) (*HTTPClient, error)` |

The GraphQL client takes a config struct rather than individual parameters, which is appropriate here since it has three configuration fields (`URL`, `APIKey`, `Timeout`). The docker and VM constructors take a single string each so raw parameters are fine. The config struct approach avoids a long parameter list and leverages the existing `config.GraphQLConfig` type, which is good.

The constructor correctly returns `(*HTTPClient, error)` since it validates input, matching `NewDockerClientManager`.

### Error Message Format

Error messages consistently use the `graphql:` prefix, which matches the `docker:` prefix pattern used throughout `manager.go`. The format is `"graphql: <action>: %w"` for wrapped errors and `"graphql: <descriptive message>"` for sentinel-style errors. This is consistent.

One observation: lines 111 and 114 produce errors without wrapping (`authentication failed (HTTP 401)` and `unexpected HTTP status %d`). These do not use `%w` because there is no underlying error to wrap -- which is correct. The same pattern appears in the docker package (e.g., `fmt.Errorf("container not found: %s", id)`).

### Documentation Completeness

All exported items have godoc comments:
- `GraphQLError` struct (line 7, types.go)
- `Client` interface (line 12, types.go)
- `HTTPClient` struct (line 19, client.go)
- `NewHTTPClient` function (line 27, client.go) -- includes behavior notes about timeout defaults and empty API key acceptance
- `Execute` method (line 74, client.go) -- includes a comprehensive list of error conditions

The package-level comment appears in both files (consistent) and accurately describes the package purpose.

### URL Normalization

The `normalizeURL` function (lines 52-58, client.go):

```go
func normalizeURL(rawURL string) string {
    u := strings.TrimRight(rawURL, "/")
    if !strings.HasSuffix(u, "/graphql") {
        u += "/graphql"
    }
    return u
}
```

This is a reasonable convenience for users who might configure `http://tower.local` vs `http://tower.local/graphql`. It matches the default URL in `config.go` line 100 (`http://localhost/graphql`).

**Minor concern:** `strings.TrimRight(rawURL, "/")` trims all trailing `/` characters individually (it is character-set based, not suffix based). For a URL like `http://tower.local///`, this would produce `http://tower.local` which is the desired result. However, a URL like `http://tower.local/api/` would also trim just the trailing slash, which is fine. No practical issue here.

**Minor concern:** There are no tests specifically for `normalizeURL`. While the function is exercised indirectly through `NewHTTPClient` tests (the URL suffix cases on lines 66-71), explicit unit tests for edge cases (URL already ending with `/graphql`, URL ending with `/graphql/`, URL with query parameters) would improve confidence. This is a non-blocking suggestion.

### Nil Safety

- `Execute` checks `c.apiKey` at line 83 before using it -- good guard.
- `NewHTTPClient` checks `cfg.URL` at line 32 before proceeding -- good guard.
- `resp.Body.Close()` is deferred at line 108 -- proper resource cleanup.
- The `gqlResp.Errors` slice is checked for length before iteration (line 122) -- correct nil/empty safety.
- `variables` being nil is handled by the `omitempty` JSON tag on `graphqlRequest.Variables` (line 63) -- clean approach.

### Test Coverage Assessment

The test file is thorough at 901 lines covering:
- Constructor validation (table-driven, 6 cases)
- Happy path execution
- Request body verification (with and without variables)
- API key header verification
- Empty API key error path
- HTTP error status codes (401, 500, plus a 6-case table-driven test for 200/401/403/500/502/503)
- GraphQL error responses (single and multiple)
- Context cancellation and deadline exceeded
- Malformed JSON response
- Connection refused
- Concurrent requests (10 goroutines)
- Request method verification
- `GraphQLError` JSON marshal/unmarshal
- Benchmarks (3)

Test naming follows the `Test_<Function>_<Case>` convention used in the docker package (e.g., `Test_ListContainers_Cases`, `Test_StartContainer_ChangesState`). This is consistent.

**One observation on test quality:** `Test_Client_InterfaceHasExecuteMethod` (lines 820-828) provides minimal value beyond the compile-time check already present at line 22. It only verifies a nil interface is nil, which is always true. Not harmful, but not adding signal either.

### Minor Suggestions (Non-Blocking)

1. **Response body size limit:** `Execute` uses `json.NewDecoder(resp.Body).Decode()` which reads the entire response body into memory. For a GraphQL client hitting a trusted internal API this is fine, but a future enhancement could add a size-limited reader (`io.LimitReader`) as a defense against unexpectedly large responses.

2. **`GraphQLError` could implement `error` interface:** Adding an `Error() string` method to `GraphQLError` would make it usable as a Go error directly, which could be useful for callers that want to inspect individual errors. Not needed for the current design but worth considering.

3. **Timeout type:** `config.GraphQLConfig.Timeout` is `int` (seconds), and `NewHTTPClient` converts it via `time.Duration(cfg.Timeout) * time.Second`. This works correctly. The conversion is well-documented in the constructor's godoc.

---

## Checklist

- [x] All exported items have documentation
- [x] Error handling follows `"package: action: %w"` pattern
- [x] Nil safety guards present (URL check, API key check, body close defer, slice length check)
- [x] Tests are well-structured with table-driven cases where appropriate
- [x] Code is readable and well-organized
- [x] Naming conventions follow Go standards (HTTPClient, not HttpClient)
- [x] No obvious logic errors or edge case gaps
- [x] Package organization matches existing codebase patterns
- [x] Constructor pattern is consistent with codebase
- [x] Interface is minimal and follows Go idioms
- [x] Exported API surface is clean with no unnecessary exports
