package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/aurora-vm/aurora/internal/domain/identity"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SessionRepository implements identity.SessionRepository using PostgreSQL.
type SessionRepository struct {
	pool *pgxpool.Pool
}

// NewSessionRepository creates a new PostgreSQL session repository.
func NewSessionRepository(pool *pgxpool.Pool) *SessionRepository {
	return &SessionRepository{pool: pool}
}

func (r *SessionRepository) Create(ctx context.Context, s *identity.RefreshSession) error {
	query := `
	INSERT INTO refresh_sessions (id, user_id, token_hash, family_id, user_agent, ip_address, expires_at, created_at, is_revoked)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);
	`
	_, err := r.pool.Exec(ctx, query,
		s.ID, s.UserID, s.TokenHash, s.FamilyID, s.UserAgent, s.IPAddress, s.ExpiresAt, s.CreatedAt, s.IsRevoked,
	)
	return err
}

func (r *SessionRepository) GetByTokenHash(ctx context.Context, tokenHash string) (*identity.RefreshSession, error) {
	query := `
	SELECT id, user_id, token_hash, family_id, user_agent, ip_address, expires_at, created_at, is_revoked, revoked_at, replaced_by_token_id
	FROM refresh_sessions WHERE token_hash = $1;
	`
	var s identity.RefreshSession
	err := r.pool.QueryRow(ctx, query, tokenHash).Scan(
		&s.ID, &s.UserID, &s.TokenHash, &s.FamilyID, &s.UserAgent, &s.IPAddress,
		&s.ExpiresAt, &s.CreatedAt, &s.IsRevoked, &s.RevokedAt, &s.ReplacedByTokenID,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, identity.ErrRefreshTokenInvalid
		}
		return nil, err
	}
	return &s, nil
}

func (r *SessionRepository) Revoke(ctx context.Context, id string, replacedByID *string) error {
	now := time.Now().UTC()
	query := `UPDATE refresh_sessions SET is_revoked = TRUE, revoked_at = $1, replaced_by_token_id = $2 WHERE id = $3;`
	_, err := r.pool.Exec(ctx, query, now, replacedByID, id)
	return err
}

func (r *SessionRepository) RevokeFamily(ctx context.Context, familyID string) error {
	now := time.Now().UTC()
	query := `UPDATE refresh_sessions SET is_revoked = TRUE, revoked_at = $1 WHERE family_id = $2;`
	_, err := r.pool.Exec(ctx, query, now, familyID)
	return err
}

func (r *SessionRepository) RevokeAllForUser(ctx context.Context, userID string) error {
	now := time.Now().UTC()
	query := `UPDATE refresh_sessions SET is_revoked = TRUE, revoked_at = $1 WHERE user_id = $2;`
	_, err := r.pool.Exec(ctx, query, now, userID)
	return err
}
