package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aurora-vm/aurora/internal/domain/compute"
	"github.com/aurora-vm/aurora/internal/domain/template"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TemplateRepository implements template.TemplateRepository using PostgreSQL.
type TemplateRepository struct {
	pool *pgxpool.Pool
}

func NewTemplateRepository(pool *pgxpool.Pool) *TemplateRepository {
	return &TemplateRepository{pool: pool}
}

func (r *TemplateRepository) Create(ctx context.Context, t *template.OSTemplate) error {
	archJSON, _ := json.Marshal(t.SupportedArchitectures)
	typesJSON, _ := json.Marshal(t.SupportedInstanceTypes)
	metaJSON, _ := json.Marshal(t.Metadata)
	now := time.Now().UTC()

	query := `
		INSERT INTO os_templates (
			id, name, slug, description, distribution, version, release, supported_architectures, supported_instance_types, min_disk_bytes, min_memory_bytes, cloud_init_supported, status, metadata, created_at, updated_at
		) VALUES (
			COALESCE(NULLIF($1, '')::uuid, gen_random_uuid()), $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16
		) RETURNING id, created_at, updated_at;
	`
	err := r.pool.QueryRow(ctx, query,
		t.ID, t.Name, t.Slug, t.Description, t.Distribution, t.Version, t.Release,
		archJSON, typesJSON, t.MinDiskBytes, t.MinMemoryBytes, t.CloudInitSupported, string(t.Status), metaJSON, now, now,
	).Scan(&t.ID, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		if strings.Contains(err.Error(), "idx_os_templates_slug") || strings.Contains(err.Error(), "os_templates_slug_key") {
			return template.ErrTemplateSlugExists
		}
		return err
	}
	return nil
}

func (r *TemplateRepository) GetByID(ctx context.Context, id string) (*template.OSTemplate, error) {
	query := `
		SELECT id, name, slug, description, distribution, version, release, supported_architectures, supported_instance_types, min_disk_bytes, min_memory_bytes, cloud_init_supported, status, metadata, created_at, updated_at
		FROM os_templates WHERE id = $1;
	`
	return r.scanTemplate(r.pool.QueryRow(ctx, query, id))
}

func (r *TemplateRepository) GetBySlug(ctx context.Context, slug string) (*template.OSTemplate, error) {
	query := `
		SELECT id, name, slug, description, distribution, version, release, supported_architectures, supported_instance_types, min_disk_bytes, min_memory_bytes, cloud_init_supported, status, metadata, created_at, updated_at
		FROM os_templates WHERE LOWER(slug) = LOWER($1);
	`
	return r.scanTemplate(r.pool.QueryRow(ctx, query, slug))
}

func (r *TemplateRepository) Update(ctx context.Context, t *template.OSTemplate) error {
	archJSON, _ := json.Marshal(t.SupportedArchitectures)
	typesJSON, _ := json.Marshal(t.SupportedInstanceTypes)
	metaJSON, _ := json.Marshal(t.Metadata)
	now := time.Now().UTC()

	query := `
		UPDATE os_templates SET
			name = $2,
			slug = $3,
			description = $4,
			distribution = $5,
			version = $6,
			release = $7,
			supported_architectures = $8,
			supported_instance_types = $9,
			min_disk_bytes = $10,
			min_memory_bytes = $11,
			cloud_init_supported = $12,
			status = $13,
			metadata = $14,
			updated_at = $15
		WHERE id = $1
		RETURNING updated_at;
	`
	err := r.pool.QueryRow(ctx, query,
		t.ID, t.Name, t.Slug, t.Description, t.Distribution, t.Version, t.Release,
		archJSON, typesJSON, t.MinDiskBytes, t.MinMemoryBytes, t.CloudInitSupported, string(t.Status), metaJSON, now,
	).Scan(&t.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return template.ErrTemplateNotFound
		}
		if strings.Contains(err.Error(), "idx_os_templates_slug") || strings.Contains(err.Error(), "os_templates_slug_key") {
			return template.ErrTemplateSlugExists
		}
		return err
	}
	return nil
}

func (r *TemplateRepository) Delete(ctx context.Context, id string) error {
	res, err := r.pool.Exec(ctx, `DELETE FROM os_templates WHERE id = $1;`, id)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return template.ErrTemplateNotFound
	}
	return nil
}

