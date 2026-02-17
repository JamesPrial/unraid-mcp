# Unraid MCP Server — Performance Review

**Date:** 2026-02-17
**Files reviewed:**
- `internal/config/config.go`
- `internal/auth/middleware.go`
- `internal/safety/filter.go`
- `internal/safety/confirm.go`
- `internal/safety/audit.go`
- `internal/system/health.go`
- `internal/docker/manager.go`
- `internal/docker/tools.go`
- `internal/vm/manager.go`
- `internal/vm/tools.go`
- `cmd/server/main.go`

---

## 1. Summary

The codebase is well-structured and conservatively written. The server is
I/O-bound by design — it proxies to the Docker daemon, libvirt, and procfs —
so raw CPU throughput is not a concern. The most impactful issues are:

1. **Unbuffered audit log writes** on every tool call (synchronous, no batching).
2. **Per-call `map[string]any` heap allocation** for audit params across every
   tool handler.
3. **`json.MarshalIndent` for all responses** adds unnecessary CPU and memory
   overhead.
4. **Stale token accumulation** in `ConfirmationTracker` — expired tokens are
   never reaped, causing unbounded map growth.
5. **`GetLogs` allocates a new `[]byte` frame per log chunk** in a tight loop.
6. **`domainToVM` in `vm/manager.go` fetches XML desc and parses it for every
   domain** during `ListVMs`, compounding RPC cost for large VM counts.
7. **`filepath.Match` called in a loop** inside `Filter.IsAllowed` — no caching
   or precompilation.
8. **`readTemperatures` calls `filepath.Rel` + two `strings.TrimSuffix`** per
   sensor inside a loop.

None of these are blocking correctness issues. All recommendations below are
prioritised by expected impact relative to effort.

---

## 2. Critical Issues

### 2.1 Confirmation Token Map — Unbounded Growth

**File:** `/Users/jamesprial/code/unraid-mcp/internal/safety/confirm.go`

**Problem:** `ConfirmationTracker.tokens` grows without bound. Expired tokens
are only removed when `Confirm` is called with that exact token. If a caller
generates a confirmation token (via `RequestConfirmation`) and then abandons the
flow, the `*pendingConfirmation` stays in the map forever. Under normal usage
this is a tiny leak, but it is a structural issue.

```go
// Current: no background reap
func (ct *ConfirmationTracker) RequestConfirmation(...) string {
    token := generateToken()
    ct.mu.Lock()
    ct.tokens[token] = &pendingConfirmation{...}
    ct.mu.Unlock()
    return token
}
```

**Fix — option A (lazy inline sweep, zero goroutines):** Sweep on every write:

```go
func (ct *ConfirmationTracker) RequestConfirmation(tool, resourceName, description string) string {
    token := generateToken()
    now := time.Now()

    ct.mu.Lock()
    // O(n) sweep; n is expected to be tiny (<20 items in practice).
    for k, v := range ct.tokens {
        if now.Sub(v.createdAt) > tokenTTL {
            delete(ct.tokens, k)
        }
    }
    ct.tokens[token] = &pendingConfirmation{
        tool:         tool,
        resourceName: resourceName,
        description:  description,
        createdAt:    now,
    }
    ct.mu.Unlock()

    return token
}
```

**Fix — option B (background reaper goroutine):** Start a `time.Ticker` in a
`go` routine at construction time and stop it on a `Close()` method. More
complex but cleaner for high-volume environments.

**Expected impact:** Prevents slow, unbounded memory growth.

---

### 2.2 Audit Logger — Synchronous Unbuffered File I/O on Every Request

**File:** `/Users/jamesprial/code/unraid-mcp/internal/safety/audit.go`

**Problem:** `AuditLogger.Log` calls `json.Marshal` and then `l.w.Write`
synchronously inside the HTTP request handler goroutine. The underlying
`io.Writer` is an `*os.File` opened with `O_WRONLY|O_APPEND`. Each call results
in at least one kernel `write(2)` syscall. With many concurrent tool invocations
this becomes a serialisation point — all callers block on the kernel call.

