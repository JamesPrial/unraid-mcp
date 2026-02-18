package ups

import (
	"bytes"
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

// mockUPSMonitor implements UPSMonitor for tool handler tests.
type mockUPSMonitor struct {
	getDevicesFunc func(ctx context.Context) ([]UPSDevice, error)
}

func (m *mockUPSMonitor) GetDevices(ctx context.Context) ([]UPSDevice, error) {
	return m.getDevicesFunc(ctx)
}

var _ UPSMonitor = (*mockUPSMonitor)(nil)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// newCallToolRequest builds an mcp.CallToolRequest with the given name and arguments map.
func newCallToolRequest(name string, args map[string]any) mcp.CallToolRequest {
	req := mcp.CallToolRequest{}
	req.Params.Name = name
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

// float64Ptr returns a pointer to a float64 value.
func float64Ptr(v float64) *float64 { return &v }

// intPtr returns a pointer to an int value.
func intPtr(v int) *int { return &v }

// ---------------------------------------------------------------------------
// Compile-time interface checks
// ---------------------------------------------------------------------------

func Test_GraphQLUPSMonitor_ImplementsUPSMonitor(t *testing.T) {
	var _ UPSMonitor = (*GraphQLUPSMonitor)(nil)
}

// ---------------------------------------------------------------------------
// Manager (GraphQLUPSMonitor) tests
// ---------------------------------------------------------------------------

func Test_GetDevices_Cases(t *testing.T) {
	tests := []struct {
		name        string
		executeFunc func(ctx context.Context, query string, variables map[string]any) ([]byte, error)
		wantErr     bool
		errContains string
		validate    func(t *testing.T, devices []UPSDevice)
	}{
		{
			name: "returns devices with full battery and power",
			executeFunc: func(ctx context.Context, query string, variables map[string]any) ([]byte, error) {
				resp := `{"ups":[{"id":"ups-1","name":"APC-1500","model":"APC Smart-UPS 1500","status":"online","battery":{"charge":95.5,"runtime":3600},"power":{"inputVoltage":120.1,"outputVoltage":119.8,"load":45.2}}]}`
				return []byte(resp), nil
			},
			wantErr: false,
			validate: func(t *testing.T, devices []UPSDevice) {
				t.Helper()
				if len(devices) != 1 {
					t.Fatalf("expected 1 device, got %d", len(devices))
				}
				d := devices[0]
				if d.ID != "ups-1" {
					t.Errorf("ID = %q, want %q", d.ID, "ups-1")
				}
				if d.Name != "APC-1500" {
					t.Errorf("Name = %q, want %q", d.Name, "APC-1500")
				}
				if d.Model != "APC Smart-UPS 1500" {
					t.Errorf("Model = %q, want %q", d.Model, "APC Smart-UPS 1500")
				}
				if d.Status != "online" {
					t.Errorf("Status = %q, want %q", d.Status, "online")
				}
				if d.Battery == nil {
					t.Fatal("Battery is nil, expected non-nil")
				}
				if d.Battery.Charge == nil || *d.Battery.Charge != 95.5 {
					t.Errorf("Battery.Charge = %v, want 95.5", d.Battery.Charge)
				}
				if d.Battery.Runtime == nil || *d.Battery.Runtime != 3600 {
					t.Errorf("Battery.Runtime = %v, want 3600", d.Battery.Runtime)
				}
				if d.Power == nil {
					t.Fatal("Power is nil, expected non-nil")
				}
				if d.Power.InputVoltage == nil || *d.Power.InputVoltage != 120.1 {
					t.Errorf("Power.InputVoltage = %v, want 120.1", d.Power.InputVoltage)
				}
				if d.Power.OutputVoltage == nil || *d.Power.OutputVoltage != 119.8 {
					t.Errorf("Power.OutputVoltage = %v, want 119.8", d.Power.OutputVoltage)
				}
				if d.Power.Load == nil || *d.Power.Load != 45.2 {
					t.Errorf("Power.Load = %v, want 45.2", d.Power.Load)
				}
			},
		},
		{
			name: "device with null battery returns nil Battery",
			executeFunc: func(ctx context.Context, query string, variables map[string]any) ([]byte, error) {
				resp := `{"ups":[{"id":"ups-2","name":"Cyberpower","model":"CP1500","status":"online","battery":null,"power":{"inputVoltage":121.0,"outputVoltage":120.0,"load":30.0}}]}`
				return []byte(resp), nil
			},
			wantErr: false,
			validate: func(t *testing.T, devices []UPSDevice) {
				t.Helper()
				if len(devices) != 1 {
					t.Fatalf("expected 1 device, got %d", len(devices))
				}
				if devices[0].Battery != nil {
					t.Errorf("Battery = %+v, want nil", devices[0].Battery)
				}
				if devices[0].Power == nil {
					t.Error("Power is nil, expected non-nil")
				}
			},
		},
		{
			name: "device with null power returns nil Power",
			executeFunc: func(ctx context.Context, query string, variables map[string]any) ([]byte, error) {
				resp := `{"ups":[{"id":"ups-3","name":"Eaton","model":"5E","status":"on battery","battery":{"charge":80.0,"runtime":1800},"power":null}]}`
				return []byte(resp), nil
			},
			wantErr: false,
			validate: func(t *testing.T, devices []UPSDevice) {
				t.Helper()
				if len(devices) != 1 {
					t.Fatalf("expected 1 device, got %d", len(devices))
				}
				if devices[0].Power != nil {
					t.Errorf("Power = %+v, want nil", devices[0].Power)
				}
				if devices[0].Battery == nil {
					t.Error("Battery is nil, expected non-nil")
				}
			},
		},
		{
			name: "device with partial battery (charge only, no runtime)",
			executeFunc: func(ctx context.Context, query string, variables map[string]any) ([]byte, error) {
				resp := `{"ups":[{"id":"ups-4","name":"Partial","model":"X","status":"online","battery":{"charge":50.0,"runtime":null},"power":null}]}`
				return []byte(resp), nil
			},
			wantErr: false,
			validate: func(t *testing.T, devices []UPSDevice) {
				t.Helper()
				if len(devices) != 1 {
					t.Fatalf("expected 1 device, got %d", len(devices))
				}
				if devices[0].Battery == nil {
					t.Fatal("Battery is nil, expected non-nil")
				}
				if devices[0].Battery.Charge == nil || *devices[0].Battery.Charge != 50.0 {
					t.Errorf("Battery.Charge = %v, want 50.0", devices[0].Battery.Charge)
				}
				if devices[0].Battery.Runtime != nil {
					t.Errorf("Battery.Runtime = %v, want nil", devices[0].Battery.Runtime)
				}
			},
		},
		{
			name: "device with partial power (load only, no voltages)",
			executeFunc: func(ctx context.Context, query string, variables map[string]any) ([]byte, error) {
				resp := `{"ups":[{"id":"ups-5","name":"LoadOnly","model":"Y","status":"online","battery":null,"power":{"inputVoltage":null,"outputVoltage":null,"load":22.5}}]}`
				return []byte(resp), nil
			},
			wantErr: false,
			validate: func(t *testing.T, devices []UPSDevice) {
				t.Helper()
				if len(devices) != 1 {
					t.Fatalf("expected 1 device, got %d", len(devices))
				}
				if devices[0].Power == nil {
					t.Fatal("Power is nil, expected non-nil")
				}
				if devices[0].Power.InputVoltage != nil {
					t.Errorf("Power.InputVoltage = %v, want nil", devices[0].Power.InputVoltage)
				}
				if devices[0].Power.OutputVoltage != nil {
					t.Errorf("Power.OutputVoltage = %v, want nil", devices[0].Power.OutputVoltage)
				}
				if devices[0].Power.Load == nil || *devices[0].Power.Load != 22.5 {
					t.Errorf("Power.Load = %v, want 22.5", devices[0].Power.Load)
				}
			},
		},
		{
			name: "empty device list returns empty slice no error",
			executeFunc: func(ctx context.Context, query string, variables map[string]any) ([]byte, error) {
				return []byte(`{"ups":[]}`), nil
			},
			wantErr: false,
			validate: func(t *testing.T, devices []UPSDevice) {
				t.Helper()
				if devices == nil {
					t.Error("expected non-nil empty slice, got nil")
				}
				if len(devices) != 0 {
					t.Errorf("expected 0 devices, got %d", len(devices))
				}
			},
		},
		{
			name: "client error is propagated",
			executeFunc: func(ctx context.Context, query string, variables map[string]any) ([]byte, error) {
				return nil, errors.New("connection refused")
			},
			wantErr:     true,
			errContains: "connection refused",
		},
		{
			name: "invalid JSON returns unmarshal error",
			executeFunc: func(ctx context.Context, query string, variables map[string]any) ([]byte, error) {
				return []byte(`not valid json`), nil
			},
			wantErr:     true,
			errContains: "invalid character",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &mockGraphQLClient{executeFunc: tt.executeFunc}
			monitor := NewGraphQLUPSMonitor(client)
			ctx := context.Background()

			devices, err := monitor.GetDevices(ctx)
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
			if tt.validate != nil {
				tt.validate(t, devices)
			}
		})
	}
}

func Test_GetDevices_QueryContainsExpectedFields(t *testing.T) {
	var capturedQuery string

	client := &mockGraphQLClient{
		executeFunc: func(ctx context.Context, query string, variables map[string]any) ([]byte, error) {
			capturedQuery = query
			return []byte(`{"ups":[]}`), nil
		},
	}
	monitor := NewGraphQLUPSMonitor(client)
	ctx := context.Background()

	_, err := monitor.GetDevices(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedFields := []string{"id", "name", "model", "status", "battery", "charge", "runtime", "power", "inputVoltage", "outputVoltage", "load"}
	for _, field := range expectedFields {
		if !strings.Contains(capturedQuery, field) {
			t.Errorf("query missing expected field %q; query = %q", field, capturedQuery)
		}
	}
}

func Test_GetDevices_MultipleDevices(t *testing.T) {
	client := &mockGraphQLClient{
		executeFunc: func(ctx context.Context, query string, variables map[string]any) ([]byte, error) {
			resp := `{"ups":[
				{"id":"ups-1","name":"UPS-A","model":"Model-A","status":"online","battery":{"charge":100.0,"runtime":7200},"power":{"inputVoltage":120.0,"outputVoltage":120.0,"load":10.0}},
				{"id":"ups-2","name":"UPS-B","model":"Model-B","status":"on battery","battery":{"charge":50.0,"runtime":900},"power":null}
			]}`
			return []byte(resp), nil
		},
	}
	monitor := NewGraphQLUPSMonitor(client)
	ctx := context.Background()

	devices, err := monitor.GetDevices(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(devices) != 2 {
		t.Fatalf("expected 2 devices, got %d", len(devices))
	}

	// Verify first device
	if devices[0].ID != "ups-1" {
		t.Errorf("devices[0].ID = %q, want %q", devices[0].ID, "ups-1")
	}
	// Verify second device
	if devices[1].ID != "ups-2" {
		t.Errorf("devices[1].ID = %q, want %q", devices[1].ID, "ups-2")
	}
	if devices[1].Power != nil {
		t.Errorf("devices[1].Power = %+v, want nil", devices[1].Power)
	}
}

func Test_GetDevices_ContextCancelled(t *testing.T) {
	client := &mockGraphQLClient{
		executeFunc: func(ctx context.Context, query string, variables map[string]any) ([]byte, error) {
			return nil, ctx.Err()
		},
	}
	monitor := NewGraphQLUPSMonitor(client)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := monitor.GetDevices(ctx)
	if err == nil {
		t.Fatal("expected error with cancelled context, got nil")
	}
}

// ---------------------------------------------------------------------------
// UPSTools registration tests
// ---------------------------------------------------------------------------

func Test_UPSTools_ReturnsOneRegistration(t *testing.T) {
	mon := &mockUPSMonitor{
		getDevicesFunc: func(ctx context.Context) ([]UPSDevice, error) {
			return nil, nil
		},
	}

	regs := UPSTools(mon, nil)
	if len(regs) != 1 {
		t.Fatalf("UPSTools() returned %d registrations, want 1", len(regs))
	}
}

func Test_UPSTools_ToolNameIsUPSStatus(t *testing.T) {
	mon := &mockUPSMonitor{
		getDevicesFunc: func(ctx context.Context) ([]UPSDevice, error) {
			return nil, nil
		},
	}

	regs := UPSTools(mon, nil)
	if len(regs) == 0 {
		t.Fatal("UPSTools() returned no registrations")
	}

	name := regs[0].Tool.Name
	if name != "ups_status" {
		t.Errorf("tool name = %q, want %q", name, "ups_status")
	}
}

func Test_UPSTools_NoRequiredParams(t *testing.T) {
	mon := &mockUPSMonitor{
		getDevicesFunc: func(ctx context.Context) ([]UPSDevice, error) {
			return nil, nil
		},
	}

	regs := UPSTools(mon, nil)
	if len(regs) == 0 {
		t.Fatal("UPSTools() returned no registrations")
	}

	tool := regs[0].Tool
	if len(tool.InputSchema.Required) != 0 {
		t.Errorf("tool has %d required params, want 0; required = %v", len(tool.InputSchema.Required), tool.InputSchema.Required)
	}
}

func Test_UPSTools_HandlerIsNotNil(t *testing.T) {
	mon := &mockUPSMonitor{
		getDevicesFunc: func(ctx context.Context) ([]UPSDevice, error) {
			return nil, nil
		},
	}

	regs := UPSTools(mon, nil)
	if len(regs) == 0 {
		t.Fatal("UPSTools() returned no registrations")
	}

	if regs[0].Handler == nil {
		t.Error("tool handler is nil")
	}
}

// ---------------------------------------------------------------------------
// ups_status handler tests
// ---------------------------------------------------------------------------

func Test_UPSStatusHandler_Cases(t *testing.T) {
	tests := []struct {
		name           string
		getDevicesFunc func(ctx context.Context) ([]UPSDevice, error)
		wantResultErr  bool
		wantContains   string
	}{
		{
			name: "happy path returns JSON array of devices",
			getDevicesFunc: func(ctx context.Context) ([]UPSDevice, error) {
				return []UPSDevice{
					{
						ID:     "ups-1",
						Name:   "APC-1500",
						Model:  "APC Smart-UPS 1500",
						Status: "online",
						Battery: &Battery{
							Charge:  float64Ptr(95.5),
							Runtime: intPtr(3600),
						},
						Power: &PowerInfo{
							InputVoltage:  float64Ptr(120.1),
							OutputVoltage: float64Ptr(119.8),
							Load:          float64Ptr(45.2),
						},
					},
				}, nil
			},
			wantResultErr: false,
			wantContains:  "ups-1",
		},
		{
			name: "empty list returns empty JSON array",
			getDevicesFunc: func(ctx context.Context) ([]UPSDevice, error) {
				return []UPSDevice{}, nil
			},
			wantResultErr: false,
			wantContains:  "[]",
		},
		{
			name: "full device with all fields produces properly formatted JSON",
			getDevicesFunc: func(ctx context.Context) ([]UPSDevice, error) {
				return []UPSDevice{
					{
						ID:     "ups-full",
						Name:   "FullUPS",
						Model:  "Model-X",
						Status: "online",
						Battery: &Battery{
							Charge:  float64Ptr(100.0),
							Runtime: intPtr(7200),
						},
						Power: &PowerInfo{
							InputVoltage:  float64Ptr(121.5),
							OutputVoltage: float64Ptr(120.0),
							Load:          float64Ptr(30.0),
						},
					},
				}, nil
			},
			wantResultErr: false,
			wantContains:  "battery",
		},
		{
			name: "device with nil optional fields handles gracefully",
			getDevicesFunc: func(ctx context.Context) ([]UPSDevice, error) {
				return []UPSDevice{
					{
						ID:      "ups-nil",
						Name:    "NilUPS",
						Model:   "Model-N",
						Status:  "unknown",
						Battery: nil,
						Power:   nil,
					},
				}, nil
			},
			wantResultErr: false,
			wantContains:  "ups-nil",
		},
		{
			name: "manager error returns error result",
			getDevicesFunc: func(ctx context.Context) ([]UPSDevice, error) {
				return nil, errors.New("failed to query UPS devices")
			},
			wantResultErr: true,
			wantContains:  "failed to query UPS devices",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mon := &mockUPSMonitor{getDevicesFunc: tt.getDevicesFunc}
			audit, _ := newTestAuditLogger(t)

			regs := UPSTools(mon, audit)
			if len(regs) == 0 {
				t.Fatal("UPSTools() returned no registrations")
			}

			handler := regs[0].Handler
			req := newCallToolRequest("ups_status", nil)

			result, err := handler(context.Background(), req)

			// Handler should NEVER return a Go error.
			if err != nil {
				t.Fatalf("handler returned non-nil error: %v, want nil", err)
			}

			if result == nil {
				t.Fatal("handler returned nil result")
			}

			text := extractResultText(t, result)

			if tt.wantResultErr {
				if !strings.Contains(strings.ToLower(text), "error") {
					t.Errorf("result text = %q, want it to contain 'error'", text)
				}
			}

			if tt.wantContains != "" && !strings.Contains(text, tt.wantContains) {
				t.Errorf("result text = %q, want it to contain %q", text, tt.wantContains)
			}
		})
	}
}

func Test_UPSStatusHandler_NeverReturnsGoError(t *testing.T) {
	scenarios := []func(ctx context.Context) ([]UPSDevice, error){
		func(ctx context.Context) ([]UPSDevice, error) {
			return []UPSDevice{}, nil
		},
		func(ctx context.Context) ([]UPSDevice, error) {
			return nil, errors.New("some error")
		},
		func(ctx context.Context) ([]UPSDevice, error) {
			return []UPSDevice{{ID: "x"}}, nil
		},
	}

	for i, fn := range scenarios {
		mon := &mockUPSMonitor{getDevicesFunc: fn}
		regs := UPSTools(mon, nil)
		handler := regs[0].Handler
		req := newCallToolRequest("ups_status", nil)

		_, err := handler(context.Background(), req)
		if err != nil {
			t.Errorf("scenario %d: handler returned non-nil error: %v", i, err)
		}
	}
}

func Test_UPSStatusHandler_HappyPathReturnsValidJSON(t *testing.T) {
	mon := &mockUPSMonitor{
		getDevicesFunc: func(ctx context.Context) ([]UPSDevice, error) {
			return []UPSDevice{
				{
					ID:     "ups-1",
					Name:   "Test",
					Model:  "M1",
					Status: "online",
					Battery: &Battery{
						Charge:  float64Ptr(99.0),
						Runtime: intPtr(5400),
					},
					Power: &PowerInfo{
						InputVoltage:  float64Ptr(120.0),
						OutputVoltage: float64Ptr(119.5),
						Load:          float64Ptr(20.0),
					},
				},
			}, nil
		},
	}

	regs := UPSTools(mon, nil)
	handler := regs[0].Handler
	req := newCallToolRequest("ups_status", nil)

	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler returned non-nil error: %v", err)
	}

	text := extractResultText(t, result)

	if !json.Valid([]byte(text)) {
		t.Errorf("result text is not valid JSON: %q", text)
	}

	// Pretty-printed JSON should contain newlines (from json.MarshalIndent).
	if !strings.Contains(text, "\n") {
		t.Errorf("result text does not appear to be pretty-printed: %q", text)
	}
}

func Test_UPSStatusHandler_EmptyListReturnsEmptyJSONArray(t *testing.T) {
	mon := &mockUPSMonitor{
		getDevicesFunc: func(ctx context.Context) ([]UPSDevice, error) {
			return []UPSDevice{}, nil
		},
	}

	regs := UPSTools(mon, nil)
	handler := regs[0].Handler
	req := newCallToolRequest("ups_status", nil)

	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler returned non-nil error: %v", err)
	}

	text := extractResultText(t, result)

	// Should be "[]" for empty JSON array.
	var parsed []json.RawMessage
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		t.Fatalf("failed to unmarshal result as JSON array: %v; text = %q", err, text)
	}
	if len(parsed) != 0 {
		t.Errorf("expected empty JSON array, got %d elements", len(parsed))
	}
}

