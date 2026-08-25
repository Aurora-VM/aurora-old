package memory

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/aurora-vm/aurora/internal/domain/compute"
	"github.com/aurora-vm/aurora/internal/domain/template"
	"github.com/google/uuid"
)

// MemoryTemplateRepo implements template.TemplateRepository in memory.
type MemoryTemplateRepo struct {
	mu        sync.RWMutex
	templates map[string]*template.OSTemplate
}

func NewMemoryTemplateRepo() *MemoryTemplateRepo {
	r := &MemoryTemplateRepo{
		templates: make(map[string]*template.OSTemplate),
	}
	r.seedDefaults()
	return r
}

func (r *MemoryTemplateRepo) seedDefaults() {
	now := time.Now().UTC()
	defaults := []*template.OSTemplate{
		{
			ID:                     "tmpl-ubuntu-24-04",
			Name:                   "Ubuntu 24.04 LTS (Noble Numbat)",
			Slug:                   "ubuntu-24.04",
			Description:            "Canonical Ubuntu Server 24.04 LTS official cloud release",
			Distribution:           "ubuntu",
			Version:                "24.04",
			Release:                "noble",
			SupportedArchitectures: []string{"x86_64", "aarch64"},
			SupportedInstanceTypes: []compute.InstanceType{compute.TypeContainer, compute.TypeVirtualMachine},
			MinDiskBytes:           5 * 1024 * 1024 * 1024,
			MinMemoryBytes:         512 * 1024 * 1024,
			CloudInitSupported:     true,
			Status:                 template.TemplateStatusActive,
			Metadata:               map[string]interface{}{"official": true},
			CreatedAt:              now,
			UpdatedAt:              now,
		},
		{
			ID:                     "tmpl-debian-12",
			Name:                   "Debian 12 (Bookworm)",
			Slug:                   "debian-12",
			Description:            "Debian GNU/Linux 12 Bookworm stable release",
			Distribution:           "debian",
			Version:                "12",
			Release:                "bookworm",
			SupportedArchitectures: []string{"x86_64", "aarch64"},
			SupportedInstanceTypes: []compute.InstanceType{compute.TypeContainer, compute.TypeVirtualMachine},
			MinDiskBytes:           3 * 1024 * 1024 * 1024,
			MinMemoryBytes:         256 * 1024 * 1024,
			CloudInitSupported:     true,
			Status:                 template.TemplateStatusActive,
			Metadata:               map[string]interface{}{"official": true},
			CreatedAt:              now,
			UpdatedAt:              now,
		},
		{
			ID:                     "tmpl-alpine-3-19",
			Name:                   "Alpine Linux 3.19",
			Slug:                   "alpine-3.19",
			Description:            "Lightweight, security-oriented container and VM distribution",
			Distribution:           "alpine",
			Version:                "3.19",
			Release:                "standard",
			SupportedArchitectures: []string{"x86_64", "aarch64"},
			SupportedInstanceTypes: []compute.InstanceType{compute.TypeContainer},
			MinDiskBytes:           1 * 1024 * 1024 * 1024,
			MinMemoryBytes:         128 * 1024 * 1024,
			CloudInitSupported:     true,
			Status:                 template.TemplateStatusActive,
			Metadata:               map[string]interface{}{"official": true},
			CreatedAt:              now,
			UpdatedAt:              now,
		},
	}

	for _, t := range defaults {
		r.templates[t.ID] = t
	}
}

func (r *MemoryTemplateRepo) Create(ctx context.Context, t *template.OSTemplate) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, existing := range r.templates {
		if existing.Slug == t.Slug {
			return template.ErrTemplateSlugExists
		}
	}

	if t.ID == "" {
		t.ID = uuid.New().String()
	}
	if t.CreatedAt.IsZero() {
		t.CreatedAt = time.Now().UTC()
	}
	t.UpdatedAt = time.Now().UTC()

	cp := *t
	r.templates[t.ID] = &cp
	return nil
}

func (r *MemoryTemplateRepo) GetByID(ctx context.Context, id string) (*template.OSTemplate, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	t, ok := r.templates[id]
	if !ok {
		return nil, template.ErrTemplateNotFound
	}
	cp := *t
	return &cp, nil
}

func (r *MemoryTemplateRepo) GetBySlug(ctx context.Context, slug string) (*template.OSTemplate, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, t := range r.templates {
		if strings.EqualFold(t.Slug, slug) {
			cp := *t
			return &cp, nil
		}
	}
	return nil, template.ErrTemplateNotFound
}

func (r *MemoryTemplateRepo) Update(ctx context.Context, t *template.OSTemplate) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	existing, ok := r.templates[t.ID]
	if !ok {
		return template.ErrTemplateNotFound
	}

	for _, other := range r.templates {
		if other.ID != t.ID && strings.EqualFold(other.Slug, t.Slug) {
			return template.ErrTemplateSlugExists
		}
	}

	t.CreatedAt = existing.CreatedAt
	t.UpdatedAt = time.Now().UTC()
	cp := *t
	r.templates[t.ID] = &cp
	return nil
}

