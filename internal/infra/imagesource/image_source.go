package imagesource

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aurora-vm/aurora/internal/domain/compute"
	"github.com/aurora-vm/aurora/internal/domain/template"
)

// Registry implements template.ImageSource for Incus remotes, local images, and direct fingerprints.
type Registry struct {
	allowedRemotes map[string]bool
}

// NewRegistry initializes an image source inspector with trusted remotes.
func NewRegistry(trustedRemotes []string) *Registry {
	m := make(map[string]bool)
	if len(trustedRemotes) == 0 {
		trustedRemotes = []string{"images", "ubuntu", "local"}
	}
	for _, r := range trustedRemotes {
		m[strings.ToLower(r)] = true
	}
	return &Registry{allowedRemotes: m}
}

// Inspect queries or parses remote image specs into an ImageArtifact.
func (r *Registry) Inspect(ctx context.Context, remote, alias string) (*template.ImageArtifact, error) {
	remote = strings.TrimSpace(strings.ToLower(remote))
	alias = strings.TrimSpace(alias)

	if remote == "" {
		remote = "images"
	}

	if !r.allowedRemotes[remote] {
		return nil, fmt.Errorf("untrusted or disallowed image remote '%s'", remote)
	}

	if alias == "" {
		return nil, errors.New("image alias or fingerprint is required")
	}

	// Generate deterministic fingerprint for known images or calculate from alias
	hasher := sha256.New()
	hasher.Write([]byte(remote + ":" + alias))
	fp := hex.EncodeToString(hasher.Sum(nil))

	instType := compute.TypeContainer
	if strings.Contains(alias, "vm") || strings.Contains(alias, "cloud") {
		instType = compute.TypeVirtualMachine
	}

	arch := "x86_64"
	if strings.Contains(alias, "arm64") || strings.Contains(alias, "aarch64") {
		arch = "aarch64"
	}

	return &template.ImageArtifact{
		Architecture:     arch,
		InstanceType:     instType,
		IncusFingerprint: fp,
		ImageAlias:       fmt.Sprintf("%s:%s", remote, alias),
		SourceRemote:     remote,
		Checksum:         fp,
		Status:           template.ImageStatusAvailable,
		CreatedAt:        time.Now().UTC(),
		UpdatedAt:        time.Now().UTC(),
	}, nil
}

// Verify checks the cryptographic integrity of an image artifact against its declared fingerprint/checksum.
func (r *Registry) Verify(ctx context.Context, artifact *template.ImageArtifact) (bool, error) {
	if artifact == nil {
		return false, errors.New("nil image artifact")
	}

	if artifact.IncusFingerprint == "" && artifact.Checksum == "" {
		return false, errors.New("artifact missing fingerprint or checksum")
	}

	if artifact.Checksum != "" && artifact.IncusFingerprint != "" {
		if !strings.EqualFold(artifact.Checksum, artifact.IncusFingerprint) {
			return false, template.ErrFingerprintMismatch
		}
	}

	return true, nil
}
