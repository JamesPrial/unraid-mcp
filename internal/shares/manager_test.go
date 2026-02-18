package shares

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/jamesprial/unraid-mcp/internal/graphql"
	"github.com/jamesprial/unraid-mcp/internal/safety"
	"github.com/jamesprial/unraid-mcp/internal/tools"
	"github.com/mark3labs/mcp-go/mcp"
)

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

// mockGraphQLClient implements graphql.Client for manager tests.
type mockGraphQLClient struct {
	executeFunc func(ctx context.Context, query string, variables map[string]any) ([]byte, error)
}

func (m *mockGraphQLClient) Execute(ctx context.Context, query string, variables map[string]any) ([]byte, error) {
	return m.executeFunc(ctx, query, variables)
}

var _ graphql.Client = (*mockGraphQLClient)(nil)

// mockShareManager implements ShareManager for tool handler tests.
type mockShareManager struct {
	listFunc func(ctx context.Context) ([]Share, error)
}

func (m *mockShareManager) List(ctx context.Context) ([]Share, error) {
	return m.listFunc(ctx)
}

var _ ShareManager = (*mockShareManager)(nil)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// newCallToolRequest builds an mcp.CallToolRequest with the given name and arguments.
func newCallToolRequest(name string, args map[string]any) mcp.CallToolRequest {
	req := mcp.CallToolRequest{}
	req.Params.Name = name
	req.Params.Arguments = args
	return req
}

// extractResultText extracts the text string from the first Content element
// of a CallToolResult. It fails the test if the result is nil, has no content,
// or the first element is not a TextContent.
func extractResultText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	if result == nil {
		t.Fatal("result is nil")
	}
	if len(result.Content) == 0 {
		t.Fatal("result has no content")
	}
	tc, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("content[0] is %T, want mcp.TextContent", result.Content[0])
	}
	return tc.Text
}

// sampleShares returns a standard set of shares for test fixtures.
func sampleShares() []Share {
	return []Share{
		{Name: "appdata", Size: 500000000000, Used: 125000000000, Free: 375000000000, Comment: "Application data"},
		{Name: "isos", Size: 1000000000000, Used: 200000000000, Free: 800000000000, Comment: "ISO images"},
		{Name: "media", Size: 4000000000000, Used: 3000000000000, Free: 1000000000000, Comment: "Media files"},
	}
}

// ---------------------------------------------------------------------------
// Compile-time interface checks
// ---------------------------------------------------------------------------

func Test_GraphQLShareManager_ImplementsShareManager(t *testing.T) {
	var _ ShareManager = (*GraphQLShareManager)(nil)
}

// ---------------------------------------------------------------------------
// GraphQLShareManager.List tests
// ---------------------------------------------------------------------------