func (r *MemoryTemplateRepo) Delete(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.templates[id]; !ok {
		return template.ErrTemplateNotFound
	}
	delete(r.templates, id)
	return nil
}

func (r *MemoryTemplateRepo) List(ctx context.Context, filter template.TemplateFilter) ([]*template.OSTemplate, int64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var matching []*template.OSTemplate
	for _, t := range r.templates {
		if filter.Distribution != "" && !strings.EqualFold(t.Distribution, filter.Distribution) {
			continue
		}
		if filter.Status != "" && t.Status != filter.Status {
			continue
		}
		if filter.Architecture != "" {
			hasArch := false
			for _, a := range t.SupportedArchitectures {
				if strings.EqualFold(a, filter.Architecture) {
					hasArch = true
					break
				}
			}
			if !hasArch {
				continue
			}
		}
		if filter.InstanceType != "" {
			hasType := false
			for _, typ := range t.SupportedInstanceTypes {
				if string(typ) == filter.InstanceType {
					hasType = true
					break
				}
			}
			if !hasType {
				continue
			}
		}
		if filter.Search != "" {
			s := strings.ToLower(filter.Search)
			if !strings.Contains(strings.ToLower(t.Name), s) &&
				!strings.Contains(strings.ToLower(t.Slug), s) &&
				!strings.Contains(strings.ToLower(t.Description), s) {
				continue
			}
		}
		cp := *t
		matching = append(matching, &cp)
	}

	total := int64(len(matching))
	if filter.Offset >= len(matching) {
		return []*template.OSTemplate{}, total, nil
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	end := filter.Offset + limit
	if end > len(matching) {
		end = len(matching)
	}

	return matching[filter.Offset:end], total, nil
}

// MemoryImageRepo implements template.ImageRepository in memory.
type MemoryImageRepo struct {
	mu           sync.RWMutex
	artifacts    map[string]*template.ImageArtifact
	availability map[string]*template.NodeImageAvailability // key: nodeID:artifactID
}

func NewMemoryImageRepo() *MemoryImageRepo {
	r := &MemoryImageRepo{
		artifacts:    make(map[string]*template.ImageArtifact),
		availability: make(map[string]*template.NodeImageAvailability),
	}
	r.seedDefaults()
	return r
}

func (r *MemoryImageRepo) seedDefaults() {
	now := time.Now().UTC()
	defaults := []*template.ImageArtifact{
		{
			ID:               "img-ubuntu-24-04-x86-c",
			TemplateID:       "tmpl-ubuntu-24-04",
			Architecture:     "x86_64",
			InstanceType:     compute.TypeContainer,
			IncusFingerprint: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
			ImageAlias:       "images:ubuntu/24.04",
			SourceRemote:     "images",
			SizeBytes:        350 * 1024 * 1024,
			Checksum:         "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
			Status:           template.ImageStatusAvailable,
			CreatedAt:        now,
			UpdatedAt:        now,
		},
		{
			ID:               "img-ubuntu-24-04-x86-vm",
			TemplateID:       "tmpl-ubuntu-24-04",
			Architecture:     "x86_64",
			InstanceType:     compute.TypeVirtualMachine,
			IncusFingerprint: "ca978112ca1bbdcaf064278e4a1f2f0dd128ab44929197d026900f9774b5c2b4",
			ImageAlias:       "images:ubuntu/24.04/cloud",
			SourceRemote:     "images",
			SizeBytes:        1200 * 1024 * 1024,
			Checksum:         "ca978112ca1bbdcaf064278e4a1f2f0dd128ab44929197d026900f9774b5c2b4",
			Status:           template.ImageStatusAvailable,
			CreatedAt:        now,
			UpdatedAt:        now,
		},
		{
			ID:               "img-alpine-3-19-x86-c",
			TemplateID:       "tmpl-alpine-3-19",
			Architecture:     "x86_64",
			InstanceType:     compute.TypeContainer,
			IncusFingerprint: "4b227777d4dd1fc61c6f884f48641d02b4d121d3fd328cb08b5531fcacdabf8a",
			ImageAlias:       "images:alpine/3.19",
			SourceRemote:     "images",
			SizeBytes:        45 * 1024 * 1024,
			Checksum:         "4b227777d4dd1fc61c6f884f48641d02b4d121d3fd328cb08b5531fcacdabf8a",
			Status:           template.ImageStatusAvailable,
			CreatedAt:        now,
			UpdatedAt:        now,
		},
	}

	for _, a := range defaults {
		r.artifacts[a.ID] = a
	}
}

func (r *MemoryImageRepo) Create(ctx context.Context, img *template.ImageArtifact) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if img.ID == "" {
		img.ID = uuid.New().String()
	}
	if img.CreatedAt.IsZero() {
		img.CreatedAt = time.Now().UTC()
	}
	img.UpdatedAt = time.Now().UTC()

	cp := *img
	r.artifacts[img.ID] = &cp
	return nil
}

