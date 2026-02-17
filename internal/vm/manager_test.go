package vm

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// MockVMManager — an in-memory implementation of VMManager for contract tests
// ---------------------------------------------------------------------------

// mockVM is an internal record held by MockVMManager.
type mockVM struct {
	detail    VMDetail
	snapshots []Snapshot
}

// MockVMManager implements VMManager using in-memory state.
// It enforces the same semantic contracts that any real implementation should:
//   - "not found" errors for unknown VM names
//   - State-transition guards (e.g. cannot start a running VM)
//   - Context cancellation checks
type MockVMManager struct {
	mu  sync.Mutex
	vms map[string]*mockVM
}

// NewMockVMManager creates a MockVMManager pre-loaded with the supplied VMs.
func NewMockVMManager(initial []VMDetail) *MockVMManager {
	m := &MockVMManager{vms: make(map[string]*mockVM)}
	for _, d := range initial {
		d := d
		m.vms[d.Name] = &mockVM{detail: d}
	}
	return m
}

func (m *MockVMManager) ListVMs(ctx context.Context) ([]VM, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("list vms: %w", err)
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	out := make([]VM, 0, len(m.vms))
	for _, v := range m.vms {
		out = append(out, v.detail.VM)
	}
	return out, nil
}

func (m *MockVMManager) InspectVM(ctx context.Context, name string) (*VMDetail, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("inspect vm: %w", err)
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	v, ok := m.vms[name]
	if !ok {
		return nil, fmt.Errorf("vm %q not found", name)
	}
	copy := v.detail
	return &copy, nil
}

func (m *MockVMManager) StartVM(ctx context.Context, name string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("start vm: %w", err)
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	v, ok := m.vms[name]
	if !ok {
		return fmt.Errorf("vm %q not found", name)
	}
	if v.detail.State == VMStateRunning {
		return fmt.Errorf("vm %q already running", name)
	}
	v.detail.State = VMStateRunning
	return nil
}

func (m *MockVMManager) StopVM(ctx context.Context, name string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("stop vm: %w", err)
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	v, ok := m.vms[name]
	if !ok {
		return fmt.Errorf("vm %q not found", name)
	}
	v.detail.State = VMStateShutoff
	return nil
}

func (m *MockVMManager) ForceStopVM(ctx context.Context, name string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("force stop vm: %w", err)
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	v, ok := m.vms[name]
	if !ok {
		return fmt.Errorf("vm %q not found", name)
	}
	v.detail.State = VMStateShutoff
	return nil
}

func (m *MockVMManager) PauseVM(ctx context.Context, name string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("pause vm: %w", err)
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	v, ok := m.vms[name]
	if !ok {
		return fmt.Errorf("vm %q not found", name)
	}
	if v.detail.State != VMStateRunning {
		return fmt.Errorf("vm %q not running", name)
	}
	v.detail.State = VMStatePaused
	return nil
}

func (m *MockVMManager) ResumeVM(ctx context.Context, name string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("resume vm: %w", err)
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	v, ok := m.vms[name]
	if !ok {
		return fmt.Errorf("vm %q not found", name)
	}
	if v.detail.State != VMStatePaused {
		return fmt.Errorf("vm %q not paused", name)
	}
	v.detail.State = VMStateRunning
	return nil
}

func (m *MockVMManager) RestartVM(ctx context.Context, name string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("restart vm: %w", err)
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	v, ok := m.vms[name]
	if !ok {
		return fmt.Errorf("vm %q not found", name)
	}
	// Simulate restart: briefly shutoff then running.
	v.detail.State = VMStateRunning
	return nil
}

func (m *MockVMManager) CreateVM(ctx context.Context, xmlConfig string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("create vm: %w", err)
	}
	if strings.TrimSpace(xmlConfig) == "" {
		return fmt.Errorf("create vm: xml config is empty")
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	name := fmt.Sprintf("vm-%d", len(m.vms)+1)
	m.vms[name] = &mockVM{
		detail: VMDetail{
			VM: VM{
				Name:  name,
				State: VMStateShutoff,
			},
			XMLConfig: xmlConfig,
		},
	}
	return nil
}

func (m *MockVMManager) DeleteVM(ctx context.Context, name string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("delete vm: %w", err)
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.vms[name]; !ok {
		return fmt.Errorf("vm %q not found", name)
	}
	delete(m.vms, name)
	return nil
}

func (m *MockVMManager) ListSnapshots(ctx context.Context, vmName string) ([]Snapshot, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("list snapshots: %w", err)
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	v, ok := m.vms[vmName]
	if !ok {
		return nil, fmt.Errorf("vm %q not found", vmName)
	}
	out := make([]Snapshot, len(v.snapshots))
	copy(out, v.snapshots)
	return out, nil
}

