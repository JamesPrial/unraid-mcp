package graphql

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/jamesprial/unraid-mcp/internal/safety"
	"github.com/mark3labs/mcp-go/mcp"
)

// ---------------------------------------------------------------------------
// Mock Client
// ---------------------------------------------------------------------------

// mockClient implements the Client interface for testing tool handlers.
type mockClient struct {
	executeFunc func(ctx context.Context, query string, variables map[string]any) ([]byte, error)
}

func (m *mockClient) Execute(ctx context.Context, query string, variables map[string]any) ([]byte, error) {
	return m.executeFunc(ctx, query, variables)
}

// Compile-time check that mockClient satisfies the Client interface.
var _ Client = (*mockClient)(nil)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// newCallToolRequest builds an mcp.CallToolRequest with the given arguments map.
func newCallToolRequest(t *testing.T, args map[string]any) mcp.CallToolRequest {
	t.Helper()
	req := mcp.CallToolRequest{}
	req.Params.Arguments = args
	return req
}

// extractResultText extracts the text string from a CallToolResult, assuming
// the first content entry is TextContent.
func extractResultText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	if result == nil {
		t.Fatal("result is nil")
	}
	if len(result.Content) == 0 {
		t.Fatal("result has no content entries")
	}
	tc, ok := mcp.AsTextContent(result.Content[0])
	if !ok {
		t.Fatalf("first content entry is not TextContent, got %T", result.Content[0])
	}
	return tc.Text
}

// newTestAuditLogger returns an AuditLogger backed by an in-memory buffer
// for test inspection.
func newTestAuditLogger(t *testing.T) (*safety.AuditLogger, *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	logger := safety.NewAuditLogger(&buf)
	return logger, &buf
}

// ---------------------------------------------------------------------------
// GraphQLTools registration tests
// ---------------------------------------------------------------------------

func Test_GraphQLTools_ReturnsExactlyOneRegistration(t *testing.T) {
	client := &mockClient{
		executeFunc: func(ctx context.Context, query string, variables map[string]any) ([]byte, error) {
			return []byte(`{}`), nil
		},
	}

	regs := GraphQLTools(client, nil)
	if len(regs) != 1 {
		t.Fatalf("GraphQLTools() returned %d registrations, want 1", len(regs))
	}
}

func Test_GraphQLTools_ToolNameIsGraphqlQuery(t *testing.T) {
	client := &mockClient{
		executeFunc: func(ctx context.Context, query string, variables map[string]any) ([]byte, error) {
			return []byte(`{}`), nil
		},
	}

	regs := GraphQLTools(client, nil)
	if len(regs) == 0 {
		t.Fatal("GraphQLTools() returned no registrations")
	}

	name := regs[0].Tool.Name
	if name != "graphql_query" {
		t.Errorf("tool name = %q, want %q", name, "graphql_query")
	}
}

func Test_GraphQLTools_SchemaHasQueryParameter(t *testing.T) {
	client := &mockClient{
		executeFunc: func(ctx context.Context, query string, variables map[string]any) ([]byte, error) {
			return []byte(`{}`), nil
		},
	}

	regs := GraphQLTools(client, nil)
	if len(regs) == 0 {
		t.Fatal("GraphQLTools() returned no registrations")
	}

	tool := regs[0].Tool

	// Check "query" is in the properties.
	queryProp, ok := tool.InputSchema.Properties["query"]
	if !ok {
		t.Fatal("tool input schema is missing 'query' property")
	}

	// Verify it is a string type.
	propMap, ok := queryProp.(map[string]any)
	if !ok {
		t.Fatalf("query property is %T, want map[string]any", queryProp)
	}
	if propMap["type"] != "string" {
		t.Errorf("query property type = %v, want %q", propMap["type"], "string")
	}

	// Verify "query" is listed as required.
	found := false
	for _, r := range tool.InputSchema.Required {
		if r == "query" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("'query' is not in required list %v", tool.InputSchema.Required)
	}
}