func (r *MemoryImageRepo) GetByID(ctx context.Context, id string) (*template.ImageArtifact, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	img, ok := r.artifacts[id]
	if !ok {
		return nil, template.ErrImageArtifactNotFound
	}
	cp := *img
	return &cp, nil
}

func (r *MemoryImageRepo) GetByFingerprint(ctx context.Context, fingerprint string) (*template.ImageArtifact, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, img := range r.artifacts {
		if strings.EqualFold(img.IncusFingerprint, fingerprint) {
			cp := *img
			return &cp, nil
		}
	}
	return nil, template.ErrImageArtifactNotFound
}

func (r *MemoryImageRepo) ListByTemplate(ctx context.Context, templateID string) ([]*template.ImageArtifact, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*template.ImageArtifact
	for _, img := range r.artifacts {
		if img.TemplateID == templateID {
			cp := *img
			result = append(result, &cp)
		}
	}
	return result, nil
}

func (r *MemoryImageRepo) FindCompatible(ctx context.Context, templateID, architecture string, instType compute.InstanceType) (*template.ImageArtifact, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	normArch := template.NormalizeArchitecture(architecture)
	for _, img := range r.artifacts {
		if img.TemplateID == templateID &&
			template.NormalizeArchitecture(img.Architecture) == normArch &&
			img.InstanceType == instType &&
			img.Status == template.ImageStatusAvailable {
			cp := *img
			return &cp, nil
		}
	}
	return nil, template.ErrNoCompatibleImage
}

func (r *MemoryImageRepo) Update(ctx context.Context, img *template.ImageArtifact) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	existing, ok := r.artifacts[img.ID]
	if !ok {
		return template.ErrImageArtifactNotFound
	}

	img.CreatedAt = existing.CreatedAt
	img.UpdatedAt = time.Now().UTC()
	cp := *img
	r.artifacts[img.ID] = &cp
	return nil
}

func (r *MemoryImageRepo) UpdateStatus(ctx context.Context, id string, status template.ImageStatus, errorMsg string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	img, ok := r.artifacts[id]
	if !ok {
		return template.ErrImageArtifactNotFound
	}
	img.Status = status
	img.ErrorMessage = errorMsg
	img.UpdatedAt = time.Now().UTC()
	return nil
}

func (r *MemoryImageRepo) Delete(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.artifacts[id]; !ok {
		return template.ErrImageArtifactNotFound
	}
	delete(r.artifacts, id)
	return nil
}

func (r *MemoryImageRepo) List(ctx context.Context, filter template.ImageFilter) ([]*template.ImageArtifact, int64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var matching []*template.ImageArtifact
	for _, img := range r.artifacts {
		if filter.TemplateID != "" && img.TemplateID != filter.TemplateID {
			continue
		}
		if filter.Architecture != "" && !strings.EqualFold(img.Architecture, filter.Architecture) {
			continue
		}
		if filter.InstanceType != "" && string(img.InstanceType) != filter.InstanceType {
			continue
		}
		if filter.Status != "" && img.Status != filter.Status {
			continue
		}
		cp := *img
		matching = append(matching, &cp)
	}

	total := int64(len(matching))
	if filter.Offset >= len(matching) {
		return []*template.ImageArtifact{}, total, nil
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	end := filter.Offset + limit
	if end > len(matching) {
		end = len(matching)
	}

	return matching[filter.Offset:end], total, nil
}

func (r *MemoryImageRepo) RecordNodeAvailability(ctx context.Context, nodeID, artifactID, status string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := nodeID + ":" + artifactID
	r.availability[key] = &template.NodeImageAvailability{
		NodeID:       nodeID,
		ArtifactID:   artifactID,
		Status:       status,
		LastSyncedAt: time.Now().UTC(),
	}
	return nil
}

func (r *MemoryImageRepo) GetNodeAvailability(ctx context.Context, nodeID, artifactID string) (*template.NodeImageAvailability, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := nodeID + ":" + artifactID
	avail, ok := r.availability[key]
	if !ok {
		return nil, nil
	}
	cp := *avail
	return &cp, nil
}

func (r *MemoryImageRepo) ListNodeAvailability(ctx context.Context, nodeID string) ([]*template.NodeImageAvailability, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var res []*template.NodeImageAvailability
	for _, a := range r.availability {
		if a.NodeID == nodeID {
			cp := *a
			res = append(res, &cp)
		}
	}
	return res, nil
}
