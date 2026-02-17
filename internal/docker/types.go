// Package docker provides Docker container and network management for the unraid-mcp server.
package docker

import (
	"context"
	"time"
)

// Container represents a Docker container summary.
type Container struct {
	ID      string
	Name    string
	Image   string
	State   string // "running", "exited", "paused", etc.
	Status  string // human-readable status like "Up 2 hours"
	Created time.Time
}

// ContainerDetail holds the full details of a Docker container.
type ContainerDetail struct {
	Container
	Config          ContainerConfig
	NetworkSettings NetworkInfo
	Mounts          []Mount
}

// ContainerConfig holds configuration details for a container.
type ContainerConfig struct {
	Env    []string
	Cmd    []string
	Labels map[string]string
}

// NetworkInfo describes a container's network settings.
type NetworkInfo struct {
	IPAddress string
	Ports     map[string]string // "80/tcp" -> "0.0.0.0:8080"
}

// Mount describes a bind mount or volume attached to a container.
type Mount struct {
	Source      string
	Destination string
	ReadOnly    bool
}

// ContainerCreateConfig holds parameters for creating a new container.
type ContainerCreateConfig struct {
	Name    string
	Image   string
	Env     []string
	Cmd     []string
	Labels  map[string]string
	Ports   map[string]string // container_port -> host_port
	Volumes map[string]string // host_path -> container_path
}

// ContainerStats holds runtime resource usage statistics for a container.
type ContainerStats struct {
	CPUPercent     float64
	MemoryUsage    uint64
	MemoryLimit    uint64
	NetworkRxBytes uint64
	NetworkTxBytes uint64
}

// Network represents a Docker network summary.
type Network struct {
	ID     string
	Name   string
	Driver string
	Scope  string
}

// NetworkDetail holds the full details of a Docker network.
type NetworkDetail struct {
	Network
	Containers []string // container IDs
	Subnet     string
	Gateway    string
}

// NetworkCreateConfig holds parameters for creating a new Docker network.
type NetworkCreateConfig struct {
	Name   string
	Driver string
	Subnet string
}

// DockerManager defines the operations available for managing Docker containers and networks.
type DockerManager interface {
	ListContainers(ctx context.Context, all bool) ([]Container, error)
	InspectContainer(ctx context.Context, id string) (*ContainerDetail, error)
	StartContainer(ctx context.Context, id string) error
	StopContainer(ctx context.Context, id string, timeout int) error
	RestartContainer(ctx context.Context, id string, timeout int) error
	RemoveContainer(ctx context.Context, id string, force bool) error
	CreateContainer(ctx context.Context, config ContainerCreateConfig) (string, error)
	PullImage(ctx context.Context, image string) error
	GetLogs(ctx context.Context, id string, tail int) (string, error)
	GetStats(ctx context.Context, id string) (*ContainerStats, error)
	ListNetworks(ctx context.Context) ([]Network, error)
	InspectNetwork(ctx context.Context, id string) (*NetworkDetail, error)
	CreateNetwork(ctx context.Context, config NetworkCreateConfig) (string, error)
	RemoveNetwork(ctx context.Context, id string) error
	ConnectNetwork(ctx context.Context, networkID, containerID string) error
	DisconnectNetwork(ctx context.Context, networkID, containerID string) error
}