The `AuditLogger` has no mutex, which means concurrent writes to the same
`*os.File` are relying on OS-level atomicity for small writes. Writes larger
than `PIPE_BUF` (4 096 bytes on Linux) can be interleaved.

```go
// Current: bare write, no internal synchronisation
func (l *AuditLogger) Log(entry AuditEntry) error {
    data, err := json.Marshal(entry)
    ...
    data = append(data, '\n')
    _, err = l.w.Write(data)
    return err
}
```

**Fix:** Wrap the file in a `bufio.Writer` protected by a `sync.Mutex`, and
flush on each entry (or periodically). This amortises syscalls across many
concurrent callers and prevents interleaved partial writes:

```go
type AuditLogger struct {
    mu  sync.Mutex
    bw  *bufio.Writer
}

func NewAuditLogger(w io.Writer) *AuditLogger {
    if w == nil {
        return nil
    }
    return &AuditLogger{bw: bufio.NewWriterSize(w, 64*1024)}
}

func (l *AuditLogger) Log(entry AuditEntry) error {
    if l == nil {
        return ErrNilWriter
    }
    data, err := json.Marshal(entry)
    if err != nil {
        return err
    }
    data = append(data, '\n')

    l.mu.Lock()
    _, err = l.bw.Write(data)
    if err == nil {
        err = l.bw.Flush()
    }
    l.mu.Unlock()
    return err
}
```

**Expected impact:** Reduces syscall overhead under concurrent load; eliminates
race condition on large audit entries.

---

## 3. Recommendations (Ordered by Impact)

### 3.1 Replace `json.MarshalIndent` with `json.Marshal` in Tool Responses

**Files:**
- `/Users/jamesprial/code/unraid-mcp/internal/docker/tools.go` — `dockerToolJSONResult`
- `/Users/jamesprial/code/unraid-mcp/internal/vm/tools.go` — `vmJSONResult`

**Problem:** Both helpers use `json.MarshalIndent(v, "", "  ")`. Indented JSON
has two costs: it produces larger output (more bytes transferred to the MCP
client) and it is significantly slower than compact marshalling because it
writes many more small fragments.

```go
// Current — allocates extra whitespace bytes and is ~20-30% slower
func dockerToolJSONResult(v any) *mcp.CallToolResult {
    data, err := json.MarshalIndent(v, "", "  ")
    ...
}
```

**Fix:** Use `json.Marshal` for compact output. If the MCP client genuinely
needs pretty-printed JSON (for human-readable display), this decision belongs
at the transport layer, not in the tool handler:

```go
func dockerToolJSONResult(v any) *mcp.CallToolResult {
    data, err := json.Marshal(v)
    if err != nil {
        return mcp.NewToolResultText(fmt.Sprintf("error marshaling result: %v", err))
    }
    return mcp.NewToolResultText(string(data))
}
```

**Expected impact:** ~20–30% reduction in JSON serialisation time and GC
pressure for list/inspect operations. Smaller response payloads reduce network
transfer to the client.

---

### 3.2 Eliminate Per-Call `map[string]any` Allocation for Audit Params

**Files:**
- `/Users/jamesprial/code/unraid-mcp/internal/docker/tools.go` — every handler
- `/Users/jamesprial/code/unraid-mcp/internal/vm/tools.go` — every handler

**Problem:** Every tool handler constructs a fresh `map[string]any{...}` for
the audit params on every invocation. This is a heap allocation on every
request, even when audit logging is disabled.

```go
// Current — allocates map unconditionally
handler := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
    start := time.Now()
    id := req.GetString("id", "")
    params := map[string]any{"id": id}   // heap alloc every call
    ...
}
```

**Fix — approach A (guard behind audit nil check):** Only build the map when
the audit logger is non-nil. This is a one-line change per handler:

```go
var params map[string]any
if audit != nil {
    params = map[string]any{"id": id}
}
```

**Fix — approach B (accept variadic pairs, build lazily in logger):** Change
`dockerToolLogAudit` to accept `...any` key-value pairs and build the map
internally only when needed. Avoids callers needing the nil guard:

```go
func dockerToolLogAudit(audit *safety.AuditLogger, toolName string, result string, start time.Time, kvs ...any) {
    if audit == nil {
        return
    }
    params := make(map[string]any, len(kvs)/2)
    for i := 0; i+1 < len(kvs); i += 2 {
        if k, ok := kvs[i].(string); ok {
            params[k] = kvs[i+1]
        }
    }
    _ = audit.Log(safety.AuditEntry{...})
}
```

**Expected impact:** Eliminates one heap allocation per tool call when audit
logging is disabled (the common case in development).

---

### 3.3 `GetLogs` — Per-Frame Heap Allocation in Tight Loop

**File:** `/Users/jamesprial/code/unraid-mcp/internal/docker/manager.go` — `GetLogs`

**Problem:** For each Docker log frame, a new `[]byte` of exactly `size` bytes
is allocated:

```go
frame := make([]byte, size)   // new allocation per frame
if _, err := io.ReadFull(resp.Body, frame); err != nil { ... }
out.Write(frame)
```

Log responses can contain thousands of frames. All these allocations are handed
to the garbage collector.

**Fix:** Use a reusable `bytes.Buffer` as the read staging area and grow it
only when needed:

```go
var out strings.Builder
header := make([]byte, 8)
buf := make([]byte, 32*1024) // reused across frames

for {
    _, err := io.ReadFull(resp.Body, header)
    if err == io.EOF || err == io.ErrUnexpectedEOF {
        break
    }
    if err != nil {
        return "", fmt.Errorf("docker: read log frame header: %w", err)
    }
    size := int(header[4])<<24 | int(header[5])<<16 | int(header[6])<<8 | int(header[7])
    if size == 0 {
        continue
    }
    if size > len(buf) {
        buf = make([]byte, size) // rare: only for unusually large frames
    }
    frame := buf[:size]
    if _, err := io.ReadFull(resp.Body, frame); err != nil {
        return "", fmt.Errorf("docker: read log frame: %w", err)
    }
    out.Write(frame)
}
```

**Expected impact:** Eliminates O(frames) small heap allocations per log fetch,
reducing GC pause frequency for log-heavy workloads.

---

### 3.4 `ListVMs` — XML RPC Per Domain During List

**File:** `/Users/jamesprial/code/unraid-mcp/internal/vm/manager.go` — `ListVMs` → `domainToVM`

**Problem:** `ListVMs` calls `domainToVM` for each domain. `domainToVM`
triggers two libvirt RPCs: `DomainGetState` and `DomainGetXMLDesc`. With N
VMs this is 2N sequential RPCs over the Unix socket. The XML is also
unmarshalled fully just to extract `memory` and `vcpu` — fields that are also
available from `DomainGetInfo` (a single, cheaper RPC).

```go
func (m *LibvirtVMManager) domainToVM(dom libvirt.Domain) (VM, error) {
    state, err := m.domainState(dom)     // RPC 1: DomainGetState
    xmlDesc, err := m.l.DomainGetXMLDesc(dom, 0)  // RPC 2: DomainGetXMLDesc
    var d domainXML
    xml.Unmarshal([]byte(xmlDesc), &d)   // full XML parse
    ...
}
```

**Fix:** Use `DomainGetInfo` which returns state, memory, and vCPU count in a
single RPC, eliminating the XML round-trip entirely for the list path:

```go
// DomainGetInfo returns: state, maxMem, memory, nrVirtCPU, cpuTime
info, err := m.l.DomainGetInfo(dom)
if err != nil {
    return VM{}, fmt.Errorf("get domain info: %w", err)
}
return VM{
    Name:   dom.Name,
    UUID:   formatUUID(dom.UUID),
    State:  libvirtStateToVMState(libvirt.DomainState(info.State)),
    Memory: info.Memory, // already in KiB
    VCPUs:  int(info.NrVirtCPU),
}, nil
```

Reserve `DomainGetXMLDesc` for `InspectVM` and `domainToVMDetail` where the
full configuration is genuinely needed.

