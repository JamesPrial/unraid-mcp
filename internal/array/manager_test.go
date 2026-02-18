package array

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/jamesprial/unraid-mcp/internal/graphql"
	"github.com/jamesprial/unraid-mcp/internal/safety"
	"github.com/jamesprial/unraid-mcp/internal/tools"
	"github.com/mark3labs/mcp-go/mcp"
)

// ===========================================================================
// Mock: GraphQL Client (for manager tests)
// ===========================================================================

// mockGraphQLClient implements graphql.Client for testing the manager layer.
type mockGraphQLClient struct {
	executeFunc func(ctx context.Context, query string, variables map[string]any) ([]byte, error)
}

var _ graphql.Client = (*mockGraphQLClient)(nil)

func (m *mockGraphQLClient) Execute(ctx context.Context, query string, variables map[string]any) ([]byte, error) {
	if m.executeFunc != nil {
		return m.executeFunc(ctx, query, variables)
	}
	return nil, fmt.Errorf("mockGraphQLClient: executeFunc not set")
}

// ===========================================================================
// Mock: ArrayManager (for tools tests)
// ===========================================================================

// mockArrayManager implements ArrayManager for testing the tool handlers.
type mockArrayManager struct {
	startFunc       func(ctx context.Context) error
	stopFunc        func(ctx context.Context) error
	parityCheckFunc func(ctx context.Context, action string) (string, error)
}

var _ ArrayManager = (*mockArrayManager)(nil)

func (m *mockArrayManager) Start(ctx context.Context) error {
	if m.startFunc != nil {
		return m.startFunc(ctx)
	}
	return nil
}

func (m *mockArrayManager) Stop(ctx context.Context) error {
	if m.stopFunc != nil {
		return m.stopFunc(ctx)
	}
	return nil
}

func (m *mockArrayManager) ParityCheck(ctx context.Context, action string) (string, error) {
	if m.parityCheckFunc != nil {
		return m.parityCheckFunc(ctx, action)
	}
	return "", nil
}

// ===========================================================================
// Helpers
// ===========================================================================

// newCallToolRequest builds a CallToolRequest with the given name and args.
func newCallToolRequest(name string, args map[string]any) mcp.CallToolRequest {
	req := mcp.CallToolRequest{}
	req.Params.Name = name
	req.Params.Arguments = args
	return req
}

// extractResultText extracts the text from the first TextContent item in a
// CallToolResult. It calls t.Fatal on any unexpected structure.
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

// findRegistration looks up a tool registration by name.
func findRegistration(t *testing.T, regs []tools.Registration, name string) tools.Registration {
	t.Helper()
	for _, r := range regs {
		if r.Tool.Name == name {
			return r
		}
	}
	t.Fatalf("registration for %q not found", name)
	return tools.Registration{} // unreachable
}

// ===========================================================================
// Compile-time interface checks
// ===========================================================================

func Test_GraphQLArrayManager_ImplementsArrayManager(t *testing.T) {
	var _ ArrayManager = (*GraphQLArrayManager)(nil)
}

// ===========================================================================
// Manager tests: Start
// ===========================================================================