func (r *TemplateRepository) List(ctx context.Context, filter template.TemplateFilter) ([]*template.OSTemplate, int64, error) {
	var whereClauses []string
	var args []interface{}
	idx := 1

	if filter.Distribution != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("LOWER(distribution) = LOWER($%d)", idx))
		args = append(args, filter.Distribution)
		idx++
	}
	if filter.Status != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("status = $%d", idx))
		args = append(args, string(filter.Status))
		idx++
	}
	if filter.Architecture != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("supported_architectures @> $%d::jsonb", idx))
		args = append(args, fmt.Sprintf("[\"%s\"]", filter.Architecture))
		idx++
	}
	if filter.InstanceType != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("supported_instance_types @> $%d::jsonb", idx))
		args = append(args, fmt.Sprintf("[\"%s\"]", filter.InstanceType))
		idx++
	}
	if filter.Search != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("(name ILIKE $%d OR slug ILIKE $%d OR description ILIKE $%d)", idx, idx, idx))
		args = append(args, "%"+filter.Search+"%")
		idx++
	}

	whereSQL := ""
	if len(whereClauses) > 0 {
		whereSQL = "WHERE " + strings.Join(whereClauses, " AND ")
	}

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM os_templates %s", whereSQL)
	var total int64
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}

	query := fmt.Sprintf(`
		SELECT id, name, slug, description, distribution, version, release, supported_architectures, supported_instance_types, min_disk_bytes, min_memory_bytes, cloud_init_supported, status, metadata, created_at, updated_at
		FROM os_templates
		%s
		ORDER BY name ASC
		LIMIT $%d OFFSET $%d;
	`, whereSQL, idx, idx+1)

	args = append(args, limit, filter.Offset)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var templates []*template.OSTemplate
	for rows.Next() {
		t, err := r.scanTemplateRow(rows)
		if err != nil {
			return nil, 0, err
		}
		templates = append(templates, t)
	}

	return templates, total, rows.Err()
}

func (r *TemplateRepository) scanTemplate(row pgx.Row) (*template.OSTemplate, error) {
	var t template.OSTemplate
	var archJSON, typesJSON, metaJSON []byte
	var statusStr string

	err := row.Scan(
		&t.ID, &t.Name, &t.Slug, &t.Description, &t.Distribution, &t.Version, &t.Release,
		&archJSON, &typesJSON, &t.MinDiskBytes, &t.MinMemoryBytes, &t.CloudInitSupported,
		&statusStr, &metaJSON, &t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, template.ErrTemplateNotFound
		}
		return nil, err
	}

	t.Status = template.TemplateStatus(statusStr)
	_ = json.Unmarshal(archJSON, &t.SupportedArchitectures)
	_ = json.Unmarshal(typesJSON, &t.SupportedInstanceTypes)
	_ = json.Unmarshal(metaJSON, &t.Metadata)
	return &t, nil
}

