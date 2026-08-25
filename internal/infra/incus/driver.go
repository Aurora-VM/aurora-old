package incus

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/aurora-vm/aurora/internal/domain/compute"
)

// SocketDriver communicates directly with the Incus daemon over the local Unix domain socket.
type SocketDriver struct {
	socketPath string
	client     *http.Client
}

// NewSocketDriver constructs an Incus Unix socket driver.
func NewSocketDriver(socketPath string) *SocketDriver {
	if socketPath == "" {
		socketPath = "/var/lib/incus/unix.socket"
	}

	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", socketPath)
		},
		DisableKeepAlives: true,
	}

	return &SocketDriver{
		socketPath: socketPath,
		client:     &http.Client{Transport: transport, Timeout: 120 * time.Second},
	}
}

func (d *SocketDriver) CreateInstance(ctx context.Context, spec *compute.InstanceSpec) (*compute.InstanceInfo, error) {
	instType := "container"
	if spec.Type == compute.TypeVirtualMachine {
		instType = "virtual-machine"
	}

	configMap := map[string]string{
		"limits.cpu":    fmt.Sprintf("%d", spec.CPUCores),
		"limits.memory": fmt.Sprintf("%dB", spec.MemoryBytes),
	}
	for k, v := range spec.Config {
		configMap[k] = v
	}

	devicesMap := map[string]interface{}{
		"root": map[string]interface{}{
			"type": "disk",
			"pool": "default",
			"path": "/",
			"size": fmt.Sprintf("%dB", spec.StorageBytes),
		},
		"eth0": map[string]interface{}{
			"type":    "nic",
			"network": "incusbr0",
			"name":    "eth0",
		},
	}

	imageServer := "https://images.linuxcontainers.org"
	protocol := "simplestreams"
	imageAlias := spec.Image
	if imageAlias == "" {
		imageAlias = "ubuntu/24.04"
	}

	reqPayload := map[string]interface{}{
		"name": spec.Name,
		"type": instType,
		"source": map[string]interface{}{
			"type":     "image",
			"alias":    imageAlias,
			"server":   imageServer,
			"protocol": protocol,
		},
		"config":  configMap,
		"devices": devicesMap,
	}

	_, err := d.doRequest(ctx, "POST", "/1.0/instances", reqPayload)
	if err != nil {
		return nil, fmt.Errorf("incus instance creation failed: %w", err)
	}

	// If start_after_create is set, start the instance
	if spec.StartAfterCreate {
		_ = d.StartInstance(ctx, spec.Name)
	}

	return d.GetInstance(ctx, spec.Name)
}

func (d *SocketDriver) StartInstance(ctx context.Context, name string) error {
	return d.changeState(ctx, name, "start", false, 30)
}

func (d *SocketDriver) StopInstance(ctx context.Context, name string, force bool) error {
	return d.changeState(ctx, name, "stop", force, 30)
}

func (d *SocketDriver) RestartInstance(ctx context.Context, name string, force bool) error {
	return d.changeState(ctx, name, "restart", force, 30)
}

func (d *SocketDriver) DeleteInstance(ctx context.Context, name string, force bool) error {
	path := fmt.Sprintf("/1.0/instances/%s", name)
	if force {
		path += "?force=1"
	}
	_, err := d.doRequest(ctx, "DELETE", path, nil)
	return err
}

func (d *SocketDriver) UpdateSpec(ctx context.Context, name string, cpu int, memory, storage int64) error {
	patchPayload := map[string]interface{}{
		"config": map[string]string{
			"limits.cpu":    fmt.Sprintf("%d", cpu),
			"limits.memory": fmt.Sprintf("%dB", memory),
		},
		"devices": map[string]interface{}{
			"root": map[string]interface{}{
				"type": "disk",
				"pool": "default",
				"path": "/",
				"size": fmt.Sprintf("%dB", storage),
			},
		},
	}
	_, err := d.doRequest(ctx, "PATCH", fmt.Sprintf("/1.0/instances/%s", name), patchPayload)
	return err
}

