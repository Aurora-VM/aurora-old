package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/aurora-vm/aurora/internal/domain/identity"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// APIKeyRepository implements identity.APIKeyRepository using PostgreSQL.
type APIKeyRepository struct {
	pool *pgxpool.Pool
}

// NewAPIKeyRepository creates a new PostgreSQL APIKey repository.
func NewAPIKeyRepository(pool *pgxpool.Pool) *APIKeyRepository {
	return &APIKeyRepository{pool: pool}
}

func (r *APIKeyRepository) Create(ctx context.Context, k *identity.APIKey) error {
	scopesJSON, _ := json.Marshal(k.Scopes)
	query := `
	INSERT INTO api_keys (id, user_id, name, key_hash, prefix, scopes, expires_at, created_at)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8);
	`
	_, err := r.pool.Exec(ctx, query, k.ID, k.UserID, k.Name, k.KeyHash, k.Prefix, scopesJSON, k.ExpiresAt, k.CreatedAt)
	return err
}

func (r *APIKeyRepository) GetByID(ctx context.Context, id string) (*identity.APIKey, error) {
	query := `SELECT id, user_id, name, key_hash, prefix, scopes, last_used_at, expires_at, created_at FROM api_keys WHERE id = $1;`
	return r.scanKey(r.pool.QueryRow(ctx, query, id))
}

func (r *APIKeyRepository) GetByKeyHash(ctx context.Context, keyHash string) (*identity.APIKey, error) {
	query := `SELECT id, user_id, name, key_hash, prefix, scopes, last_used_at, expires_at, created_at FROM api_keys WHERE key_hash = $1;`
	return r.scanKey(r.pool.QueryRow(ctx, query, keyHash))
}

func (r *APIKeyRepository) ListByUser(ctx context.Context, userID string) ([]*identity.APIKey, error) {
	query := `SELECT id, user_id, name, key_hash, prefix, scopes, last_used_at, expires_at, created_at FROM api_keys WHERE user_id = $1 ORDER BY created_at DESC;`
	rows, err := r.pool.Query(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []*identity.APIKey
	for rows.Next() {
		k, err := r.scanKey(rows)
		if err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	return keys, nil
}

func (r *APIKeyRepository) UpdateLastUsed(ctx context.Context, id string) error {
	query := `UPDATE api_keys SET last_used_at = $1 WHERE id = $2;`
	_, err := r.pool.Exec(ctx, query, time.Now().UTC(), id)
	return err
}

func (r *APIKeyRepository) Revoke(ctx context.Context, id string) error {
	query := `DELETE FROM api_keys WHERE id = $1;`
	_, err := r.pool.Exec(ctx, query, id)
	return err
}

func (r *APIKeyRepository) scanKey(row pgx.Row) (*identity.APIKey, error) {
	var k identity.APIKey
	var scopesJSON []byte

	err := row.Scan(&k.ID, &k.UserID, &k.Name, &k.KeyHash, &k.Prefix, &scopesJSON, &k.LastUsedAt, &k.ExpiresAt, &k.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, identity.ErrAPIKeyNotFound
		}
		return nil, err
	}

	if len(scopesJSON) > 0 {
		_ = json.Unmarshal(scopesJSON, &k.Scopes)
	}

	return &k, nil
}
