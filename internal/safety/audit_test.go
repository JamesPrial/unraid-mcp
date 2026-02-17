package safety

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func Test_AuditLogger_Log_Cases(t *testing.T) {
	tests := []struct {
		name     string
		entry    AuditEntry
		wantErr  bool
		validate func(t *testing.T, output string)
	}{
		{
			name: "valid entry is written successfully",
			entry: AuditEntry{
				Timestamp: time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC),
				Tool:      "docker_list",
				Params:    map[string]any{"all": true},
				Result:    "success",
				Duration:  150 * time.Millisecond,
			},
			wantErr: false,
			validate: func(t *testing.T, output string) {
				t.Helper()
				if output == "" {
					t.Error("expected non-empty output")
				}
			},
		},
		{
			name: "entry with nil params",
			entry: AuditEntry{
				Timestamp: time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC),
				Tool:      "docker_list",
				Params:    nil,
				Result:    "success",
				Duration:  100 * time.Millisecond,
			},
			wantErr: false,
			validate: func(t *testing.T, output string) {
				t.Helper()
				if output == "" {
					t.Error("expected non-empty output for nil params")
				}
			},
		},
		{
			name: "entry with empty tool name",
			entry: AuditEntry{
				Timestamp: time.Now(),
				Tool:      "",
				Params:    map[string]any{},
				Result:    "ok",
				Duration:  0,
			},
			wantErr: false,
			validate: func(t *testing.T, output string) {
				t.Helper()
				if output == "" {
					t.Error("expected non-empty output for empty tool")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger := NewAuditLogger(&buf)
			if logger == nil {
				t.Fatal("NewAuditLogger() returned nil")
			}

			err := logger.Log(tt.entry)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.validate != nil {
				tt.validate(t, buf.String())
			}
		})
	}
}

func Test_AuditLogger_Log_Format_JSON(t *testing.T) {
	var buf bytes.Buffer
	logger := NewAuditLogger(&buf)

	entry := AuditEntry{
		Timestamp: time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC),
		Tool:      "docker_list",
		Params:    map[string]any{"all": true, "filter": "running"},
		Result:    "success",
		Duration:  250 * time.Millisecond,
	}

	err := logger.Log(entry)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := strings.TrimSpace(buf.String())

	// Verify it is valid JSON.
	var parsed map[string]any
	if err := json.Unmarshal([]byte(output), &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, output)
	}

	// Verify expected fields exist in the JSON output.
	expectedFields := []string{"tool", "result"}
	for _, field := range expectedFields {
		if _, ok := parsed[field]; !ok {
			// Try common casing variants.
			found := false
			for key := range parsed {
				if strings.EqualFold(key, field) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("JSON output missing expected field %q. Got keys: %v", field, keysOf(parsed))
			}
		}
	}

	// Verify tool value.
	toolVal := findField(parsed, "tool")
	if toolVal != "docker_list" {
		t.Errorf("tool field = %v, want %q", toolVal, "docker_list")
	}

	// Verify result value.
	resultVal := findField(parsed, "result")
	if resultVal != "success" {
		t.Errorf("result field = %v, want %q", resultVal, "success")
	}
}

func Test_AuditLogger_Log_MultipleEntries(t *testing.T) {
	var buf bytes.Buffer
	logger := NewAuditLogger(&buf)

	entries := []AuditEntry{
		{
			Timestamp: time.Now(),
			Tool:      "docker_list",
			Params:    map[string]any{},
			Result:    "success",
			Duration:  100 * time.Millisecond,
		},
		{
			Timestamp: time.Now(),
			Tool:      "docker_inspect",
			Params:    map[string]any{"id": "abc123"},
			Result:    "success",
			Duration:  200 * time.Millisecond,
		},
		{
			Timestamp: time.Now(),
			Tool:      "vm_list",
			Params:    nil,
			Result:    "error: connection refused",
			Duration:  50 * time.Millisecond,
		},
	}

	for i, entry := range entries {
		if err := logger.Log(entry); err != nil {
			t.Fatalf("Log() entry %d returned error: %v", i, err)
		}
	}

	output := strings.TrimSpace(buf.String())
	lines := strings.Split(output, "\n")

	if len(lines) != 3 {
		t.Errorf("expected 3 JSON lines, got %d\noutput:\n%s", len(lines), output)
	}

	// Verify each line is valid JSON.
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var parsed map[string]any
		if err := json.Unmarshal([]byte(line), &parsed); err != nil {
			t.Errorf("line %d is not valid JSON: %v\nline: %s", i, err, line)
		}
	}
}

func Test_AuditLogger_NilWriter(t *testing.T) {
	logger := NewAuditLogger(nil)

	// When constructed with a nil writer, either:
	// 1. NewAuditLogger returns nil (and Log cannot be called), or
	// 2. Log returns an error.
	// Both are acceptable as long as no panic occurs.
	if logger == nil {
		// Acceptable: factory returned nil for nil writer.
		return
	}

	entry := AuditEntry{
		Timestamp: time.Now(),
		Tool:      "test",
		Params:    map[string]any{},
		Result:    "ok",
		Duration:  0,
	}

	err := logger.Log(entry)
	if err == nil {
		t.Error("Log() with nil writer should return an error")
	}
}

func Test_NewAuditLogger_NonNilWriter(t *testing.T) {
	var buf bytes.Buffer
	logger := NewAuditLogger(&buf)
	if logger == nil {
		t.Error("NewAuditLogger() with valid writer should not return nil")
	}
}

// Helper functions for test assertions.

func keysOf(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func findField(m map[string]any, field string) any {
	if v, ok := m[field]; ok {
		return v
	}
	for k, v := range m {
		if strings.EqualFold(k, field) {
			return v
		}
	}
	return nil
}