func (r *TemplateRepository) scanTemplateRow(rows pgx.Rows) (*template.OSTemplate, error) {
	var t template.OSTemplate
	var archJSON, typesJSON, metaJSON []byte
	var statusStr string

	err := rows.Scan(
		&t.ID, &t.Name, &t.Slug, &t.Description, &t.Distribution, &t.Version, &t.Release,
		&archJSON, &typesJSON, &t.MinDiskBytes, &t.MinMemoryBytes, &t.CloudInitSupported,
		&statusStr, &metaJSON, &t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	t.Status = template.TemplateStatus(statusStr)
	_ = json.Unmarshal(archJSON, &t.SupportedArchitectures)
	_ = json.Unmarshal(typesJSON, &t.SupportedInstanceTypes)
	_ = json.Unmarshal(metaJSON, &t.Metadata)
	return &t, nil
}

// ImageRepository implements template.ImageRepository using PostgreSQL.
type ImageRepository struct {
	pool *pgxpool.Pool
}

func NewImageRepository(pool *pgxpool.Pool) *ImageRepository {
	return &ImageRepository{pool: pool}
}

func (r *ImageRepository) Create(ctx context.Context, img *template.ImageArtifact) error {
	metaJSON, _ := json.Marshal(img.Metadata)
	now := time.Now().UTC()

	query := `
		INSERT INTO image_artifacts (
			id, template_id, architecture, instance_type, incus_fingerprint, image_alias, source_remote, source_url, size_bytes, checksum, status, error_message, metadata, created_at, updated_at
		) VALUES (
			COALESCE(NULLIF($1, '')::uuid, gen_random_uuid()), $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15
		) RETURNING id, created_at, updated_at;
	`
	return r.pool.QueryRow(ctx, query,
		img.ID, img.TemplateID, img.Architecture, string(img.InstanceType), img.IncusFingerprint,
		img.ImageAlias, img.SourceRemote, img.SourceURL, img.SizeBytes, img.Checksum, string(img.Status),
		img.ErrorMessage, metaJSON, now, now,
	).Scan(&img.ID, &img.CreatedAt, &img.UpdatedAt)
}

func (r *ImageRepository) GetByID(ctx context.Context, id string) (*template.ImageArtifact, error) {
	query := `
		SELECT id, template_id, architecture, instance_type, incus_fingerprint, image_alias, source_remote, source_url, size_bytes, checksum, status, error_message, metadata, created_at, updated_at
		FROM image_artifacts WHERE id = $1;
	`
	return r.scanImage(r.pool.QueryRow(ctx, query, id))
}

func (r *ImageRepository) GetByFingerprint(ctx context.Context, fingerprint string) (*template.ImageArtifact, error) {
	query := `
		SELECT id, template_id, architecture, instance_type, incus_fingerprint, image_alias, source_remote, source_url, size_bytes, checksum, status, error_message, metadata, created_at, updated_at
		FROM image_artifacts WHERE LOWER(incus_fingerprint) = LOWER($1)
		LIMIT 1;
	`
	return r.scanImage(r.pool.QueryRow(ctx, query, fingerprint))
}

func (r *ImageRepository) ListByTemplate(ctx context.Context, templateID string) ([]*template.ImageArtifact, error) {
	query := `
		SELECT id, template_id, architecture, instance_type, incus_fingerprint, image_alias, source_remote, source_url, size_bytes, checksum, status, error_message, metadata, created_at, updated_at
		FROM image_artifacts WHERE template_id = $1
		ORDER BY architecture, instance_type;
	`
	rows, err := r.pool.Query(ctx, query, templateID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var artifacts []*template.ImageArtifact
	for rows.Next() {
		img, err := r.scanImageRow(rows)
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, img)
	}
	return artifacts, rows.Err()
}

func (r *ImageRepository) FindCompatible(ctx context.Context, templateID, architecture string, instType compute.InstanceType) (*template.ImageArtifact, error) {
	architecture = template.NormalizeArchitecture(architecture)
	query := `
		SELECT id, template_id, architecture, instance_type, incus_fingerprint, image_alias, source_remote, source_url, size_bytes, checksum, status, error_message, metadata, created_at, updated_at
		FROM image_artifacts
		WHERE template_id = $1
		  AND LOWER(architecture) = LOWER($2)
		  AND instance_type = $3
		  AND status = 'available'
		LIMIT 1;
	`
	img, err := r.scanImage(r.pool.QueryRow(ctx, query, templateID, architecture, string(instType)))
	if err != nil {
		if errors.Is(err, template.ErrImageArtifactNotFound) {
			return nil, template.ErrNoCompatibleImage
		}
		return nil, err
	}
	return img, nil
}

func (r *ImageRepository) Update(ctx context.Context, img *template.ImageArtifact) error {
	metaJSON, _ := json.Marshal(img.Metadata)
	now := time.Now().UTC()

	query := `
		UPDATE image_artifacts SET
			architecture = $2,
			instance_type = $3,
			incus_fingerprint = $4,
			image_alias = $5,
			source_remote = $6,
			source_url = $7,
			size_bytes = $8,
			checksum = $9,
			status = $10,
			error_message = $11,
			metadata = $12,
			updated_at = $13
		WHERE id = $1
		RETURNING updated_at;
	`
	err := r.pool.QueryRow(ctx, query,
		img.ID, img.Architecture, string(img.InstanceType), img.IncusFingerprint,
		img.ImageAlias, img.SourceRemote, img.SourceURL, img.SizeBytes, img.Checksum,
		string(img.Status), img.ErrorMessage, metaJSON, now,
	).Scan(&img.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return template.ErrImageArtifactNotFound
		}
		return err
	}
	return nil
}

func (r *ImageRepository) UpdateStatus(ctx context.Context, id string, status template.ImageStatus, errorMsg string) error {
	now := time.Now().UTC()
	query := `
		UPDATE image_artifacts SET status = $2, error_message = $3, updated_at = $4
		WHERE id = $1;
	`
	res, err := r.pool.Exec(ctx, query, id, string(status), errorMsg, now)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return template.ErrImageArtifactNotFound
	}
	return nil
}

func (r *ImageRepository) Delete(ctx context.Context, id string) error {
	res, err := r.pool.Exec(ctx, `DELETE FROM image_artifacts WHERE id = $1;`, id)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return template.ErrImageArtifactNotFound
	}
	return nil
}

