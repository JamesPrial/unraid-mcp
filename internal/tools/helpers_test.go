package tools_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/jamesprial/unraid-mcp/internal/safety"
	"github.com/jamesprial/unraid-mcp/internal/tools"
	"github.com/mark3labs/mcp-go/mcp"
)

// ---------------------------------------------------------------------------
// Test helper: extract text from a *mcp.CallToolResult
// ---------------------------------------------------------------------------

// resultText extracts the text string from the first Content element of a
// CallToolResult. It fails the test if the result is nil, has no content, or
// the first element is not a TextContent.
func resultText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	if result == nil {
		t.Fatal("CallToolResult is nil")
	}
	if len(result.Content) == 0 {
		t.Fatal("CallToolResult.Content is empty")
	}
	tc, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("Content[0] is %T, want mcp.TextContent", result.Content[0])
	}
	return tc.Text
}

// ---------------------------------------------------------------------------
// Tests for JSONResult
// ---------------------------------------------------------------------------

func Test_JSONResult_Cases(t *testing.T) {
	type simpleStruct struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	}

	tests := []struct {
		name     string
		input    any
		validate func(t *testing.T, text string)
	}{
		{
			name:  "simple struct produces valid indented JSON",
			input: simpleStruct{Name: "test", Count: 42},
			validate: func(t *testing.T, text string) {
				t.Helper()

				// Must be valid JSON.
				var parsed map[string]any
				if err := json.Unmarshal([]byte(text), &parsed); err != nil {
					t.Fatalf("result is not valid JSON: %v\ntext: %s", err, text)
				}

				// Verify fields.
				if parsed["name"] != "test" {
					t.Errorf("name = %v, want %q", parsed["name"], "test")
				}
				// json.Unmarshal decodes numbers as float64.
				if parsed["count"] != float64(42) {
					t.Errorf("count = %v, want 42", parsed["count"])
				}

				// Verify indentation (2-space indent).
				if !strings.Contains(text, "  \"name\"") {
					t.Errorf("expected 2-space indented JSON, got:\n%s", text)
				}
			},
		},
		{
			name:  "nil input produces null",
			input: nil,
			validate: func(t *testing.T, text string) {
				t.Helper()
				if strings.TrimSpace(text) != "null" {
					t.Errorf("text = %q, want %q", text, "null")
				}
			},
		},
		{
			name:  "empty map produces empty JSON object",
			input: map[string]any{},
			validate: func(t *testing.T, text string) {
				t.Helper()
				if strings.TrimSpace(text) != "{}" {
					t.Errorf("text = %q, want %q", text, "{}")
				}
			},
		},
		{
			name:  "unmarshalable value returns error text",
			input: make(chan int),
			validate: func(t *testing.T, text string) {
				t.Helper()
				if !strings.Contains(text, "error marshaling result:") {
					t.Errorf("expected error prefix in text, got: %q", text)
				}
			},
		},
		{
			name:  "slice of strings produces JSON array",
			input: []string{"a", "b", "c"},
			validate: func(t *testing.T, text string) {
				t.Helper()
				var parsed []string
				if err := json.Unmarshal([]byte(text), &parsed); err != nil {
					t.Fatalf("result is not valid JSON array: %v", err)
				}
				if len(parsed) != 3 {
					t.Errorf("len = %d, want 3", len(parsed))
				}
			},
		},
		{
			name:  "nested struct produces indented JSON",
			input: struct{ Inner struct{ Val int } }{Inner: struct{ Val int }{Val: 7}},
			validate: func(t *testing.T, text string) {
				t.Helper()
				if !strings.Contains(text, "\n") {
					t.Errorf("expected multi-line indented JSON, got: %q", text)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tools.JSONResult(tt.input)
			text := resultText(t, result)
			tt.validate(t, text)
		})
	}
}

func Test_JSONResult_ReturnsNonNil(t *testing.T) {
	// Even on marshal error the result should never be nil.
	result := tools.JSONResult(make(chan int))
	if result == nil {
		t.Fatal("JSONResult returned nil for unmarshalable input")
	}
}

