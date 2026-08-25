package account

import (
	"context"
	"fmt"
	"time"

	"github.com/aurora-vm/aurora/internal/domain/audit"
	"github.com/aurora-vm/aurora/internal/domain/identity"
)

// TOTPEnrollmentResult contains secret details returned only during initial 2FA setup.
type TOTPEnrollmentResult struct {
	Secret    string `json:"secret"`
	QRCodeURL string `json:"qrCodeUrl"`
}

// Service manages account security features like 2FA enrollment and credential updates.
type Service struct {
	userRepo    identity.UserRepository
	hasher      identity.PasswordHasher
	protector   identity.SecretProtector
	totpManager identity.TOTPManager
	auditRepo   audit.Repository
}

// NewService creates a new account management service.
func NewService(
	userRepo identity.UserRepository,
	hasher identity.PasswordHasher,
	protector identity.SecretProtector,
	totpManager identity.TOTPManager,
	auditRepo audit.Repository,
) *Service {
	return &Service{
		userRepo:    userRepo,
		hasher:      hasher,
		protector:   protector,
		totpManager: totpManager,
		auditRepo:   auditRepo,
	}
}

// Setup2FA generates a new TOTP secret for the user to scan.
func (s *Service) Setup2FA(ctx context.Context, userID string) (*TOTPEnrollmentResult, error) {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, identity.ErrUserNotFound
	}

	if user.TwoFactorEnabled {
		return nil, identity.ErrTOTPAlreadyEnabled
	}

	secret, qrURL, err := s.totpManager.GenerateSecret(user.Email)
	if err != nil {
		return nil, fmt.Errorf("failed to generate totp secret: %w", err)
	}

	return &TOTPEnrollmentResult{
		Secret:    secret,
		QRCodeURL: qrURL,
	}, nil
}

// Enable2FA validates the first TOTP code, encrypts the secret at rest, and marks 2FA as enabled.
func (s *Service) Enable2FA(ctx context.Context, userID, secret, code string) error {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return identity.ErrUserNotFound
	}

	if user.TwoFactorEnabled {
		return identity.ErrTOTPAlreadyEnabled
	}

	if !s.totpManager.ValidateCode(secret, code) {
		return identity.ErrTOTPInvalid
	}

	// Encrypt the TOTP secret at rest with SecretProtector
	encryptedSecret, err := s.protector.Encrypt(ctx, []byte(secret))
	if err != nil {
		return fmt.Errorf("failed to encrypt totp secret: %w", err)
	}

	if err := s.userRepo.Update2FA(ctx, userID, true, string(encryptedSecret)); err != nil {
		return fmt.Errorf("failed to update user 2fa status: %w", err)
	}

	_ = s.auditRepo.Record(ctx, &audit.AuditLog{
		ActorID:      &userID,
		Action:       "auth.2fa.enabled",
		ResourceType: "user",
		ResourceID:   &userID,
		StatusCode:   200,
		CreatedAt:    time.Now().UTC(),
	})

	return nil
}

// Disable2FA validates the user's password and code, then clears 2FA configuration.
func (s *Service) Disable2FA(ctx context.Context, userID, password, code string) error {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return identity.ErrUserNotFound
	}

	if !user.TwoFactorEnabled {
		return identity.ErrTOTPNotEnabled
	}

	// Verify password
	valid, err := s.hasher.Verify(password, user.PasswordHash)
	if err != nil || !valid {
		return identity.ErrInvalidCredentials
	}

	// Decrypt and verify code
	decryptedSecret, err := s.protector.Decrypt(ctx, []byte(user.TwoFactorSecretEnc))
	if err != nil {
		return fmt.Errorf("failed to decrypt totp secret: %w", err)
	}

	if !s.totpManager.ValidateCode(string(decryptedSecret), code) {
		return identity.ErrTOTPInvalid
	}

	if err := s.userRepo.Update2FA(ctx, userID, false, ""); err != nil {
		return fmt.Errorf("failed to disable user 2fa: %w", err)
	}

	_ = s.auditRepo.Record(ctx, &audit.AuditLog{
		ActorID:      &userID,
		Action:       "auth.2fa.disabled",
		ResourceType: "user",
		ResourceID:   &userID,
		StatusCode:   200,
		CreatedAt:    time.Now().UTC(),
	})

	return nil
}