func (r *ImageRepository) List(ctx context.Context, filter template.ImageFilter) ([]*template.ImageArtifact, int64, error) {
	var whereClauses []string
	var args []interface{}
	idx := 1

	if filter.TemplateID != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("template_id = $%d", idx))
		args = append(args, filter.TemplateID)
		idx++
	}
	if filter.Architecture != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("LOWER(architecture) = LOWER($%d)", idx))
		args = append(args, filter.Architecture)
		idx++
	}
	if filter.InstanceType != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("instance_type = $%d", idx))
		args = append(args, filter.InstanceType)
		idx++
	}
	if filter.Status != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("status = $%d", idx))
		args = append(args, string(filter.Status))
		idx++
	}

	whereSQL := ""
	if len(whereClauses) > 0 {
		whereSQL = "WHERE " + strings.Join(whereClauses, " AND ")
	}

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM image_artifacts %s", whereSQL)
	var total int64
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}

	query := fmt.Sprintf(`
		SELECT id, template_id, architecture, instance_type, incus_fingerprint, image_alias, source_remote, source_url, size_bytes, checksum, status, error_message, metadata, created_at, updated_at
		FROM image_artifacts
		%s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d;
	`, whereSQL, idx, idx+1)

	args = append(args, limit, filter.Offset)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var artifacts []*template.ImageArtifact
	for rows.Next() {
		img, err := r.scanImageRow(rows)
		if err != nil {
			return nil, 0, err
		}
		artifacts = append(artifacts, img)
	}

	return artifacts, total, rows.Err()
}

func (r *ImageRepository) RecordNodeAvailability(ctx context.Context, nodeID, artifactID, status string) error {
	now := time.Now().UTC()
	query := `
		INSERT INTO node_image_availability (node_id, artifact_id, status, last_synced_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (node_id, artifact_id)
		DO UPDATE SET status = EXCLUDED.status, last_synced_at = EXCLUDED.last_synced_at;
	`
	_, err := r.pool.Exec(ctx, query, nodeID, artifactID, status, now)
	return err
}

func (r *ImageRepository) GetNodeAvailability(ctx context.Context, nodeID, artifactID string) (*template.NodeImageAvailability, error) {
	query := `
		SELECT node_id, artifact_id, status, last_synced_at
		FROM node_image_availability
		WHERE node_id = $1 AND artifact_id = $2;
	`
	var a template.NodeImageAvailability
	err := r.pool.QueryRow(ctx, query, nodeID, artifactID).Scan(&a.NodeID, &a.ArtifactID, &a.Status, &a.LastSyncedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &a, nil
}

func (r *ImageRepository) ListNodeAvailability(ctx context.Context, nodeID string) ([]*template.NodeImageAvailability, error) {
	query := `
		SELECT node_id, artifact_id, status, last_synced_at
		FROM node_image_availability
		WHERE node_id = $1;
	`
	rows, err := r.pool.Query(ctx, query, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*template.NodeImageAvailability
	for rows.Next() {
		var a template.NodeImageAvailability
		if err := rows.Scan(&a.NodeID, &a.ArtifactID, &a.Status, &a.LastSyncedAt); err != nil {
			return nil, err
		}
		list = append(list, &a)
	}
	return list, rows.Err()
}

func (r *ImageRepository) scanImage(row pgx.Row) (*template.ImageArtifact, error) {
	var img template.ImageArtifact
	var typeStr, statusStr string
	var metaJSON []byte

	err := row.Scan(
		&img.ID, &img.TemplateID, &img.Architecture, &typeStr, &img.IncusFingerprint,
		&img.ImageAlias, &img.SourceRemote, &img.SourceURL, &img.SizeBytes, &img.Checksum,
		&statusStr, &img.ErrorMessage, &metaJSON, &img.CreatedAt, &img.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, template.ErrImageArtifactNotFound
		}
		return nil, err
	}

	img.InstanceType = compute.InstanceType(typeStr)
	img.Status = template.ImageStatus(statusStr)
	_ = json.Unmarshal(metaJSON, &img.Metadata)
	return &img, nil
}

func (r *ImageRepository) scanImageRow(rows pgx.Rows) (*template.ImageArtifact, error) {
	var img template.ImageArtifact
	var typeStr, statusStr string
	var metaJSON []byte

	err := rows.Scan(
		&img.ID, &img.TemplateID, &img.Architecture, &typeStr, &img.IncusFingerprint,
		&img.ImageAlias, &img.SourceRemote, &img.SourceURL, &img.SizeBytes, &img.Checksum,
		&statusStr, &img.ErrorMessage, &metaJSON, &img.CreatedAt, &img.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	img.InstanceType = compute.InstanceType(typeStr)
	img.Status = template.ImageStatus(statusStr)
	_ = json.Unmarshal(metaJSON, &img.Metadata)
	return &img, nil
}