// ---------------------------------------------------------------------------
// Tests for ErrorResult
// ---------------------------------------------------------------------------

func Test_ErrorResult_Cases(t *testing.T) {
	tests := []struct {
		name    string
		msg     string
		wantTxt string
	}{
		{
			name:    "simple error message",
			msg:     "container not found",
			wantTxt: "error: container not found",
		},
		{
			name:    "empty message",
			msg:     "",
			wantTxt: "error: ",
		},
		{
			name:    "message with special characters",
			msg:     "id=\"abc\" not found: timeout after 30s",
			wantTxt: "error: id=\"abc\" not found: timeout after 30s",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tools.ErrorResult(tt.msg)
			text := resultText(t, result)
			if text != tt.wantTxt {
				t.Errorf("ErrorResult(%q) text = %q, want %q", tt.msg, text, tt.wantTxt)
			}
		})
	}
}

func Test_ErrorResult_ReturnsNonNil(t *testing.T) {
	result := tools.ErrorResult("")
	if result == nil {
		t.Fatal("ErrorResult returned nil")
	}
}

// ---------------------------------------------------------------------------
// Tests for LogAudit
// ---------------------------------------------------------------------------

func Test_LogAudit_NilLogger_NoPanic(t *testing.T) {
	// Must not panic when audit logger is nil.
	tools.LogAudit(nil, "docker_list", map[string]any{"all": true}, "ok", time.Now())
}

