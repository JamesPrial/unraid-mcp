// Package vm provides virtual machine management for Unraid systems via libvirt.
package vm

import (
	"context"
	"time"
)

// VMState represents the current state of a virtual machine.
type VMState string

const (
	VMStateRunning   VMState = "running"
	VMStateShutoff   VMState = "shutoff"
	VMStatePaused    VMState = "paused"
	VMStateCrashed   VMState = "crashed"
	VMStateSuspended VMState = "suspended"
)

// VM holds the summary information for a virtual machine.
type VM struct {
	Name      string
	UUID      string
	State     VMState
	Memory    uint64 // KB
	VCPUs     int
	Autostart bool
}

// VMDisk describes a disk attached to a virtual machine.
type VMDisk struct {
	Source string
	Target string
	Type   string // "file", "block"
}

// VMNIC describes a network interface attached to a virtual machine.
type VMNIC struct {
	MAC     string
	Network string
	Model   string
}

// VMDetail holds the full details of a virtual machine, including its
// XML configuration and attached devices.
type VMDetail struct {
	VM
	XMLConfig string
	Disks     []VMDisk
	NICs      []VMNIC
}

// Snapshot holds metadata about a virtual machine snapshot.
type Snapshot struct {
	Name        string
	Description string
	CreatedAt   time.Time
	State       string
}

// VMManager defines the interface for managing virtual machines.
type VMManager interface {
	ListVMs(ctx context.Context) ([]VM, error)
	InspectVM(ctx context.Context, name string) (*VMDetail, error)
	StartVM(ctx context.Context, name string) error
	StopVM(ctx context.Context, name string) error
	ForceStopVM(ctx context.Context, name string) error
	PauseVM(ctx context.Context, name string) error
	ResumeVM(ctx context.Context, name string) error
	RestartVM(ctx context.Context, name string) error
	CreateVM(ctx context.Context, xmlConfig string) error
	DeleteVM(ctx context.Context, name string) error
	ListSnapshots(ctx context.Context, vmName string) ([]Snapshot, error)
	CreateSnapshot(ctx context.Context, vmName, snapName string) error
}
