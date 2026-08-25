package auth

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/aurora-vm/aurora/internal/domain/audit"
	"github.com/aurora-vm/aurora/internal/domain/identity"
	"github.com/google/uuid"
)

// RegisterRequest encapsulates user registration parameters.
type RegisterRequest struct {
	Username  string `json:"username"`
	Email     string `json:"email"`
	Password  string `json:"password"`
	IPAddress string `json:"-"`
	UserAgent string `json:"-"`
}

// LoginRequest encapsulates credentials submitted for authentication.
type LoginRequest struct {
	UsernameOrEmail string `json:"usernameOrEmail"`
	Password        string `json:"password"`
	IPAddress       string `json:"-"`
	UserAgent       string `json:"-"`
}

// LoginResult contains either issued authentication tokens or a 2FA challenge.
type LoginResult struct {
	Requires2FA   bool         `json:"requires2fa"`
	ChallengeTemp string       `json:"challengeToken,omitempty"`
	Tokens        *TokenPair   `json:"tokens,omitempty"`
	User          *UserSummary `json:"user,omitempty"`
}

// TokenPair contains short-lived access token and rotatable refresh token.
type TokenPair struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	TokenType    string `json:"tokenType"`
	ExpiresIn    int64  `json:"expiresIn"` // In seconds (900 for 15m)
}

// UserSummary provides non-sensitive user metadata for client state.
type UserSummary struct {
	ID               string   `json:"id"`
	Username         string   `json:"username"`
	Email            string   `json:"email"`
	Roles            []string `json:"roles"`
	Permissions      []string `json:"permissions"`
	TwoFactorEnabled bool     `json:"twoFactorEnabled"`
}

// Service orchestrates authentication and session management.
type Service struct {
	userRepo     identity.UserRepository
	roleRepo     identity.RoleRepository
	sessionRepo  identity.SessionRepository
	hasher       identity.PasswordHasher
	protector    identity.SecretProtector
	tokenManager identity.TokenManager
	totpManager  identity.TOTPManager
	auditRepo    audit.Repository
	mu           sync.Mutex
}

// NewService constructs an authentication application service.
func NewService(
	userRepo identity.UserRepository,
	roleRepo identity.RoleRepository,
	sessionRepo identity.SessionRepository,
	hasher identity.PasswordHasher,
	protector identity.SecretProtector,
	tokenManager identity.TokenManager,
	totpManager identity.TOTPManager,
	auditRepo audit.Repository,
) *Service {
	return &Service{
		userRepo:     userRepo,
		roleRepo:     roleRepo,
		sessionRepo:  sessionRepo,
		hasher:       hasher,
		protector:    protector,
		tokenManager: tokenManager,
		totpManager:  totpManager,
		auditRepo:    auditRepo,
	}
}

// Register creates a new user account. The first registered user automatically becomes superadmin.
func (s *Service) Register(ctx context.Context, req RegisterRequest) (*identity.User, error) {
	if len(req.Password) < 8 {
		return nil, errors.New("password must be at least 8 characters long")
	}

	hash, err := s.hasher.Hash(req.Password)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	userCount, _ := s.userRepo.Count(ctx)
	isFirstUser := userCount == 0

	user := &identity.User{
		ID:           uuid.New().String(),
		Username:     req.Username,
		Email:        req.Email,
		PasswordHash: hash,
		IsActive:     true,
		Preferences:  make(map[string]interface{}),
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}

	if err := s.userRepo.Create(ctx, user); err != nil {
		return nil, err
	}

	// Assign role
	roleName := "customer"
	if isFirstUser {
		roleName = "superadmin"
	}

	role, err := s.roleRepo.GetByName(ctx, roleName)
	if err == nil && role != nil {
		_ = s.roleRepo.AssignRoleToUser(ctx, &identity.UserRoleGrant{
			ID:        uuid.New().String(),
			UserID:    user.ID,
			RoleID:    role.ID,
			RoleName:  role.Name,
			ScopeType: "global",
			CreatedAt: time.Now().UTC(),
		})
	}

	_ = s.auditRepo.Record(ctx, &audit.AuditLog{
		ActorID:      &user.ID,
		ActorIP:      req.IPAddress,
		UserAgent:    req.UserAgent,
		Action:       "auth.register",
		ResourceType: "user",
		ResourceID:   &user.ID,
		StatusCode:   201,
		Details:      map[string]interface{}{"username": user.Username, "role": roleName},
		CreatedAt:    time.Now().UTC(),
	})

	return user, nil
}