**Expected impact:** Reduces `ListVMs` from 2N to N libvirt RPCs. For 10 VMs
this halves the RPC count and eliminates 10 XML parse operations.

---

### 3.5 `Filter.IsAllowed` — `filepath.Match` Called on Every Check

**File:** `/Users/jamesprial/code/unraid-mcp/internal/safety/filter.go`

**Problem:** `filepath.Match` is called in a linear scan of the allowlist and
denylist on every `IsAllowed` call. For simple literal patterns (no glob
characters) this is wasteful — a plain map lookup would be O(1).

```go
for _, pattern := range f.denylist {
    if matchGlob(pattern, name) {   // filepath.Match every time
        return false
    }
}
```

**Fix — fast path for exact-match patterns:** At construction time, separate
patterns into "literals" (no glob metacharacters) and "globs". Literals go into
a `map[string]struct{}` for O(1) lookup; globs remain in a slice:

```go
type Filter struct {
    allowLiterals map[string]struct{}
    allowGlobs    []string
    denyLiterals  map[string]struct{}
    denyGlobs     []string
}

func hasGlobMeta(s string) bool {
    return strings.ContainsAny(s, "*?[")
}

func NewFilter(allowlist, denylist []string) *Filter {
    f := &Filter{
        allowLiterals: make(map[string]struct{}),
        denyLiterals:  make(map[string]struct{}),
    }
    for _, p := range allowlist {
        if hasGlobMeta(p) {
            f.allowGlobs = append(f.allowGlobs, p)
        } else {
            f.allowLiterals[p] = struct{}{}
        }
    }
    for _, p := range denylist {
        if hasGlobMeta(p) {
            f.denyGlobs = append(f.denyGlobs, p)
        } else {
            f.denyLiterals[p] = struct{}{}
        }
    }
    return f
}

func (f *Filter) IsAllowed(name string) bool {
    // Denylist literals — O(1)
    if _, ok := f.denyLiterals[name]; ok {
        return false
    }
    // Denylist globs
    for _, pattern := range f.denyGlobs {
        if matchGlob(pattern, name) {
            return false
        }
    }
    // Allowlist empty → permit everything not denied
    if len(f.allowLiterals) == 0 && len(f.allowGlobs) == 0 {
        return true
    }
    // Allowlist literals — O(1)
    if _, ok := f.allowLiterals[name]; ok {
        return true
    }
    // Allowlist globs
    for _, pattern := range f.allowGlobs {
        if matchGlob(pattern, name) {
            return true
        }
    }
    return false
}
```

**Expected impact:** For the common case where filter lists contain plain
container/VM names (no wildcards), every `IsAllowed` call drops from O(n)
linear scan to O(1) map lookup. Relevant on every inbound tool call.

---

### 3.6 `doRequest` — URL Built by String Concatenation on Every Call

**File:** `/Users/jamesprial/code/unraid-mcp/internal/docker/manager.go` — `doRequest`

**Problem:** `doRequest` concatenates `m.baseURL + path` on every HTTP call.
While small, this allocates a new string on every Docker operation.

```go
func (m *DockerClientManager) doRequest(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
    url := m.baseURL + path   // allocation every call
    ...
}
```

The callers also build paths via `fmt.Sprintf` (e.g., `fmt.Sprintf("/containers/%s/stop?t=%d", id, timeout)`),
which is additional allocation.

**Fix:** This is low-priority given the I/O dominates, but callers can use
`strings.Builder` or pre-built path strings for simple cases. The `doRequest`
concatenation is unavoidable without a more invasive change, so leave as-is
and focus on the `fmt.Sprintf` callers in hot paths if benchmarks show
allocation pressure here.

---

### 3.7 `readTemperatures` — `filepath.Rel` + String Operations Per Sensor

**File:** `/Users/jamesprial/code/unraid-mcp/internal/system/health.go` — `readTemperatures`

