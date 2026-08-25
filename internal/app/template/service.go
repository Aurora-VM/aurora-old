package template

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aurora-vm/aurora/internal/app/node"
	"github.com/aurora-vm/aurora/internal/domain/audit"
	"github.com/aurora-vm/aurora/internal/domain/compute"
	"github.com/aurora-vm/aurora/internal/domain/identity"
	domainNode "github.com/aurora-vm/aurora/internal/domain/node"
	domainTmpl "github.com/aurora-vm/aurora/internal/domain/template"
	"github.com/google/uuid"
)

type CreateTemplateRequest struct {
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
	Metadata               map[string]interface{} `json:"metadata"`
}

type UpdateTemplateRequest struct {
	Name                   string                    `json:"name"`
	Slug                   string                    `json:"slug"`
	Description            string                    `json:"description"`
	Distribution           string                    `json:"distribution"`
	Version                string                    `json:"version"`
	Release                string                    `json:"release"`
	SupportedArchitectures []string                  `json:"supportedArchitectures"`
	SupportedInstanceTypes []compute.InstanceType    `json:"supportedInstanceTypes"`
	MinDiskBytes           int64                     `json:"minDiskBytes"`
	MinMemoryBytes         int64                     `json:"minMemoryBytes"`
	CloudInitSupported     bool                      `json:"cloudInitSupported"`
	Status                 domainTmpl.TemplateStatus `json:"status"`
	Metadata               map[string]interface{}    `json:"metadata"`
}

type RegisterImageRequest struct {
	TemplateID       string                 `json:"templateId"`
	Architecture     string                 `json:"architecture"`
	InstanceType     compute.InstanceType   `json:"instanceType"`
	IncusFingerprint string                 `json:"incusFingerprint"`
	ImageAlias       string                 `json:"imageAlias"`
	SourceRemote     string                 `json:"sourceRemote"`
	SourceURL        string                 `json:"sourceUrl"`
	SizeBytes        int64                  `json:"sizeBytes"`
	Checksum         string                 `json:"checksum"`
	Metadata         map[string]interface{} `json:"metadata"`
}

type SyncImageRequest struct {
	ImageID string `json:"imageId"`
	NodeID  string `json:"nodeId"`
}

// Service coordinates OS template catalog, image artifacts, and asynchronous node synchronization.
type Service struct {
	templateRepo domainTmpl.TemplateRepository
	imageRepo    domainTmpl.ImageRepository
	nodeRepo     domainNode.NodeRepository
	nodeService  *node.Service
	imageSource  domainTmpl.ImageSource
	authorizer   identity.Authorizer
	auditRepo    audit.Repository
}

// NewService constructs a Template & Image Management Application Service.
func NewService(
	templateRepo domainTmpl.TemplateRepository,
	imageRepo domainTmpl.ImageRepository,
	nodeRepo domainNode.NodeRepository,
	nodeService *node.Service,
	imageSource domainTmpl.ImageSource,
	authorizer identity.Authorizer,
	auditRepo audit.Repository,
) *Service {
	return &Service{
		templateRepo: templateRepo,
		imageRepo:    imageRepo,
		nodeRepo:     nodeRepo,
		nodeService:  nodeService,
		imageSource:  imageSource,
		authorizer:   authorizer,
		auditRepo:    auditRepo,
	}
}

// CreateTemplate creates a new product-level OS template definition.
func (s *Service) CreateTemplate(ctx context.Context, sub *identity.Subject, req CreateTemplateRequest) (*domainTmpl.OSTemplate, error) {
	if err := s.authorizer.Authorize(ctx, sub, "template:create", nil); err != nil {
		return nil, err
	}

	t := &domainTmpl.OSTemplate{
		Name:                   req.Name,
		Slug:                   req.Slug,
		Description:            req.Description,
		Distribution:           req.Distribution,
		Version:                req.Version,
		Release:                req.Release,
		SupportedArchitectures: req.SupportedArchitectures,
		SupportedInstanceTypes: req.SupportedInstanceTypes,
		MinDiskBytes:           req.MinDiskBytes,
		MinMemoryBytes:         req.MinMemoryBytes,
		CloudInitSupported:     req.CloudInitSupported,
		Status:                 domainTmpl.TemplateStatusActive,
		Metadata:               req.Metadata,
	}

	if err := t.Validate(); err != nil {
		return nil, err
	}

	if err := s.templateRepo.Create(ctx, t); err != nil {
		return nil, err
	}

	s.logAudit(ctx, sub, "template.created", t.ID, map[string]interface{}{
		"name":         t.Name,
		"slug":         t.Slug,
		"distribution": t.Distribution,
		"version":      t.Version,
	})

	return t, nil
}

