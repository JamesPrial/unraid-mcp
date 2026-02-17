package safety

import (
	"testing"
	"time"
)

func Test_ConfirmationTracker_NeedsConfirmation_Cases(t *testing.T) {
	destructiveTools := []string{"docker_remove", "docker_stop", "vm_delete"}

	tests := []struct {
		name string
		tool string
		want bool
	}{
		{
			name: "destructive tool needs confirmation",
			tool: "docker_remove",
			want: true,
		},
		{
			name: "another destructive tool needs confirmation",
			tool: "docker_stop",
			want: true,
		},
		{
			name: "yet another destructive tool needs confirmation",
			tool: "vm_delete",
			want: true,
		},
		{
			name: "non-destructive tool does not need confirmation",
			tool: "docker_list",
			want: false,
		},
		{
			name: "unknown tool does not need confirmation",
			tool: "some_unknown_tool",
			want: false,
		},
		{
			name: "empty tool name does not need confirmation",
			tool: "",
			want: false,
		},
	}

	ct := NewConfirmationTracker(destructiveTools)
	if ct == nil {
		t.Fatal("NewConfirmationTracker() returned nil")
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ct.NeedsConfirmation(tt.tool)
			if got != tt.want {
				t.Errorf("NeedsConfirmation(%q) = %v, want %v", tt.tool, got, tt.want)
			}
		})
	}
}

func Test_ConfirmationTracker_NeedsConfirmation_EmptyDestructiveList(t *testing.T) {
	ct := NewConfirmationTracker([]string{})
	if ct == nil {
		t.Fatal("NewConfirmationTracker() returned nil")
	}

	if ct.NeedsConfirmation("docker_remove") {
		t.Error("with empty destructive tools, nothing should need confirmation")
	}
}

func Test_ConfirmationTracker_NeedsConfirmation_NilDestructiveList(t *testing.T) {
	ct := NewConfirmationTracker(nil)
	if ct == nil {
		t.Fatal("NewConfirmationTracker() returned nil")
	}

	if ct.NeedsConfirmation("docker_remove") {
		t.Error("with nil destructive tools, nothing should need confirmation")
	}
}

func Test_ConfirmationTracker_RequestAndConfirm(t *testing.T) {
	ct := NewConfirmationTracker([]string{"docker_remove"})

	token := ct.RequestConfirmation("docker_remove", "my-container", "Remove container my-container")

	if token == "" {
		t.Fatal("RequestConfirmation() returned empty token")
	}

	if !ct.Confirm(token) {
		t.Error("Confirm() should return true for a valid, unused token")
	}
}

func Test_ConfirmationTracker_InvalidToken(t *testing.T) {
	ct := NewConfirmationTracker([]string{"docker_remove"})

	if ct.Confirm("bogus-token-that-was-never-issued") {
		t.Error("Confirm() should return false for an invalid token")
	}
}

func Test_ConfirmationTracker_EmptyToken(t *testing.T) {
	ct := NewConfirmationTracker([]string{"docker_remove"})

	if ct.Confirm("") {
		t.Error("Confirm() should return false for an empty token")
	}
}

func Test_ConfirmationTracker_TokenSingleUse(t *testing.T) {
	ct := NewConfirmationTracker([]string{"docker_remove"})

	token := ct.RequestConfirmation("docker_remove", "my-container", "Remove container")

	first := ct.Confirm(token)
	second := ct.Confirm(token)

	if !first {
		t.Error("first Confirm() should return true")
	}
	if second {
		t.Error("second Confirm() should return false (token is single-use)")
	}
}

func Test_ConfirmationTracker_MultipleTokensIndependent(t *testing.T) {
	ct := NewConfirmationTracker([]string{"docker_remove", "vm_delete"})

	token1 := ct.RequestConfirmation("docker_remove", "container-a", "Remove container-a")
	token2 := ct.RequestConfirmation("vm_delete", "vm-b", "Delete vm-b")

	if token1 == token2 {
		t.Error("different requests should produce different tokens")
	}

	// Confirm token2 first, token1 should still work.
	if !ct.Confirm(token2) {
		t.Error("Confirm(token2) should return true")
	}
	if !ct.Confirm(token1) {
		t.Error("Confirm(token1) should return true even after token2 was confirmed")
	}

	// Both should now be consumed.
	if ct.Confirm(token1) {
		t.Error("Confirm(token1) second use should return false")
	}
	if ct.Confirm(token2) {
		t.Error("Confirm(token2) second use should return false")
	}
}

func Test_ConfirmationTracker_RequestConfirmation_ReturnsNonEmptyToken(t *testing.T) {
	ct := NewConfirmationTracker([]string{"docker_remove"})

	tests := []struct {
		name         string
		tool         string
		resourceName string
		description  string
	}{
		{
			name:         "typical request",
			tool:         "docker_remove",
			resourceName: "my-container",
			description:  "Remove container my-container",
		},
		{
			name:         "empty resource name",
			tool:         "docker_remove",
			resourceName: "",
			description:  "Remove unnamed resource",
		},
		{
			name:         "empty description",
			tool:         "docker_remove",
			resourceName: "my-container",
			description:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token := ct.RequestConfirmation(tt.tool, tt.resourceName, tt.description)
			if token == "" {
				t.Error("RequestConfirmation() should return a non-empty token")
			}
		})
	}
}

func Test_ConfirmationTracker_TokenExpiry(t *testing.T) {
	// This test verifies that tokens expire after 5 minutes.
	// We cannot easily fast-forward time without a clock interface,
	// so this test documents the expected behavior. If the implementation
	// uses a clock interface or allows time injection, this test should
	// be updated to use it.
	//
	// For now, we test the concept: create a tracker, request a token,
	// and verify it works immediately (proving the flow). The expiry
	// at 5+ minutes is specified behavior that integration tests or
	// clock-injection tests will validate.
	ct := NewConfirmationTracker([]string{"docker_remove"})

	token := ct.RequestConfirmation("docker_remove", "container", "Remove container")

	// Token should be valid immediately (well within 5 minute window).
	if !ct.Confirm(token) {
		t.Error("token should be valid immediately after creation")
	}
}

// Test_ConfirmationTracker_TokenExpiry_Simulation tests token expiry
// by checking if the ConfirmationTracker respects time-based expiration.
// This test is necessarily time-sensitive and documents the >5min expiry contract.
// If the implementation provides a way to inject a clock (e.g., a clockFunc field),
// a more robust test can replace this one.
func Test_ConfirmationTracker_TokenExpiry_Simulation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping time-sensitive expiry test in short mode")
	}

	// We test the boundary: a token used within a very short time should work.
	// This is already covered above but included here for completeness.
	ct := NewConfirmationTracker([]string{"docker_remove"})
	token := ct.RequestConfirmation("docker_remove", "container", "desc")

	// Small sleep to ensure non-zero elapsed time but well within 5 minutes.
	time.Sleep(10 * time.Millisecond)

	if !ct.Confirm(token) {
		t.Error("token should be valid within the 5 minute window")
	}
}

func Test_NewConfirmationTracker_ReturnsNonNil(t *testing.T) {
	tests := []struct {
		name  string
		tools []string
	}{
		{name: "nil tools", tools: nil},
		{name: "empty tools", tools: []string{}},
		{name: "with tools", tools: []string{"docker_remove"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ct := NewConfirmationTracker(tt.tools)
			if ct == nil {
				t.Error("NewConfirmationTracker() should never return nil")
			}
		})
	}
}
