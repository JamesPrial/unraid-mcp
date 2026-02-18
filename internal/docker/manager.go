// Package docker provides Docker container and network management for the unraid-mcp server.
package docker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// dockerAPIVersion is the minimum Docker API version this client targets.
const dockerAPIVersion = "v1.41"

// DockerClientManager is a real implementation of DockerManager that communicates
// with the Docker daemon over a Unix socket using the Docker HTTP API.
type DockerClientManager struct {
	client     *http.Client
	socketPath string
	baseURL    string
}

// NewDockerClientManager creates a new DockerClientManager that connects to the
// Docker daemon at the given Unix socket path. It configures the HTTP client with
// a custom dialer that routes all requests through the Unix socket.
func NewDockerClientManager(socketPath string) (*DockerClientManager, error) {
	if socketPath == "" {
		return nil, fmt.Errorf("docker: socket path is required")
	}

	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", socketPath)
		},
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
	}

	return &DockerClientManager{
		client:     client,
		socketPath: socketPath,
		// The host in the URL is ignored when using a Unix socket transport;
		// Docker requires a non-empty hostname so we use "localhost".
		baseURL: fmt.Sprintf("http://localhost/%s", dockerAPIVersion),
	}, nil
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

// doRequest performs an HTTP request against the Docker daemon and returns the
// response body. The caller is responsible for closing the body.
func (m *DockerClientManager) doRequest(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	url := m.baseURL + path
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, fmt.Errorf("docker: build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := m.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("docker: request failed: %w", err)
	}
	return resp, nil
}

// readBody reads the full response body and closes it.
func readBody(resp *http.Response) ([]byte, error) {
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("docker: read response body: %w", err)
	}
	return data, nil
}

// checkAPIError checks the status code and body without requiring a response
// with an open body (useful when we have already read the body).
func checkAPIError(statusCode int, body []byte, notFoundMsg string) error {
	if statusCode >= 200 && statusCode < 300 {
		return nil
	}
	if statusCode == http.StatusNotFound {
		return fmt.Errorf("%s", notFoundMsg)
	}
	var apiErr struct {
		Message string `json:"message"`
	}
	if len(body) > 0 {
		if jsonErr := json.Unmarshal(body, &apiErr); jsonErr == nil && apiErr.Message != "" {
			return fmt.Errorf("docker: %s", apiErr.Message)
		}
	}
	return fmt.Errorf("docker: unexpected status %d: %s", statusCode, string(body))
}

// ---------------------------------------------------------------------------
// Container operations
// ---------------------------------------------------------------------------

// dockerContainer is the JSON shape returned by /containers/json.
type dockerContainer struct {
	ID      string   `json:"Id"`
	Names   []string `json:"Names"`
	Image   string   `json:"Image"`
	State   string   `json:"State"`
	Status  string   `json:"Status"`
	Created int64    `json:"Created"`
}

// ListContainers returns a list of containers. If all is false, only running
// containers are returned.
func (m *DockerClientManager) ListContainers(ctx context.Context, all bool) ([]Container, error) {
	allParam := "false"
	if all {
		allParam = "true"
	}
	resp, err := m.doRequest(ctx, http.MethodGet, "/containers/json?all="+allParam, nil)
	if err != nil {
		return nil, fmt.Errorf("docker: list containers: %w", err)
	}
	body, err := readBody(resp)
	if err != nil {
		return nil, fmt.Errorf("docker: list containers: %w", err)
	}
	if err := checkAPIError(resp.StatusCode, body, "container not found"); err != nil {
		return nil, fmt.Errorf("docker: list containers: %w", err)
	}

	var raw []dockerContainer
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("docker: decode container list: %w", err)
	}

	containers := make([]Container, 0, len(raw))
	for _, c := range raw {
		name := ""
		if len(c.Names) > 0 {
			name = strings.TrimPrefix(c.Names[0], "/")
		}
		containers = append(containers, Container{
			ID:      c.ID,
			Name:    name,
			Image:   c.Image,
			State:   c.State,
			Status:  c.Status,
			Created: time.Unix(c.Created, 0),
		})
	}
	return containers, nil
}