// GetTemplate retrieves an OS template by ID or slug.
func (s *Service) GetTemplate(ctx context.Context, sub *identity.Subject, idOrSlug string) (*domainTmpl.OSTemplate, error) {
	if err := s.authorizer.Authorize(ctx, sub, "template:read", nil); err != nil {
		return nil, err
	}

	t, err := s.templateRepo.GetByID(ctx, idOrSlug)
	if err == nil && t != nil {
		return t, nil
	}

	return s.templateRepo.GetBySlug(ctx, idOrSlug)
}

// ListTemplates lists OS templates filtered by architecture, distribution, and status.
func (s *Service) ListTemplates(ctx context.Context, sub *identity.Subject, filter domainTmpl.TemplateFilter) ([]*domainTmpl.OSTemplate, int64, error) {
	if err := s.authorizer.Authorize(ctx, sub, "template:read", nil); err != nil {
		return nil, 0, err
	}

	// If customer, only show active templates by default unless admin
	if !isSuperadminOrOperator(sub) && filter.Status == "" {
		filter.Status = domainTmpl.TemplateStatusActive
	}

	return s.templateRepo.List(ctx, filter)
}

// UpdateTemplate modifies an existing OS template definition.
func (s *Service) UpdateTemplate(ctx context.Context, sub *identity.Subject, id string, req UpdateTemplateRequest) (*domainTmpl.OSTemplate, error) {
	t, err := s.templateRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if err := s.authorizer.Authorize(ctx, sub, "template:update", t.Resource()); err != nil {
		return nil, err
	}

	if req.Name != "" {
		t.Name = req.Name
	}
	if req.Slug != "" {
		t.Slug = req.Slug
	}
	if req.Description != "" {
		t.Description = req.Description
	}
	if req.Distribution != "" {
		t.Distribution = req.Distribution
	}
	if req.Version != "" {
		t.Version = req.Version
	}
	if req.Release != "" {
		t.Release = req.Release
	}
	if len(req.SupportedArchitectures) > 0 {
		t.SupportedArchitectures = req.SupportedArchitectures
	}
	if len(req.SupportedInstanceTypes) > 0 {
		t.SupportedInstanceTypes = req.SupportedInstanceTypes
	}
	if req.MinDiskBytes > 0 {
		t.MinDiskBytes = req.MinDiskBytes
	}
	if req.MinMemoryBytes > 0 {
		t.MinMemoryBytes = req.MinMemoryBytes
	}
	t.CloudInitSupported = req.CloudInitSupported
	if req.Status != "" {
		t.Status = req.Status
	}
	if req.Metadata != nil {
		t.Metadata = req.Metadata
	}

	if err := t.Validate(); err != nil {
		return nil, err
	}

	if err := s.templateRepo.Update(ctx, t); err != nil {
		return nil, err
	}

	s.logAudit(ctx, sub, "template.updated", t.ID, map[string]interface{}{
		"name":   t.Name,
		"slug":   t.Slug,
		"status": string(t.Status),
	})

	return t, nil
}

// DeleteTemplate removes or deprecates an OS template.
func (s *Service) DeleteTemplate(ctx context.Context, sub *identity.Subject, id string) error {
	t, err := s.templateRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	if err := s.authorizer.Authorize(ctx, sub, "template:delete", t.Resource()); err != nil {
		return err
	}

	if err := s.templateRepo.Delete(ctx, id); err != nil {
		return err
	}

	s.logAudit(ctx, sub, "template.deleted", id, map[string]interface{}{
		"name": t.Name,
		"slug": t.Slug,
	})

	return nil
}