**Problem:** For each temperature sensor file, `readTemperatures` calls
`filepath.Rel`, `strings.TrimSuffix`, and allocates a label string. This is
called on every `GetOverview` request and involves multiple allocations per
sensor.

```go
for _, path := range matches {
    data, err := os.ReadFile(path)     // syscall per sensor
    ...
    rel, err := filepath.Rel(m.sysPath, path)   // allocation
    label := strings.TrimSuffix(rel, "_input")  // allocation
    temps = append(temps, Temperature{Label: label, ...})
}
```

**Fix:** Pre-compute the label during the glob phase, before reading files.
More importantly, the list of sensor paths rarely changes — the `hwmon`
topology is stable at runtime. Cache the glob results and recompute only if the
number of matches changes:

```go
type FileSystemMonitor struct {
    procPath      string
    sysPath       string
    emhttpPath    string
    // cached temperature sensor paths (rebuilt if stale)
    tempPaths     []string
    tempLabels    []string
    tempPathOnce  sync.Once
}
```

For a server that may poll health every few seconds, this eliminates repeated
`filepath.Glob` syscalls.

**Expected impact:** Reduces per-`GetOverview` syscall count when there are
many hwmon sensors. Low priority unless health is polled aggressively.

---

### 3.8 `parseMemInfo` — Redundant `strings.TrimSpace` Calls

**File:** `/Users/jamesprial/code/unraid-mcp/internal/system/health.go` — `parseMemInfo`

**Problem:** The inner loop calls `strings.TrimSpace` twice on the value side
and `strings.TrimSuffix` once:

```go
valStr := strings.TrimSpace(parts[1])
valStr = strings.TrimSuffix(valStr, " kB")
valStr = strings.TrimSpace(valStr)   // redundant: TrimSuffix leaves no trailing space
```

The third `TrimSpace` is a no-op because `TrimSuffix(" kB")` matches the
trailing space as part of the suffix, leaving no trailing whitespace when the
suffix is present, and leaving the string unchanged otherwise.

**Fix:** Remove the second `strings.TrimSpace`:

```go
valStr := strings.TrimSpace(parts[1])
valStr = strings.TrimSuffix(valStr, " kB")
// No second TrimSpace needed.
val, err := strconv.ParseUint(valStr, 10, 64)
```

**Expected impact:** Minor — removes one string allocation per meminfo line
(~25–30 lines per parse). Clean-up value.

---

### 3.9 `ListContainers` / `ListNetworks` — Double Body Close Risk

**File:** `/Users/jamesprial/code/unraid-mcp/internal/docker/manager.go`

**Problem:** Several methods call `defer resp.Body.Close()` after `doRequest`
and then also pass `resp` to `checkError`, which internally calls `readBody`,
which also calls `resp.Body.Close()`:

```go
// ListContainers
resp, err := m.doRequest(...)
defer resp.Body.Close()                        // close #1

if err := checkError(resp, "container not found"); err != nil { ... }  // checkError → readBody → close #2
```

`checkError` calls `readBody` only on non-2xx responses, so in the happy path
the double-close does not occur. But on error paths, `readBody` closes the body,
and then the `defer` fires a second close on an already-closed reader. Most HTTP
response body implementations handle double-close gracefully, but it is
semantically incorrect.

**Fix:** Use a consistent pattern: either always use `defer resp.Body.Close()`
and have `checkError` not close (passing the body bytes separately), or always
consume-and-close in `checkError` and omit the `defer`. The `checkErrorFromBody`
pattern already used in `StartContainer`, `StopContainer`, etc. is the correct
approach — read the body first, then check:

```go
resp, err := m.doRequest(ctx, http.MethodGet, "/containers/json?all="+allParam, nil)
if err != nil {
    return nil, fmt.Errorf("docker: list containers: %w", err)
}
body, err := readBody(resp) // closes body
if err != nil {
    return nil, err
}
if err := checkErrorFromBody(resp.StatusCode, body, "container not found"); err != nil {
    return nil, fmt.Errorf("docker: list containers: %w", err)
}

var raw []dockerContainer
if err := json.Unmarshal(body, &raw); err != nil { ... }
```

