//go:build !libvirt

// Package vm provides virtual machine management for Unraid systems via libvirt.
//
// This file provides a stub LibvirtVMManager that is compiled when the
// "libvirt" build tag is NOT present (e.g. during unit tests or on systems
// without libvirt installed).
//
// To build with the real libvirt implementation, use:
//
//	go build -tags libvirt ./...
package vm

import (
	"context"
	"fmt"
)

// LibvirtVMManager is the production VM manager backed by libvirt.
// This stub is compiled when the "libvirt" build tag is absent.
// The real implementation (requiring github.com/digitalocean/go-libvirt) is in
// manager.go and is guarded by the "libvirt" build tag.
type LibvirtVMManager struct {
	socketPath string
}

// NewLibvirtVMManager returns an error in stub mode because the real libvirt
// client is not compiled in.  Build with -tags libvirt for production use.
func NewLibvirtVMManager(socketPath string) (*LibvirtVMManager, error) {
	return nil, fmt.Errorf(
		"libvirt support not compiled: rebuild with -tags libvirt and ensure "+
			"github.com/digitalocean/go-libvirt is in go.mod (socket: %s)",
		socketPath,
	)
}

// Close is a no-op in stub mode.
func (m *LibvirtVMManager) Close() error { return nil }

// ListVMs always returns an error in stub mode.
func (m *LibvirtVMManager) ListVMs(_ context.Context) ([]VM, error) {
	return nil, fmt.Errorf("libvirt support not compiled: rebuild with -tags libvirt")
}

// InspectVM always returns an error in stub mode.
func (m *LibvirtVMManager) InspectVM(_ context.Context, name string) (*VMDetail, error) {
	return nil, fmt.Errorf("libvirt support not compiled: rebuild with -tags libvirt")
}

// StartVM always returns an error in stub mode.
func (m *LibvirtVMManager) StartVM(_ context.Context, name string) error {
	return fmt.Errorf("libvirt support not compiled: rebuild with -tags libvirt")
}

// StopVM always returns an error in stub mode.
func (m *LibvirtVMManager) StopVM(_ context.Context, name string) error {
	return fmt.Errorf("libvirt support not compiled: rebuild with -tags libvirt")
}

// ForceStopVM always returns an error in stub mode.
func (m *LibvirtVMManager) ForceStopVM(_ context.Context, name string) error {
	return fmt.Errorf("libvirt support not compiled: rebuild with -tags libvirt")
}

// PauseVM always returns an error in stub mode.
func (m *LibvirtVMManager) PauseVM(_ context.Context, name string) error {
	return fmt.Errorf("libvirt support not compiled: rebuild with -tags libvirt")
}

// ResumeVM always returns an error in stub mode.
func (m *LibvirtVMManager) ResumeVM(_ context.Context, name string) error {
	return fmt.Errorf("libvirt support not compiled: rebuild with -tags libvirt")
}

// RestartVM always returns an error in stub mode.
func (m *LibvirtVMManager) RestartVM(_ context.Context, name string) error {
	return fmt.Errorf("libvirt support not compiled: rebuild with -tags libvirt")
}

// CreateVM always returns an error in stub mode.
func (m *LibvirtVMManager) CreateVM(_ context.Context, xmlConfig string) error {
	return fmt.Errorf("libvirt support not compiled: rebuild with -tags libvirt")
}

// DeleteVM always returns an error in stub mode.
func (m *LibvirtVMManager) DeleteVM(_ context.Context, name string) error {
	return fmt.Errorf("libvirt support not compiled: rebuild with -tags libvirt")
}

// ListSnapshots always returns an error in stub mode.
func (m *LibvirtVMManager) ListSnapshots(_ context.Context, vmName string) ([]Snapshot, error) {
	return nil, fmt.Errorf("libvirt support not compiled: rebuild with -tags libvirt")
}

// CreateSnapshot always returns an error in stub mode.
func (m *LibvirtVMManager) CreateSnapshot(_ context.Context, vmName, snapName string) error {
	return fmt.Errorf("libvirt support not compiled: rebuild with -tags libvirt")
}
