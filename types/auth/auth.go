// Source: getarcaneapp/arcane/types/auth/auth.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed OIDC and passkeys.

package auth

import (
	"time"

	"github.com/florianfinn/docklet-next/types/user"
)

// Login represents the login request body.
type Login struct {
	Username string `json:"username" minLength:"1" maxLength:"255" doc:"Username of the user" example:"admin"`
	Password string `json:"password" minLength:"1" doc:"Password of the user"`
}

// Refresh represents the token refresh request body.
type Refresh struct {
	RefreshToken string `json:"refreshToken" minLength:"1" doc:"Refresh token used to obtain a new access token"`
}

// PasswordChange represents the password change request body.
type PasswordChange struct {
	CurrentPassword string `json:"currentPassword,omitempty" doc:"Current password of the user"`
	NewPassword     string `json:"newPassword" minLength:"8" doc:"New password for the user"`
}

// SessionMeta captures request metadata associated with an authenticated session.
type SessionMeta struct {
	UserAgent string
	IPAddress string
	Source    string
}

type AuthenticationStatus string

const (
	AuthenticationStatusAuthenticated AuthenticationStatus = "authenticated"
)

// AuthenticationResponse contains local login tokens and user data.
type AuthenticationResponse struct {
	Success      bool                 `json:"success" doc:"Whether the authentication request was accepted"`
	Status       AuthenticationStatus `json:"status" doc:"Authentication state"`
	Token        string               `json:"token,omitempty" doc:"JWT access token"`
	RefreshToken string               `json:"refreshToken,omitempty" doc:"Refresh token for obtaining new access tokens"`
	ExpiresAt    *time.Time           `json:"expiresAt,omitempty" doc:"Expiration time of the access token"`
	User         *user.User           `json:"user,omitempty" doc:"Authenticated user information"`
}

// TokenRefreshResponse represents the successful token refresh response data.
type TokenRefreshResponse struct {
	Token        string    `json:"token" doc:"New JWT access token"`
	RefreshToken string    `json:"refreshToken" doc:"New refresh token"`
	ExpiresAt    time.Time `json:"expiresAt" doc:"Expiration time of the new access token"`
}

// AutoLoginConfig represents the auto-login configuration for the frontend.
// Password is intentionally excluded from this response.
type AutoLoginConfig struct {
	Enabled  bool   `json:"enabled" doc:"Whether auto-login is enabled"`
	Username string `json:"username" doc:"Username for auto-login (only returned if enabled)"`
}