func Test_GraphQLTools_SchemaHasVariablesParameter(t *testing.T) {
	client := &mockClient{
		executeFunc: func(ctx context.Context, query string, variables map[string]any) ([]byte, error) {
			return []byte(`{}`), nil
		},
	}

	regs := GraphQLTools(client, nil)
	if len(regs) == 0 {
		t.Fatal("GraphQLTools() returned no registrations")
	}

	tool := regs[0].Tool

	// Check "variables" is in the properties.
	varProp, ok := tool.InputSchema.Properties["variables"]
	if !ok {
		t.Fatal("tool input schema is missing 'variables' property")
	}

	// Verify it is a string type.
	propMap, ok := varProp.(map[string]any)
	if !ok {
		t.Fatalf("variables property is %T, want map[string]any", varProp)
	}
	if propMap["type"] != "string" {
		t.Errorf("variables property type = %v, want %q", propMap["type"], "string")
	}

	// Verify "variables" is NOT required (optional).
	for _, r := range tool.InputSchema.Required {
		if r == "variables" {
			t.Error("'variables' should be optional but is listed in required")
		}
	}
}

func Test_GraphQLTools_HandlerIsNotNil(t *testing.T) {
	client := &mockClient{
		executeFunc: func(ctx context.Context, query string, variables map[string]any) ([]byte, error) {
			return []byte(`{}`), nil
		},
	}

	regs := GraphQLTools(client, nil)
	if len(regs) == 0 {
		t.Fatal("GraphQLTools() returned no registrations")
	}

	if regs[0].Handler == nil {
		t.Error("tool handler is nil")
	}
}

// ---------------------------------------------------------------------------
// graphql_query handler tests
// ---------------------------------------------------------------------------

func Test_GraphQLQueryHandler_Cases(t *testing.T) {
	tests := []struct {
		name            string
		args            map[string]any
		executeFunc     func(ctx context.Context, query string, variables map[string]any) ([]byte, error)
		wantErrNil      bool // second return value from handler should be nil when true
		wantResultErr   bool // result text should contain "error"
		wantContains    string
		wantNotContains string
	}{
		{
			name: "valid query with no variables returns JSON result",
			args: map[string]any{
				"query": "{ info { hostname } }",
			},
			executeFunc: func(ctx context.Context, query string, variables map[string]any) ([]byte, error) {
				return []byte(`{"info":{"hostname":"tower"}}`), nil
			},
			wantErrNil:   true,
			wantContains: "hostname",
		},
		{
			name: "valid query with valid variables JSON",
			args: map[string]any{
				"query":     `query GetInfo($id: ID!) { info(id: $id) { name } }`,
				"variables": `{"id":"abc"}`,
			},
			executeFunc: func(ctx context.Context, query string, variables map[string]any) ([]byte, error) {
				// Verify variables were correctly parsed.
				if variables == nil {
					return nil, errors.New("expected non-nil variables")
				}
				if variables["id"] != "abc" {
					return nil, errors.New("expected id=abc in variables")
				}
				return []byte(`{"info":{"name":"test"}}`), nil
			},
			wantErrNil:   true,
			wantContains: "name",
		},
		{
			name: "empty variables string passes nil variables",
			args: map[string]any{
				"query":     "{ info { hostname } }",
				"variables": "",
			},
			executeFunc: func(ctx context.Context, query string, variables map[string]any) ([]byte, error) {
				if variables != nil {
					return nil, errors.New("expected nil variables for empty string")
				}
				return []byte(`{"info":{"hostname":"tower"}}`), nil
			},
			wantErrNil:   true,
			wantContains: "hostname",
		},
		{
			name: "variables key absent passes nil variables",
			args: map[string]any{
				"query": "{ info { hostname } }",
			},
			executeFunc: func(ctx context.Context, query string, variables map[string]any) ([]byte, error) {
				if variables != nil {
					return nil, errors.New("expected nil variables when key absent")
				}
				return []byte(`{"info":{"hostname":"tower"}}`), nil
			},
			wantErrNil:   true,
			wantContains: "hostname",
		},
		{
			name: "invalid variables JSON returns error result",
			args: map[string]any{
				"query":     "{ info { hostname } }",
				"variables": "not json",
			},
			executeFunc: func(ctx context.Context, query string, variables map[string]any) ([]byte, error) {
				t.Error("Execute should not be called when variables JSON is invalid")
				return nil, nil
			},
			wantErrNil:    true,
			wantResultErr: true,
			wantContains:  "parse variables JSON",
		},
		{
			name: "client returns error produces error result",
			args: map[string]any{
				"query": "{ info { hostname } }",
			},
			executeFunc: func(ctx context.Context, query string, variables map[string]any) ([]byte, error) {
				return nil, errors.New("connection refused")
			},
			wantErrNil:    true,
			wantResultErr: true,
			wantContains:  "connection refused",
		},
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &mockClient{executeFunc: tt.executeFunc}
			audit, _ := newTestAuditLogger(t)

			regs := GraphQLTools(client, audit)
			if len(regs) == 0 {
				t.Fatal("GraphQLTools() returned no registrations")
			}

			handler := regs[0].Handler
			req := newCallToolRequest(t, tt.args)

			result, err := handler(context.Background(), req)

			// Check the Go error (second return value) based on wantErrNil.
			if tt.wantErrNil {
				if err != nil {
					t.Fatalf("handler returned non-nil error: %v, want nil", err)
				}
			} else {
				if err == nil {
					t.Fatal("handler returned nil error, want non-nil")
				}
			}

			if result == nil {
				t.Fatal("handler returned nil result")
			}

			text := extractResultText(t, result)

			// Check whether the result text indicates an error.
			if tt.wantResultErr {
				if !strings.Contains(strings.ToLower(text), "error") {
					t.Errorf("result text = %q, want it to contain 'error'", text)
				}
			}

			if tt.wantContains != "" && !strings.Contains(text, tt.wantContains) {
				t.Errorf("result text = %q, want it to contain %q", text, tt.wantContains)
			}

			if tt.wantNotContains != "" && strings.Contains(text, tt.wantNotContains) {
				t.Errorf("result text = %q, want it NOT to contain %q", text, tt.wantNotContains)
			}
		})
	}
}