Apply this pattern consistently to `ListContainers`, `InspectContainer`,
`ListNetworks`, `InspectNetwork`, `GetStats`, and `GetLogs`.

**Expected impact:** Correctness fix; no measurable performance change but
eliminates subtle resource-management bugs.

---

### 3.10 `http.Server` — Missing Timeouts

**File:** `/Users/jamesprial/code/unraid-mcp/cmd/server/main.go`

**Problem:** The `http.Server` is constructed with only `Addr` and `Handler`.
There are no read, write, or idle timeouts. A slow or misbehaving MCP client
can hold connections open indefinitely, leaking goroutines and file descriptors.

```go
httpSrv := &http.Server{
    Addr:    addr,
    Handler: wrappedHandler,
    // No ReadTimeout, WriteTimeout, IdleTimeout
}
```

**Fix:** Add sensible timeouts. For an MCP server with streaming responses,
`WriteTimeout` must be generous or set to zero (streaming disables it), but
`ReadHeaderTimeout` and `IdleTimeout` should always be set:

```go
httpSrv := &http.Server{
    Addr:              addr,
    Handler:           wrappedHandler,
    ReadHeaderTimeout: 10 * time.Second,
    IdleTimeout:       120 * time.Second,
    // WriteTimeout deliberately omitted for streaming support,
    // or set generously: WriteTimeout: 5 * time.Minute,
}
```

**Expected impact:** Prevents goroutine leak from abandoned connections.

---

### 3.11 `generateToken` (confirm.go) — Unnecessary Allocation on Every Confirmation

**File:** `/Users/jamesprial/code/unraid-mcp/internal/safety/confirm.go`

**Problem:** `generateToken` allocates a `[]byte` on the heap, encodes it, and
returns a `string`. The `hex.EncodeToString` call creates a second allocation
for the output string.

```go
func generateToken() string {
    b := make([]byte, 16)
    if _, err := rand.Read(b); err != nil { ... }
    return hex.EncodeToString(b)  // two allocations: b + hex string
}
```

**Fix:** Use a fixed-size stack array for the raw bytes:

```go
func generateToken() string {
    var b [16]byte
    if _, err := rand.Read(b[:]); err != nil {
        return hex.EncodeToString([]byte(time.Now().String()))
    }
    return hex.EncodeToString(b[:])  // only one allocation: the hex string
}
```

**Expected impact:** Eliminates one 16-byte heap allocation per confirmation
request. Minor, but trivial to apply.

---

### 3.12 `Snapshot.CreatedAt` — Always Set to `time.Now()`

**File:** `/Users/jamesprial/code/unraid-mcp/internal/vm/manager.go` — `ListSnapshots`

**Problem:** The `CreatedAt` field in each `Snapshot` is set to `time.Now()`
rather than the actual snapshot creation time from libvirt:

```go
out = append(out, Snapshot{
    Name:      n,
    CreatedAt: time.Now(),  // incorrect — not the real snapshot timestamp
})
```

This is a correctness bug, not a performance issue. The libvirt API exposes
snapshot creation time via `DomainSnapshotGetXMLDesc` — fetching it would
require one additional RPC per snapshot, which is a real cost. Document the
limitation explicitly rather than returning a misleading timestamp.

**Fix (minimum):** Change `CreatedAt` to `time.Time{}` (zero value) and
document that the timestamp is not populated, or remove the field from the
`Snapshot` type for the list operation.

---

## 4. Concurrency Analysis

### Goroutine Lifecycle
- The server starts exactly one goroutine (the `httpSrv.ListenAndServe` loop).
  All request handling is done by the Go `net/http` pool. No leaks detected.
- `ConfirmationTracker` uses a `sync.Mutex` correctly. No deadlock risk.
- `AuditLogger` has no internal synchronisation (see 2.2 above).