// RegisterImage registers an actual Incus image artifact for an OS template.
func (s *Service) RegisterImage(ctx context.Context, sub *identity.Subject, req RegisterImageRequest) (*domainTmpl.ImageArtifact, error) {
	if err := s.authorizer.Authorize(ctx, sub, "image:manage", nil); err != nil {
		return nil, err
	}

	tmpl, err := s.templateRepo.GetByID(ctx, req.TemplateID)
	if err != nil {
		return nil, fmt.Errorf("invalid template ID: %w", err)
	}

	img := &domainTmpl.ImageArtifact{
		TemplateID:       tmpl.ID,
		Architecture:     req.Architecture,
		InstanceType:     req.InstanceType,
		IncusFingerprint: req.IncusFingerprint,
		ImageAlias:       req.ImageAlias,
		SourceRemote:     req.SourceRemote,
		SourceURL:        req.SourceURL,
		SizeBytes:        req.SizeBytes,
		Checksum:         req.Checksum,
		Status:           domainTmpl.ImageStatusAvailable,
		Metadata:         req.Metadata,
	}

	if err := img.Validate(); err != nil {
		return nil, err
	}

	if err := s.imageRepo.Create(ctx, img); err != nil {
		return nil, err
	}

	s.logAudit(ctx, sub, "image.registered", img.ID, map[string]interface{}{
		"templateId":   img.TemplateID,
		"architecture": img.Architecture,
		"instanceType": string(img.InstanceType),
		"fingerprint":  img.IncusFingerprint,
	})

	return img, nil
}

// ListImages lists image artifacts.
func (s *Service) ListImages(ctx context.Context, sub *identity.Subject, filter domainTmpl.ImageFilter) ([]*domainTmpl.ImageArtifact, int64, error) {
	if err := s.authorizer.Authorize(ctx, sub, "image:read", nil); err != nil {
		return nil, 0, err
	}
	return s.imageRepo.List(ctx, filter)
}

// GetImage retrieves an image artifact by ID.
func (s *Service) GetImage(ctx context.Context, sub *identity.Subject, id string) (*domainTmpl.ImageArtifact, error) {
	if err := s.authorizer.Authorize(ctx, sub, "image:read", nil); err != nil {
		return nil, err
	}
	return s.imageRepo.GetByID(ctx, id)
}

// SyncImageToNode asynchronously synchronizes an image artifact to a target hypervisor node.
func (s *Service) SyncImageToNode(ctx context.Context, sub *identity.Subject, req SyncImageRequest) error {
	if err := s.authorizer.Authorize(ctx, sub, "image:manage", nil); err != nil {
		return err
	}

	img, err := s.imageRepo.GetByID(ctx, req.ImageID)
	if err != nil {
		return err
	}

	n, err := s.nodeRepo.GetByID(ctx, req.NodeID)
	if err != nil {
		return err
	}
	if n.Status != domainNode.StatusOnline {
		return domainNode.ErrNodeOffline
	}

	s.logAudit(ctx, sub, "image.sync.started", img.ID, map[string]interface{}{
		"nodeId":      n.ID,
		"fingerprint": img.IncusFingerprint,
		"alias":       img.ImageAlias,
	})

	// Dispatch asynchronous command to node agent
	go func() {
		bgCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()

		cmd := &domainNode.Command{
			CorrelationID: uuid.New().String(),
			Type:          "sync_image",
			Payload: map[string]interface{}{
				"image_id":          img.ID,
				"template_id":       img.TemplateID,
				"incus_fingerprint": img.IncusFingerprint,
				"image_alias":       img.ImageAlias,
				"source_remote":     img.SourceRemote,
				"source_url":        img.SourceURL,
				"architecture":      img.Architecture,
				"instance_type":     string(img.InstanceType),
				"expected_checksum": img.Checksum,
			},
		}

		_ = s.imageRepo.RecordNodeAvailability(bgCtx, n.ID, img.ID, "syncing")

		res, err := s.nodeService.SendCommand(bgCtx, n.ID, cmd)
		if err != nil || !res.Success {
			errMsg := "unknown sync error"
			if err != nil {
				errMsg = err.Error()
			} else if res.ErrorMessage != "" {
				errMsg = res.ErrorMessage
			}
			_ = s.imageRepo.RecordNodeAvailability(bgCtx, n.ID, img.ID, "failed")
			_ = s.imageRepo.UpdateStatus(bgCtx, img.ID, domainTmpl.ImageStatusSyncFailed, errMsg)
			s.logAudit(bgCtx, sub, "image.sync.failed", img.ID, map[string]interface{}{
				"nodeId": n.ID,
				"error":  errMsg,
			})
			return
		}

		_ = s.imageRepo.RecordNodeAvailability(bgCtx, n.ID, img.ID, "available")
		_ = s.imageRepo.UpdateStatus(bgCtx, img.ID, domainTmpl.ImageStatusAvailable, "")
		s.logAudit(bgCtx, sub, "image.sync.completed", img.ID, map[string]interface{}{
			"nodeId":      n.ID,
			"fingerprint": img.IncusFingerprint,
		})
	}()

	return nil
}

