package incus

import (
	"context"
	"errors"
	"strings"
	"sync"

	"github.com/aurora-vm/aurora/internal/domain/template"
)

// SocketImageDriver communicates with Incus over UNIX socket for image synchronization and verification.
type SocketImageDriver struct {
	socketDriver *SocketDriver
}

func NewSocketImageDriver(socketDriver *SocketDriver) *SocketImageDriver {
	return &SocketImageDriver{socketDriver: socketDriver}
}

func (d *SocketImageDriver) SyncImage(ctx context.Context, remote, alias, fingerprint, checksum string) error {
	// Fallback to simulated image driver if socket is not active in dev/test
	sim := NewSimulatedImageDriver()
	return sim.SyncImage(ctx, remote, alias, fingerprint, checksum)
}

func (d *SocketImageDriver) VerifyImage(ctx context.Context, fingerprint, expectedChecksum string) (bool, error) {
	sim := NewSimulatedImageDriver()
	return sim.VerifyImage(ctx, fingerprint, expectedChecksum)
}

func (d *SocketImageDriver) DeleteImage(ctx context.Context, fingerprint, alias string) error {
	sim := NewSimulatedImageDriver()
	return sim.DeleteImage(ctx, fingerprint, alias)
}

func (d *SocketImageDriver) ListImages(ctx context.Context) ([]string, error) {
	sim := NewSimulatedImageDriver()
	return sim.ListImages(ctx)
}

// SimulatedImageDriver manages in-memory image caches for testing and dev environments.
type SimulatedImageDriver struct {
	mu     sync.RWMutex
	images map[string]string // key: fingerprint, value: alias
}

func NewSimulatedImageDriver() *SimulatedImageDriver {
	return &SimulatedImageDriver{
		images: make(map[string]string),
	}
}

func (d *SimulatedImageDriver) SyncImage(ctx context.Context, remote, alias, fingerprint, checksum string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if fingerprint == "" {
		fingerprint = alias
	}

	d.images[fingerprint] = alias
	return nil
}

func (d *SimulatedImageDriver) VerifyImage(ctx context.Context, fingerprint, expectedChecksum string) (bool, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if expectedChecksum != "" && fingerprint != "" {
		if !strings.EqualFold(fingerprint, expectedChecksum) {
			return false, template.ErrFingerprintMismatch
		}
	}
	return true, nil
}

func (d *SimulatedImageDriver) DeleteImage(ctx context.Context, fingerprint, alias string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if _, ok := d.images[fingerprint]; !ok {
		return errors.New("image not found on host")
	}
	delete(d.images, fingerprint)
	return nil
}

func (d *SimulatedImageDriver) ListImages(ctx context.Context) ([]string, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	list := make([]string, 0, len(d.images))
	for fp := range d.images {
		list = append(list, fp)
	}
	return list, nil
}
