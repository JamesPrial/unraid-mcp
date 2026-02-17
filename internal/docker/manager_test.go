package docker

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// MockDockerManager — in-memory implementation of DockerManager for testing
// ---------------------------------------------------------------------------

// MockDockerManager stores containers and networks in memory and implements
// the DockerManager interface. It is safe for concurrent use.
type MockDockerManager struct {
	mu         sync.RWMutex
	containers map[string]*ContainerDetail
	networks   map[string]*NetworkDetail
	logs       map[string]string
	stats      map[string]*ContainerStats
	idCounter  int

	// networkLinks tracks which containers are connected to which networks.
	// Key: networkID, Value: set of containerIDs.
	networkLinks map[string]map[string]struct{}
}

// NewMockDockerManager returns a MockDockerManager pre-populated with no data.
func NewMockDockerManager() *MockDockerManager {
	return &MockDockerManager{
		containers:   make(map[string]*ContainerDetail),
		networks:     make(map[string]*NetworkDetail),
		logs:         make(map[string]string),
		stats:        make(map[string]*ContainerStats),
		networkLinks: make(map[string]map[string]struct{}),
	}
}

// AddContainer is a test helper that inserts a container into the mock store.
func (m *MockDockerManager) AddContainer(detail *ContainerDetail) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.containers[detail.ID] = detail
}

// AddNetwork is a test helper that inserts a network into the mock store.
func (m *MockDockerManager) AddNetwork(detail *NetworkDetail) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.networks[detail.ID] = detail
	if m.networkLinks[detail.ID] == nil {
		m.networkLinks[detail.ID] = make(map[string]struct{})
	}
}

// SetLogs is a test helper that sets the logs for a container.
func (m *MockDockerManager) SetLogs(containerID, logs string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.logs[containerID] = logs
}

// SetStats is a test helper that sets the stats for a container.
func (m *MockDockerManager) SetStats(containerID string, stats *ContainerStats) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stats[containerID] = stats
}

func (m *MockDockerManager) nextID() string {
	m.idCounter++
	return fmt.Sprintf("mock-%d", m.idCounter)
}

