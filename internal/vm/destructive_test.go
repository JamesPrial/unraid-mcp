package vm

import (
	"sort"
	"testing"
)

// ---------------------------------------------------------------------------
// Tests for the exported DestructiveTools variable
// ---------------------------------------------------------------------------

func Test_DestructiveTools_Length(t *testing.T) {
	const wantLen = 5
	if got := len(DestructiveTools); got != wantLen {
		t.Errorf("len(DestructiveTools) = %d, want %d", got, wantLen)
	}
}

func Test_DestructiveTools_ContainsExpectedNames(t *testing.T) {
	expected := []string{
		"vm_stop",
		"vm_force_stop",
		"vm_restart",
		"vm_create",
		"vm_delete",
	}

	// Build a set from the actual variable for O(1) lookup.
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
		"vm_stop":       {},
		"vm_force_stop": {},
		"vm_restart":    {},
		"vm_create":     {},
		"vm_delete":     {},
	}

	for _, name := range DestructiveTools {
		if _, ok := expected[name]; !ok {
			t.Errorf("DestructiveTools contains unexpected entry %q", name)
		}
	}
}

func Test_DestructiveTools_ExactContents(t *testing.T) {
	// Comprehensive check: sort both slices and compare element-by-element.
	expected := []string{
		"vm_create",
		"vm_delete",
		"vm_force_stop",
		"vm_restart",
		"vm_stop",
	}

	got := make([]string, len(DestructiveTools))
	copy(got, DestructiveTools)
	sort.Strings(got)

	if len(got) != len(expected) {
		t.Fatalf("DestructiveTools has %d entries, want %d; got %v", len(got), len(expected), got)
	}
	for i := range expected {
		if got[i] != expected[i] {
			t.Errorf("DestructiveTools (sorted)[%d] = %q, want %q", i, got[i], expected[i])
		}
	}
}