func Test_UPSStatusHandler_NilBatteryAndPowerInJSON(t *testing.T) {
	mon := &mockUPSMonitor{
		getDevicesFunc: func(ctx context.Context) ([]UPSDevice, error) {
			return []UPSDevice{
				{
					ID:      "ups-nil",
					Name:    "NilDevice",
					Model:   "M0",
					Status:  "unknown",
					Battery: nil,
					Power:   nil,
				},
			}, nil
		},
	}

	regs := UPSTools(mon, nil)
	handler := regs[0].Handler
	req := newCallToolRequest("ups_status", nil)

	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler returned non-nil error: %v", err)
	}

	text := extractResultText(t, result)

	if !json.Valid([]byte(text)) {
		t.Fatalf("result text is not valid JSON: %q", text)
	}

	// Parse back and verify nil fields are represented as null.
	var devices []map[string]any
	if err := json.Unmarshal([]byte(text), &devices); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}
	if len(devices) != 1 {
		t.Fatalf("expected 1 device, got %d", len(devices))
	}
	if devices[0]["battery"] != nil {
		t.Errorf("expected battery to be null in JSON, got %v", devices[0]["battery"])
	}
	if devices[0]["power"] != nil {
		t.Errorf("expected power to be null in JSON, got %v", devices[0]["power"])
	}
}

