// Package ups provides types and interfaces for Unraid UPS monitoring.
package ups

import "context"

// UPSDevice represents a single UPS device.
type UPSDevice struct {
	ID      string     `json:"id"`
	Name    string     `json:"name"`
	Model   string     `json:"model"`
	Status  string     `json:"status"`
	Battery *Battery   `json:"battery"`
	Power   *PowerInfo `json:"power"`
}

// Battery contains battery charge and runtime information.
type Battery struct {
	Charge  *float64 `json:"charge"`
	Runtime *int     `json:"runtime"` // seconds
}

// PowerInfo contains power input/output and load information.
type PowerInfo struct {
	InputVoltage  *float64 `json:"inputVoltage"`
	OutputVoltage *float64 `json:"outputVoltage"`
	Load          *float64 `json:"load"`
}

// UPSMonitor defines the interface for UPS monitoring operations.
type UPSMonitor interface {
	GetDevices(ctx context.Context) ([]UPSDevice, error)
}