// Login verifies credentials and either issues tokens or requires a 2FA challenge.
func (s *Service) Login(ctx context.Context, req LoginRequest) (*LoginResult, error) {
	var user *identity.User
	var err error

	user, err = s.userRepo.GetByUsername(ctx, req.UsernameOrEmail)
	if err != nil {
		user, err = s.userRepo.GetByEmail(ctx, req.UsernameOrEmail)
	}
	if err != nil {
		_ = s.auditRepo.Record(ctx, &audit.AuditLog{
			ActorIP:    req.IPAddress,
			UserAgent:  req.UserAgent,
			Action:     "auth.login.failed",
			StatusCode: 401,
			Details:    map[string]interface{}{"identifier": req.UsernameOrEmail, "reason": "user_not_found"},
			CreatedAt:  time.Now().UTC(),
		})
		return nil, identity.ErrInvalidCredentials
	}

	if !user.IsActive {
		return nil, identity.ErrAccountDisabled
	}

	valid, err := s.hasher.Verify(req.Password, user.PasswordHash)
	if err != nil || !valid {
		_ = s.auditRepo.Record(ctx, &audit.AuditLog{
			ActorID:    &user.ID,
			ActorIP:    req.IPAddress,
			UserAgent:  req.UserAgent,
			Action:     "auth.login.failed",
			StatusCode: 401,
			Details:    map[string]interface{}{"reason": "invalid_password"},
			CreatedAt:  time.Now().UTC(),
		})
		return nil, identity.ErrInvalidCredentials
	}

	// If 2FA is enabled, issue a temporary challenge token
	if user.TwoFactorEnabled {
		challengeToken, err := s.tokenManager.GenerateAccessToken(user, []string{"2fa_challenge"}, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to generate 2fa challenge token: %w", err)
		}
		return &LoginResult{
			Requires2FA:   true,
			ChallengeTemp: challengeToken,
		}, nil
	}

	tokens, err := s.issueTokensAndRecordSession(ctx, user, req.IPAddress, req.UserAgent, uuid.New().String())
	if err != nil {
		return nil, err
	}

	roles, perms, err := s.getUserRolesAndPermissions(ctx, user.ID)
	if err != nil {
		return nil, err
	}

	_ = s.userRepo.UpdateLastLogin(ctx, user.ID)
	_ = s.auditRepo.Record(ctx, &audit.AuditLog{
		ActorID:      &user.ID,
		ActorIP:      req.IPAddress,
		UserAgent:    req.UserAgent,
		Action:       "auth.login.success",
		ResourceType: "user",
		ResourceID:   &user.ID,
		StatusCode:   200,
		CreatedAt:    time.Now().UTC(),
	})

	return &LoginResult{
		Requires2FA: false,
		Tokens:      tokens,
		User: &UserSummary{
			ID:               user.ID,
			Username:         user.Username,
			Email:            user.Email,
			Roles:            roles,
			Permissions:      perms,
			TwoFactorEnabled: user.TwoFactorEnabled,
		},
	}, nil
}