func Test_UPSStatusHandler_ErrorResultContainsErrorPrefix(t *testing.T) {
	mon := &mockUPSMonitor{
		getDevicesFunc: func(ctx context.Context) ([]UPSDevice, error) {
			return nil, errors.New("connection timeout")
		},
	}

	regs := UPSTools(mon, nil)
	handler := regs[0].Handler
	req := newCallToolRequest("ups_status", nil)

	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler returned non-nil error: %v", err)
	}

	text := extractResultText(t, result)
	if !strings.Contains(text, "error") {
		t.Errorf("error result text = %q, want it to contain 'error'", text)
	}
	if !strings.Contains(text, "connection timeout") {
		t.Errorf("error result text = %q, want it to contain 'connection timeout'", text)
	}
}

func Test_UPSStatusHandler_NilAuditLoggerNoPanic(t *testing.T) {
	mon := &mockUPSMonitor{
		getDevicesFunc: func(ctx context.Context) ([]UPSDevice, error) {
			return []UPSDevice{{ID: "ups-1"}}, nil
		},
	}

	// Pass nil for audit logger -- should not panic.
	regs := UPSTools(mon, nil)
	if len(regs) == 0 {
		t.Fatal("UPSTools() returned no registrations")
	}

	handler := regs[0].Handler
	req := newCallToolRequest("ups_status", nil)

	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler returned non-nil error: %v", err)
	}
	if result == nil {
		t.Fatal("handler returned nil result")
	}
}