func Test_List_Cases(t *testing.T) {
	tests := []struct {
		name        string
		executeFunc func(ctx context.Context, query string, variables map[string]any) ([]byte, error)
		wantErr     bool
		errContains string
		validate    func(t *testing.T, shares []Share)
	}{
		{
			name: "returns shares from GraphQL response",
			executeFunc: func(ctx context.Context, query string, variables map[string]any) ([]byte, error) {
				resp := map[string]any{
					"shares": []map[string]any{
						{"name": "appdata", "size": 500000000000, "used": 125000000000, "free": 375000000000, "comment": "Application data"},
						{"name": "isos", "size": 1000000000000, "used": 200000000000, "free": 800000000000, "comment": "ISO images"},
					},
				}
				data, _ := json.Marshal(resp)
				return data, nil
			},
			validate: func(t *testing.T, shares []Share) {
				t.Helper()
				if len(shares) != 2 {
					t.Fatalf("len(shares) = %d, want 2", len(shares))
				}
				if shares[0].Name != "appdata" {
					t.Errorf("shares[0].Name = %q, want %q", shares[0].Name, "appdata")
				}
				if shares[0].Size != 500000000000 {
					t.Errorf("shares[0].Size = %d, want 500000000000", shares[0].Size)
				}
				if shares[0].Used != 125000000000 {
					t.Errorf("shares[0].Used = %d, want 125000000000", shares[0].Used)
				}
				if shares[0].Free != 375000000000 {
					t.Errorf("shares[0].Free = %d, want 375000000000", shares[0].Free)
				}
				if shares[0].Comment != "Application data" {
					t.Errorf("shares[0].Comment = %q, want %q", shares[0].Comment, "Application data")
				}
				if shares[1].Name != "isos" {
					t.Errorf("shares[1].Name = %q, want %q", shares[1].Name, "isos")
				}
			},
		},
		{
			name: "empty array returns empty slice no error",
			executeFunc: func(ctx context.Context, query string, variables map[string]any) ([]byte, error) {
				resp := map[string]any{
					"shares": []map[string]any{},
				}
				data, _ := json.Marshal(resp)
				return data, nil
			},
			validate: func(t *testing.T, shares []Share) {
				t.Helper()
				if shares == nil {
					t.Error("expected non-nil slice, got nil")
				}
				if len(shares) != 0 {
					t.Errorf("len(shares) = %d, want 0", len(shares))
				}
			},
		},
		{
			name: "client error returns error",
			executeFunc: func(ctx context.Context, query string, variables map[string]any) ([]byte, error) {
				return nil, errors.New("connection refused")
			},
			wantErr:     true,
			errContains: "connection refused",
		},
		{
			name: "invalid JSON response returns unmarshal error",
			executeFunc: func(ctx context.Context, query string, variables map[string]any) ([]byte, error) {
				return []byte("not valid json {{{"), nil
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &mockGraphQLClient{executeFunc: tt.executeFunc}
			mgr := NewGraphQLShareManager(client)

			shares, err := mgr.List(context.Background())

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errContains != "" && !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tt.errContains)) {
					t.Errorf("error = %q, want it to contain %q", err.Error(), tt.errContains)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.validate != nil {
				tt.validate(t, shares)
			}
		})
	}
}

func Test_List_QueryContainsExpectedFields(t *testing.T) {
	var capturedQuery string
	client := &mockGraphQLClient{
		executeFunc: func(ctx context.Context, query string, variables map[string]any) ([]byte, error) {
			capturedQuery = query
			resp := map[string]any{
				"shares": []map[string]any{},
			}
			data, _ := json.Marshal(resp)
			return data, nil
		},
	}

	mgr := NewGraphQLShareManager(client)
	_, _ = mgr.List(context.Background())

	expectedFields := []string{"name", "size", "used", "free", "comment"}
	for _, field := range expectedFields {
		if !strings.Contains(capturedQuery, field) {
			t.Errorf("query is missing expected field %q\nquery: %s", field, capturedQuery)
		}
	}
}

