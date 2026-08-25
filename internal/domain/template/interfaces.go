package template

import (
	"context"

	"github.com/aurora-vm/aurora/internal/domain/compute"
)

// TemplateRepository defines persistence operations for customer-facing OS product templates.
type TemplateRepository interface {
	Create(ctx context.Context, t *OSTemplate) error
	GetByID(ctx context.Context, id string) (*OSTemplate, error)
	GetBySlug(ctx context.Context, slug string) (*OSTemplate, error)
	Update(ctx context.Context, t *OSTemplate) error
	Delete(ctx context.Context, id string) error
	List(ctx context.Context, filter TemplateFilter) ([]*OSTemplate, int64, error)
}

// ImageRepository defines persistence operations for hypervisor image artifacts.
type ImageRepository interface {
	Create(ctx context.Context, img *ImageArtifact) error
	GetByID(ctx context.Context, id string) (*ImageArtifact, error)
	GetByFingerprint(ctx context.Context, fingerprint string) (*ImageArtifact, error)
	ListByTemplate(ctx context.Context, templateID string) ([]*ImageArtifact, error)
	FindCompatible(ctx context.Context, templateID, architecture string, instType compute.InstanceType) (*ImageArtifact, error)
	Update(ctx context.Context, img *ImageArtifact) error
	UpdateStatus(ctx context.Context, id string, status ImageStatus, errorMsg string) error
	Delete(ctx context.Context, id string) error
	List(ctx context.Context, filter ImageFilter) ([]*ImageArtifact, int64, error)

	RecordNodeAvailability(ctx context.Context, nodeID, artifactID, status string) error
	GetNodeAvailability(ctx context.Context, nodeID, artifactID string) (*NodeImageAvailability, error)
	ListNodeAvailability(ctx context.Context, nodeID string) ([]*NodeImageAvailability, error)
}

// ImageSource defines the provider port for querying and verifying external image sources.
type ImageSource interface {
	Inspect(ctx context.Context, remote, alias string) (*ImageArtifact, error)
	Verify(ctx context.Context, artifact *ImageArtifact) (bool, error)
}

// HypervisorImageDriver abstracts local hypervisor node image caching and synchronization.
type HypervisorImageDriver interface {
	SyncImage(ctx context.Context, remote, alias, fingerprint, checksum string) error
	VerifyImage(ctx context.Context, fingerprint, expectedChecksum string) (bool, error)
	DeleteImage(ctx context.Context, fingerprint, alias string) error
	ListImages(ctx context.Context) ([]string, error)
}