func Test_UPSStatusHandler_AuditLogging(t *testing.T) {
	mon := &mockUPSMonitor{
		getDevicesFunc: func(ctx context.Context) ([]UPSDevice, error) {
			return []UPSDevice{{ID: "ups-1"}}, nil
		},
	}

	audit, buf := newTestAuditLogger(t)

	regs := UPSTools(mon, audit)
	handler := regs[0].Handler
	req := newCallToolRequest("ups_status", nil)

	_, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler returned non-nil error: %v", err)
	}

	logged := buf.String()
	if logged == "" {
		t.Error("expected audit log entry, got empty")
	}
	if !strings.Contains(logged, "ups_status") {
		t.Errorf("audit log = %q, want it to contain 'ups_status'", logged)
	}
}

// ---------------------------------------------------------------------------
// Type zero-value tests
// ---------------------------------------------------------------------------

func Test_UPSDevice_ZeroValue(t *testing.T) {
	var d UPSDevice
	if d.ID != "" {
		t.Errorf("zero UPSDevice.ID = %q, want empty", d.ID)
	}
	if d.Name != "" {
		t.Errorf("zero UPSDevice.Name = %q, want empty", d.Name)
	}
	if d.Model != "" {
		t.Errorf("zero UPSDevice.Model = %q, want empty", d.Model)
	}
	if d.Status != "" {
		t.Errorf("zero UPSDevice.Status = %q, want empty", d.Status)
	}
	if d.Battery != nil {
		t.Error("zero UPSDevice.Battery is not nil")
	}
	if d.Power != nil {
		t.Error("zero UPSDevice.Power is not nil")
	}
}