func Test_List_CancelledContext(t *testing.T) {
	client := &mockGraphQLClient{
		executeFunc: func(ctx context.Context, query string, variables map[string]any) ([]byte, error) {
			return nil, ctx.Err()
		},
	}

	mgr := NewGraphQLShareManager(client)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := mgr.List(ctx)
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// ---------------------------------------------------------------------------
// Share type zero-value tests
// ---------------------------------------------------------------------------

func Test_Share_ZeroValue(t *testing.T) {
	var s Share
	if s.Name != "" {
		t.Errorf("zero Share.Name = %q, want empty", s.Name)
	}
	if s.Size != 0 {
		t.Errorf("zero Share.Size = %d, want 0", s.Size)
	}
	if s.Used != 0 {
		t.Errorf("zero Share.Used = %d, want 0", s.Used)
	}
	if s.Free != 0 {
		t.Errorf("zero Share.Free = %d, want 0", s.Free)
	}
	if s.Comment != "" {
		t.Errorf("zero Share.Comment = %q, want empty", s.Comment)
	}
}

func Test_Share_JSONTags(t *testing.T) {
	s := Share{
		Name:    "media",
		Size:    4000000000000,
		Used:    3000000000000,
		Free:    1000000000000,
		Comment: "Media files",
	}

	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	expectedKeys := []string{"name", "size", "used", "free", "comment"}
	for _, key := range expectedKeys {
		if _, ok := parsed[key]; !ok {
			t.Errorf("JSON output missing expected key %q", key)
		}
	}
}

// ---------------------------------------------------------------------------
// ShareTools registration tests
// ---------------------------------------------------------------------------

func Test_ShareTools_RegistrationCount(t *testing.T) {
	mgr := &mockShareManager{
		listFunc: func(ctx context.Context) ([]Share, error) {
			return nil, nil
		},
	}

	regs := ShareTools(mgr, nil)
	if len(regs) != 1 {
		t.Fatalf("ShareTools() returned %d registrations, want 1", len(regs))
	}
}

func Test_ShareTools_ToolName(t *testing.T) {
	mgr := &mockShareManager{
		listFunc: func(ctx context.Context) ([]Share, error) {
			return nil, nil
		},
	}

	regs := ShareTools(mgr, nil)
	if len(regs) == 0 {
		t.Fatal("ShareTools() returned no registrations")
	}

	name := regs[0].Tool.Name
	if name != "shares_list" {
		t.Errorf("tool name = %q, want %q", name, "shares_list")
	}
}

func Test_ShareTools_NoRequiredParams(t *testing.T) {
	mgr := &mockShareManager{
		listFunc: func(ctx context.Context) ([]Share, error) {
			return nil, nil
		},
	}

	regs := ShareTools(mgr, nil)
	if len(regs) == 0 {
		t.Fatal("ShareTools() returned no registrations")
	}

	tool := regs[0].Tool
	if len(tool.InputSchema.Required) != 0 {
		t.Errorf("tool has %d required params, want 0; required: %v",
			len(tool.InputSchema.Required), tool.InputSchema.Required)
	}
}

func Test_ShareTools_HandlerIsNotNil(t *testing.T) {
	mgr := &mockShareManager{
		listFunc: func(ctx context.Context) ([]Share, error) {
			return nil, nil
		},
	}

	regs := ShareTools(mgr, nil)
	if len(regs) == 0 {
		t.Fatal("ShareTools() returned no registrations")
	}

	if regs[0].Handler == nil {
		t.Error("tool handler is nil")
	}
}

// ---------------------------------------------------------------------------
// shares_list handler tests
// ---------------------------------------------------------------------------

func Test_SharesListHandler_Cases(t *testing.T) {
	tests := []struct {
		name         string
		listFunc     func(ctx context.Context) ([]Share, error)
		wantGoErr    bool // second return value from handler
		wantErrInTxt bool // result text contains "error"
		validate     func(t *testing.T, text string)
	}{
		{
			name: "happy path returns JSON array of shares",
			listFunc: func(ctx context.Context) ([]Share, error) {
				return sampleShares(), nil
			},
			validate: func(t *testing.T, text string) {
				t.Helper()
				var shares []Share
				if err := json.Unmarshal([]byte(text), &shares); err != nil {
					t.Fatalf("result is not valid JSON array of shares: %v\ntext: %s", err, text)
				}
				if len(shares) != 3 {
					t.Errorf("len(shares) = %d, want 3", len(shares))
				}
				if shares[0].Name != "appdata" {
					t.Errorf("shares[0].Name = %q, want %q", shares[0].Name, "appdata")
				}
				if shares[2].Name != "media" {
					t.Errorf("shares[2].Name = %q, want %q", shares[2].Name, "media")
				}
			},
		},
		{
			name: "empty list returns empty JSON array",
			listFunc: func(ctx context.Context) ([]Share, error) {
				return []Share{}, nil
			},
			validate: func(t *testing.T, text string) {
				t.Helper()
				if strings.TrimSpace(text) != "[]" {
					t.Errorf("result text = %q, want %q", strings.TrimSpace(text), "[]")
				}
			},
		},
		{
			name: "manager error returns error result",
			listFunc: func(ctx context.Context) ([]Share, error) {
				return nil, errors.New("graphql timeout")
			},
			wantErrInTxt: true,
			validate: func(t *testing.T, text string) {
				t.Helper()
				if !strings.Contains(text, "graphql timeout") {
					t.Errorf("error result text = %q, want it to contain %q", text, "graphql timeout")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr := &mockShareManager{listFunc: tt.listFunc}
			regs := ShareTools(mgr, nil)
			if len(regs) == 0 {
				t.Fatal("ShareTools() returned no registrations")
			}

			handler := regs[0].Handler
			req := newCallToolRequest("shares_list", nil)

			result, err := handler(context.Background(), req)

			// Handler must never return a Go error.
			if err != nil {
				t.Fatalf("handler returned non-nil Go error: %v", err)
			}

			if result == nil {
				t.Fatal("handler returned nil result")
			}

			text := extractResultText(t, result)

			if tt.wantErrInTxt {
				if !strings.Contains(strings.ToLower(text), "error") {
					t.Errorf("result text = %q, want it to contain 'error'", text)
				}
			}

			if tt.validate != nil {
				tt.validate(t, text)
			}
		})
	}
}

func Test_SharesListHandler_NeverReturnsGoError(t *testing.T) {
	scenarios := []struct {
		name     string
		listFunc func(ctx context.Context) ([]Share, error)
	}{
		{
			name: "success",
			listFunc: func(ctx context.Context) ([]Share, error) {
				return sampleShares(), nil
			},
		},
		{
			name: "error",
			listFunc: func(ctx context.Context) ([]Share, error) {
				return nil, errors.New("fail")
			},
		},
		{
			name: "nil slice",
			listFunc: func(ctx context.Context) ([]Share, error) {
				return nil, nil
			},
		},
		{
			name: "empty slice",
			listFunc: func(ctx context.Context) ([]Share, error) {
				return []Share{}, nil
			},
		},
	}

	for _, tt := range scenarios {
		t.Run(tt.name, func(t *testing.T) {
			mgr := &mockShareManager{listFunc: tt.listFunc}
			regs := ShareTools(mgr, nil)
			handler := regs[0].Handler

			req := newCallToolRequest("shares_list", nil)
			_, err := handler(context.Background(), req)
			if err != nil {
				t.Errorf("handler returned non-nil Go error: %v", err)
			}
		})
	}
}

func Test_SharesListHandler_ResultIsPrettyJSON(t *testing.T) {
	mgr := &mockShareManager{
		listFunc: func(ctx context.Context) ([]Share, error) {
			return sampleShares(), nil
		},
	}

	regs := ShareTools(mgr, nil)
	handler := regs[0].Handler
	req := newCallToolRequest("shares_list", nil)

	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler returned non-nil Go error: %v", err)
	}

	text := extractResultText(t, result)

	// Valid JSON.
	if !json.Valid([]byte(text)) {
		t.Errorf("result text is not valid JSON: %q", text)
	}

	// Pretty-printed JSON should contain newlines (from json.MarshalIndent).
	if !strings.Contains(text, "\n") {
		t.Errorf("result text does not appear to be pretty-printed: %q", text)
	}
}

func Test_SharesListHandler_NilAuditLogger_NoPanic(t *testing.T) {
	mgr := &mockShareManager{
		listFunc: func(ctx context.Context) ([]Share, error) {
			return sampleShares(), nil
		},
	}

	// Pass nil audit logger -- must not panic.
	regs := ShareTools(mgr, nil)
	handler := regs[0].Handler
	req := newCallToolRequest("shares_list", nil)

	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler returned non-nil Go error: %v", err)
	}
	if result == nil {
		t.Fatal("handler returned nil result")
	}
}

