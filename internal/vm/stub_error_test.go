//go:build !libvirt

package vm

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Tests for the ErrLibvirtNotCompiled sentinel error
// ---------------------------------------------------------------------------

func Test_ErrLibvirtNotCompiled_SatisfiesErrorInterface(t *testing.T) {
	// The sentinel must satisfy the error interface. Calling .Error() must
	// not panic and must return a non-empty string.
	err := ErrLibvirtNotCompiled
	msg := err.Error()
	if msg == "" {
		t.Fatal("ErrLibvirtNotCompiled.Error() returned empty string")
	}
}

func Test_ErrLibvirtNotCompiled_ErrorMessageContent(t *testing.T) {
	msg := ErrLibvirtNotCompiled.Error()
	want := "libvirt support not compiled"
	if !strings.Contains(msg, want) {
		t.Errorf("ErrLibvirtNotCompiled.Error() = %q, want it to contain %q", msg, want)
	}
}

func Test_NewLibvirtVMManager_ReturnsWrappedSentinel(t *testing.T) {
	_, err := NewLibvirtVMManager("")
	if err == nil {
		t.Fatal("NewLibvirtVMManager(\"\") returned nil error in stub build")
	}
	if !errors.Is(err, ErrLibvirtNotCompiled) {
		t.Errorf("NewLibvirtVMManager error = %v, want errors.Is(err, ErrLibvirtNotCompiled) to be true", err)
	}
}

func Test_StubMethods_ReturnErrLibvirtNotCompiled(t *testing.T) {
	// A zero-value LibvirtVMManager is fine for stub method calls.
	m := &LibvirtVMManager{}
	ctx := context.Background()

	tests := []struct {
		name string
		call func() error
	}{
		{
			name: "ListVMs",
			call: func() error { _, err := m.ListVMs(ctx); return err },
		},
		{
			name: "InspectVM",
			call: func() error { _, err := m.InspectVM(ctx, ""); return err },
		},
		{
			name: "StartVM",
			call: func() error { return m.StartVM(ctx, "") },
		},
		{
			name: "StopVM",
			call: func() error { return m.StopVM(ctx, "") },
		},
		{
			name: "ForceStopVM",
			call: func() error { return m.ForceStopVM(ctx, "") },
		},
		{
			name: "PauseVM",
			call: func() error { return m.PauseVM(ctx, "") },
		},
		{
			name: "ResumeVM",
			call: func() error { return m.ResumeVM(ctx, "") },
		},
		{
			name: "RestartVM",
			call: func() error { return m.RestartVM(ctx, "") },
		},
		{
			name: "CreateVM",
			call: func() error { return m.CreateVM(ctx, "") },
		},
		{
			name: "DeleteVM",
			call: func() error { return m.DeleteVM(ctx, "") },
		},
		{
			name: "ListSnapshots",
			call: func() error { _, err := m.ListSnapshots(ctx, ""); return err },
		},
		{
			name: "CreateSnapshot",
			call: func() error { return m.CreateSnapshot(ctx, "", "") },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.call()
			if err == nil {
				t.Fatalf("%s returned nil error in stub build", tt.name)
			}
			if !errors.Is(err, ErrLibvirtNotCompiled) {
				t.Errorf("%s error = %v, want errors.Is(err, ErrLibvirtNotCompiled) to be true", tt.name, err)
			}
		})
	}
}
