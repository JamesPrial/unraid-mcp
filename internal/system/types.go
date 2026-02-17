// Package system provides system health monitoring for an Unraid server.
// It reads data from /proc, /sys/class/hwmon, and the emhttp state files.
package system

import "context"

// SystemOverview holds a point-in-time snapshot of the host's resource usage.
type SystemOverview struct {
	// CPUUsagePercent is the overall CPU utilisation as a percentage (0–100).
	CPUUsagePercent float64

	// Memory figures in kibibytes, as reported by /proc/meminfo.
	MemTotalKB     uint64
	MemFreeKB      uint64
	MemAvailableKB uint64
	SwapTotalKB    uint64
	SwapFreeKB     uint64

	// Temperatures lists every temperature sensor discovered under /sys/class/hwmon.
	Temperatures []Temperature
}

// Temperature represents a single hardware temperature sensor reading.
type Temperature struct {
	// Label is a human-readable identifier derived from the sensor path
	// (e.g. "hwmon0/temp1").
	Label string

	// Celsius is the sensor value converted from millidegrees to degrees Celsius.
	Celsius float64
}

// ArrayStatus describes the state of the Unraid storage array as reported by
// emhttp's var.ini file.
type ArrayStatus struct {
	// State is the raw mdState value (e.g. "STARTED", "STOPPED").
	State string

	// NumDisks is the total number of data disks in the array (mdNumDisks).
	NumDisks int

	// NumProtected is the number of disks currently protected (mdNumProtected).
	NumProtected int

	// NumInvalid is the number of disks in an invalid/error state (mdNumInvalid).
	NumInvalid int

	// SyncErrors is the cumulative parity-sync error count (sbSyncErrs).
	SyncErrors int

	// SyncProgress is the percentage of a running parity sync that has completed
	// (0 when no sync is active).
	SyncProgress float64
}

// DiskInfo describes a single disk entry from emhttp's disks.ini file.
type DiskInfo struct {
	// Name is the logical disk name used by Unraid (e.g. "disk1", "cache", "parity").
	Name string

	// Device is the kernel device node name without the /dev/ prefix (e.g. "sdb").
	Device string

	// Temp is the reported disk temperature in degrees Celsius.
	Temp int

	// Status is the Unraid disk status string (e.g. "DISK_OK", "DISK_NP").
	Status string

	// FsType is the filesystem type mounted on the disk (e.g. "xfs", "btrfs").
	FsType string

	// FsSize is the total filesystem capacity in kibibytes.
	FsSize uint64

	// FsUsed is the used filesystem space in kibibytes.
	FsUsed uint64
}

// SystemMonitor defines the read-only operations for querying system health.
type SystemMonitor interface {
	// GetOverview returns a current snapshot of CPU, memory, and temperature data.
	GetOverview(ctx context.Context) (*SystemOverview, error)

	// GetArrayStatus returns the current state of the Unraid storage array.
	GetArrayStatus(ctx context.Context) (*ArrayStatus, error)

	// GetDiskInfo returns per-disk details for every disk known to emhttp.
	GetDiskInfo(ctx context.Context) ([]DiskInfo, error)
}
