package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/aurora-vm/aurora/internal/domain/audit"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// AuditRepository implements audit.Repository using PostgreSQL.
type AuditRepository struct {
	pool *pgxpool.Pool
}

// NewAuditRepository creates a new PostgreSQL audit repository.
func NewAuditRepository(pool *pgxpool.Pool) *AuditRepository {
	return &AuditRepository{pool: pool}
}

func (r *AuditRepository) Record(ctx context.Context, a *audit.AuditLog) error {
	if a.CreatedAt.IsZero() {
		a.CreatedAt = time.Now().UTC()
	}
	if a.Severity == "" {
		a.Severity = audit.SeverityInfo
	}

	// Fetch latest log to establish prev_hash
	latest, err := r.GetLatestLog(ctx)
	if err == nil && latest != nil {
		a.PrevHash = latest.TamperProofHash
	}
	a.TamperProofHash = a.ComputeHash()

	detailsJSON, _ := json.Marshal(a.Details)
	query := `
		INSERT INTO audit_logs (
			actor_id, actor_ip, user_agent, action, resource_type, resource_id, request_id, status_code, details, severity, prev_hash, tamper_proof_hash, created_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13
		) RETURNING id;
	`
	return r.pool.QueryRow(ctx, query,
		a.ActorID, a.ActorIP, a.UserAgent, a.Action, a.ResourceType, a.ResourceID, a.RequestID, a.StatusCode, detailsJSON, string(a.Severity), a.PrevHash, a.TamperProofHash, a.CreatedAt,
	).Scan(&a.ID)
}

func (r *AuditRepository) List(ctx context.Context, limit, offset int) ([]*audit.AuditLog, error) {
	logs, _, err := r.ListFiltered(ctx, audit.AuditFilter{Limit: limit, Offset: offset})
	return logs, err
}

func (r *AuditRepository) ListFiltered(ctx context.Context, filter audit.AuditFilter) ([]*audit.AuditLog, int64, error) {
	var whereClauses []string
	var args []interface{}
	idx := 1

	if filter.ActorID != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("actor_id = $%d", idx))
		args = append(args, filter.ActorID)
		idx++
	}
	if filter.Action != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("action = $%d", idx))
		args = append(args, filter.Action)
		idx++
	}
	if filter.ResourceType != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("resource_type = $%d", idx))
		args = append(args, filter.ResourceType)
		idx++
	}
	if filter.ResourceID != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("resource_id = $%d", idx))
		args = append(args, filter.ResourceID)
		idx++
	}
	if filter.Severity != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("severity = $%d", idx))
		args = append(args, string(filter.Severity))
		idx++
	}
	if filter.From != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("created_at >= $%d", idx))
		args = append(args, *filter.From)
		idx++
	}
	if filter.To != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("created_at <= $%d", idx))
		args = append(args, *filter.To)
		idx++
	}

	whereSQL := ""
	if len(whereClauses) > 0 {
		whereSQL = "WHERE " + strings.Join(whereClauses, " AND ")
	}

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM audit_logs %s", whereSQL)
	var total int64
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}

	query := fmt.Sprintf(`
		SELECT id, actor_id, actor_ip, user_agent, action, resource_type, resource_id, request_id, status_code, details, severity, prev_hash, tamper_proof_hash, created_at
		FROM audit_logs
		%s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d
	`, whereSQL, idx, idx+1)

	args = append(args, limit, filter.Offset)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var logs []*audit.AuditLog
	for rows.Next() {
		var a audit.AuditLog
		var sevStr, prevHash, hash string
		var detailsJSON []byte

		err := rows.Scan(
			&a.ID, &a.ActorID, &a.ActorIP, &a.UserAgent, &a.Action, &a.ResourceType, &a.ResourceID,
			&a.RequestID, &a.StatusCode, &detailsJSON, &sevStr, &prevHash, &hash, &a.CreatedAt,
		)
		if err != nil {
			return nil, 0, err
		}
		a.Severity = audit.Severity(sevStr)
		a.PrevHash = prevHash
		a.TamperProofHash = hash
		if len(detailsJSON) > 0 {
			_ = json.Unmarshal(detailsJSON, &a.Details)
		}
		logs = append(logs, &a)
	}

	return logs, total, rows.Err()
}

func (r *AuditRepository) GetLatestLog(ctx context.Context) (*audit.AuditLog, error) {
	query := `
		SELECT id, actor_id, actor_ip, user_agent, action, resource_type, resource_id, request_id, status_code, details, severity, prev_hash, tamper_proof_hash, created_at
		FROM audit_logs
		ORDER BY id DESC
		LIMIT 1;
	`
	row := r.pool.QueryRow(ctx, query)
	var a audit.AuditLog
	var sevStr, prevHash, hash string
	var detailsJSON []byte

	err := row.Scan(
		&a.ID, &a.ActorID, &a.ActorIP, &a.UserAgent, &a.Action, &a.ResourceType, &a.ResourceID,
		&a.RequestID, &a.StatusCode, &detailsJSON, &sevStr, &prevHash, &hash, &a.CreatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	a.Severity = audit.Severity(sevStr)
	a.PrevHash = prevHash
	a.TamperProofHash = hash
	if len(detailsJSON) > 0 {
		_ = json.Unmarshal(detailsJSON, &a.Details)
	}
	return &a, nil
}

func (r *AuditRepository) VerifyChainIntegrity(ctx context.Context, limit int) (bool, int64, error) {
	if limit <= 0 {
		limit = 1000
	}

	query := `
		SELECT id, actor_id, actor_ip, user_agent, action, resource_type, resource_id, request_id, status_code, details, severity, prev_hash, tamper_proof_hash, created_at
		FROM audit_logs
		ORDER BY id ASC
		LIMIT $1;
	`
	rows, err := r.pool.Query(ctx, query, limit)
	if err != nil {
		return false, 0, err
	}
	defer rows.Close()

	var prevHash string
	var verifiedCount int64

	for rows.Next() {
		var a audit.AuditLog
		var sevStr string
		var detailsJSON []byte

		err := rows.Scan(
			&a.ID, &a.ActorID, &a.ActorIP, &a.UserAgent, &a.Action, &a.ResourceType, &a.ResourceID,
			&a.RequestID, &a.StatusCode, &detailsJSON, &sevStr, &a.PrevHash, &a.TamperProofHash, &a.CreatedAt,
		)
		if err != nil {
			return false, verifiedCount, err
		}
		a.Severity = audit.Severity(sevStr)
		if len(detailsJSON) > 0 {
			_ = json.Unmarshal(detailsJSON, &a.Details)
		}

		if !a.VerifyHash() {
			return false, a.ID, nil
		}

		if verifiedCount > 0 && a.PrevHash != prevHash {
			return false, a.ID, nil
		}

		prevHash = a.TamperProofHash
		verifiedCount++
	}

	return true, verifiedCount, rows.Err()
}