// checkCtx returns a context error if the context is done.
func checkCtx(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

func (m *MockDockerManager) ListContainers(ctx context.Context, all bool) ([]Container, error) {
	if err := checkCtx(ctx); err != nil {
		return nil, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []Container
	for _, c := range m.containers {
		if all || c.State == "running" {
			result = append(result, c.Container)
		}
	}
	return result, nil
}

func (m *MockDockerManager) InspectContainer(ctx context.Context, id string) (*ContainerDetail, error) {
	if err := checkCtx(ctx); err != nil {
		return nil, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()

	c, ok := m.containers[id]
	if !ok {
		return nil, fmt.Errorf("container not found: %s", id)
	}
	return c, nil
}

func (m *MockDockerManager) StartContainer(ctx context.Context, id string) error {
	if err := checkCtx(ctx); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	c, ok := m.containers[id]
	if !ok {
		return fmt.Errorf("container not found: %s", id)
	}
	c.State = "running"
	c.Status = "Up 0 seconds"
	return nil
}

func (m *MockDockerManager) StopContainer(ctx context.Context, id string, timeout int) error {
	if err := checkCtx(ctx); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	c, ok := m.containers[id]
	if !ok {
		return fmt.Errorf("container not found: %s", id)
	}
	c.State = "exited"
	c.Status = "Exited (0)"
	return nil
}

func (m *MockDockerManager) RestartContainer(ctx context.Context, id string, timeout int) error {
	if err := checkCtx(ctx); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	c, ok := m.containers[id]
	if !ok {
		return fmt.Errorf("container not found: %s", id)
	}
	c.State = "running"
	c.Status = "Up 0 seconds"
	return nil
}

func (m *MockDockerManager) RemoveContainer(ctx context.Context, id string, force bool) error {
	if err := checkCtx(ctx); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	c, ok := m.containers[id]
	if !ok {
		return fmt.Errorf("container not found: %s", id)
	}
	if !force && c.State == "running" {
		return fmt.Errorf("container is running, use force to remove: %s", id)
	}
	delete(m.containers, id)
	delete(m.logs, id)
	delete(m.stats, id)
	return nil
}

func (m *MockDockerManager) CreateContainer(ctx context.Context, config ContainerCreateConfig) (string, error) {
	if err := checkCtx(ctx); err != nil {
		return "", err
	}
	if config.Image == "" {
		return "", fmt.Errorf("image is required")
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	id := m.nextID()
	var mounts []Mount
	for hostPath, containerPath := range config.Volumes {
		mounts = append(mounts, Mount{
			Source:      hostPath,
			Destination: containerPath,
		})
	}
	detail := &ContainerDetail{
		Container: Container{
			ID:      id,
			Name:    config.Name,
			Image:   config.Image,
			State:   "created",
			Status:  "Created",
			Created: time.Now(),
		},
		Config: ContainerConfig{
			Env:    config.Env,
			Cmd:    config.Cmd,
			Labels: config.Labels,
		},
		Mounts: mounts,
	}
	m.containers[id] = detail
	return id, nil
}

func (m *MockDockerManager) PullImage(ctx context.Context, image string) error {
	if err := checkCtx(ctx); err != nil {
		return err
	}
	if image == "" {
		return fmt.Errorf("image name is required")
	}
	return nil
}

func (m *MockDockerManager) GetLogs(ctx context.Context, id string, tail int) (string, error) {
	if err := checkCtx(ctx); err != nil {
		return "", err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()

	if _, ok := m.containers[id]; !ok {
		return "", fmt.Errorf("container not found: %s", id)
	}
	logs, ok := m.logs[id]
	if !ok {
		return "", nil
	}
	// Simple tail implementation.
	lines := strings.Split(logs, "\n")
	if tail > 0 && tail < len(lines) {
		lines = lines[len(lines)-tail:]
	}
	return strings.Join(lines, "\n"), nil
}

func (m *MockDockerManager) GetStats(ctx context.Context, id string) (*ContainerStats, error) {
	if err := checkCtx(ctx); err != nil {
		return nil, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()

	if _, ok := m.containers[id]; !ok {
		return nil, fmt.Errorf("container not found: %s", id)
	}
	stats, ok := m.stats[id]
	if !ok {
		return &ContainerStats{}, nil
	}
	return stats, nil
}

func (m *MockDockerManager) ListNetworks(ctx context.Context) ([]Network, error) {
	if err := checkCtx(ctx); err != nil {
		return nil, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []Network
	for _, n := range m.networks {
		result = append(result, n.Network)
	}
	return result, nil
}

func (m *MockDockerManager) InspectNetwork(ctx context.Context, id string) (*NetworkDetail, error) {
	if err := checkCtx(ctx); err != nil {
		return nil, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()

	n, ok := m.networks[id]
	if !ok {
		return nil, fmt.Errorf("network not found: %s", id)
	}
	return n, nil
}

func (m *MockDockerManager) CreateNetwork(ctx context.Context, config NetworkCreateConfig) (string, error) {
	if err := checkCtx(ctx); err != nil {
		return "", err
	}
	if config.Name == "" {
		return "", fmt.Errorf("network name is required")
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	id := m.nextID()
	detail := &NetworkDetail{
		Network: Network{
			ID:     id,
			Name:   config.Name,
			Driver: config.Driver,
			Scope:  "local",
		},
		Subnet:     config.Subnet,
		Containers: nil,
	}
	m.networks[id] = detail
	m.networkLinks[id] = make(map[string]struct{})
	return id, nil
}

func (m *MockDockerManager) RemoveNetwork(ctx context.Context, id string) error {
	if err := checkCtx(ctx); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.networks[id]; !ok {
		return fmt.Errorf("network not found: %s", id)
	}
	delete(m.networks, id)
	delete(m.networkLinks, id)
	return nil
}

func (m *MockDockerManager) ConnectNetwork(ctx context.Context, networkID, containerID string) error {
	if err := checkCtx(ctx); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.networks[networkID]; !ok {
		return fmt.Errorf("network not found: %s", networkID)
	}
	if _, ok := m.containers[containerID]; !ok {
		return fmt.Errorf("container not found: %s", containerID)
	}
	m.networkLinks[networkID][containerID] = struct{}{}
	return nil
}

func (m *MockDockerManager) DisconnectNetwork(ctx context.Context, networkID, containerID string) error {
	if err := checkCtx(ctx); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.networks[networkID]; !ok {
		return fmt.Errorf("network not found: %s", networkID)
	}
	if _, ok := m.containers[containerID]; !ok {
		return fmt.Errorf("container not found: %s", containerID)
	}
	delete(m.networkLinks[networkID], containerID)
	return nil
}

// ---------------------------------------------------------------------------
// Interface compliance
// ---------------------------------------------------------------------------

// Test_MockDockerManager_ImplementsInterface is a compile-time check that
// MockDockerManager satisfies the DockerManager interface.
func Test_MockDockerManager_ImplementsInterface(t *testing.T) {
	var _ DockerManager = (*MockDockerManager)(nil)
}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// newPopulatedMock returns a mock with pre-seeded containers and networks.
func newPopulatedMock(t *testing.T) *MockDockerManager {
	t.Helper()
	m := NewMockDockerManager()

	m.AddContainer(&ContainerDetail{
		Container: Container{
			ID:      "abc123",
			Name:    "plex",
			Image:   "plexinc/pms-docker:latest",
			State:   "running",
			Status:  "Up 2 hours",
			Created: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		Config: ContainerConfig{
			Env:    []string{"TZ=America/New_York"},
			Cmd:    []string{"/init"},
			Labels: map[string]string{"app": "plex"},
		},
		NetworkSettings: NetworkInfo{
			IPAddress: "172.17.0.2",
			Ports:     map[string]string{"32400/tcp": "0.0.0.0:32400"},
		},
		Mounts: []Mount{
			{Source: "/mnt/user/media", Destination: "/data", ReadOnly: true},
		},
	})

	m.AddContainer(&ContainerDetail{
		Container: Container{
			ID:      "def456",
			Name:    "sonarr",
			Image:   "linuxserver/sonarr:latest",
			State:   "exited",
			Status:  "Exited (0) 1 hour ago",
			Created: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
		},
		Config: ContainerConfig{
			Env:    []string{"TZ=UTC"},
			Cmd:    []string{"/start"},
			Labels: map[string]string{"app": "sonarr"},
		},
	})

	m.AddContainer(&ContainerDetail{
		Container: Container{
			ID:      "ghi789",
			Name:    "radarr",
			Image:   "linuxserver/radarr:latest",
			State:   "running",
			Status:  "Up 30 minutes",
			Created: time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC),
		},
	})

	m.SetLogs("abc123", "line1\nline2\nline3\nline4\nline5")
	m.SetStats("abc123", &ContainerStats{
		CPUPercent:     25.5,
		MemoryUsage:    1024 * 1024 * 512,  // 512 MiB
		MemoryLimit:    1024 * 1024 * 1024, // 1 GiB
		NetworkRxBytes: 1000000,
		NetworkTxBytes: 500000,
	})

	m.AddNetwork(&NetworkDetail{
		Network: Network{
			ID:     "net1",
			Name:   "bridge",
			Driver: "bridge",
			Scope:  "local",
		},
		Containers: []string{"abc123"},
		Subnet:     "172.17.0.0/16",
		Gateway:    "172.17.0.1",
	})

	m.AddNetwork(&NetworkDetail{
		Network: Network{
			ID:     "net2",
			Name:   "custom-net",
			Driver: "bridge",
			Scope:  "local",
		},
		Containers: nil,
		Subnet:     "10.0.0.0/24",
		Gateway:    "10.0.0.1",
	})

	return m
}

// ---------------------------------------------------------------------------
// Container listing tests
// ---------------------------------------------------------------------------

func Test_ListContainers_Cases(t *testing.T) {
	tests := []struct {
		name      string
		all       bool
		wantCount int
		wantErr   bool
	}{
		{
			name:      "list all containers returns running and stopped",
			all:       true,
			wantCount: 3,
			wantErr:   false,
		},
		{
			name:      "list running only excludes stopped",
			all:       false,
			wantCount: 2, // "plex" and "radarr" are running
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newPopulatedMock(t)
			ctx := context.Background()

			containers, err := m.ListContainers(ctx, tt.all)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(containers) != tt.wantCount {
				t.Errorf("ListContainers(ctx, %v) returned %d containers, want %d",
					tt.all, len(containers), tt.wantCount)
			}
		})
	}
}

func Test_ListContainers_EmptyMock(t *testing.T) {
	m := NewMockDockerManager()
	ctx := context.Background()

	containers, err := m.ListContainers(ctx, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(containers) != 0 {
		t.Errorf("expected 0 containers from empty mock, got %d", len(containers))
	}
}

func Test_ListContainers_ReturnsContainerFields(t *testing.T) {
	m := newPopulatedMock(t)
	ctx := context.Background()

	containers, err := m.ListContainers(ctx, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Find plex container and verify all fields are populated.
	var found bool
	for _, c := range containers {
		if c.ID == "abc123" {
			found = true
			if c.Name != "plex" {
				t.Errorf("Name = %q, want %q", c.Name, "plex")
			}
			if c.Image != "plexinc/pms-docker:latest" {
				t.Errorf("Image = %q, want %q", c.Image, "plexinc/pms-docker:latest")
			}
			if c.State != "running" {
				t.Errorf("State = %q, want %q", c.State, "running")
			}
			if c.Status != "Up 2 hours" {
				t.Errorf("Status = %q, want %q", c.Status, "Up 2 hours")
			}
			if c.Created.IsZero() {
				t.Error("Created is zero, expected a valid time")
			}
			break
		}
	}
	if !found {
		t.Error("container abc123 not found in ListContainers result")
	}
}

// ---------------------------------------------------------------------------
// Container inspect tests
// ---------------------------------------------------------------------------

func Test_InspectContainer_Cases(t *testing.T) {
	tests := []struct {
		name        string
		id          string
		wantErr     bool
		errContains string
		validate    func(t *testing.T, detail *ContainerDetail)
	}{
		{
			name:    "inspect existing container returns detail",
			id:      "abc123",
			wantErr: false,
			validate: func(t *testing.T, detail *ContainerDetail) {
				t.Helper()
				if detail == nil {
					t.Fatal("expected non-nil ContainerDetail")
				}
				if detail.ID != "abc123" {
					t.Errorf("ID = %q, want %q", detail.ID, "abc123")
				}
				if detail.Name != "plex" {
					t.Errorf("Name = %q, want %q", detail.Name, "plex")
				}
				if detail.Config.Env[0] != "TZ=America/New_York" {
					t.Errorf("Env[0] = %q, want %q", detail.Config.Env[0], "TZ=America/New_York")
				}
				if detail.NetworkSettings.IPAddress != "172.17.0.2" {
					t.Errorf("IPAddress = %q, want %q", detail.NetworkSettings.IPAddress, "172.17.0.2")
				}
				if len(detail.Mounts) != 1 {
					t.Fatalf("Mounts count = %d, want 1", len(detail.Mounts))
				}
				if detail.Mounts[0].Source != "/mnt/user/media" {
					t.Errorf("Mounts[0].Source = %q, want %q", detail.Mounts[0].Source, "/mnt/user/media")
				}
				if !detail.Mounts[0].ReadOnly {
					t.Error("Mounts[0].ReadOnly = false, want true")
				}
			},
		},
		{
			name:        "inspect nonexistent container returns error",
			id:          "nonexistent",
			wantErr:     true,
			errContains: "not found",
		},
		{
			name:        "inspect empty id returns error",
			id:          "",
			wantErr:     true,
			errContains: "not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newPopulatedMock(t)
			ctx := context.Background()

			detail, err := m.InspectContainer(ctx, tt.id)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errContains != "" && !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tt.errContains)) {
					t.Errorf("error = %q, want it to contain %q", err.Error(), tt.errContains)
				}
				if detail != nil {
					t.Error("expected nil detail on error")
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

// ---------------------------------------------------------------------------
// Container lifecycle tests
// ---------------------------------------------------------------------------

func Test_StartContainer_Cases(t *testing.T) {
	tests := []struct {
		name        string
		id          string
		wantErr     bool
		errContains string
	}{
		{
			name:    "start existing container succeeds",
			id:      "abc123",
			wantErr: false,
		},
		{
			name:    "start stopped container succeeds",
			id:      "def456",
			wantErr: false,
		},
		{
			name:        "start nonexistent container returns error",
			id:          "nonexistent",
			wantErr:     true,
			errContains: "not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newPopulatedMock(t)
			ctx := context.Background()

			err := m.StartContainer(ctx, tt.id)
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
		})
	}
}

func Test_StartContainer_ChangesState(t *testing.T) {
	m := newPopulatedMock(t)
	ctx := context.Background()

	// Verify sonarr starts as exited.
	detail, err := m.InspectContainer(ctx, "def456")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if detail.State != "exited" {
		t.Fatalf("precondition failed: container state = %q, want %q", detail.State, "exited")
	}

	if err := m.StartContainer(ctx, "def456"); err != nil {
		t.Fatalf("StartContainer failed: %v", err)
	}

	detail, err = m.InspectContainer(ctx, "def456")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if detail.State != "running" {
		t.Errorf("after Start, State = %q, want %q", detail.State, "running")
	}
}

func Test_StopContainer_Cases(t *testing.T) {
	tests := []struct {
		name        string
		id          string
		timeout     int
		wantErr     bool
		errContains string
	}{
		{
			name:    "stop running container succeeds",
			id:      "abc123",
			timeout: 10,
			wantErr: false,
		},
		{
			name:    "stop with zero timeout succeeds",
			id:      "abc123",
			timeout: 0,
			wantErr: false,
		},
		{
			name:        "stop nonexistent container returns error",
			id:          "nonexistent",
			timeout:     10,
			wantErr:     true,
			errContains: "not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newPopulatedMock(t)
			ctx := context.Background()

			err := m.StopContainer(ctx, tt.id, tt.timeout)
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
		})
	}
}

func Test_StopContainer_ChangesState(t *testing.T) {
	m := newPopulatedMock(t)
	ctx := context.Background()

	if err := m.StopContainer(ctx, "abc123", 10); err != nil {
		t.Fatalf("StopContainer failed: %v", err)
	}

	detail, err := m.InspectContainer(ctx, "abc123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if detail.State != "exited" {
		t.Errorf("after Stop, State = %q, want %q", detail.State, "exited")
	}
}

func Test_RestartContainer_Cases(t *testing.T) {
	tests := []struct {
		name        string
		id          string
		timeout     int
		wantErr     bool
		errContains string
	}{
		{
			name:    "restart running container succeeds",
			id:      "abc123",
			timeout: 10,
			wantErr: false,
		},
		{
			name:    "restart stopped container succeeds",
			id:      "def456",
			timeout: 5,
			wantErr: false,
		},
		{
			name:        "restart nonexistent container returns error",
			id:          "nonexistent",
			timeout:     10,
			wantErr:     true,
			errContains: "not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newPopulatedMock(t)
			ctx := context.Background()

			err := m.RestartContainer(ctx, tt.id, tt.timeout)
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
		})
	}
}

func Test_RestartContainer_ChangesState(t *testing.T) {
	m := newPopulatedMock(t)
	ctx := context.Background()

	// Restart a stopped container.
	if err := m.RestartContainer(ctx, "def456", 5); err != nil {
		t.Fatalf("RestartContainer failed: %v", err)
	}

	detail, err := m.InspectContainer(ctx, "def456")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if detail.State != "running" {
		t.Errorf("after Restart, State = %q, want %q", detail.State, "running")
	}
}

// ---------------------------------------------------------------------------
// Container remove tests
// ---------------------------------------------------------------------------

func Test_RemoveContainer_Cases(t *testing.T) {
	tests := []struct {
		name        string
		id          string
		force       bool
		wantErr     bool
		errContains string
	}{
		{
			name:    "remove stopped container without force succeeds",
			id:      "def456",
			force:   false,
			wantErr: false,
		},
		{
			name:    "remove running container with force succeeds",
			id:      "abc123",
			force:   true,
			wantErr: false,
		},
		{
			name:        "remove running container without force returns error",
			id:          "abc123",
			force:       false,
			wantErr:     true,
			errContains: "running",
		},
		{
			name:        "remove nonexistent container returns error",
			id:          "nonexistent",
			force:       false,
			wantErr:     true,
			errContains: "not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newPopulatedMock(t)
			ctx := context.Background()

			err := m.RemoveContainer(ctx, tt.id, tt.force)
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

			// Verify container is actually gone.
			_, inspErr := m.InspectContainer(ctx, tt.id)
			if inspErr == nil {
				t.Error("expected InspectContainer to fail after removal, but it succeeded")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Container create tests
// ---------------------------------------------------------------------------

func Test_CreateContainer_Cases(t *testing.T) {
	tests := []struct {
		name        string
		config      ContainerCreateConfig
		wantErr     bool
		errContains string
	}{
		{
			name: "create with valid config returns ID",
			config: ContainerCreateConfig{
				Name:    "test-container",
				Image:   "nginx:latest",
				Env:     []string{"FOO=bar"},
				Cmd:     []string{"nginx", "-g", "daemon off;"},
				Labels:  map[string]string{"env": "test"},
				Ports:   map[string]string{"80/tcp": "8080"},
				Volumes: map[string]string{"/host/data": "/data"},
			},
			wantErr: false,
		},
		{
			name: "create with minimal config (image only) succeeds",
			config: ContainerCreateConfig{
				Image: "alpine:latest",
			},
			wantErr: false,
		},
		{
			name: "create with empty image returns error",
			config: ContainerCreateConfig{
				Name:  "no-image",
				Image: "",
			},
			wantErr:     true,
			errContains: "image",
		},
		{
			name:        "create with zero-value config returns error",
			config:      ContainerCreateConfig{},
			wantErr:     true,
			errContains: "image",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewMockDockerManager()
			ctx := context.Background()

			id, err := m.CreateContainer(ctx, tt.config)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errContains != "" && !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tt.errContains)) {
					t.Errorf("error = %q, want it to contain %q", err.Error(), tt.errContains)
				}
				if id != "" {
					t.Errorf("expected empty ID on error, got %q", id)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if id == "" {
				t.Error("expected non-empty container ID")
			}

			// Verify the container is inspectable after creation.
			detail, inspErr := m.InspectContainer(ctx, id)
			if inspErr != nil {
				t.Fatalf("InspectContainer after create: %v", inspErr)
			}
			if detail.Image != tt.config.Image {
				t.Errorf("created container Image = %q, want %q", detail.Image, tt.config.Image)
			}
			if detail.Name != tt.config.Name {
				t.Errorf("created container Name = %q, want %q", detail.Name, tt.config.Name)
			}
		})
	}
}

func Test_CreateContainer_UniqueIDs(t *testing.T) {
	m := NewMockDockerManager()
	ctx := context.Background()

	cfg := ContainerCreateConfig{Image: "alpine:latest"}

	id1, err := m.CreateContainer(ctx, cfg)
	if err != nil {
		t.Fatalf("first create: %v", err)
	}
	id2, err := m.CreateContainer(ctx, cfg)
	if err != nil {
		t.Fatalf("second create: %v", err)
	}

	if id1 == id2 {
		t.Errorf("CreateContainer returned duplicate IDs: %q", id1)
	}
}

// ---------------------------------------------------------------------------
// PullImage tests
// ---------------------------------------------------------------------------

func Test_PullImage_Cases(t *testing.T) {
	tests := []struct {
		name        string
		image       string
		wantErr     bool
		errContains string
	}{
		{
			name:    "pull valid image succeeds",
			image:   "nginx:latest",
			wantErr: false,
		},
		{
			name:    "pull image with tag succeeds",
			image:   "ubuntu:22.04",
			wantErr: false,
		},
		{
			name:        "pull empty image returns error",
			image:       "",
			wantErr:     true,
			errContains: "image",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewMockDockerManager()
			ctx := context.Background()

			err := m.PullImage(ctx, tt.image)
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
		})
	}
}

// ---------------------------------------------------------------------------
// GetLogs tests
// ---------------------------------------------------------------------------

func Test_GetLogs_Cases(t *testing.T) {
	tests := []struct {
		name        string
		id          string
		tail        int
		wantErr     bool
		errContains string
		validate    func(t *testing.T, logs string)
	}{
		{
			name:    "get all logs",
			id:      "abc123",
			tail:    0,
			wantErr: false,
			validate: func(t *testing.T, logs string) {
				t.Helper()
				if !strings.Contains(logs, "line1") {
					t.Error("expected logs to contain 'line1'")
				}
				if !strings.Contains(logs, "line5") {
					t.Error("expected logs to contain 'line5'")
				}
			},
		},
		{
			name:    "get tail 2 lines",
			id:      "abc123",
			tail:    2,
			wantErr: false,
			validate: func(t *testing.T, logs string) {
				t.Helper()
				if strings.Contains(logs, "line1") {
					t.Error("tail 2 should not contain 'line1'")
				}
				if !strings.Contains(logs, "line5") {
					t.Error("tail 2 should contain 'line5'")
				}
			},
		},
		{
			name:    "get logs for container with no logs returns empty string",
			id:      "ghi789",
			tail:    100,
			wantErr: false,
			validate: func(t *testing.T, logs string) {
				t.Helper()
				if logs != "" {
					t.Errorf("expected empty logs, got %q", logs)
				}
			},
		},
		{
			name:        "get logs for nonexistent container returns error",
			id:          "nonexistent",
			tail:        100,
			wantErr:     true,
			errContains: "not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newPopulatedMock(t)
			ctx := context.Background()

			logs, err := m.GetLogs(ctx, tt.id, tt.tail)
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
			if tt.validate != nil {
				tt.validate(t, logs)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// GetStats tests
// ---------------------------------------------------------------------------

func Test_GetStats_Cases(t *testing.T) {
	tests := []struct {
		name        string
		id          string
		wantErr     bool
		errContains string
		validate    func(t *testing.T, stats *ContainerStats)
	}{
		{
			name:    "get stats for container with stats returns populated stats",
			id:      "abc123",
			wantErr: false,
			validate: func(t *testing.T, stats *ContainerStats) {
				t.Helper()
				if stats == nil {
					t.Fatal("expected non-nil ContainerStats")
				}
				if stats.CPUPercent != 25.5 {
					t.Errorf("CPUPercent = %f, want 25.5", stats.CPUPercent)
				}
				if stats.MemoryUsage != 1024*1024*512 {
					t.Errorf("MemoryUsage = %d, want %d", stats.MemoryUsage, 1024*1024*512)
				}
				if stats.MemoryLimit != 1024*1024*1024 {
					t.Errorf("MemoryLimit = %d, want %d", stats.MemoryLimit, 1024*1024*1024)
				}
				if stats.NetworkRxBytes != 1000000 {
					t.Errorf("NetworkRxBytes = %d, want 1000000", stats.NetworkRxBytes)
				}
				if stats.NetworkTxBytes != 500000 {
					t.Errorf("NetworkTxBytes = %d, want 500000", stats.NetworkTxBytes)
				}
			},
		},
		{
			name:    "get stats for container without stats returns zero stats",
			id:      "def456",
			wantErr: false,
			validate: func(t *testing.T, stats *ContainerStats) {
				t.Helper()
				if stats == nil {
					t.Fatal("expected non-nil ContainerStats")
				}
				if stats.CPUPercent != 0 {
					t.Errorf("CPUPercent = %f, want 0", stats.CPUPercent)
				}
			},
		},
		{
			name:        "get stats for nonexistent container returns error",
			id:          "nonexistent",
			wantErr:     true,
			errContains: "not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newPopulatedMock(t)
			ctx := context.Background()

			stats, err := m.GetStats(ctx, tt.id)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errContains != "" && !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tt.errContains)) {
					t.Errorf("error = %q, want it to contain %q", err.Error(), tt.errContains)
				}
				if stats != nil {
					t.Error("expected nil stats on error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.validate != nil {
				tt.validate(t, stats)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Network listing tests
// ---------------------------------------------------------------------------

func Test_ListNetworks_Cases(t *testing.T) {
	t.Run("list networks returns all networks", func(t *testing.T) {
		m := newPopulatedMock(t)
		ctx := context.Background()

		networks, err := m.ListNetworks(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(networks) != 2 {
			t.Errorf("ListNetworks returned %d networks, want 2", len(networks))
		}
	})

	t.Run("list networks from empty mock returns empty slice", func(t *testing.T) {
		m := NewMockDockerManager()
		ctx := context.Background()

		networks, err := m.ListNetworks(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(networks) != 0 {
			t.Errorf("expected 0 networks from empty mock, got %d", len(networks))
		}
	})
}

func Test_ListNetworks_ReturnsNetworkFields(t *testing.T) {
	m := newPopulatedMock(t)
	ctx := context.Background()

	networks, err := m.ListNetworks(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var found bool
	for _, n := range networks {
		if n.ID == "net1" {
			found = true
			if n.Name != "bridge" {
				t.Errorf("Name = %q, want %q", n.Name, "bridge")
			}
			if n.Driver != "bridge" {
				t.Errorf("Driver = %q, want %q", n.Driver, "bridge")
			}
			if n.Scope != "local" {
				t.Errorf("Scope = %q, want %q", n.Scope, "local")
			}
			break
		}
	}
	if !found {
		t.Error("network net1 not found in ListNetworks result")
	}
}

// ---------------------------------------------------------------------------
// Network inspect tests
// ---------------------------------------------------------------------------

func Test_InspectNetwork_Cases(t *testing.T) {
	tests := []struct {
		name        string
		id          string
		wantErr     bool
		errContains string
		validate    func(t *testing.T, detail *NetworkDetail)
	}{
		{
			name:    "inspect existing network returns detail",
			id:      "net1",
			wantErr: false,
			validate: func(t *testing.T, detail *NetworkDetail) {
				t.Helper()
				if detail == nil {
					t.Fatal("expected non-nil NetworkDetail")
				}
				if detail.ID != "net1" {
					t.Errorf("ID = %q, want %q", detail.ID, "net1")
				}
				if detail.Name != "bridge" {
					t.Errorf("Name = %q, want %q", detail.Name, "bridge")
				}
				if detail.Subnet != "172.17.0.0/16" {
					t.Errorf("Subnet = %q, want %q", detail.Subnet, "172.17.0.0/16")
				}
				if detail.Gateway != "172.17.0.1" {
					t.Errorf("Gateway = %q, want %q", detail.Gateway, "172.17.0.1")
				}
			},
		},
		{
			name:        "inspect nonexistent network returns error",
			id:          "nonexistent",
			wantErr:     true,
			errContains: "not found",
		},
		{
			name:        "inspect empty id returns error",
			id:          "",
			wantErr:     true,
			errContains: "not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newPopulatedMock(t)
			ctx := context.Background()

			detail, err := m.InspectNetwork(ctx, tt.id)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errContains != "" && !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tt.errContains)) {
					t.Errorf("error = %q, want it to contain %q", err.Error(), tt.errContains)
				}
				if detail != nil {
					t.Error("expected nil detail on error")
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

// ---------------------------------------------------------------------------
// Network create tests
// ---------------------------------------------------------------------------

func Test_CreateNetwork_Cases(t *testing.T) {
	tests := []struct {
		name        string
		config      NetworkCreateConfig
		wantErr     bool
		errContains string
	}{
		{
			name: "create network with valid config returns ID",
			config: NetworkCreateConfig{
				Name:   "my-network",
				Driver: "bridge",
				Subnet: "192.168.1.0/24",
			},
			wantErr: false,
		},
		{
			name: "create network with name only succeeds",
			config: NetworkCreateConfig{
				Name: "simple-net",
			},
			wantErr: false,
		},
		{
			name: "create network with empty name returns error",
			config: NetworkCreateConfig{
				Name:   "",
				Driver: "bridge",
			},
			wantErr:     true,
			errContains: "name",
		},
		{
			name:        "create network with zero-value config returns error",
			config:      NetworkCreateConfig{},
			wantErr:     true,
			errContains: "name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewMockDockerManager()
			ctx := context.Background()

			id, err := m.CreateNetwork(ctx, tt.config)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errContains != "" && !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tt.errContains)) {
					t.Errorf("error = %q, want it to contain %q", err.Error(), tt.errContains)
				}
				if id != "" {
					t.Errorf("expected empty ID on error, got %q", id)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if id == "" {
				t.Error("expected non-empty network ID")
			}

			// Verify the network is inspectable.
			detail, inspErr := m.InspectNetwork(ctx, id)
			if inspErr != nil {
				t.Fatalf("InspectNetwork after create: %v", inspErr)
			}
			if detail.Name != tt.config.Name {
				t.Errorf("created network Name = %q, want %q", detail.Name, tt.config.Name)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Network remove tests
// ---------------------------------------------------------------------------

func Test_RemoveNetwork_Cases(t *testing.T) {
	tests := []struct {
		name        string
		id          string
		wantErr     bool
		errContains string
	}{
		{
			name:    "remove existing network succeeds",
			id:      "net2",
			wantErr: false,
		},
		{
			name:        "remove nonexistent network returns error",
			id:          "nonexistent",
			wantErr:     true,
			errContains: "not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newPopulatedMock(t)
			ctx := context.Background()

			err := m.RemoveNetwork(ctx, tt.id)
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

			// Verify network is gone.
			_, inspErr := m.InspectNetwork(ctx, tt.id)
			if inspErr == nil {
				t.Error("expected InspectNetwork to fail after removal, but it succeeded")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Network connect/disconnect tests
// ---------------------------------------------------------------------------

func Test_ConnectNetwork_Cases(t *testing.T) {
	tests := []struct {
		name        string
		networkID   string
		containerID string
		wantErr     bool
		errContains string
	}{
		{
			name:        "connect existing container to existing network succeeds",
			networkID:   "net1",
			containerID: "abc123",
			wantErr:     false,
		},
		{
			name:        "connect to nonexistent network returns error",
			networkID:   "nonexistent",
			containerID: "abc123",
			wantErr:     true,
			errContains: "not found",
		},
		{
			name:        "connect nonexistent container returns error",
			networkID:   "net1",
			containerID: "nonexistent",
			wantErr:     true,
			errContains: "not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newPopulatedMock(t)
			ctx := context.Background()

			err := m.ConnectNetwork(ctx, tt.networkID, tt.containerID)
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
		})
	}
}

func Test_DisconnectNetwork_Cases(t *testing.T) {
	tests := []struct {
		name        string
		networkID   string
		containerID string
		wantErr     bool
		errContains string
	}{
		{
			name:        "disconnect existing container from existing network succeeds",
			networkID:   "net1",
			containerID: "abc123",
			wantErr:     false,
		},
		{
			name:        "disconnect from nonexistent network returns error",
			networkID:   "nonexistent",
			containerID: "abc123",
			wantErr:     true,
			errContains: "not found",
		},
		{
			name:        "disconnect nonexistent container returns error",
			networkID:   "net1",
			containerID: "nonexistent",
			wantErr:     true,
			errContains: "not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newPopulatedMock(t)
			ctx := context.Background()

			err := m.DisconnectNetwork(ctx, tt.networkID, tt.containerID)
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
		})
	}
}

// ---------------------------------------------------------------------------
// Context cancellation tests
// ---------------------------------------------------------------------------

func Test_ContextCancellation_AllMethods(t *testing.T) {
	m := newPopulatedMock(t)

	// Create an already-cancelled context.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	tests := []struct {
		name string
		fn   func() error
	}{
		{
			name: "ListContainers",
			fn:   func() error { _, err := m.ListContainers(ctx, true); return err },
		},
		{
			name: "InspectContainer",
			fn:   func() error { _, err := m.InspectContainer(ctx, "abc123"); return err },
		},
		{
			name: "StartContainer",
			fn:   func() error { return m.StartContainer(ctx, "abc123") },
		},
		{
			name: "StopContainer",
			fn:   func() error { return m.StopContainer(ctx, "abc123", 10) },
		},
		{
			name: "RestartContainer",
			fn:   func() error { return m.RestartContainer(ctx, "abc123", 10) },
		},
		{
			name: "RemoveContainer",
			fn:   func() error { return m.RemoveContainer(ctx, "abc123", true) },
		},
		{
			name: "CreateContainer",
			fn: func() error {
				_, err := m.CreateContainer(ctx, ContainerCreateConfig{Image: "test"})
				return err
			},
		},
		{
			name: "PullImage",
			fn:   func() error { return m.PullImage(ctx, "test") },
		},
		{
			name: "GetLogs",
			fn:   func() error { _, err := m.GetLogs(ctx, "abc123", 10); return err },
		},
		{
			name: "GetStats",
			fn:   func() error { _, err := m.GetStats(ctx, "abc123"); return err },
		},
		{
			name: "ListNetworks",
			fn:   func() error { _, err := m.ListNetworks(ctx); return err },
		},
		{
			name: "InspectNetwork",
			fn:   func() error { _, err := m.InspectNetwork(ctx, "net1"); return err },
		},
		{
			name: "CreateNetwork",
			fn: func() error {
				_, err := m.CreateNetwork(ctx, NetworkCreateConfig{Name: "test"})
				return err
			},
		},
		{
			name: "RemoveNetwork",
			fn:   func() error { return m.RemoveNetwork(ctx, "net1") },
		},
		{
			name: "ConnectNetwork",
			fn:   func() error { return m.ConnectNetwork(ctx, "net1", "abc123") },
		},
		{
			name: "DisconnectNetwork",
			fn:   func() error { return m.DisconnectNetwork(ctx, "net1", "abc123") },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.fn()
			if err == nil {
				t.Fatal("expected error with cancelled context, got nil")
			}
			if err != context.Canceled {
				t.Errorf("expected context.Canceled, got %v", err)
			}
		})
	}
}

func Test_ContextDeadlineExceeded(t *testing.T) {
	m := newPopulatedMock(t)

	ctx, cancel := context.WithTimeout(context.Background(), 0)
	defer cancel()

	// Allow the timeout to expire.
	time.Sleep(time.Millisecond)

	_, err := m.ListContainers(ctx, true)
	if err == nil {
		t.Fatal("expected error with expired deadline, got nil")
	}
	if err != context.DeadlineExceeded {
		t.Errorf("expected context.DeadlineExceeded, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Type zero-value tests
// ---------------------------------------------------------------------------

func Test_Container_ZeroValue(t *testing.T) {
	var c Container
	if c.ID != "" {
		t.Errorf("zero Container.ID = %q, want empty", c.ID)
	}
	if c.Name != "" {
		t.Errorf("zero Container.Name = %q, want empty", c.Name)
	}
	if c.State != "" {
		t.Errorf("zero Container.State = %q, want empty", c.State)
	}
	if !c.Created.IsZero() {
		t.Errorf("zero Container.Created = %v, want zero", c.Created)
	}
}

func Test_ContainerStats_ZeroValue(t *testing.T) {
	var s ContainerStats
	if s.CPUPercent != 0 {
		t.Errorf("zero CPUPercent = %f, want 0", s.CPUPercent)
	}
	if s.MemoryUsage != 0 {
		t.Errorf("zero MemoryUsage = %d, want 0", s.MemoryUsage)
	}
	if s.MemoryLimit != 0 {
		t.Errorf("zero MemoryLimit = %d, want 0", s.MemoryLimit)
	}
}

func Test_ContainerCreateConfig_NilFields(t *testing.T) {
	cfg := ContainerCreateConfig{
		Image: "test",
	}
	// Nil slices and maps should be safe to use.
	if cfg.Env != nil {
		t.Error("expected nil Env")
	}
	if cfg.Cmd != nil {
		t.Error("expected nil Cmd")
	}
	if cfg.Labels != nil {
		t.Error("expected nil Labels")
	}
	if cfg.Ports != nil {
		t.Error("expected nil Ports")
	}
	if cfg.Volumes != nil {
		t.Error("expected nil Volumes")
	}
}

func Test_NetworkCreateConfig_ZeroValue(t *testing.T) {
	var cfg NetworkCreateConfig
	if cfg.Name != "" {
		t.Errorf("zero Name = %q, want empty", cfg.Name)
	}
	if cfg.Driver != "" {
		t.Errorf("zero Driver = %q, want empty", cfg.Driver)
	}
	if cfg.Subnet != "" {
		t.Errorf("zero Subnet = %q, want empty", cfg.Subnet)
	}
}

// ---------------------------------------------------------------------------
// Concurrent access tests
// ---------------------------------------------------------------------------

func Test_ConcurrentListAndCreate(t *testing.T) {
	m := NewMockDockerManager()
	ctx := context.Background()

	const goroutines = 20
	var wg sync.WaitGroup
	wg.Add(goroutines * 2)

	// Half goroutines create containers, half list them.
	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			cfg := ContainerCreateConfig{
				Name:  fmt.Sprintf("container-%d", idx),
				Image: "alpine:latest",
			}
			_, err := m.CreateContainer(ctx, cfg)
			if err != nil {
				t.Errorf("goroutine %d CreateContainer: %v", idx, err)
			}
		}(i)

		go func(idx int) {
			defer wg.Done()
			_, err := m.ListContainers(ctx, true)
			if err != nil {
				t.Errorf("goroutine %d ListContainers: %v", idx, err)
			}
		}(i)
	}

	wg.Wait()

	// All containers should have been created.
	containers, err := m.ListContainers(ctx, true)
	if err != nil {
		t.Fatalf("final ListContainers: %v", err)
	}
	if len(containers) != goroutines {
		t.Errorf("expected %d containers after concurrent create, got %d", goroutines, len(containers))
	}
}

func Test_ConcurrentNetworkOperations(t *testing.T) {
	m := newPopulatedMock(t)
	ctx := context.Background()

	const goroutines = 10
	var wg sync.WaitGroup
	wg.Add(goroutines * 2)

	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			_, err := m.ListNetworks(ctx)
			if err != nil {
				t.Errorf("goroutine %d ListNetworks: %v", idx, err)
			}
		}(i)

		go func(idx int) {
			defer wg.Done()
			cfg := NetworkCreateConfig{
				Name:   fmt.Sprintf("net-%d", idx),
				Driver: "bridge",
			}
			_, err := m.CreateNetwork(ctx, cfg)
			if err != nil {
				t.Errorf("goroutine %d CreateNetwork: %v", idx, err)
			}
		}(i)
	}

	wg.Wait()
}

// ---------------------------------------------------------------------------
// Full lifecycle integration test
// ---------------------------------------------------------------------------

func Test_ContainerFullLifecycle(t *testing.T) {
	m := NewMockDockerManager()
	ctx := context.Background()

	// 1. Create a container.
	cfg := ContainerCreateConfig{
		Name:    "lifecycle-test",
		Image:   "nginx:latest",
		Env:     []string{"ENV=test"},
		Cmd:     []string{"nginx"},
		Labels:  map[string]string{"test": "true"},
		Ports:   map[string]string{"80/tcp": "8080"},
		Volumes: map[string]string{"/host/html": "/usr/share/nginx/html"},
	}
	id, err := m.CreateContainer(ctx, cfg)
	if err != nil {
		t.Fatalf("CreateContainer: %v", err)
	}
	if id == "" {
		t.Fatal("expected non-empty ID from CreateContainer")
	}

	// 2. Verify it appears in list.
	containers, err := m.ListContainers(ctx, true)
	if err != nil {
		t.Fatalf("ListContainers: %v", err)
	}
	if len(containers) != 1 {
		t.Fatalf("expected 1 container, got %d", len(containers))
	}
	if containers[0].ID != id {
		t.Errorf("listed container ID = %q, want %q", containers[0].ID, id)
	}

	// 3. Inspect it.
	detail, err := m.InspectContainer(ctx, id)
	if err != nil {
		t.Fatalf("InspectContainer: %v", err)
	}
	if detail.Name != "lifecycle-test" {
		t.Errorf("Name = %q, want %q", detail.Name, "lifecycle-test")
	}
	if detail.Image != "nginx:latest" {
		t.Errorf("Image = %q, want %q", detail.Image, "nginx:latest")
	}

	// 4. Start it.
	if err := m.StartContainer(ctx, id); err != nil {
		t.Fatalf("StartContainer: %v", err)
	}
	detail, _ = m.InspectContainer(ctx, id)
	if detail.State != "running" {
		t.Errorf("after start, State = %q, want %q", detail.State, "running")
	}

	// 5. Stop it.
	if err := m.StopContainer(ctx, id, 10); err != nil {
		t.Fatalf("StopContainer: %v", err)
	}
	detail, _ = m.InspectContainer(ctx, id)
	if detail.State != "exited" {
		t.Errorf("after stop, State = %q, want %q", detail.State, "exited")
	}

	// 6. Restart it.
	if err := m.RestartContainer(ctx, id, 5); err != nil {
		t.Fatalf("RestartContainer: %v", err)
	}
	detail, _ = m.InspectContainer(ctx, id)
	if detail.State != "running" {
		t.Errorf("after restart, State = %q, want %q", detail.State, "running")
	}

	// 7. Remove it (force since it is running).
	if err := m.RemoveContainer(ctx, id, true); err != nil {
		t.Fatalf("RemoveContainer: %v", err)
	}

	// 8. Verify it is gone.
	containers, err = m.ListContainers(ctx, true)
	if err != nil {
		t.Fatalf("ListContainers after remove: %v", err)
	}
	if len(containers) != 0 {
		t.Errorf("expected 0 containers after removal, got %d", len(containers))
	}
}

func Test_NetworkFullLifecycle(t *testing.T) {
	m := NewMockDockerManager()
	ctx := context.Background()

	// Create a container first for connect/disconnect tests.
	containerID, err := m.CreateContainer(ctx, ContainerCreateConfig{
		Name:  "net-test-container",
		Image: "alpine:latest",
	})
	if err != nil {
		t.Fatalf("CreateContainer: %v", err)
	}

	// 1. Create a network.
	netCfg := NetworkCreateConfig{
		Name:   "test-network",
		Driver: "bridge",
		Subnet: "10.10.0.0/24",
	}
	netID, err := m.CreateNetwork(ctx, netCfg)
	if err != nil {
		t.Fatalf("CreateNetwork: %v", err)
	}
	if netID == "" {
		t.Fatal("expected non-empty network ID")
	}

	// 2. Verify it appears in list.
	networks, err := m.ListNetworks(ctx)
	if err != nil {
		t.Fatalf("ListNetworks: %v", err)
	}
	if len(networks) != 1 {
		t.Fatalf("expected 1 network, got %d", len(networks))
	}

	// 3. Inspect it.
	detail, err := m.InspectNetwork(ctx, netID)
	if err != nil {
		t.Fatalf("InspectNetwork: %v", err)
	}
	if detail.Name != "test-network" {
		t.Errorf("Name = %q, want %q", detail.Name, "test-network")
	}
	if detail.Subnet != "10.10.0.0/24" {
		t.Errorf("Subnet = %q, want %q", detail.Subnet, "10.10.0.0/24")
	}

	// 4. Connect a container.
	if err := m.ConnectNetwork(ctx, netID, containerID); err != nil {
		t.Fatalf("ConnectNetwork: %v", err)
	}

	// 5. Disconnect the container.
	if err := m.DisconnectNetwork(ctx, netID, containerID); err != nil {
		t.Fatalf("DisconnectNetwork: %v", err)
	}

	// 6. Remove the network.
	if err := m.RemoveNetwork(ctx, netID); err != nil {
		t.Fatalf("RemoveNetwork: %v", err)
	}

	// 7. Verify it is gone.
	networks, err = m.ListNetworks(ctx)
	if err != nil {
		t.Fatalf("ListNetworks after remove: %v", err)
	}
	if len(networks) != 0 {
		t.Errorf("expected 0 networks after removal, got %d", len(networks))
	}
}

// ---------------------------------------------------------------------------
// Benchmarks
// ---------------------------------------------------------------------------

func Benchmark_ListContainers_All(b *testing.B) {
	m := NewMockDockerManager()
	ctx := context.Background()

	// Seed with 100 containers.
	for i := 0; i < 100; i++ {
		m.AddContainer(&ContainerDetail{
			Container: Container{
				ID:    fmt.Sprintf("bench-%d", i),
				Name:  fmt.Sprintf("container-%d", i),
				Image: "alpine:latest",
				State: "running",
			},
		})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = m.ListContainers(ctx, true)
	}
}

func Benchmark_InspectContainer(b *testing.B) {
	m := NewMockDockerManager()
	ctx := context.Background()
	m.AddContainer(&ContainerDetail{
		Container: Container{
			ID:    "bench-1",
			Name:  "bench-container",
			Image: "alpine:latest",
			State: "running",
		},
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = m.InspectContainer(ctx, "bench-1")
	}
}

func Benchmark_CreateContainer(b *testing.B) {
	m := NewMockDockerManager()
	ctx := context.Background()
	cfg := ContainerCreateConfig{
		Name:  "bench",
		Image: "alpine:latest",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = m.CreateContainer(ctx, cfg)
	}
}