func Test_GraphQLQueryHandler_NeverReturnsGoError(t *testing.T) {
	// Exhaustive check: try multiple scenarios and confirm err is always nil.
	scenarios := []map[string]any{
		{"query": "{ info }"},
		{"query": "{ info }", "variables": ""},
		{"query": "{ info }", "variables": `{"id":"abc"}`},
		{"query": "{ info }", "variables": "bad json"},
		{"query": ""},
	}

	for i, args := range scenarios {
		client := &mockClient{
			executeFunc: func(ctx context.Context, query string, variables map[string]any) ([]byte, error) {
				return []byte(`{"data":"ok"}`), nil
			},
		}

		regs := GraphQLTools(client, nil)
		handler := regs[0].Handler
		req := newCallToolRequest(t, args)

		_, err := handler(context.Background(), req)
		if err != nil {
			t.Errorf("scenario %d: handler returned non-nil error: %v", i, err)
		}
	}
}

func Test_GraphQLQueryHandler_NilAuditLogger_NoPanic(t *testing.T) {
	client := &mockClient{
		executeFunc: func(ctx context.Context, query string, variables map[string]any) ([]byte, error) {
			return []byte(`{"info":{"hostname":"tower"}}`), nil
		},
	}

	// Pass nil for audit logger.
	regs := GraphQLTools(client, nil)
	if len(regs) == 0 {
		t.Fatal("GraphQLTools() returned no registrations")
	}

	handler := regs[0].Handler
	req := newCallToolRequest(t, map[string]any{
		"query": "{ info { hostname } }",
	})

	// This should not panic.
	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler returned non-nil error: %v", err)
	}
	if result == nil {
		t.Fatal("handler returned nil result")
	}
}