func Test_SharesListHandler_WithAuditLogger(t *testing.T) {
	mgr := &mockShareManager{
		listFunc: func(ctx context.Context) ([]Share, error) {
			return sampleShares(), nil
		},
	}

	var buf strings.Builder
	audit := safety.NewAuditLogger(&buf)

	regs := ShareTools(mgr, audit)
	handler := regs[0].Handler
	req := newCallToolRequest("shares_list", nil)

	_, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler returned non-nil Go error: %v", err)
	}

	logged := buf.String()
	if logged == "" {
		t.Error("expected audit log entry, got empty")
	}
	if !strings.Contains(logged, "shares_list") {
		t.Errorf("audit log = %q, want it to contain 'shares_list'", logged)
	}
}

func Test_SharesListHandler_ShareFieldsInResult(t *testing.T) {
	mgr := &mockShareManager{
		listFunc: func(ctx context.Context) ([]Share, error) {
			return []Share{
				{Name: "test-share", Size: 100, Used: 50, Free: 50, Comment: "test"},
			}, nil
		},
	}

	regs := ShareTools(mgr, nil)
	handler := regs[0].Handler
	req := newCallToolRequest("shares_list", nil)

	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler returned non-nil Go error: %v", err)
	}

	text := extractResultText(t, result)

	var shares []Share
	if err := json.Unmarshal([]byte(text), &shares); err != nil {
		t.Fatalf("could not unmarshal result: %v", err)
	}

	if len(shares) != 1 {
		t.Fatalf("len(shares) = %d, want 1", len(shares))
	}

	s := shares[0]
	if s.Name != "test-share" {
		t.Errorf("Name = %q, want %q", s.Name, "test-share")
	}
	if s.Size != 100 {
		t.Errorf("Size = %d, want 100", s.Size)
	}
	if s.Used != 50 {
		t.Errorf("Used = %d, want 50", s.Used)
	}
	if s.Free != 50 {
		t.Errorf("Free = %d, want 50", s.Free)
	}
	if s.Comment != "test" {
		t.Errorf("Comment = %q, want %q", s.Comment, "test")
	}
}

