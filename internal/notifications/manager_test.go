package notifications

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/jamesprial/unraid-mcp/internal/graphql"
	"github.com/jamesprial/unraid-mcp/internal/safety"
	"github.com/jamesprial/unraid-mcp/internal/tools"
	"github.com/mark3labs/mcp-go/mcp"
)

// ============================================================================
// Mock: GraphQL Client (for manager tests)
// ============================================================================

// mockGraphQLClient implements graphql.Client for testing the
// GraphQLNotificationManager. Each method delegates to a function field,
// allowing per-test control of behaviour.
type mockGraphQLClient struct {
	executeFunc func(ctx context.Context, query string, variables map[string]any) ([]byte, error)
}

var _ graphql.Client = (*mockGraphQLClient)(nil)

func (m *mockGraphQLClient) Execute(ctx context.Context, query string, variables map[string]any) ([]byte, error) {
	if m.executeFunc != nil {
		return m.executeFunc(ctx, query, variables)
	}
	return nil, fmt.Errorf("mockGraphQLClient.Execute not configured")
}

// ============================================================================
// Mock: NotificationManager (for tool handler tests)
// ============================================================================

// mockNotificationManager implements NotificationManager for testing tool
// handlers in isolation from the real GraphQL client.
type mockNotificationManager struct {
	listFunc       func(ctx context.Context, filterType string, limit int) ([]Notification, error)
	archiveFunc    func(ctx context.Context, id string) error
	unarchiveFunc  func(ctx context.Context, id string) error
	deleteFunc     func(ctx context.Context, id string) error
	archiveAllFunc func(ctx context.Context) error
	deleteAllFunc  func(ctx context.Context) error
}

var _ NotificationManager = (*mockNotificationManager)(nil)

func (m *mockNotificationManager) List(ctx context.Context, filterType string, limit int) ([]Notification, error) {
	if m.listFunc != nil {
		return m.listFunc(ctx, filterType, limit)
	}
	return nil, nil
}

func (m *mockNotificationManager) Archive(ctx context.Context, id string) error {
	if m.archiveFunc != nil {
		return m.archiveFunc(ctx, id)
	}
	return nil
}

func (m *mockNotificationManager) Unarchive(ctx context.Context, id string) error {
	if m.unarchiveFunc != nil {
		return m.unarchiveFunc(ctx, id)
	}
	return nil
}

func (m *mockNotificationManager) Delete(ctx context.Context, id string) error {
	if m.deleteFunc != nil {
		return m.deleteFunc(ctx, id)
	}
	return nil
}

func (m *mockNotificationManager) ArchiveAll(ctx context.Context) error {
	if m.archiveAllFunc != nil {
		return m.archiveAllFunc(ctx)
	}
	return nil
}

func (m *mockNotificationManager) DeleteAll(ctx context.Context) error {
	if m.deleteAllFunc != nil {
		return m.deleteAllFunc(ctx)
	}
	return nil
}

// ============================================================================
// Test Helpers
// ============================================================================

// newCallToolRequest constructs an mcp.CallToolRequest suitable for invoking
// a tool handler in tests.
func newCallToolRequest(name string, args map[string]any) mcp.CallToolRequest {
	req := mcp.CallToolRequest{}
	req.Params.Name = name
	req.Params.Arguments = args
	return req
}

// extractResultText pulls the text string from the first Content element of a
// CallToolResult. It fails the test if the result is nil, has no content, or
// the first element is not a TextContent.
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

// tokenPattern matches confirmation_token="<hex>" or confirmation_token=<hex>
// in the ConfirmPrompt output text.
var tokenPattern = regexp.MustCompile(`confirmation_token="?([a-f0-9]+)"?`)