func Test_GraphQLQueryHandler_VariablesPassedToClient(t *testing.T) {
	// Verify the parsed variables are correctly forwarded to client.Execute.
	var capturedVars map[string]any

	client := &mockClient{
		executeFunc: func(ctx context.Context, query string, variables map[string]any) ([]byte, error) {
			capturedVars = variables
			return []byte(`{"result":"ok"}`), nil
		},
	}

	regs := GraphQLTools(client, nil)
	handler := regs[0].Handler

	inputVars := map[string]any{
		"id":    "abc-123",
		"count": float64(42),
	}
	varsJSON, err := json.Marshal(inputVars)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	req := newCallToolRequest(t, map[string]any{
		"query":     `query GetInfo($id: ID!, $count: Int!) { info(id: $id, count: $count) { name } }`,
		"variables": string(varsJSON),
	})

	_, err = handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler returned non-nil error: %v", err)
	}

	if capturedVars == nil {
		t.Fatal("client.Execute was called with nil variables, expected parsed variables")
	}

	if capturedVars["id"] != "abc-123" {
		t.Errorf("captured variables['id'] = %v, want %q", capturedVars["id"], "abc-123")
	}

	// JSON numbers decode as float64.
	if capturedVars["count"] != float64(42) {
		t.Errorf("captured variables['count'] = %v, want %v", capturedVars["count"], float64(42))
	}
}

func Test_GraphQLQueryHandler_QueryPassedToClient(t *testing.T) {
	var capturedQuery string

	client := &mockClient{
		executeFunc: func(ctx context.Context, query string, variables map[string]any) ([]byte, error) {
			capturedQuery = query
			return []byte(`{"result":"ok"}`), nil
		},
	}

	regs := GraphQLTools(client, nil)
	handler := regs[0].Handler

	expectedQuery := `query { dockerContainers { name status } }`
	req := newCallToolRequest(t, map[string]any{
		"query": expectedQuery,
	})

	_, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler returned non-nil error: %v", err)
	}

	if capturedQuery != expectedQuery {
		t.Errorf("captured query = %q, want %q", capturedQuery, expectedQuery)
	}
}

func Test_GraphQLQueryHandler_SuccessResultIsPrettyJSON(t *testing.T) {
	client := &mockClient{
		executeFunc: func(ctx context.Context, query string, variables map[string]any) ([]byte, error) {
			return []byte(`{"info":{"hostname":"tower","version":"6.12"}}`), nil
		},
	}

	regs := GraphQLTools(client, nil)
	handler := regs[0].Handler
	req := newCallToolRequest(t, map[string]any{
		"query": "{ info { hostname version } }",
	})

	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler returned non-nil error: %v", err)
	}

	text := extractResultText(t, result)

	// The result should be valid JSON.
	if !json.Valid([]byte(text)) {
		t.Errorf("result text is not valid JSON: %q", text)
	}

	// Pretty-printed JSON should contain newlines (from json.MarshalIndent).
	if !strings.Contains(text, "\n") {
		t.Errorf("result text does not appear to be pretty-printed: %q", text)
	}
}

func Test_GraphQLQueryHandler_ErrorResultContainsErrorPrefix(t *testing.T) {
	client := &mockClient{
		executeFunc: func(ctx context.Context, query string, variables map[string]any) ([]byte, error) {
			return nil, errors.New("something went wrong")
		},
	}

	regs := GraphQLTools(client, nil)
	handler := regs[0].Handler
	req := newCallToolRequest(t, map[string]any{
		"query": "{ info { hostname } }",
	})

	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler returned non-nil error: %v", err)
	}

	text := extractResultText(t, result)

	if !strings.Contains(text, "error") {
		t.Errorf("error result text = %q, want it to contain 'error'", text)
	}
	if !strings.Contains(text, "something went wrong") {
		t.Errorf("error result text = %q, want it to contain 'something went wrong'", text)
	}
}