func Test_LogAudit_ValidLogger_Cases(t *testing.T) {
	tests := []struct {
		name     string
		toolName string
		params   map[string]any
		result   string
		validate func(t *testing.T, parsed map[string]any)
	}{
		{
			name:     "basic entry is written",
			toolName: "docker_list",
			params:   map[string]any{"all": true},
			result:   "ok",
			validate: func(t *testing.T, parsed map[string]any) {
				t.Helper()
				if parsed["tool"] != "docker_list" {
					t.Errorf("tool = %v, want %q", parsed["tool"], "docker_list")
				}
				if parsed["result"] != "ok" {
					t.Errorf("result = %v, want %q", parsed["result"], "ok")
				}
			},
		},
		{
			name:     "params are preserved",
			toolName: "docker_inspect",
			params:   map[string]any{"id": "abc", "force": true},
			result:   "ok",
			validate: func(t *testing.T, parsed map[string]any) {
				t.Helper()
				paramsRaw, ok := parsed["params"].(map[string]any)
				if !ok {
					t.Fatalf("params is %T, want map[string]any", parsed["params"])
				}
				if paramsRaw["id"] != "abc" {
					t.Errorf("params.id = %v, want %q", paramsRaw["id"], "abc")
				}
				if paramsRaw["force"] != true {
					t.Errorf("params.force = %v, want true", paramsRaw["force"])
				}
			},
		},
		{
			name:     "nil params are accepted",
			toolName: "vm_list",
			params:   nil,
			result:   "ok",
			validate: func(t *testing.T, parsed map[string]any) {
				t.Helper()
				if parsed["tool"] != "vm_list" {
					t.Errorf("tool = %v, want %q", parsed["tool"], "vm_list")
				}
			},
		},
		{
			name:     "empty tool name is accepted",
			toolName: "",
			params:   map[string]any{},
			result:   "error: something",
			validate: func(t *testing.T, parsed map[string]any) {
				t.Helper()
				if parsed["tool"] != "" {
					t.Errorf("tool = %v, want empty string", parsed["tool"])
				}
				if parsed["result"] != "error: something" {
					t.Errorf("result = %v, want %q", parsed["result"], "error: something")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			audit := safety.NewAuditLogger(&buf)
			if audit == nil {
				t.Fatal("NewAuditLogger returned nil for valid writer")
			}

			start := time.Now()
			tools.LogAudit(audit, tt.toolName, tt.params, tt.result, start)

			output := strings.TrimSpace(buf.String())
			if output == "" {
				t.Fatal("audit logger produced no output")
			}

			var parsed map[string]any
			if err := json.Unmarshal([]byte(output), &parsed); err != nil {
				t.Fatalf("audit output is not valid JSON: %v\noutput: %s", err, output)
			}

			tt.validate(t, parsed)
		})
	}
}

func Test_LogAudit_DurationPositive(t *testing.T) {
	var buf bytes.Buffer
	audit := safety.NewAuditLogger(&buf)

	// Use a start time slightly in the past to guarantee positive duration.
	start := time.Now().Add(-10 * time.Millisecond)
	tools.LogAudit(audit, "test_tool", map[string]any{}, "ok", start)

	output := strings.TrimSpace(buf.String())
	var parsed map[string]any
	if err := json.Unmarshal([]byte(output), &parsed); err != nil {
		t.Fatalf("audit output is not valid JSON: %v", err)
	}

	durationRaw, ok := parsed["duration_ns"]
	if !ok {
		t.Fatal("audit output missing duration_ns field")
	}

	// JSON numbers are decoded as float64.
	duration, ok := durationRaw.(float64)
	if !ok {
		t.Fatalf("duration_ns is %T, want float64", durationRaw)
	}

	if duration <= 0 {
		t.Errorf("duration_ns = %v, want > 0", duration)
	}
}

func Test_LogAudit_TimestampMatchesStart(t *testing.T) {
	var buf bytes.Buffer
	audit := safety.NewAuditLogger(&buf)

	start := time.Date(2026, 2, 17, 12, 0, 0, 0, time.UTC)
	tools.LogAudit(audit, "test_tool", map[string]any{}, "ok", start)

	output := strings.TrimSpace(buf.String())
	var parsed map[string]any
	if err := json.Unmarshal([]byte(output), &parsed); err != nil {
		t.Fatalf("audit output is not valid JSON: %v", err)
	}

	tsRaw, ok := parsed["timestamp"]
	if !ok {
		t.Fatal("audit output missing timestamp field")
	}

	tsStr, ok := tsRaw.(string)
	if !ok {
		t.Fatalf("timestamp is %T, want string", tsRaw)
	}

	ts, err := time.Parse(time.RFC3339Nano, tsStr)
	if err != nil {
		t.Fatalf("could not parse timestamp %q: %v", tsStr, err)
	}

	if !ts.Equal(start) {
		t.Errorf("timestamp = %v, want %v", ts, start)
	}
}

// ---------------------------------------------------------------------------
// Tests for ConfirmPrompt
// ---------------------------------------------------------------------------

func Test_ConfirmPrompt_StandardPrompt(t *testing.T) {
	confirm := safety.NewConfirmationTracker([]string{"docker_stop"})

	result := tools.ConfirmPrompt(confirm, "docker_stop", "my-container", "stop it")
	text := resultText(t, result)

	// Verify all required substrings are present.
	required := []string{
		"Confirmation required for docker_stop",
		`"my-container"`,
		"stop it",
		"confirmation_token=",
	}
	for _, substr := range required {
		if !strings.Contains(text, substr) {
			t.Errorf("result text missing %q\nfull text:\n%s", substr, text)
		}
	}
}

func Test_ConfirmPrompt_FormatStructure(t *testing.T) {
	confirm := safety.NewConfirmationTracker([]string{"docker_stop"})

	result := tools.ConfirmPrompt(confirm, "docker_stop", "my-container", "Stop the container gracefully")
	text := resultText(t, result)

	// The expected format is:
	// Confirmation required for %s on %q.\n\n%s\n\nTo proceed, call %s again with confirmation_token=%q.
	// So the text should contain the tool name twice and the resource once (quoted).

	// Tool name should appear at least twice (once in "Confirmation required for",
	// once in "call <tool> again").
	toolCount := strings.Count(text, "docker_stop")
	if toolCount < 2 {
		t.Errorf("expected tool name to appear at least twice, found %d times\ntext:\n%s", toolCount, text)
	}

	// Resource should appear quoted.
	if !strings.Contains(text, `"my-container"`) {
		t.Errorf("resource should appear quoted in the output\ntext:\n%s", text)
	}

	// Description should appear as-is.
	if !strings.Contains(text, "Stop the container gracefully") {
		t.Errorf("description missing from output\ntext:\n%s", text)
	}
}

func Test_ConfirmPrompt_TokenUnique(t *testing.T) {
	confirm := safety.NewConfirmationTracker([]string{"docker_stop"})

	result1 := tools.ConfirmPrompt(confirm, "docker_stop", "container-a", "stop a")
	result2 := tools.ConfirmPrompt(confirm, "docker_stop", "container-a", "stop a")

	text1 := resultText(t, result1)
	text2 := resultText(t, result2)

	token1 := extractToken(t, text1)
	token2 := extractToken(t, text2)

	if token1 == token2 {
		t.Errorf("two calls returned the same token %q; tokens must be unique", token1)
	}
}

func Test_ConfirmPrompt_TokenConsumable(t *testing.T) {
	confirm := safety.NewConfirmationTracker([]string{"docker_stop"})

	result := tools.ConfirmPrompt(confirm, "docker_stop", "my-container", "stop it")
	text := resultText(t, result)
	token := extractToken(t, text)

	if token == "" {
		t.Fatal("extracted empty token from result text")
	}

	// First confirmation should succeed.
	if !confirm.Confirm(token) {
		t.Error("Confirm(token) should return true on first use")
	}

	// Second confirmation should fail (single-use).
	if confirm.Confirm(token) {
		t.Error("Confirm(token) should return false on second use (single-use token)")
	}
}

func Test_ConfirmPrompt_DifferentToolsAndResources(t *testing.T) {
	tests := []struct {
		name        string
		tool        string
		resource    string
		description string
	}{
		{
			name:        "docker stop",
			tool:        "docker_stop",
			resource:    "web-server",
			description: "Stop web-server container",
		},
		{
			name:        "vm delete",
			tool:        "vm_delete",
			resource:    "test-vm",
			description: "Delete the test virtual machine",
		},
		{
			name:        "docker remove with empty description",
			tool:        "docker_remove",
			resource:    "old-container",
			description: "",
		},
		{
			name:        "empty resource name",
			tool:        "docker_create",
			resource:    "",
			description: "Create a new container",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			confirm := safety.NewConfirmationTracker([]string{tt.tool})

			result := tools.ConfirmPrompt(confirm, tt.tool, tt.resource, tt.description)
			text := resultText(t, result)

			// Tool name must appear in the text.
			if !strings.Contains(text, tt.tool) {
				t.Errorf("text missing tool name %q\ntext:\n%s", tt.tool, text)
			}

			// The token must be extractable and non-empty.
			token := extractToken(t, text)
			if token == "" {
				t.Error("extracted empty token")
			}
		})
	}
}