// extractToken pulls the confirmation token value from a ConfirmPrompt result
// text. It fails the test if no token is found.
func extractToken(t *testing.T, text string) string {
	t.Helper()
	matches := tokenPattern.FindStringSubmatch(text)
	if len(matches) < 2 {
		idx := strings.Index(text, "confirmation_token=")
		if idx == -1 {
			t.Fatalf("no confirmation_token= found in text:\n%s", text)
		}
		after := text[idx+len("confirmation_token="):]
		after = strings.TrimPrefix(after, "\"")
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

// findToolByName locates a Registration by tool name from a slice, failing
// the test if the tool is not found.
func findToolByName(t *testing.T, registrations []tools.Registration, name string) tools.Registration {
	t.Helper()
	for _, r := range registrations {
		if r.Tool.Name == name {
			return r
		}
	}
	t.Fatalf("tool %q not found in %d registrations", name, len(registrations))
	return tools.Registration{} // unreachable
}

// sampleNotifications returns a set of notifications for testing.
func sampleNotifications() []Notification {
	ts := "2026-02-18T10:00:00Z"
	return []Notification{
		{
			ID:          "notif-1",
			Title:       "Array Started",
			Subject:     "System",
			Description: "The array has been started.",
			Importance:  "normal",
			Timestamp:   &ts,
		},
		{
			ID:          "notif-2",
			Title:       "Disk Warning",
			Subject:     "Disk 1",
			Description: "Disk 1 temperature is high.",
			Importance:  "warning",
			Timestamp:   &ts,
		},
		{
			ID:          "notif-3",
			Title:       "Parity Error",
			Subject:     "Parity",
			Description: "Parity check found errors.",
			Importance:  "alert",
			Timestamp:   nil,
		},
	}
}

// ============================================================================
// Compile-time interface checks
// ============================================================================

func Test_CompileTimeInterfaceCheck_GraphQLNotificationManager(t *testing.T) {
	var _ NotificationManager = (*GraphQLNotificationManager)(nil)
}

func Test_CompileTimeInterfaceCheck_MockNotificationManager(t *testing.T) {
	var _ NotificationManager = (*mockNotificationManager)(nil)
}

func Test_CompileTimeInterfaceCheck_MockGraphQLClient(t *testing.T) {
	var _ graphql.Client = (*mockGraphQLClient)(nil)
}

// ============================================================================
// Manager Tests: GraphQLNotificationManager
// ============================================================================

func Test_Manager_List_Cases(t *testing.T) {
	ts := "2026-02-18T10:00:00Z"

	tests := []struct {
		name       string
		filterType string
		limit      int
		response   string
		clientErr  error
		wantErr    bool
		wantCount  int
		validate   func(t *testing.T, notifs []Notification)
	}{
		{
			name:       "returns parsed notifications",
			filterType: "UNREAD",
			limit:      20,
			response: `{"notifications":{"list":[` +
				`{"id":"n1","title":"Test","subject":"Sys","description":"desc","importance":"normal","timestamp":"` + ts + `"}` +
				`]}}`,
			wantCount: 1,
			validate: func(t *testing.T, notifs []Notification) {
				t.Helper()
				if notifs[0].ID != "n1" {
					t.Errorf("ID = %q, want %q", notifs[0].ID, "n1")
				}
				if notifs[0].Title != "Test" {
					t.Errorf("Title = %q, want %q", notifs[0].Title, "Test")
				}
				if notifs[0].Subject != "Sys" {
					t.Errorf("Subject = %q, want %q", notifs[0].Subject, "Sys")
				}
				if notifs[0].Description != "desc" {
					t.Errorf("Description = %q, want %q", notifs[0].Description, "desc")
				}
				if notifs[0].Importance != "normal" {
					t.Errorf("Importance = %q, want %q", notifs[0].Importance, "normal")
				}
				if notifs[0].Timestamp == nil || *notifs[0].Timestamp != ts {
					t.Errorf("Timestamp = %v, want %q", notifs[0].Timestamp, ts)
				}
			},
		},
		{
			name:       "empty list returns empty slice",
			filterType: "UNREAD",
			limit:      20,
			response:   `{"notifications":{"list":[]}}`,
			wantCount:  0,
		},
		{
			name:       "client error returns error",
			filterType: "UNREAD",
			limit:      20,
			clientErr:  fmt.Errorf("connection refused"),
			wantErr:    true,
		},
		{
			name:       "filter type is passed to query",
			filterType: "ARCHIVE",
			limit:      10,
			response:   `{"notifications":{"list":[]}}`,
			wantCount:  0,
		},
		{
			name:       "limit is passed to query",
			filterType: "UNREAD",
			limit:      5,
			response:   `{"notifications":{"list":[]}}`,
			wantCount:  0,
		},
		{
			name:       "multiple notifications parsed correctly",
			filterType: "ALL",
			limit:      50,
			response: `{"notifications":{"list":[` +
				`{"id":"n1","title":"T1","subject":"S1","description":"D1","importance":"normal","timestamp":null},` +
				`{"id":"n2","title":"T2","subject":"S2","description":"D2","importance":"warning","timestamp":"` + ts + `"},` +
				`{"id":"n3","title":"T3","subject":"S3","description":"D3","importance":"alert","timestamp":"` + ts + `"}` +
				`]}}`,
			wantCount: 3,
			validate: func(t *testing.T, notifs []Notification) {
				t.Helper()
				if notifs[0].Timestamp != nil {
					t.Errorf("notifs[0].Timestamp should be nil, got %v", *notifs[0].Timestamp)
				}
				if notifs[2].Importance != "alert" {
					t.Errorf("notifs[2].Importance = %q, want %q", notifs[2].Importance, "alert")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedQuery string
			var capturedVars map[string]any

			client := &mockGraphQLClient{
				executeFunc: func(ctx context.Context, query string, variables map[string]any) ([]byte, error) {
					capturedQuery = query
					capturedVars = variables
					if tt.clientErr != nil {
						return nil, tt.clientErr
					}
					return []byte(tt.response), nil
				},
			}

			mgr := NewGraphQLNotificationManager(client)
			ctx := context.Background()

			notifs, err := mgr.List(ctx, tt.filterType, tt.limit)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(notifs) != tt.wantCount {
				t.Errorf("len(notifs) = %d, want %d", len(notifs), tt.wantCount)
			}

			// Verify the query was called with expected filter/limit.
			if tt.filterType != "" && !strings.Contains(capturedQuery, "notification") && capturedVars == nil {
				// Just verify Execute was called; actual query verification is
				// lenient since we do not want to couple to query text.
			}
			_ = capturedQuery // suppress unused warning

			if tt.validate != nil {
				tt.validate(t, notifs)
			}
		})
	}
}

func Test_Manager_List_QueryContainsFilterType(t *testing.T) {
	var capturedQuery string
	var capturedVars map[string]any

	client := &mockGraphQLClient{
		executeFunc: func(ctx context.Context, query string, variables map[string]any) ([]byte, error) {
			capturedQuery = query
			capturedVars = variables
			return []byte(`{"notifications":{"list":[]}}`), nil
		},
	}

	mgr := NewGraphQLNotificationManager(client)
	_, _ = mgr.List(context.Background(), "ARCHIVE", 10)

	// The filter type should appear somewhere -- either in the query or variables.
	queryOrVarsContains := strings.Contains(capturedQuery, "ARCHIVE")
	if capturedVars != nil {
		for _, v := range capturedVars {
			if s, ok := v.(string); ok && s == "ARCHIVE" {
				queryOrVarsContains = true
			}
		}
	}
	if !queryOrVarsContains {
		t.Error("expected filter type ARCHIVE to appear in query or variables")
	}
}

func Test_Manager_List_QueryContainsLimit(t *testing.T) {
	var capturedQuery string
	var capturedVars map[string]any

	client := &mockGraphQLClient{
		executeFunc: func(ctx context.Context, query string, variables map[string]any) ([]byte, error) {
			capturedQuery = query
			capturedVars = variables
			return []byte(`{"notifications":{"list":[]}}`), nil
		},
	}

	mgr := NewGraphQLNotificationManager(client)
	_, _ = mgr.List(context.Background(), "UNREAD", 42)

	// The limit should appear somewhere -- either in the query or variables.
	queryOrVarsContains := strings.Contains(capturedQuery, "42")
	if capturedVars != nil {
		for _, v := range capturedVars {
			switch v := v.(type) {
			case int:
				if v == 42 {
					queryOrVarsContains = true
				}
			case float64:
				if v == 42 {
					queryOrVarsContains = true
				}
			}
		}
	}
	if !queryOrVarsContains {
		t.Error("expected limit 42 to appear in query or variables")
	}
}

func Test_Manager_Archive_Cases(t *testing.T) {
	tests := []struct {
		name      string
		id        string
		clientErr error
		wantErr   bool
	}{
		{
			name: "archive calls mutation with id",
			id:   "notif-1",
		},
		{
			name:      "archive client error returns error",
			id:        "notif-1",
			clientErr: fmt.Errorf("network error"),
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedQuery string

			client := &mockGraphQLClient{
				executeFunc: func(ctx context.Context, query string, variables map[string]any) ([]byte, error) {
					capturedQuery = query
					if tt.clientErr != nil {
						return nil, tt.clientErr
					}
					return []byte(`{"data":{}}`), nil
				},
			}

			mgr := NewGraphQLNotificationManager(client)
			err := mgr.Archive(context.Background(), tt.id)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Verify mutation was called (query string should contain mutation-related text).
			if capturedQuery == "" {
				t.Error("expected Execute to be called with a query")
			}
		})
	}
}

func Test_Manager_Unarchive_CallsMutation(t *testing.T) {
	var called bool

	client := &mockGraphQLClient{
		executeFunc: func(ctx context.Context, query string, variables map[string]any) ([]byte, error) {
			called = true
			return []byte(`{"data":{}}`), nil
		},
	}

	mgr := NewGraphQLNotificationManager(client)
	err := mgr.Unarchive(context.Background(), "notif-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("expected Execute to be called for Unarchive mutation")
	}
}

func Test_Manager_Delete_CallsMutation(t *testing.T) {
	var called bool

	client := &mockGraphQLClient{
		executeFunc: func(ctx context.Context, query string, variables map[string]any) ([]byte, error) {
			called = true
			return []byte(`{"data":{}}`), nil
		},
	}

	mgr := NewGraphQLNotificationManager(client)
	err := mgr.Delete(context.Background(), "notif-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("expected Execute to be called for Delete mutation")
	}
}

func Test_Manager_ArchiveAll_CallsMutation(t *testing.T) {
	var called bool

	client := &mockGraphQLClient{
		executeFunc: func(ctx context.Context, query string, variables map[string]any) ([]byte, error) {
			called = true
			return []byte(`{"data":{}}`), nil
		},
	}

	mgr := NewGraphQLNotificationManager(client)
	err := mgr.ArchiveAll(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("expected Execute to be called for ArchiveAll mutation")
	}
}

func Test_Manager_DeleteAll_CallsMutation(t *testing.T) {
	var called bool

	client := &mockGraphQLClient{
		executeFunc: func(ctx context.Context, query string, variables map[string]any) ([]byte, error) {
			called = true
			return []byte(`{"data":{}}`), nil
		},
	}

	mgr := NewGraphQLNotificationManager(client)
	err := mgr.DeleteAll(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("expected Execute to be called for DeleteAll mutation")
	}
}

func Test_Manager_Unarchive_ClientError(t *testing.T) {
	client := &mockGraphQLClient{
		executeFunc: func(ctx context.Context, query string, variables map[string]any) ([]byte, error) {
			return nil, fmt.Errorf("timeout")
		},
	}

	mgr := NewGraphQLNotificationManager(client)
	err := mgr.Unarchive(context.Background(), "notif-1")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func Test_Manager_Delete_ClientError(t *testing.T) {
	client := &mockGraphQLClient{
		executeFunc: func(ctx context.Context, query string, variables map[string]any) ([]byte, error) {
			return nil, fmt.Errorf("timeout")
		},
	}

	mgr := NewGraphQLNotificationManager(client)
	err := mgr.Delete(context.Background(), "notif-1")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func Test_Manager_ArchiveAll_ClientError(t *testing.T) {
	client := &mockGraphQLClient{
		executeFunc: func(ctx context.Context, query string, variables map[string]any) ([]byte, error) {
			return nil, fmt.Errorf("timeout")
		},
	}

	mgr := NewGraphQLNotificationManager(client)
	err := mgr.ArchiveAll(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func Test_Manager_DeleteAll_ClientError(t *testing.T) {
	client := &mockGraphQLClient{
		executeFunc: func(ctx context.Context, query string, variables map[string]any) ([]byte, error) {
			return nil, fmt.Errorf("timeout")
		},
	}

	mgr := NewGraphQLNotificationManager(client)
	err := mgr.DeleteAll(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func Test_Manager_CancelledContext_Cases(t *testing.T) {
	client := &mockGraphQLClient{
		executeFunc: func(ctx context.Context, query string, variables map[string]any) ([]byte, error) {
			return nil, ctx.Err()
		},
	}

	mgr := NewGraphQLNotificationManager(client)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	methods := []struct {
		name string
		call func() error
	}{
		{"List", func() error { _, err := mgr.List(ctx, "UNREAD", 20); return err }},
		{"Archive", func() error { return mgr.Archive(ctx, "id") }},
		{"Unarchive", func() error { return mgr.Unarchive(ctx, "id") }},
		{"Delete", func() error { return mgr.Delete(ctx, "id") }},
		{"ArchiveAll", func() error { return mgr.ArchiveAll(ctx) }},
		{"DeleteAll", func() error { return mgr.DeleteAll(ctx) }},
	}

	for _, m := range methods {
		t.Run(m.name, func(t *testing.T) {
			err := m.call()
			if err == nil {
				t.Fatalf("%s: expected error for cancelled context, got nil", m.name)
			}
		})
	}
}

// ============================================================================
// Type Tests
// ============================================================================

func Test_Notification_ZeroValue(t *testing.T) {
	var n Notification
	if n.ID != "" {
		t.Errorf("zero Notification.ID = %q, want empty", n.ID)
	}
	if n.Title != "" {
		t.Errorf("zero Notification.Title = %q, want empty", n.Title)
	}
	if n.Subject != "" {
		t.Errorf("zero Notification.Subject = %q, want empty", n.Subject)
	}
	if n.Description != "" {
		t.Errorf("zero Notification.Description = %q, want empty", n.Description)
	}
	if n.Importance != "" {
		t.Errorf("zero Notification.Importance = %q, want empty", n.Importance)
	}
	if n.Timestamp != nil {
		t.Errorf("zero Notification.Timestamp = %v, want nil", n.Timestamp)
	}
}

func Test_Notification_JSONRoundTrip(t *testing.T) {
	ts := "2026-02-18T12:00:00Z"
	original := Notification{
		ID:          "abc123",
		Title:       "Test Notification",
		Subject:     "System",
		Description: "A test notification.",
		Importance:  "warning",
		Timestamp:   &ts,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded Notification
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if decoded.ID != original.ID {
		t.Errorf("ID = %q, want %q", decoded.ID, original.ID)
	}
	if decoded.Title != original.Title {
		t.Errorf("Title = %q, want %q", decoded.Title, original.Title)
	}
	if decoded.Subject != original.Subject {
		t.Errorf("Subject = %q, want %q", decoded.Subject, original.Subject)
	}
	if decoded.Description != original.Description {
		t.Errorf("Description = %q, want %q", decoded.Description, original.Description)
	}
	if decoded.Importance != original.Importance {
		t.Errorf("Importance = %q, want %q", decoded.Importance, original.Importance)
	}
	if decoded.Timestamp == nil || *decoded.Timestamp != ts {
		t.Errorf("Timestamp = %v, want %q", decoded.Timestamp, ts)
	}
}

func Test_Notification_JSON_NilTimestamp(t *testing.T) {
	original := Notification{
		ID:        "id1",
		Timestamp: nil,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded Notification
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if decoded.Timestamp != nil {
		t.Errorf("Timestamp = %v, want nil", decoded.Timestamp)
	}
}

// ============================================================================
// DestructiveTools Tests
// ============================================================================

func Test_DestructiveTools_ContainsExpectedNames(t *testing.T) {
	expected := []string{"notifications_manage"}

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

func Test_DestructiveTools_Length(t *testing.T) {
	if got := len(DestructiveTools); got != 1 {
		t.Errorf("len(DestructiveTools) = %d, want 1", got)
	}
}

func Test_DestructiveTools_ExactContents(t *testing.T) {
	expected := []string{"notifications_manage"}

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

// ============================================================================
// NotificationTools Registration Tests
// ============================================================================

func Test_NotificationTools_RegistrationCount(t *testing.T) {
	mgr := &mockNotificationManager{}
	confirm := safety.NewConfirmationTracker(DestructiveTools)
	regs := NotificationTools(mgr, confirm, nil)

	if len(regs) != 2 {
		t.Errorf("NotificationTools returned %d registrations, want 2", len(regs))
	}
}

func Test_NotificationTools_ToolNames(t *testing.T) {
	mgr := &mockNotificationManager{}
	confirm := safety.NewConfirmationTracker(DestructiveTools)
	regs := NotificationTools(mgr, confirm, nil)

	names := make(map[string]bool)
	for _, r := range regs {
		names[r.Tool.Name] = true
	}

	expectedNames := []string{"notifications_list", "notifications_manage"}
	for _, name := range expectedNames {
		if !names[name] {
			t.Errorf("expected tool %q not found in registrations", name)
		}
	}

	// No unexpected tools.
	for name := range names {
		found := false
		for _, expected := range expectedNames {
			if name == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("unexpected tool %q found in registrations", name)
		}
	}
}

// ============================================================================
// notifications_list Tool Handler Tests
// ============================================================================

func Test_NotificationsList_DefaultFilter(t *testing.T) {
	var capturedFilterType string
	var capturedLimit int

	mgr := &mockNotificationManager{
		listFunc: func(ctx context.Context, filterType string, limit int) ([]Notification, error) {
			capturedFilterType = filterType
			capturedLimit = limit
			return nil, nil
		},
	}

	confirm := safety.NewConfirmationTracker(DestructiveTools)
	regs := NotificationTools(mgr, confirm, nil)
	listTool := findToolByName(t, regs, "notifications_list")

	req := newCallToolRequest("notifications_list", map[string]any{})
	result, err := listTool.Handler(context.Background(), req)

	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if result == nil {
		t.Fatal("handler returned nil result")
	}

	if capturedFilterType != "UNREAD" {
		t.Errorf("filter_type = %q, want %q", capturedFilterType, "UNREAD")
	}
	_ = capturedLimit
}

func Test_NotificationsList_DefaultLimit(t *testing.T) {
	var capturedLimit int

	mgr := &mockNotificationManager{
		listFunc: func(ctx context.Context, filterType string, limit int) ([]Notification, error) {
			capturedLimit = limit
			return nil, nil
		},
	}

	confirm := safety.NewConfirmationTracker(DestructiveTools)
	regs := NotificationTools(mgr, confirm, nil)
	listTool := findToolByName(t, regs, "notifications_list")

	req := newCallToolRequest("notifications_list", map[string]any{})
	result, err := listTool.Handler(context.Background(), req)

	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if result == nil {
		t.Fatal("handler returned nil result")
	}

	if capturedLimit != 20 {
		t.Errorf("limit = %d, want 20", capturedLimit)
	}
}

func Test_NotificationsList_CustomFilterAndLimit(t *testing.T) {
	var capturedFilterType string
	var capturedLimit int

	mgr := &mockNotificationManager{
		listFunc: func(ctx context.Context, filterType string, limit int) ([]Notification, error) {
			capturedFilterType = filterType
			capturedLimit = limit
			return nil, nil
		},
	}

	confirm := safety.NewConfirmationTracker(DestructiveTools)
	regs := NotificationTools(mgr, confirm, nil)
	listTool := findToolByName(t, regs, "notifications_list")

	req := newCallToolRequest("notifications_list", map[string]any{
		"filter_type": "ARCHIVE",
		"limit":       float64(50), // JSON numbers decode as float64
	})
	result, err := listTool.Handler(context.Background(), req)

	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if result == nil {
		t.Fatal("handler returned nil result")
	}

	if capturedFilterType != "ARCHIVE" {
		t.Errorf("filter_type = %q, want %q", capturedFilterType, "ARCHIVE")
	}
	if capturedLimit != 50 {
		t.Errorf("limit = %d, want 50", capturedLimit)
	}
}

func Test_NotificationsList_EmptyList(t *testing.T) {
	mgr := &mockNotificationManager{
		listFunc: func(ctx context.Context, filterType string, limit int) ([]Notification, error) {
			return []Notification{}, nil
		},
	}

	confirm := safety.NewConfirmationTracker(DestructiveTools)
	regs := NotificationTools(mgr, confirm, nil)
	listTool := findToolByName(t, regs, "notifications_list")

	req := newCallToolRequest("notifications_list", map[string]any{})
	result, err := listTool.Handler(context.Background(), req)

	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	text := extractResultText(t, result)
	// When the list is empty, the output should indicate no notifications.
	lowerText := strings.ToLower(text)
	if !strings.Contains(lowerText, "no ") && !strings.Contains(lowerText, "0 ") && !strings.Contains(lowerText, "empty") && !strings.Contains(lowerText, "[]") {
		t.Errorf("expected text indicating no notifications, got:\n%s", text)
	}
}

func Test_NotificationsList_FormattedOutput(t *testing.T) {
	mgr := &mockNotificationManager{
		listFunc: func(ctx context.Context, filterType string, limit int) ([]Notification, error) {
			return sampleNotifications(), nil
		},
	}

	confirm := safety.NewConfirmationTracker(DestructiveTools)
	regs := NotificationTools(mgr, confirm, nil)
	listTool := findToolByName(t, regs, "notifications_list")

	req := newCallToolRequest("notifications_list", map[string]any{})
	result, err := listTool.Handler(context.Background(), req)

	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	text := extractResultText(t, result)

	// The formatted output should contain notification titles or IDs.
	if !strings.Contains(text, "Array Started") && !strings.Contains(text, "notif-1") {
		t.Errorf("expected output to contain notification title or ID, got:\n%s", text)
	}
	if !strings.Contains(text, "Disk Warning") && !strings.Contains(text, "notif-2") {
		t.Errorf("expected output to contain second notification, got:\n%s", text)
	}
}

func Test_NotificationsList_ManagerError(t *testing.T) {
	mgr := &mockNotificationManager{
		listFunc: func(ctx context.Context, filterType string, limit int) ([]Notification, error) {
			return nil, fmt.Errorf("graphql: connection refused")
		},
	}

	confirm := safety.NewConfirmationTracker(DestructiveTools)
	regs := NotificationTools(mgr, confirm, nil)
	listTool := findToolByName(t, regs, "notifications_list")

	req := newCallToolRequest("notifications_list", map[string]any{})
	result, err := listTool.Handler(context.Background(), req)

	// Handlers return (result, nil) -- never (nil, error).
	if err != nil {
		t.Fatalf("handler returned non-nil error: %v", err)
	}
	if result == nil {
		t.Fatal("handler returned nil result on error")
	}

	text := extractResultText(t, result)
	if !strings.Contains(text, "error") {
		t.Errorf("expected error text in result, got:\n%s", text)
	}
}

func Test_NotificationsList_HandlerReturnsNilError(t *testing.T) {
	mgr := &mockNotificationManager{
		listFunc: func(ctx context.Context, filterType string, limit int) ([]Notification, error) {
			return sampleNotifications(), nil
		},
	}

	confirm := safety.NewConfirmationTracker(DestructiveTools)
	regs := NotificationTools(mgr, confirm, nil)
	listTool := findToolByName(t, regs, "notifications_list")

	req := newCallToolRequest("notifications_list", map[string]any{})
	_, err := listTool.Handler(context.Background(), req)

	if err != nil {
		t.Errorf("handler should return nil error, got: %v", err)
	}
}

// ============================================================================
// notifications_manage Tool Handler Tests
// ============================================================================

func Test_NotificationsManage_ArchiveSucceeds(t *testing.T) {
	var archiveCalled bool
	var archivedID string

	mgr := &mockNotificationManager{
		archiveFunc: func(ctx context.Context, id string) error {
			archiveCalled = true
			archivedID = id
			return nil
		},
	}

	confirm := safety.NewConfirmationTracker(DestructiveTools)
	regs := NotificationTools(mgr, confirm, nil)
	manageTool := findToolByName(t, regs, "notifications_manage")

	req := newCallToolRequest("notifications_manage", map[string]any{
		"action": "archive",
		"id":     "notif-1",
	})
	result, err := manageTool.Handler(context.Background(), req)

	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	text := extractResultText(t, result)

	if !archiveCalled {
		t.Error("expected Archive to be called")
	}
	if archivedID != "notif-1" {
		t.Errorf("archived ID = %q, want %q", archivedID, "notif-1")
	}

	lowerText := strings.ToLower(text)
	if !strings.Contains(lowerText, "success") && !strings.Contains(lowerText, "archived") && !strings.Contains(lowerText, "ok") {
		t.Errorf("expected success message, got:\n%s", text)
	}
}

func Test_NotificationsManage_UnarchiveSucceeds(t *testing.T) {
	var called bool

	mgr := &mockNotificationManager{
		unarchiveFunc: func(ctx context.Context, id string) error {
			called = true
			return nil
		},
	}

	confirm := safety.NewConfirmationTracker(DestructiveTools)
	regs := NotificationTools(mgr, confirm, nil)
	manageTool := findToolByName(t, regs, "notifications_manage")

	req := newCallToolRequest("notifications_manage", map[string]any{
		"action": "unarchive",
		"id":     "notif-1",
	})
	result, err := manageTool.Handler(context.Background(), req)

	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	text := extractResultText(t, result)
	if !called {
		t.Error("expected Unarchive to be called")
	}

	lowerText := strings.ToLower(text)
	if !strings.Contains(lowerText, "success") && !strings.Contains(lowerText, "unarchive") && !strings.Contains(lowerText, "ok") {
		t.Errorf("expected success message, got:\n%s", text)
	}
}

func Test_NotificationsManage_DeleteNoConfirmation(t *testing.T) {
	mgr := &mockNotificationManager{
		deleteFunc: func(ctx context.Context, id string) error {
			t.Error("Delete should not be called without confirmation")
			return nil
		},
	}

	confirm := safety.NewConfirmationTracker(DestructiveTools)
	regs := NotificationTools(mgr, confirm, nil)
	manageTool := findToolByName(t, regs, "notifications_manage")

	req := newCallToolRequest("notifications_manage", map[string]any{
		"action": "delete",
		"id":     "notif-1",
	})
	result, err := manageTool.Handler(context.Background(), req)

	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	text := extractResultText(t, result)

	// Should contain a confirmation prompt.
	if !strings.Contains(text, "confirmation_token") && !strings.Contains(text, "Confirmation") {
		t.Errorf("expected confirmation prompt, got:\n%s", text)
	}
}

func Test_NotificationsManage_DeleteAllNoConfirmation(t *testing.T) {
	mgr := &mockNotificationManager{
		deleteAllFunc: func(ctx context.Context) error {
			t.Error("DeleteAll should not be called without confirmation")
			return nil
		},
	}

	confirm := safety.NewConfirmationTracker(DestructiveTools)
	regs := NotificationTools(mgr, confirm, nil)
	manageTool := findToolByName(t, regs, "notifications_manage")

	req := newCallToolRequest("notifications_manage", map[string]any{
		"action": "delete_all",
	})
	result, err := manageTool.Handler(context.Background(), req)

	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	text := extractResultText(t, result)

	// Should contain a confirmation prompt.
	if !strings.Contains(text, "confirmation_token") && !strings.Contains(text, "Confirmation") {
		t.Errorf("expected confirmation prompt, got:\n%s", text)
	}
}

func Test_NotificationsManage_DeleteWithValidToken(t *testing.T) {
	var deletedID string

	mgr := &mockNotificationManager{
		deleteFunc: func(ctx context.Context, id string) error {
			deletedID = id
			return nil
		},
	}

	confirm := safety.NewConfirmationTracker(DestructiveTools)
	regs := NotificationTools(mgr, confirm, nil)
	manageTool := findToolByName(t, regs, "notifications_manage")

	// First call: get confirmation token.
	req1 := newCallToolRequest("notifications_manage", map[string]any{
		"action": "delete",
		"id":     "notif-1",
	})
	result1, err := manageTool.Handler(context.Background(), req1)
	if err != nil {
		t.Fatalf("first call returned error: %v", err)
	}
	text1 := extractResultText(t, result1)
	token := extractToken(t, text1)

	// Second call: use the token.
	req2 := newCallToolRequest("notifications_manage", map[string]any{
		"action":             "delete",
		"id":                 "notif-1",
		"confirmation_token": token,
	})
	result2, err := manageTool.Handler(context.Background(), req2)
	if err != nil {
		t.Fatalf("second call returned error: %v", err)
	}

	text2 := extractResultText(t, result2)
	if deletedID != "notif-1" {
		t.Errorf("deleted ID = %q, want %q", deletedID, "notif-1")
	}

	lowerText := strings.ToLower(text2)
	if !strings.Contains(lowerText, "success") && !strings.Contains(lowerText, "deleted") && !strings.Contains(lowerText, "ok") {
		t.Errorf("expected success message, got:\n%s", text2)
	}
}

func Test_NotificationsManage_ArchiveAllSucceeds(t *testing.T) {
	var called bool

	mgr := &mockNotificationManager{
		archiveAllFunc: func(ctx context.Context) error {
			called = true
			return nil
		},
	}

	confirm := safety.NewConfirmationTracker(DestructiveTools)
	regs := NotificationTools(mgr, confirm, nil)
	manageTool := findToolByName(t, regs, "notifications_manage")

	req := newCallToolRequest("notifications_manage", map[string]any{
		"action": "archive_all",
	})
	result, err := manageTool.Handler(context.Background(), req)

	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	text := extractResultText(t, result)

	// archive_all may or may not need confirmation -- test verifies it eventually succeeds.
	// If it returns a confirmation prompt, consume the token.
	if strings.Contains(text, "confirmation_token") {
		token := extractToken(t, text)
		req2 := newCallToolRequest("notifications_manage", map[string]any{
			"action":             "archive_all",
			"confirmation_token": token,
		})
		result2, err := manageTool.Handler(context.Background(), req2)
		if err != nil {
			t.Fatalf("confirmed call returned error: %v", err)
		}
		text = extractResultText(t, result2)
	}

	if !called {
		t.Error("expected ArchiveAll to be called")
	}

	lowerText := strings.ToLower(text)
	if !strings.Contains(lowerText, "success") && !strings.Contains(lowerText, "archived") && !strings.Contains(lowerText, "ok") {
		t.Errorf("expected success message, got:\n%s", text)
	}
}

func Test_NotificationsManage_DeleteAllWithValidToken(t *testing.T) {
	var called bool

	mgr := &mockNotificationManager{
		deleteAllFunc: func(ctx context.Context) error {
			called = true
			return nil
		},
	}

	confirm := safety.NewConfirmationTracker(DestructiveTools)
	regs := NotificationTools(mgr, confirm, nil)
	manageTool := findToolByName(t, regs, "notifications_manage")

	// First call: get confirmation token.
	req1 := newCallToolRequest("notifications_manage", map[string]any{
		"action": "delete_all",
	})
	result1, err := manageTool.Handler(context.Background(), req1)
	if err != nil {
		t.Fatalf("first call returned error: %v", err)
	}
	text1 := extractResultText(t, result1)
	token := extractToken(t, text1)

	// Second call: use the token.
	req2 := newCallToolRequest("notifications_manage", map[string]any{
		"action":             "delete_all",
		"confirmation_token": token,
	})
	result2, err := manageTool.Handler(context.Background(), req2)
	if err != nil {
		t.Fatalf("second call returned error: %v", err)
	}

	text2 := extractResultText(t, result2)
	if !called {
		t.Error("expected DeleteAll to be called")
	}

	lowerText := strings.ToLower(text2)
	if !strings.Contains(lowerText, "success") && !strings.Contains(lowerText, "deleted") && !strings.Contains(lowerText, "ok") {
		t.Errorf("expected success message, got:\n%s", text2)
	}
}

func Test_NotificationsManage_UnknownAction(t *testing.T) {
	mgr := &mockNotificationManager{}

	confirm := safety.NewConfirmationTracker(DestructiveTools)
	regs := NotificationTools(mgr, confirm, nil)
	manageTool := findToolByName(t, regs, "notifications_manage")

	req := newCallToolRequest("notifications_manage", map[string]any{
		"action": "explode",
		"id":     "notif-1",
	})
	result, err := manageTool.Handler(context.Background(), req)

	// Handler should return (result, nil) not (nil, error).
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if result == nil {
		t.Fatal("handler returned nil result")
	}

	text := extractResultText(t, result)
	if !strings.Contains(strings.ToLower(text), "error") && !strings.Contains(strings.ToLower(text), "unknown") && !strings.Contains(strings.ToLower(text), "invalid") {
		t.Errorf("expected error about unknown action, got:\n%s", text)
	}
}

func Test_NotificationsManage_SingleItemActionWithoutID(t *testing.T) {
	mgr := &mockNotificationManager{}

	confirm := safety.NewConfirmationTracker(DestructiveTools)
	regs := NotificationTools(mgr, confirm, nil)
	manageTool := findToolByName(t, regs, "notifications_manage")

	singleItemActions := []string{"archive", "unarchive", "delete"}

	for _, action := range singleItemActions {
		t.Run(action+"_without_id", func(t *testing.T) {
			req := newCallToolRequest("notifications_manage", map[string]any{
				"action": action,
			})
			result, err := manageTool.Handler(context.Background(), req)

			if err != nil {
				t.Fatalf("handler returned error: %v", err)
			}
			if result == nil {
				t.Fatal("handler returned nil result")
			}

			text := extractResultText(t, result)
			if !strings.Contains(strings.ToLower(text), "error") && !strings.Contains(strings.ToLower(text), "id") && !strings.Contains(strings.ToLower(text), "required") {
				t.Errorf("expected error about missing id, got:\n%s", text)
			}
		})
	}
}

func Test_NotificationsManage_ManagerError(t *testing.T) {
	mgr := &mockNotificationManager{
		archiveFunc: func(ctx context.Context, id string) error {
			return fmt.Errorf("graphql: internal server error")
		},
	}

	confirm := safety.NewConfirmationTracker(DestructiveTools)
	regs := NotificationTools(mgr, confirm, nil)
	manageTool := findToolByName(t, regs, "notifications_manage")

	req := newCallToolRequest("notifications_manage", map[string]any{
		"action": "archive",
		"id":     "notif-1",
	})
	result, err := manageTool.Handler(context.Background(), req)

	// Handler returns (result, nil) -- never (nil, error).
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if result == nil {
		t.Fatal("handler returned nil result")
	}

	text := extractResultText(t, result)
	if !strings.Contains(strings.ToLower(text), "error") {
		t.Errorf("expected error text in result, got:\n%s", text)
	}
}

func Test_NotificationsManage_HandlerAlwaysReturnsNilError(t *testing.T) {
	mgr := &mockNotificationManager{
		archiveFunc: func(ctx context.Context, id string) error {
			return fmt.Errorf("fail")
		},
		deleteFunc: func(ctx context.Context, id string) error {
			return fmt.Errorf("fail")
		},
	}

	confirm := safety.NewConfirmationTracker(DestructiveTools)
	regs := NotificationTools(mgr, confirm, nil)
	manageTool := findToolByName(t, regs, "notifications_manage")

	tests := []struct {
		name string
		args map[string]any
	}{
		{
			name: "archive error",
			args: map[string]any{"action": "archive", "id": "x"},
		},
		{
			name: "unknown action",
			args: map[string]any{"action": "bogus"},
		},
		{
			name: "missing action",
			args: map[string]any{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := newCallToolRequest("notifications_manage", tt.args)
			_, err := manageTool.Handler(context.Background(), req)
			if err != nil {
				t.Errorf("handler returned non-nil error: %v", err)
			}
		})
	}
}

// ============================================================================
// Security Tests: Input Validation
// ============================================================================

// TestList_InvalidFilterType verifies that List rejects filter types not in
// the validFilterTypes allowlist. This prevents GraphQL query injection via
// the filter type parameter which is interpolated directly into the query.
func TestList_InvalidFilterType(t *testing.T) {
	// The mock client should never be called for invalid filter types because
	// validation must reject the input before any network call.
	client := &mockGraphQLClient{
		executeFunc: func(ctx context.Context, query string, variables map[string]any) ([]byte, error) {
			t.Error("Execute should not be called for invalid filter type")
			return []byte(`{"notifications":{"list":[]}}`), nil
		},
	}

	mgr := NewGraphQLNotificationManager(client)
	ctx := context.Background()

	tests := []struct {
		name       string
		filterType string
	}{
		{
			name:       "arbitrary invalid type",
			filterType: "INVALID",
		},
		{
			name:       "SQL injection attempt",
			filterType: "'; DROP TABLE;",
		},
		{
			name:       "empty string",
			filterType: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := mgr.List(ctx, tt.filterType, 20)
			if err == nil {
				t.Fatalf("List(%q) returned nil error, expected error for invalid filter type", tt.filterType)
			}
			if !strings.Contains(strings.ToLower(err.Error()), "invalid filter type") {
				t.Errorf("List(%q) error = %q, expected it to contain %q", tt.filterType, err.Error(), "invalid filter type")
			}
		})
	}
}

// TestValidateID_InjectionPrevention verifies that Archive, Unarchive, and
// Delete all reject notification IDs containing characters that could enable
// GraphQL query injection (double quotes, single quotes, backslashes). IDs
// are interpolated into mutation strings via fmt.Sprintf, so these characters
// must be blocked by validateID.
func TestValidateID_InjectionPrevention(t *testing.T) {
	// The mock client should never be called when validateID rejects the ID.
	client := &mockGraphQLClient{
		executeFunc: func(ctx context.Context, query string, variables map[string]any) ([]byte, error) {
			t.Error("Execute should not be called for invalid ID")
			return []byte(`{"data":{}}`), nil
		},
	}

	mgr := NewGraphQLNotificationManager(client)
	ctx := context.Background()

	type methodFunc func(ctx context.Context, id string) error

	methods := []struct {
		name   string
		method methodFunc
	}{
		{"Archive", mgr.Archive},
		{"Unarchive", mgr.Unarchive},
		{"Delete", mgr.Delete},
	}

	injectionIDs := []struct {
		name string
		id   string
	}{
		{
			name: "double quote injection",
			id:   `bad"id`,
		},
		{
			name: "single quote injection",
			id:   "bad'id",
		},
		{
			name: "backslash injection",
			id:   `bad\id`,
		},
		{
			name: "empty string ID",
			id:   "",
		},
	}

	for _, m := range methods {
		t.Run(m.name, func(t *testing.T) {
			for _, tc := range injectionIDs {
				t.Run(tc.name, func(t *testing.T) {
					err := m.method(ctx, tc.id)
					if err == nil {
						t.Errorf("%s(%q) returned nil error, expected validation error", m.name, tc.id)
					}
				})
			}
		})
	}
}

// ============================================================================
// Constructor nil client tests
// ============================================================================

func TestNewGraphQLNotificationManager_NilClient(t *testing.T) {
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
	NewGraphQLNotificationManager(nil)
}

// ============================================================================
// Benchmarks
// ============================================================================

func Benchmark_NotificationsList_Handler(b *testing.B) {
	mgr := &mockNotificationManager{
		listFunc: func(ctx context.Context, filterType string, limit int) ([]Notification, error) {
			return sampleNotifications(), nil
		},
	}

	confirm := safety.NewConfirmationTracker(DestructiveTools)
	regs := NotificationTools(mgr, confirm, nil)
	var listTool tools.Registration
	for _, r := range regs {
		if r.Tool.Name == "notifications_list" {
			listTool = r
			break
		}
	}

	req := newCallToolRequest("notifications_list", map[string]any{})
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = listTool.Handler(ctx, req)
	}
}

func Benchmark_NotificationsManage_Archive(b *testing.B) {
	mgr := &mockNotificationManager{
		archiveFunc: func(ctx context.Context, id string) error {
			return nil
		},
	}

	confirm := safety.NewConfirmationTracker(DestructiveTools)
	regs := NotificationTools(mgr, confirm, nil)
	var manageTool tools.Registration
	for _, r := range regs {
		if r.Tool.Name == "notifications_manage" {
			manageTool = r
			break
		}
	}

	req := newCallToolRequest("notifications_manage", map[string]any{
		"action": "archive",
		"id":     "notif-1",
	})
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = manageTool.Handler(ctx, req)
	}
}