func Test_GraphQLQueryHandler_InvalidVariablesDoesNotCallClient(t *testing.T) {
	clientCalled := false

	client := &mockClient{
		executeFunc: func(ctx context.Context, query string, variables map[string]any) ([]byte, error) {
			clientCalled = true
			return []byte(`{}`), nil
		},
	}

	regs := GraphQLTools(client, nil)
	handler := regs[0].Handler
	req := newCallToolRequest(t, map[string]any{
		"query":     "{ info }",
		"variables": "{invalid json",
	})

	_, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler returned non-nil error: %v", err)
	}

	if clientCalled {
		t.Error("client.Execute should not be called when variables JSON is invalid")
	}
}

func Test_GraphQLQueryHandler_ComplexVariablesJSON(t *testing.T) {
	var capturedVars map[string]any

	client := &mockClient{
		executeFunc: func(ctx context.Context, query string, variables map[string]any) ([]byte, error) {
			capturedVars = variables
			return []byte(`{"result":"ok"}`), nil
		},
	}

	regs := GraphQLTools(client, nil)
	handler := regs[0].Handler

	// Nested JSON with arrays and objects.
	complexVars := `{"filter":{"status":["running","stopped"]},"limit":10,"nested":{"key":"value"}}`
	req := newCallToolRequest(t, map[string]any{
		"query":     "query GetContainers($filter: FilterInput, $limit: Int) { containers(filter: $filter, limit: $limit) { name } }",
		"variables": complexVars,
	})

	_, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler returned non-nil error: %v", err)
	}

	if capturedVars == nil {
		t.Fatal("expected non-nil captured variables")
	}

	// Verify nested object was parsed.
	filter, ok := capturedVars["filter"]
	if !ok {
		t.Fatal("expected 'filter' in parsed variables")
	}
	filterMap, ok := filter.(map[string]any)
	if !ok {
		t.Fatalf("filter is %T, want map[string]any", filter)
	}
	statuses, ok := filterMap["status"]
	if !ok {
		t.Fatal("expected 'status' in filter")
	}
	statusArr, ok := statuses.([]any)
	if !ok {
		t.Fatalf("status is %T, want []any", statuses)
	}
	if len(statusArr) != 2 {
		t.Errorf("len(status) = %d, want 2", len(statusArr))
	}
}

func Test_GraphQLQueryHandler_AuditLogging(t *testing.T) {
	client := &mockClient{
		executeFunc: func(ctx context.Context, query string, variables map[string]any) ([]byte, error) {
			return []byte(`{"info":{"hostname":"tower"}}`), nil
		},
	}

	audit, buf := newTestAuditLogger(t)

	regs := GraphQLTools(client, audit)
	handler := regs[0].Handler
	req := newCallToolRequest(t, map[string]any{
		"query": "{ info { hostname } }",
	})

	_, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler returned non-nil error: %v", err)
	}

	// The audit logger should have received at least one entry.
	logged := buf.String()
	if logged == "" {
		t.Error("expected audit log entry, got empty")
	}

	// The logged entry should mention the tool name.
	if !strings.Contains(logged, "graphql_query") {
		t.Errorf("audit log = %q, want it to contain 'graphql_query'", logged)
	}
}

// ---------------------------------------------------------------------------
// Benchmarks
// ---------------------------------------------------------------------------

func Benchmark_GraphQLQueryHandler_HappyPath(b *testing.B) {
	client := &mockClient{
		executeFunc: func(ctx context.Context, query string, variables map[string]any) ([]byte, error) {
			return []byte(`{"info":{"hostname":"tower","version":"6.12.0"}}`), nil
		},
	}

	regs := GraphQLTools(client, nil)
	handler := regs[0].Handler

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"query": "{ info { hostname version } }",
	}

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = handler(ctx, req)
	}
}

func Benchmark_GraphQLQueryHandler_WithVariables(b *testing.B) {
	client := &mockClient{
		executeFunc: func(ctx context.Context, query string, variables map[string]any) ([]byte, error) {
			return []byte(`{"container":{"name":"nginx","status":"running"}}`), nil
		},
	}

	regs := GraphQLTools(client, nil)
	handler := regs[0].Handler

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"query":     `query GetContainer($id: ID!) { container(id: $id) { name status } }`,
		"variables": `{"id":"abc-123"}`,
	}

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = handler(ctx, req)
	}
}