// VerifyTOTP completes authentication when 2FA is enabled.
func (s *Service) VerifyTOTP(ctx context.Context, challengeToken, code, ipAddress, userAgent string) (*LoginResult, error) {
	subject, err := s.tokenManager.ValidateAccessToken(challengeToken)
	if err != nil {
		return nil, identity.ErrTokenInvalid
	}

	user, err := s.userRepo.GetByID(ctx, subject.UserID)
	if err != nil {
		return nil, identity.ErrUserNotFound
	}

	if !user.TwoFactorEnabled || user.TwoFactorSecretEnc == "" {
		return nil, identity.ErrTOTPNotEnabled
	}

	// Decrypt TOTP secret
	decryptedSecret, err := s.protector.Decrypt(ctx, []byte(user.TwoFactorSecretEnc))
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt totp secret: %w", err)
	}

	if !s.totpManager.ValidateCode(string(decryptedSecret), code) {
		_ = s.auditRepo.Record(ctx, &audit.AuditLog{
			ActorID:    &user.ID,
			ActorIP:    ipAddress,
			UserAgent:  userAgent,
			Action:     "auth.2fa.failed",
			StatusCode: 401,
			CreatedAt:  time.Now().UTC(),
		})
		return nil, identity.ErrTOTPInvalid
	}

	tokens, err := s.issueTokensAndRecordSession(ctx, user, ipAddress, userAgent, uuid.New().String())
	if err != nil {
		return nil, err
	}

	roles, perms, err := s.getUserRolesAndPermissions(ctx, user.ID)
	if err != nil {
		return nil, err
	}

	_ = s.userRepo.UpdateLastLogin(ctx, user.ID)
	_ = s.auditRepo.Record(ctx, &audit.AuditLog{
		ActorID:      &user.ID,
		ActorIP:      ipAddress,
		UserAgent:    userAgent,
		Action:       "auth.login.2fa_success",
		ResourceType: "user",
		ResourceID:   &user.ID,
		StatusCode:   200,
		CreatedAt:    time.Now().UTC(),
	})

	return &LoginResult{
		Requires2FA: false,
		Tokens:      tokens,
		User: &UserSummary{
			ID:               user.ID,
			Username:         user.Username,
			Email:            user.Email,
			Roles:            roles,
			Permissions:      perms,
			TwoFactorEnabled: user.TwoFactorEnabled,
		},
	}, nil
}

// RefreshSession implements refresh token rotation and token reuse detection.
func (s *Service) RefreshSession(ctx context.Context, plaintextRefreshToken, ipAddress, userAgent string) (*TokenPair, error) {
	tokenHash := s.tokenManager.HashRefreshToken(plaintextRefreshToken)
	session, err := s.sessionRepo.GetByTokenHash(ctx, tokenHash)
	if err != nil {
		return nil, identity.ErrRefreshTokenInvalid
	}

	// Detect refresh token reuse
	if session.IsRevoked {
		_ = s.sessionRepo.RevokeFamily(ctx, session.FamilyID)
		_ = s.auditRepo.Record(ctx, &audit.AuditLog{
			ActorID:      &session.UserID,
			ActorIP:      ipAddress,
			UserAgent:    userAgent,
			Action:       "auth.refresh.reuse_detected",
			ResourceType: "session",
			ResourceID:   &session.ID,
			StatusCode:   401,
			Details:      map[string]interface{}{"familyId": session.FamilyID},
			CreatedAt:    time.Now().UTC(),
		})
		return nil, identity.ErrRefreshTokenReused
	}

	if time.Now().After(session.ExpiresAt) {
		return nil, identity.ErrRefreshTokenInvalid
	}

	user, err := s.userRepo.GetByID(ctx, session.UserID)
	if err != nil || !user.IsActive {
		return nil, identity.ErrAccountDisabled
	}

	// Rotate session
	newPlaintext, newHash, err := s.tokenManager.GenerateRefreshToken()
	if err != nil {
		return nil, err
	}

	newSessionID := uuid.New().String()
	newSession := &identity.RefreshSession{
		ID:        newSessionID,
		UserID:    user.ID,
		TokenHash: newHash,
		FamilyID:  session.FamilyID,
		UserAgent: userAgent,
		IPAddress: ipAddress,
		ExpiresAt: time.Now().Add(7 * 24 * time.Hour).UTC(),
		CreatedAt: time.Now().UTC(),
		IsRevoked: false,
	}

	if err := s.sessionRepo.Create(ctx, newSession); err != nil {
		return nil, fmt.Errorf("failed to create rotated session: %w", err)
	}

	_ = s.sessionRepo.Revoke(ctx, session.ID, &newSessionID)

	roles, perms, err := s.getUserRolesAndPermissions(ctx, user.ID)
	if err != nil {
		return nil, err
	}

	accessToken, err := s.tokenManager.GenerateAccessToken(user, roles, perms)
	if err != nil {
		return nil, err
	}

	return &TokenPair{
		AccessToken:  accessToken,
		RefreshToken: newPlaintext,
		TokenType:    "Bearer",
		ExpiresIn:    900,
	}, nil
}

