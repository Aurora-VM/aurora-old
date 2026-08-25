package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/aurora-vm/aurora/internal/domain/identity"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// UserRepository implements identity.UserRepository using PostgreSQL.
type UserRepository struct {
	pool *pgxpool.Pool
}

// NewUserRepository creates a new PostgreSQL user repository.
func NewUserRepository(pool *pgxpool.Pool) *UserRepository {
	return &UserRepository{pool: pool}
}

func (r *UserRepository) Create(ctx context.Context, u *identity.User) error {
	prefsJSON, err := json.Marshal(u.Preferences)
	if err != nil {
		prefsJSON = []byte("{}")
	}

	query := `
	INSERT INTO users (id, username, email, password_hash, is_active, two_factor_secret_enc, two_factor_enabled, preferences, created_at, updated_at)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10);
	`
	_, err = r.pool.Exec(ctx, query,
		u.ID, u.Username, u.Email, u.PasswordHash, u.IsActive, u.TwoFactorSecretEnc, u.TwoFactorEnabled, prefsJSON, u.CreatedAt, u.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to insert user: %w", err)
	}
	return nil
}

func (r *UserRepository) GetByID(ctx context.Context, id string) (*identity.User, error) {
	query := `
	SELECT id, username, email, password_hash, is_active, two_factor_secret_enc, two_factor_enabled, preferences, created_at, updated_at, last_login_at
	FROM users WHERE id = $1;
	`
	return r.scanUser(r.pool.QueryRow(ctx, query, id))
}

func (r *UserRepository) GetByUsername(ctx context.Context, username string) (*identity.User, error) {
	query := `
	SELECT id, username, email, password_hash, is_active, two_factor_secret_enc, two_factor_enabled, preferences, created_at, updated_at, last_login_at
	FROM users WHERE username = $1;
	`
	return r.scanUser(r.pool.QueryRow(ctx, query, username))
}

func (r *UserRepository) GetByEmail(ctx context.Context, email string) (*identity.User, error) {
	query := `
	SELECT id, username, email, password_hash, is_active, two_factor_secret_enc, two_factor_enabled, preferences, created_at, updated_at, last_login_at
	FROM users WHERE email = $1;
	`
	return r.scanUser(r.pool.QueryRow(ctx, query, email))
}

func (r *UserRepository) Update(ctx context.Context, u *identity.User) error {
	prefsJSON, _ := json.Marshal(u.Preferences)
	query := `
	UPDATE users SET username = $1, email = $2, is_active = $3, preferences = $4, updated_at = $5
	WHERE id = $6;
	`
	_, err := r.pool.Exec(ctx, query, u.Username, u.Email, u.IsActive, prefsJSON, time.Now().UTC(), u.ID)
	return err
}

func (r *UserRepository) UpdatePassword(ctx context.Context, id, passwordHash string) error {
	query := `UPDATE users SET password_hash = $1, updated_at = $2 WHERE id = $3;`
	_, err := r.pool.Exec(ctx, query, passwordHash, time.Now().UTC(), id)
	return err
}

func (r *UserRepository) Update2FA(ctx context.Context, id string, enabled bool, secretEnc string) error {
	query := `UPDATE users SET two_factor_enabled = $1, two_factor_secret_enc = $2, updated_at = $3 WHERE id = $4;`
	_, err := r.pool.Exec(ctx, query, enabled, secretEnc, time.Now().UTC(), id)
	return err
}

func (r *UserRepository) UpdateLastLogin(ctx context.Context, id string) error {
	query := `UPDATE users SET last_login_at = $1 WHERE id = $2;`
	_, err := r.pool.Exec(ctx, query, time.Now().UTC(), id)
	return err
}

func (r *UserRepository) Count(ctx context.Context) (int64, error) {
	var count int64
	query := `SELECT COUNT(*) FROM users;`
	err := r.pool.QueryRow(ctx, query).Scan(&count)
	return count, err
}

func (r *UserRepository) scanUser(row pgx.Row) (*identity.User, error) {
	var u identity.User
	var prefsJSON []byte
	var secretEnc *string

	err := row.Scan(
		&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.IsActive,
		&secretEnc, &u.TwoFactorEnabled, &prefsJSON, &u.CreatedAt, &u.UpdatedAt, &u.LastLoginAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, identity.ErrUserNotFound
		}
		return nil, err
	}

	if secretEnc != nil {
		u.TwoFactorSecretEnc = *secretEnc
	}

	if len(prefsJSON) > 0 {
		_ = json.Unmarshal(prefsJSON, &u.Preferences)
	}

	return &u, nil
}
