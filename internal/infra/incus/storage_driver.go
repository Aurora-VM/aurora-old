package incus

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
)

// SocketStorageDriver implements storage.StorageDriver against Incus via UNIX socket.
type SocketStorageDriver struct {
	client *http.Client
}

// NewSocketStorageDriver creates a storage driver using an Incus socket client.
func NewSocketStorageDriver(sockDriver *SocketDriver) *SocketStorageDriver {
	return &SocketStorageDriver{
		client: sockDriver.client,
	}
}

func (d *SocketStorageDriver) CreateVolume(ctx context.Context, poolName, volumeName string, sizeBytes int64, contentType string) error {
	payload := map[string]interface{}{
		"name":         volumeName,
		"content_type": contentType,
		"config": map[string]string{
			"size": fmt.Sprintf("%d", sizeBytes),
		},
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("http://unix/1.0/storage-pools/%s/volumes/custom", poolName), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to create incus custom volume: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("incus create volume returned status %d", resp.StatusCode)
	}
	return nil
}

func (d *SocketStorageDriver) ResizeVolume(ctx context.Context, poolName, volumeName string, newSizeBytes int64) error {
	payload := map[string]interface{}{
		"config": map[string]string{
			"size": fmt.Sprintf("%d", newSizeBytes),
		},
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, "PATCH", fmt.Sprintf("http://unix/1.0/storage-pools/%s/volumes/custom/%s", poolName, volumeName), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to resize incus custom volume: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("incus resize volume returned status %d", resp.StatusCode)
	}
	return nil
}

func (d *SocketStorageDriver) AttachVolume(ctx context.Context, instanceName, poolName, volumeName, mountPath string, readOnly bool) error {
	payload := map[string]interface{}{
		"devices": map[string]interface{}{
			volumeName: map[string]interface{}{
				"type":      "disk",
				"pool":      poolName,
				"source":    volumeName,
				"path":      mountPath,
				"readonly":  readOnly,
			},
		},
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, "PATCH", fmt.Sprintf("http://unix/1.0/instances/%s", instanceName), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to attach volume in incus: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("incus attach volume returned status %d", resp.StatusCode)
	}
	return nil
}

func (d *SocketStorageDriver) DetachVolume(ctx context.Context, instanceName, volumeName string) error {
	// In Incus, omitting or removing device from instance devices map detaches it
	payload := map[string]interface{}{
		"devices": map[string]interface{}{
			volumeName: nil,
		},
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, "PATCH", fmt.Sprintf("http://unix/1.0/instances/%s", instanceName), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to detach volume in incus: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("incus detach volume returned status %d", resp.StatusCode)
	}
	return nil
}

func (d *SocketStorageDriver) DeleteVolume(ctx context.Context, poolName, volumeName string) error {
	req, err := http.NewRequestWithContext(ctx, "DELETE", fmt.Sprintf("http://unix/1.0/storage-pools/%s/volumes/custom/%s", poolName, volumeName), nil)
	if err != nil {
		return err
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to delete volume in incus: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("incus delete volume returned status %d", resp.StatusCode)
	}
	return nil
}

func (d *SocketStorageDriver) CreateSnapshot(ctx context.Context, poolName, volumeName, snapshotName string) error {
	payload := map[string]interface{}{
		"name": snapshotName,
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("http://unix/1.0/storage-pools/%s/volumes/custom/%s/snapshots", poolName, volumeName), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to snapshot volume in incus: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("incus create snapshot returned status %d", resp.StatusCode)
	}
	return nil
}

func (d *SocketStorageDriver) RestoreSnapshot(ctx context.Context, poolName, volumeName, snapshotName string) error {
	payload := map[string]interface{}{
		"restore": snapshotName,
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, "PUT", fmt.Sprintf("http://unix/1.0/storage-pools/%s/volumes/custom/%s", poolName, volumeName), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to restore volume snapshot in incus: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("incus restore snapshot returned status %d", resp.StatusCode)
	}
	return nil
}

func (d *SocketStorageDriver) DeleteSnapshot(ctx context.Context, poolName, volumeName, snapshotName string) error {
	req, err := http.NewRequestWithContext(ctx, "DELETE", fmt.Sprintf("http://unix/1.0/storage-pools/%s/volumes/custom/%s/snapshots/%s", poolName, volumeName, snapshotName), nil)
	if err != nil {
		return err
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to delete volume snapshot in incus: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("incus delete snapshot returned status %d", resp.StatusCode)
	}
	return nil
}

// SimulatedStorageDriver implements storage.StorageDriver in memory.
type SimulatedStorageDriver struct {
	mu        sync.RWMutex
	volumes   map[string]map[string]int64               // poolName -> volumeName -> sizeBytes
	attached  map[string]map[string]string              // instanceName -> volumeName -> mountPath
	snapshots map[string]map[string]map[string]struct{} // poolName -> volumeName -> snapshotName -> struct{}
}

func NewSimulatedStorageDriver() *SimulatedStorageDriver {
	return &SimulatedStorageDriver{
		volumes:   make(map[string]map[string]int64),
		attached:  make(map[string]map[string]string),
		snapshots: make(map[string]map[string]map[string]struct{}),
	}
}

func (s *SimulatedStorageDriver) CreateVolume(ctx context.Context, poolName, volumeName string, sizeBytes int64, contentType string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.volumes[poolName] == nil {
		s.volumes[poolName] = make(map[string]int64)
	}
	s.volumes[poolName][volumeName] = sizeBytes
	return nil
}

func (s *SimulatedStorageDriver) ResizeVolume(ctx context.Context, poolName, volumeName string, newSizeBytes int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.volumes[poolName] == nil {
		s.volumes[poolName] = make(map[string]int64)
	}
	s.volumes[poolName][volumeName] = newSizeBytes
	return nil
}

func (s *SimulatedStorageDriver) AttachVolume(ctx context.Context, instanceName, poolName, volumeName, mountPath string, readOnly bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.attached[instanceName] == nil {
		s.attached[instanceName] = make(map[string]string)
	}
	s.attached[instanceName][volumeName] = mountPath
	return nil
}

func (s *SimulatedStorageDriver) DetachVolume(ctx context.Context, instanceName, volumeName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.attached[instanceName] != nil {
		delete(s.attached[instanceName], volumeName)
	}
	return nil
}

func (s *SimulatedStorageDriver) DeleteVolume(ctx context.Context, poolName, volumeName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.volumes[poolName] != nil {
		delete(s.volumes[poolName], volumeName)
	}
	return nil
}

func (s *SimulatedStorageDriver) CreateSnapshot(ctx context.Context, poolName, volumeName, snapshotName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.snapshots[poolName] == nil {
		s.snapshots[poolName] = make(map[string]map[string]struct{})
	}
	if s.snapshots[poolName][volumeName] == nil {
		s.snapshots[poolName][volumeName] = make(map[string]struct{})
	}
	s.snapshots[poolName][volumeName][snapshotName] = struct{}{}
	return nil
}

func (s *SimulatedStorageDriver) RestoreSnapshot(ctx context.Context, poolName, volumeName, snapshotName string) error {
	return nil
}

func (s *SimulatedStorageDriver) DeleteSnapshot(ctx context.Context, poolName, volumeName, snapshotName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.snapshots[poolName] != nil && s.snapshots[poolName][volumeName] != nil {
		delete(s.snapshots[poolName][volumeName], snapshotName)
	}
	return nil
}