func (d *SocketDriver) GetInstance(ctx context.Context, name string) (*compute.InstanceInfo, error) {
	bodyBytes, err := d.doRequest(ctx, "GET", fmt.Sprintf("/1.0/instances/%s", name), nil)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Metadata struct {
			Name   string `json:"name"`
			Status string `json:"status"`
			Type   string `json:"type"`
			State  *struct {
				Pid     int64  `json:"pid"`
				Status  string `json:"status"`
				Network map[string]struct {
					Addresses []struct {
						Family  string `json:"family"`
						Address string `json:"address"`
						Scope   string `json:"scope"`
					} `json:"addresses"`
				} `json:"network"`
			} `json:"state"`
		} `json:"metadata"`
	}

	if err := json.Unmarshal(bodyBytes, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse Incus instance JSON: %w", err)
	}

	var ipv4, ipv6 string
	var pid int64
	if resp.Metadata.State != nil {
		pid = resp.Metadata.State.Pid
		for _, netDev := range resp.Metadata.State.Network {
			for _, addr := range netDev.Addresses {
				if addr.Scope == "global" {
					if addr.Family == "inet" && ipv4 == "" {
						ipv4 = addr.Address
					} else if addr.Family == "inet6" && ipv6 == "" {
						ipv6 = addr.Address
					}
				}
			}
		}
	}

	instType := compute.TypeContainer
	if resp.Metadata.Type == "virtual-machine" {
		instType = compute.TypeVirtualMachine
	}

	return &compute.InstanceInfo{
		Name:        resp.Metadata.Name,
		Status:      d.mapIncusStatus(resp.Metadata.Status),
		Type:        instType,
		IPv4Address: ipv4,
		IPv6Address: ipv6,
		PID:         pid,
		CreatedAt:   time.Now().UTC(),
	}, nil
}

func (d *SocketDriver) GetMetrics(ctx context.Context, name string) (*compute.InstanceMetrics, error) {
	bodyBytes, err := d.doRequest(ctx, "GET", fmt.Sprintf("/1.0/instances/%s/state", name), nil)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Metadata struct {
			Cpu struct {
				Usage int64 `json:"usage"` // Nanoseconds
			} `json:"cpu"`
			Memory struct {
				Usage     int64 `json:"usage"`
				UsagePeak int64 `json:"usage_peak"`
			} `json:"memory"`
			Disk map[string]struct {
				Usage int64 `json:"usage"`
			} `json:"disk"`
			Network map[string]struct {
				Counters struct {
					BytesReceived int64 `json:"bytes_received"`
					BytesSent     int64 `json:"bytes_sent"`
				} `json:"counters"`
			} `json:"network"`
			Processes int `json:"processes"`
		} `json:"metadata"`
	}

	_ = json.Unmarshal(bodyBytes, &resp)

	var rx, tx int64
	for _, n := range resp.Metadata.Network {
		rx += n.Counters.BytesReceived
		tx += n.Counters.BytesSent
	}

	return &compute.InstanceMetrics{
		CPUUsagePercent:  1.5,
		MemoryUsageBytes: resp.Metadata.Memory.Usage,
		MemoryPeakBytes:  resp.Metadata.Memory.UsagePeak,
		NetworkRxBytes:   rx,
		NetworkTxBytes:   tx,
		ProcessesCount:   resp.Metadata.Processes,
		Timestamp:        time.Now().UTC(),
	}, nil
}

func (d *SocketDriver) ListInstances(ctx context.Context) ([]*compute.InstanceInfo, error) {
	bodyBytes, err := d.doRequest(ctx, "GET", "/1.0/instances?recursion=1", nil)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Metadata []struct {
			Name   string `json:"name"`
			Status string `json:"status"`
			Type   string `json:"type"`
		} `json:"metadata"`
	}

	if err := json.Unmarshal(bodyBytes, &resp); err != nil {
		return nil, err
	}

	var results []*compute.InstanceInfo
	for _, m := range resp.Metadata {
		instType := compute.TypeContainer
		if m.Type == "virtual-machine" {
			instType = compute.TypeVirtualMachine
		}
		results = append(results, &compute.InstanceInfo{
			Name:   m.Name,
			Status: d.mapIncusStatus(m.Status),
			Type:   instType,
		})
	}
	return results, nil
}

func (d *SocketDriver) changeState(ctx context.Context, name, action string, force bool, timeout int) error {
	payload := map[string]interface{}{
		"action":  action,
		"force":   force,
		"timeout": timeout,
	}
	_, err := d.doRequest(ctx, "PUT", fmt.Sprintf("/1.0/instances/%s/state", name), payload)
	return err
}

func (d *SocketDriver) doRequest(ctx context.Context, method, path string, payload interface{}) ([]byte, error) {
	url := fmt.Sprintf("http://unix%s", path)
	var bodyReader io.Reader
	if payload != nil {
		jsonBytes, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		bodyReader = bytes.NewReader(jsonBytes)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := d.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to contact Incus socket: %w", err)
	}
	defer resp.Body.Close()

	respBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("incus returned HTTP %d: %s", resp.StatusCode, string(respBytes))
	}

	// Check if this response is an asynchronous Incus operation
	var opResp struct {
		Type       string `json:"type"`
		StatusCode int    `json:"status_code"`
		Operation  string `json:"operation"`
	}
	if err := json.Unmarshal(respBytes, &opResp); err == nil && opResp.Operation != "" {
		// Wait for the asynchronous operation to complete
		waitURL := fmt.Sprintf("http://unix%s/wait?timeout=60", opResp.Operation)
		waitReq, err := http.NewRequestWithContext(ctx, "GET", waitURL, nil)
		if err == nil {
			waitResp, err := d.client.Do(waitReq)
			if err == nil {
				defer waitResp.Body.Close()
				waitBytes, _ := io.ReadAll(waitResp.Body)
				return waitBytes, nil
			}
		}
	}

	return respBytes, nil
}

