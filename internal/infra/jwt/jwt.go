package jwt

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/aurora-vm/aurora/internal/domain/identity"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// Claims represents the JWT claims payload for Aurora access tokens.
type Claims struct {
	UserID      string   `json:"uid"`
	Username    string   `json:"usr"`
	Email       string   `json:"eml"`
	Roles       []string `json:"rol"`
	Permissions []string `json:"prm"`
	Scopes      []string `json:"scp,omitempty"`
	TokenType   string   `json:"typ"`
	jwt.RegisteredClaims
}

// TokenManager handles issuing and verifying signed JWT access tokens and refresh tokens.
type TokenManager struct {
	secretKey     []byte
	issuer        string
	accessExpiry  time.Duration
	refreshExpiry time.Duration
}

// NewTokenManager creates a TokenManager configured with standard expiration lifecycles.
func NewTokenManager(secretKey string) (*TokenManager, error) {
	if len(secretKey) < 32 {
		return nil, errors.New("jwt secret key must be at least 32 characters long")
	}
	return &TokenManager{
		secretKey:     []byte(secretKey),
		issuer:        "aurora-control-plane",
		accessExpiry:  15 * time.Minute,
		refreshExpiry: 7 * 24 * time.Hour,
	}, nil
}

// GenerateAccessToken signs a short-lived access token with subject claims.
func (tm *TokenManager) GenerateAccessToken(user *identity.User, roles, permissions []string) (string, error) {
	now := time.Now().UTC()
	claims := Claims{
		UserID:      user.ID,
		Username:    user.Username,
		Email:       user.Email,
		Roles:       roles,
		Permissions: permissions,
		TokenType:   "access",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    tm.issuer,
			Subject:   user.ID,
			Audience:  jwt.ClaimStrings{"aurora-api"},
			ExpiresAt: jwt.NewNumericDate(now.Add(tm.accessExpiry)),
			NotBefore: jwt.NewNumericDate(now),
			IssuedAt:  jwt.NewNumericDate(now),
			ID:        uuid.New().String(),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(tm.secretKey)
	if err != nil {
		return "", fmt.Errorf("failed to sign access token: %w", err)
	}

	return signed, nil
}

// ValidateAccessToken parses and validates a signed access token.
func (tm *TokenManager) ValidateAccessToken(tokenString string) (*identity.Subject, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return tm.secretKey, nil
	}, jwt.WithIssuer(tm.issuer), jwt.WithAudience("aurora-api"))

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, identity.ErrTokenExpired
		}
		return nil, identity.ErrTokenInvalid
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid || claims.TokenType != "access" {
		return nil, identity.ErrTokenInvalid
	}

	return &identity.Subject{
		Type:        "user",
		ID:          claims.UserID,
		UserID:      claims.UserID,
		Username:    claims.Username,
		Email:       claims.Email,
		Roles:       claims.Roles,
		Permissions: claims.Permissions,
		Scopes:      claims.Scopes,
	}, nil
}

// GenerateRefreshToken generates a secure random refresh token string and its SHA-256 hash.
func (tm *TokenManager) GenerateRefreshToken() (string, string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", fmt.Errorf("failed to generate random token: %w", err)
	}

	plaintext := base64.RawURLEncoding.EncodeToString(b)
	hash := tm.HashRefreshToken(plaintext)

	return plaintext, hash, nil
}

// HashRefreshToken calculates the deterministic SHA-256 hash of a plaintext token.
func (tm *TokenManager) HashRefreshToken(plaintextToken string) string {
	h := sha256.Sum256([]byte(plaintextToken))
	return hex.EncodeToString(h[:])
}