// dockerContainerInspect is the JSON shape returned by /containers/{id}/json.
type dockerContainerInspect struct {
	ID      string `json:"Id"`
	Name    string `json:"Name"`
	Created string `json:"Created"`
	State   struct {
		Status string `json:"Status"`
	} `json:"State"`
	Config struct {
		Image  string            `json:"Image"`
		Env    []string          `json:"Env"`
		Cmd    []string          `json:"Cmd"`
		Labels map[string]string `json:"Labels"`
	} `json:"Config"`
	NetworkSettings struct {
		IPAddress string `json:"IPAddress"`
		Ports     map[string][]struct {
			HostIP   string `json:"HostIp"`
			HostPort string `json:"HostPort"`
		} `json:"Ports"`
	} `json:"NetworkSettings"`
	Mounts []struct {
		Source      string `json:"Source"`
		Destination string `json:"Destination"`
		Mode        string `json:"Mode"`
		RW          bool   `json:"RW"`
	} `json:"Mounts"`
	// StatusText is the human-readable status (e.g., "Up 2 hours").
	// It lives under State.Status in the API response.
}

// InspectContainer returns detailed information about a container.
func (m *DockerClientManager) InspectContainer(ctx context.Context, id string) (*ContainerDetail, error) {
	if id == "" {
		return nil, fmt.Errorf("container not found: %s", id)
	}
	resp, err := m.doRequest(ctx, http.MethodGet, "/containers/"+id+"/json", nil)
	if err != nil {
		return nil, fmt.Errorf("docker: inspect container: %w", err)
	}
	body, err := readBody(resp)
	if err != nil {
		return nil, fmt.Errorf("docker: inspect container: %w", err)
	}
	if err := checkAPIError(resp.StatusCode, body, fmt.Sprintf("container not found: %s", id)); err != nil {
		return nil, fmt.Errorf("docker: inspect container: %w", err)
	}

	var raw dockerContainerInspect
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("docker: decode container inspect: %w", err)
	}

	created, _ := time.Parse(time.RFC3339Nano, raw.Created)

	ports := make(map[string]string)
	for portProto, bindings := range raw.NetworkSettings.Ports {
		if len(bindings) > 0 {
			ports[portProto] = bindings[0].HostIP + ":" + bindings[0].HostPort
		}
	}

	mounts := make([]Mount, 0, len(raw.Mounts))
	for _, mnt := range raw.Mounts {
		mounts = append(mounts, Mount{
			Source:      mnt.Source,
			Destination: mnt.Destination,
			ReadOnly:    !mnt.RW,
		})
	}

	name := strings.TrimPrefix(raw.Name, "/")

	return &ContainerDetail{
		Container: Container{
			ID:      raw.ID,
			Name:    name,
			Image:   raw.Config.Image,
			State:   raw.State.Status,
			Status:  raw.State.Status, // Docker API does not expose the human status in inspect
			Created: created,
		},
		Config: ContainerConfig{
			Env:    raw.Config.Env,
			Cmd:    raw.Config.Cmd,
			Labels: raw.Config.Labels,
		},
		NetworkSettings: NetworkInfo{
			IPAddress: raw.NetworkSettings.IPAddress,
			Ports:     ports,
		},
		Mounts: mounts,
	}, nil
}

// StartContainer starts a stopped container.
func (m *DockerClientManager) StartContainer(ctx context.Context, id string) error {
	resp, err := m.doRequest(ctx, http.MethodPost, "/containers/"+id+"/start", nil)
	if err != nil {
		return fmt.Errorf("docker: start container: %w", err)
	}
	// 204 No Content on success; 304 Not Modified if already running.
	// 404 Not Found if container does not exist.
	body, readErr := readBody(resp)
	if readErr != nil {
		return readErr
	}
	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("container not found: %s", id)
	}
	if resp.StatusCode == http.StatusNotModified || (resp.StatusCode >= 200 && resp.StatusCode < 300) {
		return nil
	}
	if err := checkAPIError(resp.StatusCode, body, fmt.Sprintf("container not found: %s", id)); err != nil {
		return fmt.Errorf("docker: start container: %w", err)
	}
	return nil
}

// StopContainer stops a running container. The timeout specifies how many seconds
// to wait before forcibly killing the container.
func (m *DockerClientManager) StopContainer(ctx context.Context, id string, timeout int) error {
	path := fmt.Sprintf("/containers/%s/stop?t=%d", id, timeout)
	resp, err := m.doRequest(ctx, http.MethodPost, path, nil)
	if err != nil {
		return fmt.Errorf("docker: stop container: %w", err)
	}
	body, readErr := readBody(resp)
	if readErr != nil {
		return readErr
	}
	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("container not found: %s", id)
	}
	// 204 No Content on success; 304 Not Modified if already stopped.
	if resp.StatusCode == http.StatusNotModified || (resp.StatusCode >= 200 && resp.StatusCode < 300) {
		return nil
	}
	if err := checkAPIError(resp.StatusCode, body, fmt.Sprintf("container not found: %s", id)); err != nil {
		return fmt.Errorf("docker: stop container: %w", err)
	}
	return nil
}