func Test_ConfirmPrompt_ReturnsNonNil(t *testing.T) {
	confirm := safety.NewConfirmationTracker([]string{"test_tool"})
	result := tools.ConfirmPrompt(confirm, "test_tool", "res", "desc")
	if result == nil {
		t.Fatal("ConfirmPrompt returned nil")
	}
}

// ---------------------------------------------------------------------------
// Benchmark tests
// ---------------------------------------------------------------------------

func Benchmark_JSONResult_SimpleStruct(b *testing.B) {
	input := struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	}{Name: "bench", Count: 100}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tools.JSONResult(input)
	}
}

func Benchmark_ErrorResult(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tools.ErrorResult("something went wrong")
	}
}

func Benchmark_LogAudit(b *testing.B) {
	var buf bytes.Buffer
	audit := safety.NewAuditLogger(&buf)
	params := map[string]any{"id": "abc123"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		tools.LogAudit(audit, "docker_list", params, "ok", time.Now())
	}
}

func Benchmark_ConfirmPrompt(b *testing.B) {
	confirm := safety.NewConfirmationTracker([]string{"test_tool"})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tools.ConfirmPrompt(confirm, "test_tool", "resource", "description")
	}
}

// ---------------------------------------------------------------------------
// Test helper: extract confirmation token from result text
// ---------------------------------------------------------------------------