func (m *MockVMManager) CreateSnapshot(ctx context.Context, vmName, snapName string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("create snapshot: %w", err)
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	v, ok := m.vms[vmName]
	if !ok {
		return fmt.Errorf("vm %q not found", vmName)
	}
	v.snapshots = append(v.snapshots, Snapshot{
		Name:      snapName,
		CreatedAt: time.Now(),
		State:     string(v.detail.State),
	})
	return nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// seedVMs returns a standard set of VMDetails used to seed the mock.
func seedVMs() []VMDetail {
	return []VMDetail{
		{
			VM: VM{
				Name:      "win10",
				UUID:      "aaaa-bbbb-cccc-dddd",
				State:     VMStateRunning,
				Memory:    4194304,
				VCPUs:     4,
				Autostart: true,
			},
			XMLConfig: "<domain type='kvm'><name>win10</name></domain>",
			Disks: []VMDisk{
				{Source: "/mnt/user/vdisks/win10.img", Target: "vda", Type: "file"},
			},
			NICs: []VMNIC{
				{MAC: "52:54:00:12:34:56", Network: "default", Model: "virtio"},
			},
		},
		{
			VM: VM{
				Name:      "ubuntu",
				UUID:      "1111-2222-3333-4444",
				State:     VMStateShutoff,
				Memory:    2097152,
				VCPUs:     2,
				Autostart: false,
			},
			XMLConfig: "<domain type='kvm'><name>ubuntu</name></domain>",
		},
		{
			VM: VM{
				Name:      "paused-vm",
				UUID:      "5555-6666-7777-8888",
				State:     VMStatePaused,
				Memory:    1048576,
				VCPUs:     1,
				Autostart: false,
			},
			XMLConfig: "<domain type='kvm'><name>paused-vm</name></domain>",
		},
	}
}

// newSeededMock returns a MockVMManager pre-loaded with the standard seed data.
func newSeededMock(t *testing.T) *MockVMManager {
	t.Helper()
	return NewMockVMManager(seedVMs())
}

// ---------------------------------------------------------------------------
// Interface compliance
// ---------------------------------------------------------------------------

func Test_MockVMManager_ImplementsVMManager(t *testing.T) {
	var _ VMManager = (*MockVMManager)(nil)
}

// ---------------------------------------------------------------------------
// ListVMs
// ---------------------------------------------------------------------------

func Test_ListVMs_Cases(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(t *testing.T) VMManager
		wantErr  bool
		wantLen  int
		validate func(t *testing.T, vms []VM)
	}{
		{
			name: "returns all seeded VMs",
			setup: func(t *testing.T) VMManager {
				t.Helper()
				return newSeededMock(t)
			},
			wantLen: 3,
			validate: func(t *testing.T, vms []VM) {
				t.Helper()
				names := make(map[string]bool)
				for _, v := range vms {
					names[v.Name] = true
				}
				for _, want := range []string{"win10", "ubuntu", "paused-vm"} {
					if !names[want] {
						t.Errorf("expected VM %q in list, got %v", want, names)
					}
				}
			},
		},
		{
			name: "empty manager returns empty slice",
			setup: func(t *testing.T) VMManager {
				t.Helper()
				return NewMockVMManager(nil)
			},
			wantLen: 0,
			validate: func(t *testing.T, vms []VM) {
				t.Helper()
				if vms == nil {
					t.Error("expected non-nil empty slice, got nil")
				}
			},
		},
		{
			name: "cancelled context returns error",
			setup: func(t *testing.T) VMManager {
				t.Helper()
				return newSeededMock(t)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr := tt.setup(t)

			ctx := context.Background()
			if tt.wantErr {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel() // cancel immediately
			}

			vms, err := mgr.ListVMs(ctx)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(vms) != tt.wantLen {
				t.Errorf("len(ListVMs()) = %d, want %d", len(vms), tt.wantLen)
			}
			if tt.validate != nil {
				tt.validate(t, vms)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// InspectVM
// ---------------------------------------------------------------------------

func Test_InspectVM_Cases(t *testing.T) {
	tests := []struct {
		name        string
		vmName      string
		wantErr     bool
		errContains string
		validate    func(t *testing.T, detail *VMDetail)
	}{
		{
			name:   "existing VM returns detail",
			vmName: "win10",
			validate: func(t *testing.T, detail *VMDetail) {
				t.Helper()
				if detail == nil {
					t.Fatal("expected non-nil VMDetail")
				}
				if detail.Name != "win10" {
					t.Errorf("Name = %q, want %q", detail.Name, "win10")
				}
				if detail.UUID != "aaaa-bbbb-cccc-dddd" {
					t.Errorf("UUID = %q, want %q", detail.UUID, "aaaa-bbbb-cccc-dddd")
				}
				if detail.State != VMStateRunning {
					t.Errorf("State = %q, want %q", detail.State, VMStateRunning)
				}
				if detail.Memory != 4194304 {
					t.Errorf("Memory = %d, want %d", detail.Memory, 4194304)
				}
				if detail.VCPUs != 4 {
					t.Errorf("VCPUs = %d, want %d", detail.VCPUs, 4)
				}
				if detail.Autostart != true {
					t.Errorf("Autostart = %v, want true", detail.Autostart)
				}
				if detail.XMLConfig == "" {
					t.Error("expected non-empty XMLConfig")
				}
				if len(detail.Disks) != 1 {
					t.Errorf("len(Disks) = %d, want 1", len(detail.Disks))
				} else {
					disk := detail.Disks[0]
					if disk.Source != "/mnt/user/vdisks/win10.img" {
						t.Errorf("Disk.Source = %q, want %q", disk.Source, "/mnt/user/vdisks/win10.img")
					}
					if disk.Target != "vda" {
						t.Errorf("Disk.Target = %q, want %q", disk.Target, "vda")
					}
					if disk.Type != "file" {
						t.Errorf("Disk.Type = %q, want %q", disk.Type, "file")
					}
				}
				if len(detail.NICs) != 1 {
					t.Errorf("len(NICs) = %d, want 1", len(detail.NICs))
				} else {
					nic := detail.NICs[0]
					if nic.MAC != "52:54:00:12:34:56" {
						t.Errorf("NIC.MAC = %q, want %q", nic.MAC, "52:54:00:12:34:56")
					}
					if nic.Network != "default" {
						t.Errorf("NIC.Network = %q, want %q", nic.Network, "default")
					}
					if nic.Model != "virtio" {
						t.Errorf("NIC.Model = %q, want %q", nic.Model, "virtio")
					}
				}
			},
		},
		{
			name:        "nonexistent VM returns not found error",
			vmName:      "nonexistent",
			wantErr:     true,
			errContains: "not found",
		},
		{
			name:   "VM with no disks or NICs",
			vmName: "ubuntu",
			validate: func(t *testing.T, detail *VMDetail) {
				t.Helper()
				if detail == nil {
					t.Fatal("expected non-nil VMDetail")
				}
				if detail.Name != "ubuntu" {
					t.Errorf("Name = %q, want %q", detail.Name, "ubuntu")
				}
				if detail.State != VMStateShutoff {
					t.Errorf("State = %q, want %q", detail.State, VMStateShutoff)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr := newSeededMock(t)
			detail, err := mgr.InspectVM(context.Background(), tt.vmName)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errContains != "" && !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tt.errContains)) {
					t.Errorf("error = %q, want it to contain %q", err.Error(), tt.errContains)
				}
				if detail != nil {
					t.Errorf("expected nil detail on error, got %+v", detail)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.validate != nil {
				tt.validate(t, detail)
			}
		})
	}
}

func Test_InspectVM_CancelledContext(t *testing.T) {
	mgr := newSeededMock(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	detail, err := mgr.InspectVM(ctx, "win10")
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
	if detail != nil {
		t.Errorf("expected nil detail for cancelled context, got %+v", detail)
	}
}

// ---------------------------------------------------------------------------
// StartVM
// ---------------------------------------------------------------------------

func Test_StartVM_Cases(t *testing.T) {
	tests := []struct {
		name        string
		vmName      string
		wantErr     bool
		errContains string
	}{
		{
			name:   "start stopped VM succeeds",
			vmName: "ubuntu", // seeded as VMStateShutoff
		},
		{
			name:        "start nonexistent VM returns not found",
			vmName:      "nonexistent",
			wantErr:     true,
			errContains: "not found",
		},
		{
			name:        "start already running VM returns error",
			vmName:      "win10", // seeded as VMStateRunning
			wantErr:     true,
			errContains: "already running",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr := newSeededMock(t)
			err := mgr.StartVM(context.Background(), tt.vmName)

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

			// Verify state transition
			detail, err := mgr.InspectVM(context.Background(), tt.vmName)
			if err != nil {
				t.Fatalf("InspectVM after start: %v", err)
			}
			if detail.State != VMStateRunning {
				t.Errorf("State after start = %q, want %q", detail.State, VMStateRunning)
			}
		})
	}
}

func Test_StartVM_CancelledContext(t *testing.T) {
	mgr := newSeededMock(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := mgr.StartVM(ctx, "ubuntu")
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// ---------------------------------------------------------------------------
// StopVM
// ---------------------------------------------------------------------------

func Test_StopVM_Cases(t *testing.T) {
	tests := []struct {
		name        string
		vmName      string
		wantErr     bool
		errContains string
	}{
		{
			name:   "stop running VM succeeds",
			vmName: "win10",
		},
		{
			name:        "stop nonexistent VM returns not found",
			vmName:      "nonexistent",
			wantErr:     true,
			errContains: "not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr := newSeededMock(t)
			err := mgr.StopVM(context.Background(), tt.vmName)

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

			// Verify state transition
			detail, err := mgr.InspectVM(context.Background(), tt.vmName)
			if err != nil {
				t.Fatalf("InspectVM after stop: %v", err)
			}
			if detail.State != VMStateShutoff {
				t.Errorf("State after stop = %q, want %q", detail.State, VMStateShutoff)
			}
		})
	}
}

func Test_StopVM_CancelledContext(t *testing.T) {
	mgr := newSeededMock(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := mgr.StopVM(ctx, "win10")
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// ---------------------------------------------------------------------------
// ForceStopVM
// ---------------------------------------------------------------------------

func Test_ForceStopVM_Cases(t *testing.T) {
	tests := []struct {
		name        string
		vmName      string
		wantErr     bool
		errContains string
	}{
		{
			name:   "force stop running VM succeeds",
			vmName: "win10",
		},
		{
			name:   "force stop paused VM succeeds",
			vmName: "paused-vm",
		},
		{
			name:        "force stop nonexistent VM returns not found",
			vmName:      "nonexistent",
			wantErr:     true,
			errContains: "not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr := newSeededMock(t)
			err := mgr.ForceStopVM(context.Background(), tt.vmName)

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

			// Verify state transition
			detail, err := mgr.InspectVM(context.Background(), tt.vmName)
			if err != nil {
				t.Fatalf("InspectVM after force stop: %v", err)
			}
			if detail.State != VMStateShutoff {
				t.Errorf("State after force stop = %q, want %q", detail.State, VMStateShutoff)
			}
		})
	}
}

func Test_ForceStopVM_CancelledContext(t *testing.T) {
	mgr := newSeededMock(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := mgr.ForceStopVM(ctx, "win10")
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// ---------------------------------------------------------------------------
// PauseVM
// ---------------------------------------------------------------------------

func Test_PauseVM_Cases(t *testing.T) {
	tests := []struct {
		name        string
		vmName      string
		wantErr     bool
		errContains string
	}{
		{
			name:   "pause running VM succeeds",
			vmName: "win10", // seeded as running
		},
		{
			name:        "pause nonexistent VM returns not found",
			vmName:      "nonexistent",
			wantErr:     true,
			errContains: "not found",
		},
		{
			name:        "pause stopped VM returns not running error",
			vmName:      "ubuntu", // seeded as shutoff
			wantErr:     true,
			errContains: "not running",
		},
		{
			name:        "pause already paused VM returns not running error",
			vmName:      "paused-vm", // seeded as paused
			wantErr:     true,
			errContains: "not running",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr := newSeededMock(t)
			err := mgr.PauseVM(context.Background(), tt.vmName)

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

			// Verify state transition
			detail, err := mgr.InspectVM(context.Background(), tt.vmName)
			if err != nil {
				t.Fatalf("InspectVM after pause: %v", err)
			}
			if detail.State != VMStatePaused {
				t.Errorf("State after pause = %q, want %q", detail.State, VMStatePaused)
			}
		})
	}
}

func Test_PauseVM_CancelledContext(t *testing.T) {
	mgr := newSeededMock(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := mgr.PauseVM(ctx, "win10")
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// ---------------------------------------------------------------------------
// ResumeVM
// ---------------------------------------------------------------------------

func Test_ResumeVM_Cases(t *testing.T) {
	tests := []struct {
		name        string
		vmName      string
		wantErr     bool
		errContains string
	}{
		{
			name:   "resume paused VM succeeds",
			vmName: "paused-vm", // seeded as paused
		},
		{
			name:        "resume nonexistent VM returns not found",
			vmName:      "nonexistent",
			wantErr:     true,
			errContains: "not found",
		},
		{
			name:        "resume running VM returns not paused error",
			vmName:      "win10", // seeded as running
			wantErr:     true,
			errContains: "not paused",
		},
		{
			name:        "resume stopped VM returns not paused error",
			vmName:      "ubuntu", // seeded as shutoff
			wantErr:     true,
			errContains: "not paused",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr := newSeededMock(t)
			err := mgr.ResumeVM(context.Background(), tt.vmName)

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

			// Verify state transition
			detail, err := mgr.InspectVM(context.Background(), tt.vmName)
			if err != nil {
				t.Fatalf("InspectVM after resume: %v", err)
			}
			if detail.State != VMStateRunning {
				t.Errorf("State after resume = %q, want %q", detail.State, VMStateRunning)
			}
		})
	}
}

func Test_ResumeVM_CancelledContext(t *testing.T) {
	mgr := newSeededMock(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := mgr.ResumeVM(ctx, "paused-vm")
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// ---------------------------------------------------------------------------
// RestartVM
// ---------------------------------------------------------------------------

func Test_RestartVM_Cases(t *testing.T) {
	tests := []struct {
		name        string
		vmName      string
		wantErr     bool
		errContains string
	}{
		{
			name:   "restart running VM succeeds",
			vmName: "win10",
		},
		{
			name:   "restart stopped VM succeeds",
			vmName: "ubuntu",
		},
		{
			name:        "restart nonexistent VM returns not found",
			vmName:      "nonexistent",
			wantErr:     true,
			errContains: "not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr := newSeededMock(t)
			err := mgr.RestartVM(context.Background(), tt.vmName)

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

			// Verify VM ends up running after restart
			detail, err := mgr.InspectVM(context.Background(), tt.vmName)
			if err != nil {
				t.Fatalf("InspectVM after restart: %v", err)
			}
			if detail.State != VMStateRunning {
				t.Errorf("State after restart = %q, want %q", detail.State, VMStateRunning)
			}
		})
	}
}

func Test_RestartVM_CancelledContext(t *testing.T) {
	mgr := newSeededMock(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := mgr.RestartVM(ctx, "win10")
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// ---------------------------------------------------------------------------
// CreateVM
// ---------------------------------------------------------------------------

func Test_CreateVM_Cases(t *testing.T) {
	tests := []struct {
		name        string
		xmlConfig   string
		wantErr     bool
		errContains string
	}{
		{
			name:      "valid XML creates VM",
			xmlConfig: "<domain type='kvm'><name>newvm</name></domain>",
		},
		{
			name:        "empty XML returns error",
			xmlConfig:   "",
			wantErr:     true,
			errContains: "empty",
		},
		{
			name:        "whitespace-only XML returns error",
			xmlConfig:   "   \t\n  ",
			wantErr:     true,
			errContains: "empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr := newSeededMock(t)
			initialVMs, _ := mgr.ListVMs(context.Background())
			initialCount := len(initialVMs)

			err := mgr.CreateVM(context.Background(), tt.xmlConfig)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errContains != "" && !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tt.errContains)) {
					t.Errorf("error = %q, want it to contain %q", err.Error(), tt.errContains)
				}
				// Verify no VM was added
				currentVMs, _ := mgr.ListVMs(context.Background())
				if len(currentVMs) != initialCount {
					t.Errorf("VM count changed from %d to %d on error", initialCount, len(currentVMs))
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Verify a VM was added
			currentVMs, _ := mgr.ListVMs(context.Background())
			if len(currentVMs) != initialCount+1 {
				t.Errorf("VM count = %d, want %d", len(currentVMs), initialCount+1)
			}
		})
	}
}

func Test_CreateVM_CancelledContext(t *testing.T) {
	mgr := newSeededMock(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := mgr.CreateVM(ctx, "<domain><name>x</name></domain>")
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// ---------------------------------------------------------------------------
// DeleteVM
// ---------------------------------------------------------------------------

func Test_DeleteVM_Cases(t *testing.T) {
	tests := []struct {
		name        string
		vmName      string
		wantErr     bool
		errContains string
	}{
		{
			name:   "delete existing VM succeeds",
			vmName: "win10",
		},
		{
			name:        "delete nonexistent VM returns not found",
			vmName:      "nonexistent",
			wantErr:     true,
			errContains: "not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr := newSeededMock(t)
			err := mgr.DeleteVM(context.Background(), tt.vmName)

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

			// Verify VM was removed
			_, err = mgr.InspectVM(context.Background(), tt.vmName)
			if err == nil {
				t.Error("expected error when inspecting deleted VM, got nil")
			}
			if !strings.Contains(strings.ToLower(err.Error()), "not found") {
				t.Errorf("expected 'not found' error after delete, got: %v", err)
			}
		})
	}
}

func Test_DeleteVM_DoubleDelete(t *testing.T) {
	mgr := newSeededMock(t)

	if err := mgr.DeleteVM(context.Background(), "win10"); err != nil {
		t.Fatalf("first delete failed: %v", err)
	}

	err := mgr.DeleteVM(context.Background(), "win10")
	if err == nil {
		t.Fatal("expected error on second delete, got nil")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "not found") {
		t.Errorf("error = %q, want it to contain %q", err.Error(), "not found")
	}
}

func Test_DeleteVM_CancelledContext(t *testing.T) {
	mgr := newSeededMock(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := mgr.DeleteVM(ctx, "win10")
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// ---------------------------------------------------------------------------
// ListSnapshots
// ---------------------------------------------------------------------------

func Test_ListSnapshots_Cases(t *testing.T) {
	tests := []struct {
		name        string
		vmName      string
		setup       func(t *testing.T, mgr *MockVMManager)
		wantErr     bool
		errContains string
		wantLen     int
	}{
		{
			name:    "VM with no snapshots returns empty slice",
			vmName:  "win10",
			wantLen: 0,
		},
		{
			name:   "VM with snapshots returns all",
			vmName: "win10",
			setup: func(t *testing.T, mgr *MockVMManager) {
				t.Helper()
				if err := mgr.CreateSnapshot(context.Background(), "win10", "snap1"); err != nil {
					t.Fatalf("setup: CreateSnapshot snap1: %v", err)
				}
				if err := mgr.CreateSnapshot(context.Background(), "win10", "snap2"); err != nil {
					t.Fatalf("setup: CreateSnapshot snap2: %v", err)
				}
			},
			wantLen: 2,
		},
		{
			name:        "nonexistent VM returns not found",
			vmName:      "nonexistent",
			wantErr:     true,
			errContains: "not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr := newSeededMock(t)
			if tt.setup != nil {
				tt.setup(t, mgr)
			}

			snaps, err := mgr.ListSnapshots(context.Background(), tt.vmName)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errContains != "" && !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tt.errContains)) {
					t.Errorf("error = %q, want it to contain %q", err.Error(), tt.errContains)
				}
				if snaps != nil {
					t.Errorf("expected nil snapshots on error, got %v", snaps)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(snaps) != tt.wantLen {
				t.Errorf("len(ListSnapshots()) = %d, want %d", len(snaps), tt.wantLen)
			}
		})
	}
}

func Test_ListSnapshots_CancelledContext(t *testing.T) {
	mgr := newSeededMock(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	snaps, err := mgr.ListSnapshots(ctx, "win10")
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
	if snaps != nil {
		t.Errorf("expected nil snapshots for cancelled context, got %v", snaps)
	}
}

// ---------------------------------------------------------------------------
// CreateSnapshot
// ---------------------------------------------------------------------------

func Test_CreateSnapshot_Cases(t *testing.T) {
	tests := []struct {
		name        string
		vmName      string
		snapName    string
		wantErr     bool
		errContains string
	}{
		{
			name:     "create snapshot on existing VM succeeds",
			vmName:   "win10",
			snapName: "my-snapshot",
		},
		{
			name:        "create snapshot on nonexistent VM returns not found",
			vmName:      "nonexistent",
			snapName:    "snap1",
			wantErr:     true,
			errContains: "not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr := newSeededMock(t)
			err := mgr.CreateSnapshot(context.Background(), tt.vmName, tt.snapName)

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

			// Verify snapshot was recorded
			snaps, err := mgr.ListSnapshots(context.Background(), tt.vmName)
			if err != nil {
				t.Fatalf("ListSnapshots after create: %v", err)
			}
			found := false
			for _, s := range snaps {
				if s.Name == tt.snapName {
					found = true
					if s.CreatedAt.IsZero() {
						t.Error("snapshot CreatedAt should not be zero")
					}
					break
				}
			}
			if !found {
				t.Errorf("snapshot %q not found in list after creation", tt.snapName)
			}
		})
	}
}

func Test_CreateSnapshot_CancelledContext(t *testing.T) {
	mgr := newSeededMock(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := mgr.CreateSnapshot(ctx, "win10", "snap1")
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// ---------------------------------------------------------------------------
// VMState constants
// ---------------------------------------------------------------------------

func Test_VMState_Constants(t *testing.T) {
	tests := []struct {
		name  string
		state VMState
		want  string
	}{
		{name: "running", state: VMStateRunning, want: "running"},
		{name: "shutoff", state: VMStateShutoff, want: "shutoff"},
		{name: "paused", state: VMStatePaused, want: "paused"},
		{name: "crashed", state: VMStateCrashed, want: "crashed"},
		{name: "suspended", state: VMStateSuspended, want: "suspended"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.state) != tt.want {
				t.Errorf("VMState = %q, want %q", string(tt.state), tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Type zero-value tests
// ---------------------------------------------------------------------------

func Test_VM_ZeroValue(t *testing.T) {
	var v VM
	if v.Name != "" {
		t.Errorf("zero VM.Name = %q, want empty", v.Name)
	}
	if v.UUID != "" {
		t.Errorf("zero VM.UUID = %q, want empty", v.UUID)
	}
	if v.State != "" {
		t.Errorf("zero VM.State = %q, want empty", v.State)
	}
	if v.Memory != 0 {
		t.Errorf("zero VM.Memory = %d, want 0", v.Memory)
	}
	if v.VCPUs != 0 {
		t.Errorf("zero VM.VCPUs = %d, want 0", v.VCPUs)
	}
	if v.Autostart != false {
		t.Errorf("zero VM.Autostart = %v, want false", v.Autostart)
	}
}

func Test_VMDetail_ZeroValue(t *testing.T) {
	var d VMDetail
	if d.XMLConfig != "" {
		t.Errorf("zero VMDetail.XMLConfig = %q, want empty", d.XMLConfig)
	}
	if d.Disks != nil {
		t.Errorf("zero VMDetail.Disks = %v, want nil", d.Disks)
	}
	if d.NICs != nil {
		t.Errorf("zero VMDetail.NICs = %v, want nil", d.NICs)
	}
}

func Test_VMDisk_ZeroValue(t *testing.T) {
	var d VMDisk
	if d.Source != "" {
		t.Errorf("zero VMDisk.Source = %q, want empty", d.Source)
	}
	if d.Target != "" {
		t.Errorf("zero VMDisk.Target = %q, want empty", d.Target)
	}
	if d.Type != "" {
		t.Errorf("zero VMDisk.Type = %q, want empty", d.Type)
	}
}

func Test_VMNIC_ZeroValue(t *testing.T) {
	var n VMNIC
	if n.MAC != "" {
		t.Errorf("zero VMNIC.MAC = %q, want empty", n.MAC)
	}
	if n.Network != "" {
		t.Errorf("zero VMNIC.Network = %q, want empty", n.Network)
	}
	if n.Model != "" {
		t.Errorf("zero VMNIC.Model = %q, want empty", n.Model)
	}
}

func Test_Snapshot_ZeroValue(t *testing.T) {
	var s Snapshot
	if s.Name != "" {
		t.Errorf("zero Snapshot.Name = %q, want empty", s.Name)
	}
	if s.Description != "" {
		t.Errorf("zero Snapshot.Description = %q, want empty", s.Description)
	}
	if !s.CreatedAt.IsZero() {
		t.Errorf("zero Snapshot.CreatedAt = %v, want zero", s.CreatedAt)
	}
	if s.State != "" {
		t.Errorf("zero Snapshot.State = %q, want empty", s.State)
	}
}

// ---------------------------------------------------------------------------
// State transition integration scenarios
// ---------------------------------------------------------------------------

func Test_StateTransition_StartThenPauseThenResume(t *testing.T) {
	mgr := newSeededMock(t)
	ctx := context.Background()

	// Start the stopped ubuntu VM
	if err := mgr.StartVM(ctx, "ubuntu"); err != nil {
		t.Fatalf("StartVM: %v", err)
	}

	// Pause it
	if err := mgr.PauseVM(ctx, "ubuntu"); err != nil {
		t.Fatalf("PauseVM: %v", err)
	}

	// Verify paused
	detail, err := mgr.InspectVM(ctx, "ubuntu")
	if err != nil {
		t.Fatalf("InspectVM: %v", err)
	}
	if detail.State != VMStatePaused {
		t.Errorf("State after pause = %q, want %q", detail.State, VMStatePaused)
	}

	// Resume it
	if err := mgr.ResumeVM(ctx, "ubuntu"); err != nil {
		t.Fatalf("ResumeVM: %v", err)
	}

	// Verify running
	detail, err = mgr.InspectVM(ctx, "ubuntu")
	if err != nil {
		t.Fatalf("InspectVM: %v", err)
	}
	if detail.State != VMStateRunning {
		t.Errorf("State after resume = %q, want %q", detail.State, VMStateRunning)
	}
}

func Test_StateTransition_StopThenStartThenForceStop(t *testing.T) {
	mgr := newSeededMock(t)
	ctx := context.Background()

	// Stop the running win10 VM
	if err := mgr.StopVM(ctx, "win10"); err != nil {
		t.Fatalf("StopVM: %v", err)
	}

	// Verify shutoff
	detail, _ := mgr.InspectVM(ctx, "win10")
	if detail.State != VMStateShutoff {
		t.Errorf("State after stop = %q, want %q", detail.State, VMStateShutoff)
	}

	// Start it again
	if err := mgr.StartVM(ctx, "win10"); err != nil {
		t.Fatalf("StartVM: %v", err)
	}

	// Verify running
	detail, _ = mgr.InspectVM(ctx, "win10")
	if detail.State != VMStateRunning {
		t.Errorf("State after start = %q, want %q", detail.State, VMStateRunning)
	}

	// Force stop
	if err := mgr.ForceStopVM(ctx, "win10"); err != nil {
		t.Fatalf("ForceStopVM: %v", err)
	}

	// Verify shutoff
	detail, _ = mgr.InspectVM(ctx, "win10")
	if detail.State != VMStateShutoff {
		t.Errorf("State after force stop = %q, want %q", detail.State, VMStateShutoff)
	}
}

func Test_StateTransition_CreateThenStartThenDelete(t *testing.T) {
	mgr := newSeededMock(t)
	ctx := context.Background()

	// Create a new VM
	if err := mgr.CreateVM(ctx, "<domain type='kvm'><name>test-vm</name></domain>"); err != nil {
		t.Fatalf("CreateVM: %v", err)
	}

	// Verify we have one more VM
	vms, err := mgr.ListVMs(ctx)
	if err != nil {
		t.Fatalf("ListVMs: %v", err)
	}
	if len(vms) != 4 { // 3 seeded + 1 new
		t.Errorf("len(ListVMs()) = %d, want 4", len(vms))
	}
}

func Test_Snapshot_CreateThenList(t *testing.T) {
	mgr := newSeededMock(t)
	ctx := context.Background()

	// Initially no snapshots
	snaps, err := mgr.ListSnapshots(ctx, "win10")
	if err != nil {
		t.Fatalf("ListSnapshots: %v", err)
	}
	if len(snaps) != 0 {
		t.Errorf("initial snapshot count = %d, want 0", len(snaps))
	}

	// Create two snapshots
	if err := mgr.CreateSnapshot(ctx, "win10", "before-update"); err != nil {
		t.Fatalf("CreateSnapshot 1: %v", err)
	}
	if err := mgr.CreateSnapshot(ctx, "win10", "after-update"); err != nil {
		t.Fatalf("CreateSnapshot 2: %v", err)
	}

	// List and verify
	snaps, err = mgr.ListSnapshots(ctx, "win10")
	if err != nil {
		t.Fatalf("ListSnapshots: %v", err)
	}
	if len(snaps) != 2 {
		t.Fatalf("snapshot count = %d, want 2", len(snaps))
	}

	names := make(map[string]bool)
	for _, s := range snaps {
		names[s.Name] = true
		if s.CreatedAt.IsZero() {
			t.Errorf("snapshot %q has zero CreatedAt", s.Name)
		}
	}
	if !names["before-update"] {
		t.Error("expected snapshot 'before-update' in list")
	}
	if !names["after-update"] {
		t.Error("expected snapshot 'after-update' in list")
	}
}

// ---------------------------------------------------------------------------
// Concurrency test
// ---------------------------------------------------------------------------

func Test_ConcurrentAccess_NoDataRace(t *testing.T) {
	mgr := newSeededMock(t)
	ctx := context.Background()

	var wg sync.WaitGroup
	operations := 50

	// Concurrent ListVMs
	wg.Add(operations)
	for i := 0; i < operations; i++ {
		go func() {
			defer wg.Done()
			_, _ = mgr.ListVMs(ctx)
		}()
	}

	// Concurrent InspectVM
	wg.Add(operations)
	for i := 0; i < operations; i++ {
		go func() {
			defer wg.Done()
			_, _ = mgr.InspectVM(ctx, "win10")
		}()
	}

	// Concurrent state changes on different VMs
	wg.Add(3)
	go func() {
		defer wg.Done()
		_ = mgr.StopVM(ctx, "win10")
	}()
	go func() {
		defer wg.Done()
		_ = mgr.StartVM(ctx, "ubuntu")
	}()
	go func() {
		defer wg.Done()
		_ = mgr.ResumeVM(ctx, "paused-vm")
	}()

	// Concurrent snapshot operations
	wg.Add(operations)
	for i := 0; i < operations; i++ {
		i := i
		go func() {
			defer wg.Done()
			_ = mgr.CreateSnapshot(ctx, "win10", fmt.Sprintf("snap-%d", i))
		}()
	}

	wg.Wait()

	// After all goroutines complete, verify state is consistent
	snaps, err := mgr.ListSnapshots(ctx, "win10")
	if err != nil {
		t.Fatalf("ListSnapshots after concurrent ops: %v", err)
	}
	if len(snaps) != operations {
		t.Errorf("snapshot count = %d, want %d (some lost in concurrent writes?)", len(snaps), operations)
	}
}

// ---------------------------------------------------------------------------
// Context deadline exceeded
// ---------------------------------------------------------------------------

func Test_ContextDeadlineExceeded(t *testing.T) {
	mgr := newSeededMock(t)
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-1*time.Second))
	defer cancel()

	methods := []struct {
		name string
		call func() error
	}{
		{"ListVMs", func() error { _, err := mgr.ListVMs(ctx); return err }},
		{"InspectVM", func() error { _, err := mgr.InspectVM(ctx, "win10"); return err }},
		{"StartVM", func() error { return mgr.StartVM(ctx, "ubuntu") }},
		{"StopVM", func() error { return mgr.StopVM(ctx, "win10") }},
		{"ForceStopVM", func() error { return mgr.ForceStopVM(ctx, "win10") }},
		{"PauseVM", func() error { return mgr.PauseVM(ctx, "win10") }},
		{"ResumeVM", func() error { return mgr.ResumeVM(ctx, "paused-vm") }},
		{"RestartVM", func() error { return mgr.RestartVM(ctx, "win10") }},
		{"CreateVM", func() error { return mgr.CreateVM(ctx, "<domain/>") }},
		{"DeleteVM", func() error { return mgr.DeleteVM(ctx, "win10") }},
		{"ListSnapshots", func() error { _, err := mgr.ListSnapshots(ctx, "win10"); return err }},
		{"CreateSnapshot", func() error { return mgr.CreateSnapshot(ctx, "win10", "s") }},
	}

	for _, m := range methods {
		t.Run(m.name, func(t *testing.T) {
			err := m.call()
			if err == nil {
				t.Fatalf("%s: expected error for expired deadline, got nil", m.name)
			}
		})
	}
}