func Test_Battery_ZeroValue(t *testing.T) {
	var b Battery
	if b.Charge != nil {
		t.Errorf("zero Battery.Charge = %v, want nil", b.Charge)
	}
	if b.Runtime != nil {
		t.Errorf("zero Battery.Runtime = %v, want nil", b.Runtime)
	}
}

func Test_PowerInfo_ZeroValue(t *testing.T) {
	var p PowerInfo
	if p.InputVoltage != nil {
		t.Errorf("zero PowerInfo.InputVoltage = %v, want nil", p.InputVoltage)
	}
	if p.OutputVoltage != nil {
		t.Errorf("zero PowerInfo.OutputVoltage = %v, want nil", p.OutputVoltage)
	}
	if p.Load != nil {
		t.Errorf("zero PowerInfo.Load = %v, want nil", p.Load)
	}
}

// ---------------------------------------------------------------------------
// JSON serialization tests
// ---------------------------------------------------------------------------

func Test_UPSDevice_JSONRoundTrip(t *testing.T) {
	original := UPSDevice{
		ID:     "ups-rt",
		Name:   "RoundTrip",
		Model:  "RT-500",
		Status: "online",
		Battery: &Battery{
			Charge:  float64Ptr(88.5),
			Runtime: intPtr(2700),
		},
		Power: &PowerInfo{
			InputVoltage:  float64Ptr(122.0),
			OutputVoltage: float64Ptr(121.0),
			Load:          float64Ptr(35.0),
		},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var decoded UPSDevice
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if decoded.ID != original.ID {
		t.Errorf("ID = %q, want %q", decoded.ID, original.ID)
	}
	if decoded.Name != original.Name {
		t.Errorf("Name = %q, want %q", decoded.Name, original.Name)
	}
	if decoded.Battery == nil {
		t.Fatal("decoded Battery is nil")
	}
	if *decoded.Battery.Charge != *original.Battery.Charge {
		t.Errorf("Battery.Charge = %v, want %v", *decoded.Battery.Charge, *original.Battery.Charge)
	}
	if *decoded.Battery.Runtime != *original.Battery.Runtime {
		t.Errorf("Battery.Runtime = %v, want %v", *decoded.Battery.Runtime, *original.Battery.Runtime)
	}
	if decoded.Power == nil {
		t.Fatal("decoded Power is nil")
	}
	if *decoded.Power.Load != *original.Power.Load {
		t.Errorf("Power.Load = %v, want %v", *decoded.Power.Load, *original.Power.Load)
	}
}

func Test_UPSDevice_JSONWithNilFields(t *testing.T) {
	device := UPSDevice{
		ID:      "ups-nil-json",
		Name:    "NilJSON",
		Model:   "NJ-1",
		Status:  "offline",
		Battery: nil,
		Power:   nil,
	}

	data, err := json.Marshal(device)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	// Verify the JSON contains null for battery and power.
	jsonStr := string(data)
	if !strings.Contains(jsonStr, `"battery":null`) {
		t.Errorf("expected JSON to contain '\"battery\":null', got %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"power":null`) {
		t.Errorf("expected JSON to contain '\"power\":null', got %s", jsonStr)
	}
}

// ---------------------------------------------------------------------------
// Constructor nil client tests
// ---------------------------------------------------------------------------

func TestNewGraphQLUPSMonitor_NilClient(t *testing.T) {
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
	NewGraphQLUPSMonitor(nil)
}

// ---------------------------------------------------------------------------
// Benchmarks
// ---------------------------------------------------------------------------

func Benchmark_UPSStatusHandler_HappyPath(b *testing.B) {
	devices := []UPSDevice{
		{
			ID:     "ups-bench",
			Name:   "BenchUPS",
			Model:  "B-1000",
			Status: "online",
			Battery: &Battery{
				Charge:  float64Ptr(100.0),
				Runtime: intPtr(7200),
			},
			Power: &PowerInfo{
				InputVoltage:  float64Ptr(120.0),
				OutputVoltage: float64Ptr(119.5),
				Load:          float64Ptr(25.0),
			},
		},
	}

	mon := &mockUPSMonitor{
		getDevicesFunc: func(ctx context.Context) ([]UPSDevice, error) {
			return devices, nil
		},
	}

	regs := UPSTools(mon, nil)
	handler := regs[0].Handler
	req := newCallToolRequest("ups_status", nil)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = handler(ctx, req)
	}
}

func Benchmark_GetDevices(b *testing.B) {
	resp := []byte(`{"ups":[{"id":"ups-1","name":"APC-1500","model":"APC Smart-UPS 1500","status":"online","battery":{"charge":95.5,"runtime":3600},"power":{"inputVoltage":120.1,"outputVoltage":119.8,"load":45.2}}]}`)
	client := &mockGraphQLClient{
		executeFunc: func(ctx context.Context, query string, variables map[string]any) ([]byte, error) {
			return resp, nil
		},
	}
	monitor := NewGraphQLUPSMonitor(client)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = monitor.GetDevices(ctx)
	}
}

// ---------------------------------------------------------------------------
// Ensure UPSTools uses the tools.Registration type (compile-time check)
// ---------------------------------------------------------------------------

func Test_UPSTools_ReturnsToolsRegistrations(t *testing.T) {
	mon := &mockUPSMonitor{
		getDevicesFunc: func(ctx context.Context) ([]UPSDevice, error) {
			return nil, nil
		},
	}

	// This is a compile-time type check: UPSTools must return []tools.Registration.
	var regs []tools.Registration = UPSTools(mon, nil)
	if regs == nil {
		t.Fatal("UPSTools() returned nil")
	}
}