// RestartContainer restarts a container.
func (m *DockerClientManager) RestartContainer(ctx context.Context, id string, timeout int) error {
	path := fmt.Sprintf("/containers/%s/restart?t=%d", id, timeout)
	resp, err := m.doRequest(ctx, http.MethodPost, path, nil)
	if err != nil {
		return fmt.Errorf("docker: restart container: %w", err)
	}
	body, readErr := readBody(resp)
	if readErr != nil {
		return readErr
	}
	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("container not found: %s", id)
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	if err := checkAPIError(resp.StatusCode, body, fmt.Sprintf("container not found: %s", id)); err != nil {
		return fmt.Errorf("docker: restart container: %w", err)
	}
	return nil
}

// RemoveContainer removes a container. If force is true, the container is removed
// even if it is running.
func (m *DockerClientManager) RemoveContainer(ctx context.Context, id string, force bool) error {
	forceParam := "false"
	if force {
		forceParam = "true"
	}
	path := fmt.Sprintf("/containers/%s?force=%s", id, forceParam)
	resp, err := m.doRequest(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return fmt.Errorf("docker: remove container: %w", err)
	}
	body, readErr := readBody(resp)
	if readErr != nil {
		return readErr
	}
	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("container not found: %s", id)
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	if err := checkAPIError(resp.StatusCode, body, fmt.Sprintf("container not found: %s", id)); err != nil {
		return fmt.Errorf("docker: remove container: %w", err)
	}
	return nil
}

// containerCreateRequest is the request body for POST /containers/create.
type containerCreateRequest struct {
	Image        string              `json:"Image"`
	Env          []string            `json:"Env,omitempty"`
	Cmd          []string            `json:"Cmd,omitempty"`
	Labels       map[string]string   `json:"Labels,omitempty"`
	ExposedPorts map[string]struct{} `json:"ExposedPorts,omitempty"`
	HostConfig   containerHostConfig `json:"HostConfig"`
}

type containerHostConfig struct {
	PortBindings map[string][]portBinding `json:"PortBindings,omitempty"`
	Binds        []string                 `json:"Binds,omitempty"`
}

type portBinding struct {
	HostPort string `json:"HostPort"`
}

