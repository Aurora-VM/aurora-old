package template

import (
	"regexp"
	"strings"
	"time"

	"github.com/aurora-vm/aurora/internal/domain/compute"
	"github.com/aurora-vm/aurora/internal/domain/identity"
)

type TemplateStatus string

const (
	TemplateStatusActive     TemplateStatus = "active"
	TemplateStatusDeprecated TemplateStatus = "deprecated"
	TemplateStatusRetired    TemplateStatus = "retired"
)

type ImageStatus string

const (
	ImageStatusQueued             ImageStatus = "queued"
	ImageStatusSyncing            ImageStatus = "syncing"
	ImageStatusVerifying          ImageStatus = "verifying"
	ImageStatusAvailable          ImageStatus = "available"
	ImageStatusVerificationFailed ImageStatus = "verification_failed"
	ImageStatusSyncFailed         ImageStatus = "sync_failed"
	ImageStatusRetired            ImageStatus = "retired"
)

var (
	hex64Regex = regexp.MustCompile(`^[a-fA-F0-9]{64}$`)
	slugRegex  = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{1,62}[a-z0-9]$`)
)

// NormalizeArchitecture converts architecture variants (e.g. amd64/x86_64, arm64/aarch64) to canonical form.
func NormalizeArchitecture(arch string) string {
	a := strings.ToLower(strings.TrimSpace(arch))
	switch a {
	case "amd64", "x86_64", "x86-64", "x64":
		return "x86_64"
	case "arm64", "aarch64", "armv8", "armv8l":
		return "aarch64"
	default:
		return a
	}
}

// OSTemplate represents customer-facing OS product templates (e.g. Ubuntu 24.04 LTS).
type OSTemplate struct {
	ID                     string                 `json:"id"`
	Name                   string                 `json:"name"`
	Slug                   string                 `json:"slug"`
	Description            string                 `json:"description"`
	Distribution           string                 `json:"distribution"`
	Version                string                 `json:"version"`
	Release                string                 `json:"release"`
	SupportedArchitectures []string               `json:"supportedArchitectures"`
	SupportedInstanceTypes []compute.InstanceType `json:"supportedInstanceTypes"`
	MinDiskBytes           int64                  `json:"minDiskBytes"`
	MinMemoryBytes         int64                  `json:"minMemoryBytes"`
	CloudInitSupported     bool                   `json:"cloudInitSupported"`
	Status                 TemplateStatus         `json:"status"`
	Metadata               map[string]interface{} `json:"metadata"`
	CreatedAt              time.Time              `json:"createdAt"`
	UpdatedAt              time.Time              `json:"updatedAt"`
}

// Resource returns the authorization resource object for RBAC checks.
func (t *OSTemplate) Resource() *identity.Resource {
	return &identity.Resource{
		Type: "template",
		ID:   t.ID,
	}
}

// Validate checks the structural validity of an OS template.
func (t *OSTemplate) Validate() error {
	t.Name = strings.TrimSpace(t.Name)
	t.Slug = strings.TrimSpace(strings.ToLower(t.Slug))
	t.Distribution = strings.TrimSpace(strings.ToLower(t.Distribution))
	t.Version = strings.TrimSpace(t.Version)

	if t.Name == "" || t.Distribution == "" || t.Version == "" {
		return ErrInvalidTemplateSpec
	}
	if !slugRegex.MatchString(t.Slug) {
		return ErrInvalidTemplateSpec
	}
	if len(t.SupportedArchitectures) == 0 {
		t.SupportedArchitectures = []string{"x86_64"}
	} else {
		for i, a := range t.SupportedArchitectures {
			t.SupportedArchitectures[i] = NormalizeArchitecture(a)
		}
	}
	if len(t.SupportedInstanceTypes) == 0 {
		t.SupportedInstanceTypes = []compute.InstanceType{compute.TypeContainer, compute.TypeVirtualMachine}
	}
	if t.MinDiskBytes <= 0 {
		t.MinDiskBytes = 5 * 1024 * 1024 * 1024 // 5 GB default
	}
	if t.MinMemoryBytes <= 0 {
		t.MinMemoryBytes = 512 * 1024 * 1024 // 512 MB default
	}
	if t.Status == "" {
		t.Status = TemplateStatusActive
	}
	return nil
}

// ImageArtifact represents an actual hypervisor image artifact (Incus fingerprint / image binary).
type ImageArtifact struct {
	ID               string                 `json:"id"`
	TemplateID       string                 `json:"templateId"`
	Architecture     string                 `json:"architecture"`
	InstanceType     compute.InstanceType   `json:"instanceType"`
	IncusFingerprint string                 `json:"incusFingerprint"`
	ImageAlias       string                 `json:"imageAlias"`
	SourceRemote     string                 `json:"sourceRemote"`
	SourceURL        string                 `json:"sourceUrl,omitempty"`
	SizeBytes        int64                  `json:"sizeBytes"`
	Checksum         string                 `json:"checksum"`
	Status           ImageStatus            `json:"status"`
	ErrorMessage     string                 `json:"errorMessage,omitempty"`
	Metadata         map[string]interface{} `json:"metadata"`
	CreatedAt        time.Time              `json:"createdAt"`
	UpdatedAt        time.Time              `json:"updatedAt"`
}

// Resource returns the authorization resource object for RBAC checks.
func (img *ImageArtifact) Resource() *identity.Resource {
	return &identity.Resource{
		Type: "image",
		ID:   img.ID,
	}
}

// Validate checks the structural validity of an image artifact.
func (img *ImageArtifact) Validate() error {
	img.TemplateID = strings.TrimSpace(img.TemplateID)
	img.Architecture = NormalizeArchitecture(img.Architecture)
	img.IncusFingerprint = strings.TrimSpace(strings.ToLower(img.IncusFingerprint))
	img.Checksum = strings.TrimSpace(strings.ToLower(img.Checksum))

	if img.TemplateID == "" || img.Architecture == "" {
		return ErrInvalidImageSpec
	}
	if img.InstanceType != compute.TypeContainer && img.InstanceType != compute.TypeVirtualMachine {
		return ErrUnsupportedInstanceType
	}
	if img.IncusFingerprint != "" && !hex64Regex.MatchString(img.IncusFingerprint) {
		return ErrInvalidFingerprint
	}
	if img.Checksum != "" && !hex64Regex.MatchString(img.Checksum) {
		return ErrInvalidFingerprint
	}
	if img.SourceRemote == "" {
		img.SourceRemote = "images"
	}
	if img.Status == "" {
		img.Status = ImageStatusAvailable
	}
	return nil
}

// NodeImageAvailability tracks whether an image artifact is present locally on a given hypervisor node.
type NodeImageAvailability struct {
	NodeID       string    `json:"nodeId"`
	ArtifactID   string    `json:"artifactId"`
	Status       string    `json:"status"` // "available", "syncing", "failed"
	LastSyncedAt time.Time `json:"lastSyncedAt"`
}

// TemplateFilter specifies filtering criteria for listing templates.
type TemplateFilter struct {
	Distribution string         `json:"distribution,omitempty"`
	Architecture string         `json:"architecture,omitempty"`
	InstanceType string         `json:"instanceType,omitempty"`
	Status       TemplateStatus `json:"status,omitempty"`
	Search       string         `json:"search,omitempty"`
	Limit        int            `json:"limit"`
	Offset       int            `json:"offset"`
}

// ImageFilter specifies filtering criteria for listing image artifacts.
type ImageFilter struct {
	TemplateID   string      `json:"templateId,omitempty"`
	Architecture string      `json:"architecture,omitempty"`
	InstanceType string      `json:"instanceType,omitempty"`
	Status       ImageStatus `json:"status,omitempty"`
	NodeID       string      `json:"nodeId,omitempty"`
	Limit        int         `json:"limit"`
	Offset       int         `json:"offset"`
}