// Logout revokes the session.
func (s *Service) Logout(ctx context.Context, plaintextRefreshToken string) error {
	tokenHash := s.tokenManager.HashRefreshToken(plaintextRefreshToken)
	session, err := s.sessionRepo.GetByTokenHash(ctx, tokenHash)
	if err != nil {
		return nil
	}

	_ = s.sessionRepo.Revoke(ctx, session.ID, nil)
	_ = s.auditRepo.Record(ctx, &audit.AuditLog{
		ActorID:      &session.UserID,
		Action:       "auth.logout",
		ResourceType: "session",
		ResourceID:   &session.ID,
		StatusCode:   200,
		CreatedAt:    time.Now().UTC(),
	})

	return nil
}

func (s *Service) issueTokensAndRecordSession(ctx context.Context, user *identity.User, ipAddress, userAgent, familyID string) (*TokenPair, error) {
	roles, perms, err := s.getUserRolesAndPermissions(ctx, user.ID)
	if err != nil {
		return nil, err
	}

	accessToken, err := s.tokenManager.GenerateAccessToken(user, roles, perms)
	if err != nil {
		return nil, fmt.Errorf("failed to generate access token: %w", err)
	}

	plaintextRefresh, refreshHash, err := s.tokenManager.GenerateRefreshToken()
	if err != nil {
		return nil, fmt.Errorf("failed to generate refresh token: %w", err)
	}

	session := &identity.RefreshSession{
		ID:        uuid.New().String(),
		UserID:    user.ID,
		TokenHash: refreshHash,
		FamilyID:  familyID,
		UserAgent: userAgent,
		IPAddress: ipAddress,
		ExpiresAt: time.Now().Add(7 * 24 * time.Hour).UTC(),
		CreatedAt: time.Now().UTC(),
		IsRevoked: false,
	}

	if err := s.sessionRepo.Create(ctx, session); err != nil {
		return nil, fmt.Errorf("failed to persist refresh session: %w", err)
	}

	return &TokenPair{
		AccessToken:  accessToken,
		RefreshToken: plaintextRefresh,
		TokenType:    "Bearer",
		ExpiresIn:    900,
	}, nil
}

func (s *Service) getUserRolesAndPermissions(ctx context.Context, userID string) ([]string, []string, error) {
	grants, err := s.roleRepo.GetGrantsForUser(ctx, userID)
	if err != nil {
		return nil, nil, err
	}

	roleNames := make([]string, 0, len(grants))
	for _, g := range grants {
		if g.RoleName != "" {
			roleNames = append(roleNames, g.RoleName)
		} else if g.RoleID != "" {
			role, err := s.roleRepo.GetByID(ctx, g.RoleID)
			if err == nil && role != nil {
				roleNames = append(roleNames, role.Name)
			}
		}
	}

	perms, err := s.roleRepo.GetUserPermissions(ctx, userID)
	if err != nil {
		return nil, nil, err
	}

	return roleNames, perms, nil
}