func Test_Manager_Start_Cases(t *testing.T) {
	tests := []struct {
		name        string
		executeFunc func(ctx context.Context, query string, variables map[string]any) ([]byte, error)
		wantErr     bool
		errContains string
		wantQuery   string
	}{
		{
			name: "calls start mutation",
			executeFunc: func(ctx context.Context, query string, variables map[string]any) ([]byte, error) {
				if !strings.Contains(query, "mutation") || !strings.Contains(query, "array") || !strings.Contains(query, "start") {
					return nil, fmt.Errorf("unexpected query: %s", query)
				}
				return []byte(`{"data":{"array":{"start":true}}}`), nil
			},
			wantErr: false,
		},
		{
			name: "client error propagates",
			executeFunc: func(ctx context.Context, query string, variables map[string]any) ([]byte, error) {
				return nil, fmt.Errorf("connection refused")
			},
			wantErr:     true,
			errContains: "connection refused",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &mockGraphQLClient{executeFunc: tt.executeFunc}
			mgr := NewGraphQLArrayManager(client)

			err := mgr.Start(context.Background())

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("error = %q, want it to contain %q", err.Error(), tt.errContains)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func Test_Manager_Start_QueryContent(t *testing.T) {
	var capturedQuery string
	client := &mockGraphQLClient{
		executeFunc: func(ctx context.Context, query string, variables map[string]any) ([]byte, error) {
			capturedQuery = query
			return []byte(`{"data":{"array":{"start":true}}}`), nil
		},
	}
	mgr := NewGraphQLArrayManager(client)

	if err := mgr.Start(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(capturedQuery, "mutation") {
		t.Errorf("query should be a mutation, got: %q", capturedQuery)
	}
	if !strings.Contains(capturedQuery, "array") {
		t.Errorf("query should reference array, got: %q", capturedQuery)
	}
	if !strings.Contains(capturedQuery, "start") {
		t.Errorf("query should reference start, got: %q", capturedQuery)
	}
}

// ===========================================================================
// Manager tests: Stop
// ===========================================================================

func Test_Manager_Stop_Cases(t *testing.T) {
	tests := []struct {
		name        string
		executeFunc func(ctx context.Context, query string, variables map[string]any) ([]byte, error)
		wantErr     bool
		errContains string
	}{
		{
			name: "calls stop mutation",
			executeFunc: func(ctx context.Context, query string, variables map[string]any) ([]byte, error) {
				if !strings.Contains(query, "mutation") || !strings.Contains(query, "array") || !strings.Contains(query, "stop") {
					return nil, fmt.Errorf("unexpected query: %s", query)
				}
				return []byte(`{"data":{"array":{"stop":true}}}`), nil
			},
			wantErr: false,
		},
		{
			name: "client error propagates",
			executeFunc: func(ctx context.Context, query string, variables map[string]any) ([]byte, error) {
				return nil, fmt.Errorf("timeout")
			},
			wantErr:     true,
			errContains: "timeout",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &mockGraphQLClient{executeFunc: tt.executeFunc}
			mgr := NewGraphQLArrayManager(client)

			err := mgr.Stop(context.Background())

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("error = %q, want it to contain %q", err.Error(), tt.errContains)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func Test_Manager_Stop_QueryContent(t *testing.T) {
	var capturedQuery string
	client := &mockGraphQLClient{
		executeFunc: func(ctx context.Context, query string, variables map[string]any) ([]byte, error) {
			capturedQuery = query
			return []byte(`{"data":{"array":{"stop":true}}}`), nil
		},
	}
	mgr := NewGraphQLArrayManager(client)

	if err := mgr.Stop(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(capturedQuery, "mutation") {
		t.Errorf("query should be a mutation, got: %q", capturedQuery)
	}
	if !strings.Contains(capturedQuery, "array") {
		t.Errorf("query should reference array, got: %q", capturedQuery)
	}
	if !strings.Contains(capturedQuery, "stop") {
		t.Errorf("query should reference stop, got: %q", capturedQuery)
	}
}

// ===========================================================================
// Manager tests: ParityCheck
// ===========================================================================

func Test_Manager_ParityCheck_Cases(t *testing.T) {
	tests := []struct {
		name        string
		action      string
		executeFunc func(ctx context.Context, query string, variables map[string]any) ([]byte, error)
		wantErr     bool
		errContains string
	}{
		{
			name:   "start action sends correct mutation",
			action: "start",
			executeFunc: func(ctx context.Context, query string, variables map[string]any) ([]byte, error) {
				if !strings.Contains(query, "start") || !strings.Contains(query, "correct: false") {
					return nil, fmt.Errorf("unexpected query for start: %s", query)
				}
				return []byte(`{"data":{"array":{"parityCheck":{"start":true}}}}`), nil
			},
			wantErr: false,
		},
		{
			name:   "start_correct action sends correct mutation",
			action: "start_correct",
			executeFunc: func(ctx context.Context, query string, variables map[string]any) ([]byte, error) {
				if !strings.Contains(query, "start") || !strings.Contains(query, "correct: true") {
					return nil, fmt.Errorf("unexpected query for start_correct: %s", query)
				}
				return []byte(`{"data":{"array":{"parityCheck":{"start":true}}}}`), nil
			},
			wantErr: false,
		},
		{
			name:   "pause action sends pause mutation",
			action: "pause",
			executeFunc: func(ctx context.Context, query string, variables map[string]any) ([]byte, error) {
				if !strings.Contains(query, "pause") {
					return nil, fmt.Errorf("unexpected query for pause: %s", query)
				}
				return []byte(`{"data":{"array":{"parityCheck":{"pause":true}}}}`), nil
			},
			wantErr: false,
		},
		{
			name:   "resume action sends resume mutation",
			action: "resume",
			executeFunc: func(ctx context.Context, query string, variables map[string]any) ([]byte, error) {
				if !strings.Contains(query, "resume") {
					return nil, fmt.Errorf("unexpected query for resume: %s", query)
				}
				return []byte(`{"data":{"array":{"parityCheck":{"resume":true}}}}`), nil
			},
			wantErr: false,
		},
		{
			name:   "cancel action sends cancel mutation",
			action: "cancel",
			executeFunc: func(ctx context.Context, query string, variables map[string]any) ([]byte, error) {
				if !strings.Contains(query, "cancel") {
					return nil, fmt.Errorf("unexpected query for cancel: %s", query)
				}
				return []byte(`{"data":{"array":{"parityCheck":{"cancel":true}}}}`), nil
			},
			wantErr: false,
		},
		{
			name:   "invalid action returns error without calling client",
			action: "invalid_action",
			executeFunc: func(ctx context.Context, query string, variables map[string]any) ([]byte, error) {
				t.Error("Execute should not be called for invalid action")
				return nil, nil
			},
			wantErr:     true,
			errContains: "invalid",
		},
		{
			name:   "empty action returns error without calling client",
			action: "",
			executeFunc: func(ctx context.Context, query string, variables map[string]any) ([]byte, error) {
				t.Error("Execute should not be called for empty action")
				return nil, nil
			},
			wantErr:     true,
			errContains: "invalid",
		},
		{
			name:   "client error propagates",
			action: "start",
			executeFunc: func(ctx context.Context, query string, variables map[string]any) ([]byte, error) {
				return nil, fmt.Errorf("network error")
			},
			wantErr:     true,
			errContains: "network error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &mockGraphQLClient{executeFunc: tt.executeFunc}
			mgr := NewGraphQLArrayManager(client)

			_, err := mgr.ParityCheck(context.Background(), tt.action)

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
		})
	}
}

func Test_Manager_ParityCheck_StartQueryContainsCorrectFalse(t *testing.T) {
	var capturedQuery string
	client := &mockGraphQLClient{
		executeFunc: func(ctx context.Context, query string, variables map[string]any) ([]byte, error) {
			capturedQuery = query
			return []byte(`{"data":{"array":{"parityCheck":{"start":true}}}}`), nil
		},
	}
	mgr := NewGraphQLArrayManager(client)

	if _, err := mgr.ParityCheck(context.Background(), "start"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(capturedQuery, "correct: false") {
		t.Errorf("start query should contain 'correct: false', got: %q", capturedQuery)
	}
}

func Test_Manager_ParityCheck_StartCorrectQueryContainsCorrectTrue(t *testing.T) {
	var capturedQuery string
	client := &mockGraphQLClient{
		executeFunc: func(ctx context.Context, query string, variables map[string]any) ([]byte, error) {
			capturedQuery = query
			return []byte(`{"data":{"array":{"parityCheck":{"start":true}}}}`), nil
		},
	}
	mgr := NewGraphQLArrayManager(client)

	if _, err := mgr.ParityCheck(context.Background(), "start_correct"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(capturedQuery, "correct: true") {
		t.Errorf("start_correct query should contain 'correct: true', got: %q", capturedQuery)
	}
}

// ===========================================================================
// Manager tests: context cancellation
// ===========================================================================

func Test_Manager_CancelledContext(t *testing.T) {
	client := &mockGraphQLClient{
		executeFunc: func(ctx context.Context, query string, variables map[string]any) ([]byte, error) {
			// Check if context is cancelled before processing.
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			return []byte(`{}`), nil
		},
	}
	mgr := NewGraphQLArrayManager(client)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	tests := []struct {
		name string
		call func() error
	}{
		{"Start", func() error { return mgr.Start(ctx) }},
		{"Stop", func() error { return mgr.Stop(ctx) }},
		{"ParityCheck", func() error { _, err := mgr.ParityCheck(ctx, "start"); return err }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.call()
			if err == nil {
				t.Fatal("expected error for cancelled context, got nil")
			}
		})
	}
}

// ===========================================================================
// DestructiveTools variable tests
// ===========================================================================

func Test_DestructiveTools_Length(t *testing.T) {
	const wantLen = 3
	if got := len(DestructiveTools); got != wantLen {
		t.Errorf("len(DestructiveTools) = %d, want %d", got, wantLen)
	}
}

func Test_DestructiveTools_ContainsExpectedNames(t *testing.T) {
	expected := []string{
		"array_start",
		"array_stop",
		"parity_check",
	}

	actual := make(map[string]struct{}, len(DestructiveTools))
	for _, name := range DestructiveTools {
		actual[name] = struct{}{}
	}

	for _, name := range expected {
		t.Run(name, func(t *testing.T) {
			if _, ok := actual[name]; !ok {
				t.Errorf("DestructiveTools is missing expected entry %q", name)
			}
		})
	}
}

func Test_DestructiveTools_NoUnexpectedEntries(t *testing.T) {
	expected := map[string]struct{}{
		"array_start":  {},
		"array_stop":   {},
		"parity_check": {},
	}

	for _, name := range DestructiveTools {
		if _, ok := expected[name]; !ok {
			t.Errorf("DestructiveTools contains unexpected entry %q", name)
		}
	}
}

func Test_DestructiveTools_ExactContents(t *testing.T) {
	expected := []string{
		"array_start",
		"array_stop",
		"parity_check",
	}

	got := make([]string, len(DestructiveTools))
	copy(got, DestructiveTools)
	sort.Strings(got)
	sort.Strings(expected)

	if len(got) != len(expected) {
		t.Fatalf("DestructiveTools has %d entries, want %d; got %v", len(got), len(expected), got)
	}
	for i := range expected {
		if got[i] != expected[i] {
			t.Errorf("DestructiveTools (sorted)[%d] = %q, want %q", i, got[i], expected[i])
		}
	}
}

// ===========================================================================
// Tools: Registration tests
// ===========================================================================

func Test_ArrayTools_RegistrationCount(t *testing.T) {
	mgr := &mockArrayManager{}
	confirm := safety.NewConfirmationTracker(DestructiveTools)
	audit := safety.NewAuditLogger(nil)

	regs := ArrayTools(mgr, confirm, audit)

	const wantCount = 3
	if got := len(regs); got != wantCount {
		t.Errorf("ArrayTools() returned %d registrations, want %d", got, wantCount)
	}
}

func Test_ArrayTools_ToolNames(t *testing.T) {
	mgr := &mockArrayManager{}
	confirm := safety.NewConfirmationTracker(DestructiveTools)
	audit := safety.NewAuditLogger(nil)

	regs := ArrayTools(mgr, confirm, audit)

	expected := map[string]bool{
		"array_start":  false,
		"array_stop":   false,
		"parity_check": false,
	}

	for _, r := range regs {
		if _, ok := expected[r.Tool.Name]; ok {
			expected[r.Tool.Name] = true
		} else {
			t.Errorf("unexpected tool registration: %q", r.Tool.Name)
		}
	}

	for name, found := range expected {
		if !found {
			t.Errorf("expected tool %q not found in registrations", name)
		}
	}
}

// ===========================================================================
// Tools: array_start
// ===========================================================================

func Test_Tool_ArrayStart_Cases(t *testing.T) {
	tests := []struct {
		name        string
		withToken   bool
		startErr    error
		wantConfirm bool
		wantContain string
		wantError   bool
	}{
		{
			name:        "no confirmation token returns confirmation prompt",
			withToken:   false,
			wantConfirm: true,
			wantContain: "Confirmation required",
		},
		{
			name:        "valid token and start succeeds",
			withToken:   true,
			startErr:    nil,
			wantContain: "Array start command issued successfully.",
		},
		{
			name:        "valid token and manager error",
			withToken:   true,
			startErr:    fmt.Errorf("array is already started"),
			wantContain: "error",
			wantError:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr := &mockArrayManager{
				startFunc: func(ctx context.Context) error {
					return tt.startErr
				},
			}
			confirm := safety.NewConfirmationTracker(DestructiveTools)
			audit := safety.NewAuditLogger(nil)

			regs := ArrayTools(mgr, confirm, audit)
			reg := findRegistration(t, regs, "array_start")

			args := map[string]any{}
			if tt.withToken {
				// Get a real token by requesting confirmation first.
				token := confirm.RequestConfirmation("array_start", "array", "Start the array")
				args["confirmation_token"] = token
			}

			req := newCallToolRequest("array_start", args)
			result, err := reg.Handler(context.Background(), req)

			// Handlers return (result, nil) — never (nil, error).
			if err != nil {
				t.Fatalf("handler returned unexpected error: %v", err)
			}

			text := extractResultText(t, result)

			if tt.wantConfirm {
				if !strings.Contains(text, "Confirmation required") {
					t.Errorf("expected confirmation prompt, got: %q", text)
				}
				if !strings.Contains(text, "confirmation_token") {
					t.Errorf("expected token in prompt, got: %q", text)
				}
				return
			}

			if tt.wantContain != "" && !strings.Contains(text, tt.wantContain) {
				t.Errorf("result text = %q, want it to contain %q", text, tt.wantContain)
			}
		})
	}
}

func Test_Tool_ArrayStart_ConfirmationFlow(t *testing.T) {
	mgr := &mockArrayManager{
		startFunc: func(ctx context.Context) error { return nil },
	}
	confirm := safety.NewConfirmationTracker(DestructiveTools)
	audit := safety.NewAuditLogger(nil)

	regs := ArrayTools(mgr, confirm, audit)
	reg := findRegistration(t, regs, "array_start")

	// Step 1: Call without token, get confirmation prompt.
	req1 := newCallToolRequest("array_start", map[string]any{})
	result1, err := reg.Handler(context.Background(), req1)
	if err != nil {
		t.Fatalf("handler returned unexpected error: %v", err)
	}
	text1 := extractResultText(t, result1)
	if !strings.Contains(text1, "Confirmation required") {
		t.Fatalf("expected confirmation prompt, got: %q", text1)
	}

	// Extract token from the prompt text.
	// The prompt format is: ... confirmation_token="<token>".
	tokenIdx := strings.Index(text1, "confirmation_token=")
	if tokenIdx == -1 {
		t.Fatal("could not find confirmation_token in prompt")
	}
	tokenStart := tokenIdx + len("confirmation_token=") + 1 // skip the opening quote
	tokenEnd := strings.Index(text1[tokenStart:], "\"")
	if tokenEnd == -1 {
		t.Fatal("could not find closing quote for confirmation_token")
	}
	token := text1[tokenStart : tokenStart+tokenEnd]

	// Step 2: Call with the token, should succeed.
	req2 := newCallToolRequest("array_start", map[string]any{
		"confirmation_token": token,
	})
	result2, err := reg.Handler(context.Background(), req2)
	if err != nil {
		t.Fatalf("handler returned unexpected error: %v", err)
	}
	text2 := extractResultText(t, result2)
	if !strings.Contains(text2, "Array start command issued successfully.") {
		t.Errorf("expected success message, got: %q", text2)
	}
}

// ===========================================================================
// Tools: array_stop
// ===========================================================================

func Test_Tool_ArrayStop_Cases(t *testing.T) {
	tests := []struct {
		name        string
		withToken   bool
		stopErr     error
		wantConfirm bool
		wantContain string
		wantError   bool
	}{
		{
			name:        "no confirmation token returns confirmation prompt",
			withToken:   false,
			wantConfirm: true,
			wantContain: "Confirmation required",
		},
		{
			name:        "valid token and stop succeeds",
			withToken:   true,
			stopErr:     nil,
			wantContain: "Array stop command issued successfully.",
		},
		{
			name:        "valid token and manager error",
			withToken:   true,
			stopErr:     fmt.Errorf("array is already stopped"),
			wantContain: "error",
			wantError:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr := &mockArrayManager{
				stopFunc: func(ctx context.Context) error {
					return tt.stopErr
				},
			}
			confirm := safety.NewConfirmationTracker(DestructiveTools)
			audit := safety.NewAuditLogger(nil)

			regs := ArrayTools(mgr, confirm, audit)
			reg := findRegistration(t, regs, "array_stop")

			args := map[string]any{}
			if tt.withToken {
				token := confirm.RequestConfirmation("array_stop", "array", "Stop the array")
				args["confirmation_token"] = token
			}

			req := newCallToolRequest("array_stop", args)
			result, err := reg.Handler(context.Background(), req)

			if err != nil {
				t.Fatalf("handler returned unexpected error: %v", err)
			}

			text := extractResultText(t, result)

			if tt.wantConfirm {
				if !strings.Contains(text, "Confirmation required") {
					t.Errorf("expected confirmation prompt, got: %q", text)
				}
				if !strings.Contains(text, "confirmation_token") {
					t.Errorf("expected token in prompt, got: %q", text)
				}
				return
			}

			if tt.wantContain != "" && !strings.Contains(text, tt.wantContain) {
				t.Errorf("result text = %q, want it to contain %q", text, tt.wantContain)
			}
		})
	}
}

func Test_Tool_ArrayStop_ConfirmationFlow(t *testing.T) {
	mgr := &mockArrayManager{
		stopFunc: func(ctx context.Context) error { return nil },
	}
	confirm := safety.NewConfirmationTracker(DestructiveTools)
	audit := safety.NewAuditLogger(nil)

	regs := ArrayTools(mgr, confirm, audit)
	reg := findRegistration(t, regs, "array_stop")

	// Step 1: Call without token.
	req1 := newCallToolRequest("array_stop", map[string]any{})
	result1, err := reg.Handler(context.Background(), req1)
	if err != nil {
		t.Fatalf("handler returned unexpected error: %v", err)
	}
	text1 := extractResultText(t, result1)
	if !strings.Contains(text1, "Confirmation required") {
		t.Fatalf("expected confirmation prompt, got: %q", text1)
	}

	// Extract token.
	tokenIdx := strings.Index(text1, "confirmation_token=")
	if tokenIdx == -1 {
		t.Fatal("could not find confirmation_token in prompt")
	}
	tokenStart := tokenIdx + len("confirmation_token=") + 1
	tokenEnd := strings.Index(text1[tokenStart:], "\"")
	if tokenEnd == -1 {
		t.Fatal("could not find closing quote for confirmation_token")
	}
	token := text1[tokenStart : tokenStart+tokenEnd]

	// Step 2: Call with the token.
	req2 := newCallToolRequest("array_stop", map[string]any{
		"confirmation_token": token,
	})
	result2, err := reg.Handler(context.Background(), req2)
	if err != nil {
		t.Fatalf("handler returned unexpected error: %v", err)
	}
	text2 := extractResultText(t, result2)
	if !strings.Contains(text2, "Array stop command issued successfully.") {
		t.Errorf("expected success message, got: %q", text2)
	}
}

// ===========================================================================
// Tools: parity_check
// ===========================================================================

func Test_Tool_ParityCheck_Cases(t *testing.T) {
	tests := []struct {
		name            string
		action          string
		withToken       bool
		parityCheckErr  error
		parityCheckResp string
		wantConfirm     bool
		wantContain     string
		wantError       bool
	}{
		{
			name:        "no confirmation token returns confirmation prompt",
			action:      "start",
			withToken:   false,
			wantConfirm: true,
			wantContain: "Confirmation required",
		},
		{
			name:            "valid token with start action succeeds",
			action:          "start",
			withToken:       true,
			parityCheckResp: "Parity check started",
			wantContain:     "Parity check started",
		},
		{
			name:            "valid token with start_correct action succeeds",
			action:          "start_correct",
			withToken:       true,
			parityCheckResp: "Parity check started with corrections",
			wantContain:     "Parity check started with corrections",
		},
		{
			name:            "valid token with pause action succeeds",
			action:          "pause",
			withToken:       true,
			parityCheckResp: "Parity check paused",
			wantContain:     "Parity check paused",
		},
		{
			name:            "valid token with resume action succeeds",
			action:          "resume",
			withToken:       true,
			parityCheckResp: "Parity check resumed",
			wantContain:     "Parity check resumed",
		},
		{
			name:            "valid token with cancel action succeeds",
			action:          "cancel",
			withToken:       true,
			parityCheckResp: "Parity check cancelled",
			wantContain:     "Parity check cancelled",
		},
		{
			name:        "invalid action returns error before confirmation check",
			action:      "bogus",
			withToken:   false,
			wantContain: "error",
			wantError:   true,
		},
		{
			name:           "valid token and manager error",
			action:         "start",
			withToken:      true,
			parityCheckErr: fmt.Errorf("parity disk offline"),
			wantContain:    "error",
			wantError:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr := &mockArrayManager{
				parityCheckFunc: func(ctx context.Context, action string) (string, error) {
					if tt.parityCheckErr != nil {
						return "", tt.parityCheckErr
					}
					return tt.parityCheckResp, nil
				},
			}
			confirm := safety.NewConfirmationTracker(DestructiveTools)
			audit := safety.NewAuditLogger(nil)

			regs := ArrayTools(mgr, confirm, audit)
			reg := findRegistration(t, regs, "parity_check")

			args := map[string]any{"action": tt.action}
			if tt.withToken {
				token := confirm.RequestConfirmation("parity_check", "array", "Parity check")
				args["confirmation_token"] = token
			}

			req := newCallToolRequest("parity_check", args)
			result, err := reg.Handler(context.Background(), req)

			if err != nil {
				t.Fatalf("handler returned unexpected error: %v", err)
			}

			text := extractResultText(t, result)

			if tt.wantConfirm {
				if !strings.Contains(text, "Confirmation required") {
					t.Errorf("expected confirmation prompt, got: %q", text)
				}
				return
			}

			if tt.wantContain != "" && !strings.Contains(text, tt.wantContain) {
				t.Errorf("result text = %q, want it to contain %q", text, tt.wantContain)
			}
		})
	}
}

func Test_Tool_ParityCheck_InvalidAction_NoConfirmationNeeded(t *testing.T) {
	// The parity_check handler should reject invalid actions BEFORE checking
	// confirmation, so even without a token we should get an error, not a prompt.
	mgr := &mockArrayManager{
		parityCheckFunc: func(ctx context.Context, action string) (string, error) {
			t.Error("ParityCheck should not be called for invalid action")
			return "", nil
		},
	}
	confirm := safety.NewConfirmationTracker(DestructiveTools)
	audit := safety.NewAuditLogger(nil)

	regs := ArrayTools(mgr, confirm, audit)
	reg := findRegistration(t, regs, "parity_check")

	// Call without token AND with an invalid action.
	req := newCallToolRequest("parity_check", map[string]any{"action": "invalid_action"})
	result, err := reg.Handler(context.Background(), req)

	if err != nil {
		t.Fatalf("handler returned unexpected error: %v", err)
	}

	text := extractResultText(t, result)

	// Should be an error result, not a confirmation prompt.
	if strings.Contains(text, "Confirmation required") {
		t.Errorf("invalid action should not trigger confirmation prompt, got: %q", text)
	}
	if !strings.Contains(strings.ToLower(text), "error") {
		t.Errorf("expected error message for invalid action, got: %q", text)
	}
}

func Test_Tool_ParityCheck_ValidActions(t *testing.T) {
	// Verify that all valid parity check actions are accepted.
	validActions := []string{"start", "start_correct", "pause", "resume", "cancel"}

	for _, action := range validActions {
		t.Run(action, func(t *testing.T) {
			mgr := &mockArrayManager{
				parityCheckFunc: func(ctx context.Context, a string) (string, error) {
					return fmt.Sprintf("action %s completed", a), nil
				},
			}
			confirm := safety.NewConfirmationTracker(DestructiveTools)
			audit := safety.NewAuditLogger(nil)

			regs := ArrayTools(mgr, confirm, audit)
			reg := findRegistration(t, regs, "parity_check")

			token := confirm.RequestConfirmation("parity_check", "array", "Parity check")
			req := newCallToolRequest("parity_check", map[string]any{
				"action":             action,
				"confirmation_token": token,
			})
			result, err := reg.Handler(context.Background(), req)

			if err != nil {
				t.Fatalf("handler returned unexpected error: %v", err)
			}

			text := extractResultText(t, result)
			if strings.Contains(strings.ToLower(text), "error") {
				t.Errorf("valid action %q should not produce error, got: %q", action, text)
			}
		})
	}
}

func Test_Tool_ParityCheck_ConfirmationFlow(t *testing.T) {
	mgr := &mockArrayManager{
		parityCheckFunc: func(ctx context.Context, action string) (string, error) {
			return "Parity check started", nil
		},
	}
	confirm := safety.NewConfirmationTracker(DestructiveTools)
	audit := safety.NewAuditLogger(nil)

	regs := ArrayTools(mgr, confirm, audit)
	reg := findRegistration(t, regs, "parity_check")

	// Step 1: Call without token.
	req1 := newCallToolRequest("parity_check", map[string]any{"action": "start"})
	result1, err := reg.Handler(context.Background(), req1)
	if err != nil {
		t.Fatalf("handler returned unexpected error: %v", err)
	}
	text1 := extractResultText(t, result1)
	if !strings.Contains(text1, "Confirmation required") {
		t.Fatalf("expected confirmation prompt, got: %q", text1)
	}

	// Extract token.
	tokenIdx := strings.Index(text1, "confirmation_token=")
	if tokenIdx == -1 {
		t.Fatal("could not find confirmation_token in prompt")
	}
	tokenStart := tokenIdx + len("confirmation_token=") + 1
	tokenEnd := strings.Index(text1[tokenStart:], "\"")
	if tokenEnd == -1 {
		t.Fatal("could not find closing quote for confirmation_token")
	}
	token := text1[tokenStart : tokenStart+tokenEnd]

	// Step 2: Call with the token.
	req2 := newCallToolRequest("parity_check", map[string]any{
		"action":             "start",
		"confirmation_token": token,
	})
	result2, err := reg.Handler(context.Background(), req2)
	if err != nil {
		t.Fatalf("handler returned unexpected error: %v", err)
	}
	text2 := extractResultText(t, result2)
	if strings.Contains(strings.ToLower(text2), "error") {
		t.Errorf("expected success, got: %q", text2)
	}
}

// ===========================================================================
// Tools: handler error contract (handlers return (result, nil))
// ===========================================================================

func Test_AllHandlers_ReturnNilError(t *testing.T) {
	mgr := &mockArrayManager{
		startFunc:       func(ctx context.Context) error { return fmt.Errorf("fail") },
		stopFunc:        func(ctx context.Context) error { return fmt.Errorf("fail") },
		parityCheckFunc: func(ctx context.Context, action string) (string, error) { return "", fmt.Errorf("fail") },
	}
	confirm := safety.NewConfirmationTracker(DestructiveTools)
	audit := safety.NewAuditLogger(nil)

	regs := ArrayTools(mgr, confirm, audit)

	// All handlers should return (result, nil) even when the manager errors.
	// We need tokens for destructive tools.
	toolArgs := map[string]map[string]any{
		"array_start":  {},
		"array_stop":   {},
		"parity_check": {"action": "start"},
	}

	for _, reg := range regs {
		t.Run(reg.Tool.Name, func(t *testing.T) {
			args := toolArgs[reg.Tool.Name]
			if args == nil {
				args = map[string]any{}
			}
			// Add a valid confirmation token.
			token := confirm.RequestConfirmation(reg.Tool.Name, "test", "test")
			args["confirmation_token"] = token

			req := newCallToolRequest(reg.Tool.Name, args)
			result, err := reg.Handler(context.Background(), req)

			if err != nil {
				t.Errorf("handler for %q returned non-nil error: %v", reg.Tool.Name, err)
			}
			if result == nil {
				t.Errorf("handler for %q returned nil result", reg.Tool.Name)
			}
		})
	}
}

// ===========================================================================
// Tools: tokens are single-use
// ===========================================================================

func Test_ConfirmationToken_SingleUse(t *testing.T) {
	mgr := &mockArrayManager{
		startFunc: func(ctx context.Context) error { return nil },
	}
	confirm := safety.NewConfirmationTracker(DestructiveTools)
	audit := safety.NewAuditLogger(nil)

	regs := ArrayTools(mgr, confirm, audit)
	reg := findRegistration(t, regs, "array_start")

	// Get a token.
	token := confirm.RequestConfirmation("array_start", "array", "Start the array")

	// First use should succeed.
	req1 := newCallToolRequest("array_start", map[string]any{"confirmation_token": token})
	result1, _ := reg.Handler(context.Background(), req1)
	text1 := extractResultText(t, result1)
	if !strings.Contains(text1, "Array start command issued successfully.") {
		t.Errorf("first use should succeed, got: %q", text1)
	}

	// Second use of the same token should trigger a new confirmation prompt.
	req2 := newCallToolRequest("array_start", map[string]any{"confirmation_token": token})
	result2, _ := reg.Handler(context.Background(), req2)
	text2 := extractResultText(t, result2)
	if !strings.Contains(text2, "Confirmation required") {
		t.Errorf("second use should trigger confirmation prompt, got: %q", text2)
	}
}

// ===========================================================================
// Constructor nil client tests
// ===========================================================================

func TestNewGraphQLArrayManager_NilClient(t *testing.T) {
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
	NewGraphQLArrayManager(nil)
}

// ===========================================================================
// IsValidParityAction
// ===========================================================================

func TestIsValidParityAction(t *testing.T) {
	valid := []string{"start", "start_correct", "pause", "resume", "cancel"}
	for _, a := range valid {
		if !IsValidParityAction(a) {
			t.Errorf("IsValidParityAction(%q) = false, want true", a)
		}
	}
	invalid := []string{"", "stop", "START", "invalid", "drop_table"}
	for _, a := range invalid {
		if IsValidParityAction(a) {
			t.Errorf("IsValidParityAction(%q) = true, want false", a)
		}
	}
}

// ===========================================================================
// Benchmarks
// ===========================================================================

func Benchmark_ArrayTools_Registration(b *testing.B) {
	mgr := &mockArrayManager{}
	confirm := safety.NewConfirmationTracker(DestructiveTools)
	audit := safety.NewAuditLogger(nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ArrayTools(mgr, confirm, audit)
	}
}

func Benchmark_ArrayStart_WithToken(b *testing.B) {
	mgr := &mockArrayManager{
		startFunc: func(ctx context.Context) error { return nil },
	}
	confirm := safety.NewConfirmationTracker(DestructiveTools)
	audit := safety.NewAuditLogger(nil)

	regs := ArrayTools(mgr, confirm, audit)
	reg := findRegistration(&testing.T{}, regs, "array_start")

	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		token := confirm.RequestConfirmation("array_start", "array", "Start")
		req := newCallToolRequest("array_start", map[string]any{"confirmation_token": token})
		_, _ = reg.Handler(ctx, req)
	}
}