// CreateContainer creates a new container from the given configuration and returns
// the new container's ID.
func (m *DockerClientManager) CreateContainer(ctx context.Context, config ContainerCreateConfig) (string, error) {
	if config.Image == "" {
		return "", fmt.Errorf("image is required")
	}

	exposedPorts := make(map[string]struct{})
	portBindings := make(map[string][]portBinding)
	for containerPort, hostPort := range config.Ports {
		exposedPorts[containerPort] = struct{}{}
		portBindings[containerPort] = []portBinding{{HostPort: hostPort}}
	}

	var binds []string
	for hostPath, containerPath := range config.Volumes {
		binds = append(binds, hostPath+":"+containerPath)
	}

	reqBody := containerCreateRequest{
		Image:        config.Image,
		Env:          config.Env,
		Cmd:          config.Cmd,
		Labels:       config.Labels,
		ExposedPorts: exposedPorts,
		HostConfig: containerHostConfig{
			PortBindings: portBindings,
			Binds:        binds,
		},
	}

	bodyData, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("docker: encode create request: %w", err)
	}

	path := "/containers/create"
	if config.Name != "" {
		path += "?name=" + config.Name
	}

	resp, err := m.doRequest(ctx, http.MethodPost, path, bytes.NewReader(bodyData))
	if err != nil {
		return "", fmt.Errorf("docker: create container: %w", err)
	}
	body, err := readBody(resp)
	if err != nil {
		return "", err
	}

	if err := checkAPIError(resp.StatusCode, body, "container not found"); err != nil {
		return "", fmt.Errorf("docker: create container: %w", err)
	}

	var result struct {
		ID string `json:"Id"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("docker: decode create response: %w", err)
	}
	if result.ID == "" {
		return "", fmt.Errorf("docker: create container returned empty ID")
	}
	return result.ID, nil
}

// PullImage pulls the specified image from a registry.
func (m *DockerClientManager) PullImage(ctx context.Context, image string) error {
	if image == "" {
		return fmt.Errorf("image name is required")
	}
	path := "/images/create?fromImage=" + image
	resp, err := m.doRequest(ctx, http.MethodPost, path, nil)
	if err != nil {
		return fmt.Errorf("docker: pull image: %w", err)
	}
	// Drain and discard the streaming JSON progress output.
	io.Copy(io.Discard, resp.Body) //nolint:errcheck
	_ = resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("image not found: %s", image)
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("docker: pull image %q failed with status %d", image, resp.StatusCode)
	}
	return nil
}

// GetLogs retrieves the container's stdout and stderr. If tail > 0, only the
// last tail lines are returned.
func (m *DockerClientManager) GetLogs(ctx context.Context, id string, tail int) (string, error) {
	tailParam := "all"
	if tail > 0 {
		tailParam = strconv.Itoa(tail)
	}
	path := fmt.Sprintf("/containers/%s/logs?stdout=true&stderr=true&tail=%s", id, tailParam)
	resp, err := m.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return "", fmt.Errorf("docker: get logs: %w", err)
	}

	if resp.StatusCode == http.StatusNotFound {
		_ = resp.Body.Close()
		return "", fmt.Errorf("container not found: %s", id)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := readBody(resp)
		if err := checkAPIError(resp.StatusCode, body, fmt.Sprintf("container not found: %s", id)); err != nil {
			return "", fmt.Errorf("docker: get logs: %w", err)
		}
	}
	defer func() { _ = resp.Body.Close() }()

	// Docker multiplexes stdout/stderr using an 8-byte header per frame.
	// We demultiplex manually: header[0] is stream type, header[4:8] is size.
	var out strings.Builder
	header := make([]byte, 8)
	for {
		_, err := io.ReadFull(resp.Body, header)
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("docker: read log frame header: %w", err)
		}
		size := int(header[4])<<24 | int(header[5])<<16 | int(header[6])<<8 | int(header[7])
		if size == 0 {
			continue
		}
		frame := make([]byte, size)
		if _, err := io.ReadFull(resp.Body, frame); err != nil {
			return "", fmt.Errorf("docker: read log frame: %w", err)
		}
		out.Write(frame)
	}
	return out.String(), nil
}

// dockerStatsResponse is the JSON shape returned by /containers/{id}/stats?stream=false.
type dockerStatsResponse struct {
	CPUStats struct {
		CPUUsage struct {
			TotalUsage uint64 `json:"total_usage"`
		} `json:"cpu_usage"`
		SystemCPUUsage uint64 `json:"system_cpu_usage"`
		OnlineCPUs     int    `json:"online_cpus"`
	} `json:"cpu_stats"`
	PreCPUStats struct {
		CPUUsage struct {
			TotalUsage uint64 `json:"total_usage"`
		} `json:"cpu_usage"`
		SystemCPUUsage uint64 `json:"system_cpu_usage"`
	} `json:"precpu_stats"`
	MemoryStats struct {
		Usage uint64 `json:"usage"`
		Limit uint64 `json:"limit"`
		Stats struct {
			Cache uint64 `json:"cache"`
		} `json:"stats"`
	} `json:"memory_stats"`
	Networks map[string]struct {
		RxBytes uint64 `json:"rx_bytes"`
		TxBytes uint64 `json:"tx_bytes"`
	} `json:"networks"`
}

// GetStats retrieves a single-shot resource usage snapshot for the container.
func (m *DockerClientManager) GetStats(ctx context.Context, id string) (*ContainerStats, error) {
	path := fmt.Sprintf("/containers/%s/stats?stream=false", id)
	resp, err := m.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, fmt.Errorf("docker: get stats: %w", err)
	}
	body, err := readBody(resp)
	if err != nil {
		return nil, fmt.Errorf("docker: get stats: %w", err)
	}
	if err := checkAPIError(resp.StatusCode, body, fmt.Sprintf("container not found: %s", id)); err != nil {
		return nil, fmt.Errorf("docker: get stats: %w", err)
	}

	var raw dockerStatsResponse
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("docker: decode stats: %w", err)
	}

	// Calculate CPU percent using the delta method.
	cpuDelta := float64(raw.CPUStats.CPUUsage.TotalUsage) - float64(raw.PreCPUStats.CPUUsage.TotalUsage)
	systemDelta := float64(raw.CPUStats.SystemCPUUsage) - float64(raw.PreCPUStats.SystemCPUUsage)
	cpuPercent := 0.0
	numCPUs := raw.CPUStats.OnlineCPUs
	if numCPUs == 0 {
		numCPUs = 1
	}
	if systemDelta > 0 && cpuDelta > 0 {
		cpuPercent = (cpuDelta / systemDelta) * float64(numCPUs) * 100.0
	}

	// Subtract page cache from memory usage (cgroup v1).
	memUsage := raw.MemoryStats.Usage
	if raw.MemoryStats.Stats.Cache <= memUsage {
		memUsage -= raw.MemoryStats.Stats.Cache
	}

	var rxBytes, txBytes uint64
	for _, net := range raw.Networks {
		rxBytes += net.RxBytes
		txBytes += net.TxBytes
	}

	return &ContainerStats{
		CPUPercent:     cpuPercent,
		MemoryUsage:    memUsage,
		MemoryLimit:    raw.MemoryStats.Limit,
		NetworkRxBytes: rxBytes,
		NetworkTxBytes: txBytes,
	}, nil
}

// ---------------------------------------------------------------------------
// Network operations
// ---------------------------------------------------------------------------

// dockerNetwork is the JSON shape returned by /networks.
type dockerNetwork struct {
	ID     string `json:"Id"`
	Name   string `json:"Name"`
	Driver string `json:"Driver"`
	Scope  string `json:"Scope"`
}

// ListNetworks returns a list of all Docker networks.
func (m *DockerClientManager) ListNetworks(ctx context.Context) ([]Network, error) {
	resp, err := m.doRequest(ctx, http.MethodGet, "/networks", nil)
	if err != nil {
		return nil, fmt.Errorf("docker: list networks: %w", err)
	}
	body, err := readBody(resp)
	if err != nil {
		return nil, fmt.Errorf("docker: list networks: %w", err)
	}
	if err := checkAPIError(resp.StatusCode, body, "network not found"); err != nil {
		return nil, fmt.Errorf("docker: list networks: %w", err)
	}

	var raw []dockerNetwork
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("docker: decode network list: %w", err)
	}

	networks := make([]Network, 0, len(raw))
	for _, n := range raw {
		networks = append(networks, Network(n))
	}
	return networks, nil
}

// dockerNetworkInspect is the JSON shape returned by /networks/{id}.
type dockerNetworkInspect struct {
	ID         string `json:"Id"`
	Name       string `json:"Name"`
	Driver     string `json:"Driver"`
	Scope      string `json:"Scope"`
	Containers map[string]struct {
		Name string `json:"Name"`
	} `json:"Containers"`
	IPAM struct {
		Config []struct {
			Subnet  string `json:"Subnet"`
			Gateway string `json:"Gateway"`
		} `json:"Config"`
	} `json:"IPAM"`
}

// InspectNetwork returns detailed information about a network.
func (m *DockerClientManager) InspectNetwork(ctx context.Context, id string) (*NetworkDetail, error) {
	if id == "" {
		return nil, fmt.Errorf("network not found: %s", id)
	}
	resp, err := m.doRequest(ctx, http.MethodGet, "/networks/"+id, nil)
	if err != nil {
		return nil, fmt.Errorf("docker: inspect network: %w", err)
	}
	body, err := readBody(resp)
	if err != nil {
		return nil, fmt.Errorf("docker: inspect network: %w", err)
	}
	if err := checkAPIError(resp.StatusCode, body, fmt.Sprintf("network not found: %s", id)); err != nil {
		return nil, fmt.Errorf("docker: inspect network: %w", err)
	}

	var raw dockerNetworkInspect
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("docker: decode network inspect: %w", err)
	}

	containerIDs := make([]string, 0, len(raw.Containers))
	for cid := range raw.Containers {
		containerIDs = append(containerIDs, cid)
	}

	subnet := ""
	gateway := ""
	if len(raw.IPAM.Config) > 0 {
		subnet = raw.IPAM.Config[0].Subnet
		gateway = raw.IPAM.Config[0].Gateway
	}

	return &NetworkDetail{
		Network: Network{
			ID:     raw.ID,
			Name:   raw.Name,
			Driver: raw.Driver,
			Scope:  raw.Scope,
		},
		Containers: containerIDs,
		Subnet:     subnet,
		Gateway:    gateway,
	}, nil
}

// networkCreateRequest is the request body for POST /networks/create.
type networkCreateRequest struct {
	Name           string            `json:"Name"`
	Driver         string            `json:"Driver,omitempty"`
	CheckDuplicate bool              `json:"CheckDuplicate"`
	IPAM           *networkIPAM      `json:"IPAM,omitempty"`
	Labels         map[string]string `json:"Labels,omitempty"`
}

type networkIPAM struct {
	Driver string           `json:"Driver"`
	Config []networkIPAMCfg `json:"Config"`
}

type networkIPAMCfg struct {
	Subnet string `json:"Subnet,omitempty"`
}

// CreateNetwork creates a new Docker network and returns its ID.
func (m *DockerClientManager) CreateNetwork(ctx context.Context, config NetworkCreateConfig) (string, error) {
	if config.Name == "" {
		return "", fmt.Errorf("network name is required")
	}

	reqBody := networkCreateRequest{
		Name:           config.Name,
		Driver:         config.Driver,
		CheckDuplicate: true,
	}
	if config.Subnet != "" {
		reqBody.IPAM = &networkIPAM{
			Driver: "default",
			Config: []networkIPAMCfg{{Subnet: config.Subnet}},
		}
	}

	bodyData, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("docker: encode network create request: %w", err)
	}

	resp, err := m.doRequest(ctx, http.MethodPost, "/networks/create", bytes.NewReader(bodyData))
	if err != nil {
		return "", fmt.Errorf("docker: create network: %w", err)
	}
	body, err := readBody(resp)
	if err != nil {
		return "", err
	}

	if err := checkAPIError(resp.StatusCode, body, "network not found"); err != nil {
		return "", fmt.Errorf("docker: create network: %w", err)
	}

	var result struct {
		ID string `json:"Id"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("docker: decode network create response: %w", err)
	}
	if result.ID == "" {
		return "", fmt.Errorf("docker: create network returned empty ID")
	}
	return result.ID, nil
}

