package identity

import "errors"

// Standard domain errors for authentication, authorization, and identity management.
var (
	ErrInvalidCredentials       = errors.New("invalid username, email or password")
	ErrAccountDisabled          = errors.New("account is disabled")
	ErrUserNotFound             = errors.New("user not found")
	ErrUsernameExists           = errors.New("username is already registered")
	ErrEmailExists              = errors.New("email is already registered")
	ErrTokenExpired             = errors.New("access token has expired")
	ErrTokenInvalid             = errors.New("access token is invalid")
	ErrRefreshTokenInvalid      = errors.New("refresh token is invalid")
	ErrRefreshTokenRevoked      = errors.New("refresh token has been revoked")
	ErrRefreshTokenReused       = errors.New("refresh token reuse detected")
	ErrTOTPRequired             = errors.New("totp two-factor authentication is required")
	ErrTOTPInvalid              = errors.New("invalid or expired totp code")
	ErrTOTPAlreadyEnabled       = errors.New("totp two-factor authentication is already enabled")
	ErrTOTPNotEnabled           = errors.New("totp two-factor authentication is not enabled")
	ErrAPIKeyInvalid            = errors.New("invalid api key")
	ErrAPIKeyExpired            = errors.New("api key has expired")
	ErrAPIKeyRevoked            = errors.New("api key has been revoked")
	ErrAPIKeyNotFound           = errors.New("api key not found")
	ErrRoleNotFound             = errors.New("role not found")
	ErrPermissionNotFound       = errors.New("permission not found")
	ErrInsufficientPermission   = errors.New("insufficient permission to perform action")
	ErrResourceForbidden        = errors.New("access to resource is forbidden")
	ErrUnauthorized             = errors.New("unauthorized request")
)