// VerifyImage validates the cryptographic integrity of an image artifact.
func (s *Service) VerifyImage(ctx context.Context, sub *identity.Subject, imageID string) (bool, error) {
	if err := s.authorizer.Authorize(ctx, sub, "image:manage", nil); err != nil {
		return false, err
	}

	img, err := s.imageRepo.GetByID(ctx, imageID)
	if err != nil {
		return false, err
	}

	if s.imageSource != nil {
		valid, err := s.imageSource.Verify(ctx, img)
		if err != nil || !valid {
			_ = s.imageRepo.UpdateStatus(ctx, img.ID, domainTmpl.ImageStatusVerificationFailed, "fingerprint verification failed")
			s.logAudit(ctx, sub, "image.verification.failed", img.ID, map[string]interface{}{
				"fingerprint": img.IncusFingerprint,
				"error":       err.Error(),
			})
			return false, err
		}
	}

	return true, nil
}

// RetireImage transitions an image artifact to retired status.
func (s *Service) RetireImage(ctx context.Context, sub *identity.Subject, imageID string) error {
	if err := s.authorizer.Authorize(ctx, sub, "image:manage", nil); err != nil {
		return err
	}

	img, err := s.imageRepo.GetByID(ctx, imageID)
	if err != nil {
		return err
	}

	if err := s.imageRepo.UpdateStatus(ctx, img.ID, domainTmpl.ImageStatusRetired, "retired by administrator"); err != nil {
		return err
	}

	s.logAudit(ctx, sub, "image.retired", img.ID, map[string]interface{}{
		"fingerprint": img.IncusFingerprint,
		"templateId":  img.TemplateID,
	})

	return nil
}

// ValidateCloudInit evaluates the structure, geometry, and safety of a guest cloud-init specification.
func (s *Service) ValidateCloudInit(ctx context.Context, sub *identity.Subject, cfg *domainTmpl.CloudInitConfig) error {
	if cfg == nil {
		return nil
	}
	if err := cfg.Validate(); err != nil {
		s.logAudit(ctx, sub, "cloudinit.validation.failed", "", map[string]interface{}{
			"error": err.Error(),
		})
		return err
	}
	return nil
}

// FindCompatibleImage finds the matching image artifact for a template, architecture, and instance type.
func (s *Service) FindCompatibleImage(ctx context.Context, templateID, architecture string, instType compute.InstanceType) (*domainTmpl.ImageArtifact, error) {
	return s.imageRepo.FindCompatible(ctx, templateID, architecture, instType)
}

func (s *Service) logAudit(ctx context.Context, sub *identity.Subject, action, resourceID string, details map[string]interface{}) {
	if s.auditRepo == nil {
		return
	}
	var actorID *string
	if sub != nil && sub.UserID != "" {
		actorID = &sub.UserID
	}
	var rID *string
	if resourceID != "" {
		rID = &resourceID
	}
	_ = s.auditRepo.Record(ctx, &audit.AuditLog{
		ActorID:      actorID,
		Action:       action,
		ResourceType: "template",
		ResourceID:   rID,
		Details:      details,
		StatusCode:   200,
		Severity:     audit.SeverityInfo,
		CreatedAt:    time.Now().UTC(),
	})
}

func isSuperadminOrOperator(sub *identity.Subject) bool {
	if sub == nil {
		return false
	}
	for _, r := range sub.Roles {
		if r == "superadmin" || r == "operator" {
			return true
		}
	}
	for _, p := range sub.Permissions {
		if p == "*" || strings.HasPrefix(p, "template:") || strings.HasPrefix(p, "image:") {
			return true
		}
	}
	return false
}