// RemoveNetwork removes the network with the given ID.
func (m *DockerClientManager) RemoveNetwork(ctx context.Context, id string) error {
	resp, err := m.doRequest(ctx, http.MethodDelete, "/networks/"+id, nil)
	if err != nil {
		return fmt.Errorf("docker: remove network: %w", err)
	}
	body, readErr := readBody(resp)
	if readErr != nil {
		return readErr
	}
	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("network not found: %s", id)
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	if err := checkAPIError(resp.StatusCode, body, fmt.Sprintf("network not found: %s", id)); err != nil {
		return fmt.Errorf("docker: remove network: %w", err)
	}
	return nil
}

// networkConnectRequest is the request body for POST /networks/{id}/connect.
type networkConnectRequest struct {
	Container string `json:"Container"`
}

// ConnectNetwork connects a container to a network.
func (m *DockerClientManager) ConnectNetwork(ctx context.Context, networkID, containerID string) error {
	reqBody := networkConnectRequest{Container: containerID}
	bodyData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("docker: encode connect request: %w", err)
	}

	resp, err := m.doRequest(ctx, http.MethodPost, "/networks/"+networkID+"/connect", bytes.NewReader(bodyData))
	if err != nil {
		return fmt.Errorf("docker: connect network: %w", err)
	}
	body, readErr := readBody(resp)
	if readErr != nil {
		return readErr
	}
	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("network not found: %s", networkID)
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	if err := checkAPIError(resp.StatusCode, body, fmt.Sprintf("network not found: %s", networkID)); err != nil {
		return fmt.Errorf("docker: connect network: %w", err)
	}
	return nil
}

// networkDisconnectRequest is the request body for POST /networks/{id}/disconnect.
type networkDisconnectRequest struct {
	Container string `json:"Container"`
	Force     bool   `json:"Force"`
}

// DisconnectNetwork disconnects a container from a network.
func (m *DockerClientManager) DisconnectNetwork(ctx context.Context, networkID, containerID string) error {
	reqBody := networkDisconnectRequest{Container: containerID, Force: false}
	bodyData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("docker: encode disconnect request: %w", err)
	}

	resp, err := m.doRequest(ctx, http.MethodPost, "/networks/"+networkID+"/disconnect", bytes.NewReader(bodyData))
	if err != nil {
		return fmt.Errorf("docker: disconnect network: %w", err)
	}
	body, readErr := readBody(resp)
	if readErr != nil {
		return readErr
	}
	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("network not found: %s", networkID)
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	if err := checkAPIError(resp.StatusCode, body, fmt.Sprintf("network not found: %s", networkID)); err != nil {
		return fmt.Errorf("docker: disconnect network: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Interface compliance check
// ---------------------------------------------------------------------------

// Ensure DockerClientManager satisfies the DockerManager interface at compile time.
var _ DockerManager = (*DockerClientManager)(nil)