// ---------------------------------------------------------------------------
// Constructor test
// ---------------------------------------------------------------------------

func Test_NewGraphQLShareManager_ReturnsNonNil(t *testing.T) {
	client := &mockGraphQLClient{
		executeFunc: func(ctx context.Context, query string, variables map[string]any) ([]byte, error) {
			return []byte(`{}`), nil
		},
	}

	mgr := NewGraphQLShareManager(client)
	if mgr == nil {
		t.Fatal("NewGraphQLShareManager() returned nil")
	}
}

// ---------------------------------------------------------------------------
// ShareManager interface method count
// ---------------------------------------------------------------------------

func Test_ShareManager_HasListMethod(t *testing.T) {
	// This test verifies at compile time that ShareManager has a List method
	// by constructing a mock that implements it.
	var mgr ShareManager = &mockShareManager{
		listFunc: func(ctx context.Context) ([]Share, error) {
			return nil, nil
		},
	}
	_, _ = mgr.List(context.Background())
}

// ---------------------------------------------------------------------------
// No DestructiveTools for shares (all read-only)
// ---------------------------------------------------------------------------

func Test_ShareTools_AllReadOnly(t *testing.T) {
	mgr := &mockShareManager{
		listFunc: func(ctx context.Context) ([]Share, error) {
			return nil, nil
		},
	}

	regs := ShareTools(mgr, nil)

	// All share tools are read-only, so none should have "confirmation_token"
	// in their required params.
	for _, reg := range regs {
		for _, req := range reg.Tool.InputSchema.Required {
			if req == "confirmation_token" {
				t.Errorf("tool %q has confirmation_token as required -- shares should be read-only",
					reg.Tool.Name)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Constructor nil client tests
// ---------------------------------------------------------------------------

func TestNewGraphQLShareManager_NilClient(t *testing.T) {
	t.Helper()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for nil client, got none")
		}
		msg := fmt.Sprint(r)
		if !strings.Contains(msg, "nil") {
			t.Fatalf("panic message should mention nil, got: %s", msg)
		}
	}()
	NewGraphQLShareManager(nil)
}

// ---------------------------------------------------------------------------
// Benchmarks
// ---------------------------------------------------------------------------

func Benchmark_List(b *testing.B) {
	client := &mockGraphQLClient{
		executeFunc: func(ctx context.Context, query string, variables map[string]any) ([]byte, error) {
			resp := map[string]any{
				"shares": []map[string]any{
					{"name": "appdata", "size": 500000000000, "used": 125000000000, "free": 375000000000, "comment": "Application data"},
					{"name": "isos", "size": 1000000000000, "used": 200000000000, "free": 800000000000, "comment": "ISO images"},
					{"name": "media", "size": 4000000000000, "used": 3000000000000, "free": 1000000000000, "comment": "Media files"},
				},
			}
			data, _ := json.Marshal(resp)
			return data, nil
		},
	}

	mgr := NewGraphQLShareManager(client)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = mgr.List(ctx)
	}
}

func Benchmark_SharesListHandler(b *testing.B) {
	mgr := &mockShareManager{
		listFunc: func(ctx context.Context) ([]Share, error) {
			return sampleShares(), nil
		},
	}

	regs := ShareTools(mgr, nil)
	handler := regs[0].Handler

	req := newCallToolRequest("shares_list", nil)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = handler(ctx, req)
	}
}

// Ensure the tools import is used.
var _ = tools.JSONResult