// tokenPattern matches confirmation_token="<hex>" or confirmation_token=<hex>
// in the ConfirmPrompt output text.
var tokenPattern = regexp.MustCompile(`confirmation_token="?([a-f0-9]+)"?`)

// extractToken pulls the confirmation token value from a ConfirmPrompt result
// text. It fails the test if no token is found.
func extractToken(t *testing.T, text string) string {
	t.Helper()
	matches := tokenPattern.FindStringSubmatch(text)
	if len(matches) < 2 {
		// Fallback: try to find any hex-like token after "confirmation_token=".
		idx := strings.Index(text, "confirmation_token=")
		if idx == -1 {
			t.Fatalf("no confirmation_token= found in text:\n%s", text)
		}
		after := text[idx+len("confirmation_token="):]
		// Strip leading quote if present.
		after = strings.TrimPrefix(after, "\"")
		// Read until end-of-token (quote, period, newline, space).
		end := strings.IndexAny(after, "\".\n ")
		if end == -1 {
			end = len(after)
		}
		token := after[:end]
		if token == "" {
			t.Fatalf("extracted empty token from text:\n%s", text)
		}
		return token
	}
	return matches[1]
}

// ---------------------------------------------------------------------------
// Edge case: JSONResult with various Go types
// ---------------------------------------------------------------------------

func Test_JSONResult_IntegerValue(t *testing.T) {
	result := tools.JSONResult(42)
	text := resultText(t, result)
	if strings.TrimSpace(text) != "42" {
		t.Errorf("JSONResult(42) text = %q, want %q", text, "42")
	}
}

func Test_JSONResult_StringValue(t *testing.T) {
	result := tools.JSONResult("hello world")
	text := resultText(t, result)
	if strings.TrimSpace(text) != `"hello world"` {
		t.Errorf("JSONResult(\"hello world\") text = %q, want %q", text, `"hello world"`)
	}
}

func Test_JSONResult_BoolValue(t *testing.T) {
	result := tools.JSONResult(true)
	text := resultText(t, result)
	if strings.TrimSpace(text) != "true" {
		t.Errorf("JSONResult(true) text = %q, want %q", text, "true")
	}
}

func Test_JSONResult_MapWithNestedValues(t *testing.T) {
	input := map[string]any{
		"containers": []string{"a", "b"},
		"count":      2,
	}
	result := tools.JSONResult(input)
	text := resultText(t, result)

	var parsed map[string]any
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		t.Fatalf("result is not valid JSON: %v", err)
	}

	if parsed["count"] != float64(2) {
		t.Errorf("count = %v, want 2", parsed["count"])
	}
}

// ---------------------------------------------------------------------------
// Integration-style test: JSONResult round-trip
// ---------------------------------------------------------------------------

func Test_JSONResult_RoundTrip(t *testing.T) {
	type container struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Status string `json:"status"`
	}

	original := container{ID: "abc123", Name: "web", Status: "running"}
	result := tools.JSONResult(original)
	text := resultText(t, result)

	var decoded container
	if err := json.Unmarshal([]byte(text), &decoded); err != nil {
		t.Fatalf("could not unmarshal result JSON: %v", err)
	}

	if decoded != original {
		t.Errorf("round-trip mismatch: got %+v, want %+v", decoded, original)
	}
}

// ---------------------------------------------------------------------------
// ErrorResult format consistency
// ---------------------------------------------------------------------------

func Test_ErrorResult_PrefixFormat(t *testing.T) {
	// Verify the "error: " prefix is always present and consistently formatted.
	msgs := []string{
		"not found",
		"",
		"timeout after 30s",
		"access to container \"web\" is not allowed",
	}

	for _, msg := range msgs {
		t.Run(fmt.Sprintf("msg=%q", msg), func(t *testing.T) {
			result := tools.ErrorResult(msg)
			text := resultText(t, result)
			expected := "error: " + msg
			if text != expected {
				t.Errorf("ErrorResult(%q) = %q, want %q", msg, text, expected)
			}
		})
	}
}