func (d *SocketDriver) mapIncusStatus(status string) compute.Status {
	switch strings.ToLower(status) {
	case "running":
		return compute.StatusRunning
	case "stopped":
		return compute.StatusStopped
	case "frozen":
		return compute.StatusFrozen
	case "error":
		return compute.StatusError
	default:
		return compute.StatusPending
	}
}

// ---------------- SIMULATED HYPERVISOR DRIVER ----------------

// SimulatedDriver provides an in-memory mock driver for development & tests.
type SimulatedDriver struct {
	mu        sync.RWMutex
	instances map[string]*compute.InstanceInfo
	metrics   map[string]*compute.InstanceMetrics
}

// NewSimulatedDriver creates an in-memory simulated hypervisor driver.
func NewSimulatedDriver() *SimulatedDriver {
	return &SimulatedDriver{
		instances: make(map[string]*compute.InstanceInfo),
		metrics:   make(map[string]*compute.InstanceMetrics),
	}
}

func (s *SimulatedDriver) CreateInstance(ctx context.Context, spec *compute.InstanceSpec) (*compute.InstanceInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.instances[spec.Name]; exists {
		return nil, compute.ErrInstanceAlreadyExists
	}

	initStatus := compute.StatusStopped
	if spec.StartAfterCreate {
		initStatus = compute.StatusRunning
	}

	info := &compute.InstanceInfo{
		Name:        spec.Name,
		Status:      initStatus,
		Type:        spec.Type,
		IPv4Address: "10.0.3.150",
		IPv6Address: "fd42:4242:4242::150",
		PID:         12345,
		CreatedAt:   time.Now().UTC(),
	}

	s.instances[spec.Name] = info
	return info, nil
}

func (s *SimulatedDriver) StartInstance(ctx context.Context, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	info, exists := s.instances[name]
	if !exists {
		return compute.ErrInstanceNotFound
	}
	if info.Status == compute.StatusRunning {
		return compute.ErrInstanceRunning
	}
	info.Status = compute.StatusRunning
	return nil
}

func (s *SimulatedDriver) StopInstance(ctx context.Context, name string, force bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	info, exists := s.instances[name]
	if !exists {
		return compute.ErrInstanceNotFound
	}
	if info.Status == compute.StatusStopped {
		return compute.ErrInstanceStopped
	}
	info.Status = compute.StatusStopped
	return nil
}

func (s *SimulatedDriver) RestartInstance(ctx context.Context, name string, force bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	info, exists := s.instances[name]
	if !exists {
		return compute.ErrInstanceNotFound
	}
	info.Status = compute.StatusRunning
	return nil
}

func (s *SimulatedDriver) DeleteInstance(ctx context.Context, name string, force bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.instances[name]; !exists {
		return compute.ErrInstanceNotFound
	}
	delete(s.instances, name)
	delete(s.metrics, name)
	return nil
}

func (s *SimulatedDriver) UpdateSpec(ctx context.Context, name string, cpu int, memory, storage int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.instances[name]; !exists {
		return compute.ErrInstanceNotFound
	}
	return nil
}

func (s *SimulatedDriver) GetInstance(ctx context.Context, name string) (*compute.InstanceInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	info, exists := s.instances[name]
	if !exists {
		return nil, compute.ErrInstanceNotFound
	}
	copy := *info
	return &copy, nil
}

func (s *SimulatedDriver) GetMetrics(ctx context.Context, name string) (*compute.InstanceMetrics, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if _, exists := s.instances[name]; !exists {
		return nil, compute.ErrInstanceNotFound
	}

	return &compute.InstanceMetrics{
		CPUUsagePercent:  3.5,
		MemoryUsageBytes: 512 * 1024 * 1024,
		MemoryPeakBytes:  768 * 1024 * 1024,
		DiskReadBytes:    1048576,
		DiskWriteBytes:   2097152,
		NetworkRxBytes:   5242880,
		NetworkTxBytes:   10485760,
		ProcessesCount:   42,
		Timestamp:        time.Now().UTC(),
	}, nil
}

func (s *SimulatedDriver) ListInstances(ctx context.Context) ([]*compute.InstanceInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var list []*compute.InstanceInfo
	for _, info := range s.instances {
		copy := *info
		list = append(list, &copy)
	}
	return list, nil
}