### Race Conditions
- `AuditLogger.Log` performs no synchronisation when writing to the underlying
  `io.Writer`. If two goroutines call `Log` concurrently (the normal case under
  load), writes to `*os.File` rely on OS atomicity guarantees that do not apply
  for writes larger than `PIPE_BUF`. **This is a data race at the application
  level** even if the Go race detector does not flag it (the write itself is
  atomic at the runtime level through the `w.Write` interface call, but two
  sequential writes — `data` bytes then `'\n'` — are not atomic as a unit). The
  `append(data, '\n')` merges them into one write, so in practice atomicity is
  maintained for entries up to ~4 KB. Entries above that size can be
  interleaved.

### Context Propagation
- All Docker and libvirt operations accept and check `ctx`. Context cancellation
  is propagated correctly to the HTTP client and libvirt calls.
- `GetOverview`, `GetArrayStatus`, and `GetDiskInfo` accept a `ctx` parameter
  but never pass it to the underlying file operations. This is acceptable for
  procfs/sysfs reads (which complete near-instantly), but worth noting.

---

## 5. I/O Patterns

| Path | Pattern | Notes |
|------|---------|-------|
| Docker API | Long-lived HTTP/Unix socket, pooled | Good. Single `http.Client` with transport reuse. |
| Libvirt | Persistent TCP-over-Unix connection | Good. Connection established once at startup. |
| Procfs/Sysfs (`/proc/stat`, `/proc/meminfo`) | Open → scan → close per call | Acceptable. Files are tiny. |
| Temperature sensors (`/sys/hwmon/*/temp*_input`) | `os.ReadFile` per sensor per call | Repeated `filepath.Glob` on every `GetOverview`. Cache recommended (see 3.7). |
| Disk/Array INI files | Open → scan → close per call | Acceptable. Files are small. |
| Audit log | Synchronous `write(2)` per entry | Should be buffered (see 2.2). |

---

## 6. Memory Allocation Patterns

| Location | Allocation | Frequency | Recommendation |
|----------|-----------|-----------|----------------|
| `docker/tools.go` — every handler | `map[string]any` for params | Every tool call | Guard behind `audit != nil` check (3.2) |
| `docker/tools.go` — `dockerToolJSONResult` | Indented JSON bytes + string | Every read/inspect | Use `json.Marshal` (3.1) |
| `docker/manager.go` — `GetLogs` | `[]byte` per log frame | Per frame in log stream | Reuse buffer (3.3) |
| `safety/confirm.go` — `generateToken` | 16-byte slice + hex string | Per confirmation | Use stack array (3.11) |
| `vm/manager.go` — `domainToVM` | XML string from libvirt + parse | Per VM per list | Use `DomainGetInfo` instead (3.4) |
| `safety/confirm.go` — `tokens` map | `*pendingConfirmation` per token | Leaked on abandoned flows | Add reaper (2.1) |

---

## 7. Easy Wins Summary

These changes are low-risk and can be made in under an hour:

1. **Remove second `strings.TrimSpace` in `parseMemInfo`** (3.8) — one line.
2. **Stack array in `generateToken`** (3.11) — two lines.
3. **`json.Marshal` instead of `json.MarshalIndent`** in both tool packages (3.1) — two lines.
4. **Add `ReadHeaderTimeout` and `IdleTimeout` to `http.Server`** (3.10) — two lines.
5. **Lazy sweep in `RequestConfirmation`** for expired tokens (2.1 option A) — ~10 lines.
6. **Guard `params` map allocation behind `audit != nil`** in tool handlers (3.2) — one guard per handler.

---

## 8. Next Steps

- Run `go test -race ./...` to confirm no data races surface under concurrent
  test execution (audit logger write pattern is the primary concern).
- Add a benchmark for `Filter.IsAllowed` with realistic pattern counts to
  quantify the impact of the literal-vs-glob split (3.5).
- Profile a production log fetch (`docker_logs` with large tail) with
  `go test -bench=BenchmarkGetLogs -memprofile=mem.prof` to validate the
  frame-buffer recommendation (3.3).
- Consider adding `MaxSizeMB` log rotation for the audit file — the config
  struct has the field but `main.go` does not implement rotation. A growing
  unbounded log file is an operational issue on embedded Unraid hardware.
