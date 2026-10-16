// Source: getarcaneapp/arcane/backend/internal/auth/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed federated credentials; functional cut: removed OIDC and passkeys.

package auth

import (
	"context"
	"crypto/mldsa"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"uuid"

	"github.com/florianfinn/docklet-next/types/auth"
	"github.com/lestrrat-go/jwx/v4/jwa"
	"github.com/lestrrat-go/jwx/v4/jwt"
	"github.com/samber/hot"
	"go.getarcane.app/kit/pkg"

	"github.com/florianfinn/docklet-next/backend/internal/common"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/event"
	"github.com/florianfinn/docklet-next/backend/internal/session"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/backend/internal/user"
	"github.com/florianfinn/docklet-next/backend/pkg/utils"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/validation"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrLocalAuthDisabled  = errors.New("local authentication is disabled")
)

type TokenPair struct {
	AccessToken  string    `json:"accessToken"`
	BrowserToken string    `json:"-"`
	RefreshToken string    `json:"refreshToken"`
	ExpiresAt    time.Time `json:"expiresAt"`
}

type verifiedTokenEntry struct {
	User             user.User
	SessionID        string
	TokenExpiresAt   time.Time
	SessionExpiresAt time.Time
}

type AuthService struct {
	userService       *user.UserService
	settingsService   *settings.SettingsService
	eventService      *event.EventService
	sessionService    *session.SessionService
	signingKeyMu      sync.Mutex
	signingKey        *mldsa.PrivateKey
	browserSigningKey []byte
	refreshExpiry     time.Duration
	config            *config.Config
	recoveryOnce      sync.Once
	recoveryErr       error
	// tokenCache is a per-process in-memory cache for verified tokens. A hit
	// skips signature verification and the user/session lookups; revocation in
	// this process purges entries, revocation by another process is visible
	// once the TTL lapses.
	tokenCache  *hot.HotCache[string, verifiedTokenEntry]
	cacheGen    atomic.Uint64
	cacheFillMu sync.Mutex
}

func NewAuthService(
	userService *user.UserService,
	settingsService *settings.SettingsService,
	eventService *event.EventService,
	sessionService *session.SessionService,
	cfg *config.Config,
) *AuthService {
	return &AuthService{
		userService:     userService,
		settingsService: settingsService,
		eventService:    eventService,
		sessionService:  sessionService,
		refreshExpiry:   cfg.JWTRefreshExpiry,
		config:          cfg,
		tokenCache: hot.NewHotCache[string, verifiedTokenEntry](hot.LRU, 4096).
			WithTTL(60 * time.Second).
			WithJanitor().
			Build(),
	}
}

func (s *AuthService) GetSessionTimeout(ctx context.Context) (int, error) {
	localSettings, err := s.settingsService.GetSettings(ctx)
	if err != nil {
		return 60, err
	}

	minutes := localSettings.AuthSessionTimeout.AsInt()
	if minutes <= 0 {
		minutes = 60
	}

	if minutes < 15 {
		minutes = 15
	} else if minutes > 525600 {
		minutes = 525600
	}

	return minutes, nil
}

// AuthenticateLocalPrimary validates the local primary factor without
// creating a session.
func (s *AuthService) AuthenticateLocalPrimary(ctx context.Context, username, password string) (*user.User, error) {
	localEnabled, err := s.IsLocalAuthEnabled(ctx)
	if err != nil {
		return nil, err
	}
	if !localEnabled {
		return nil, ErrLocalAuthDisabled
	}

	localUser, err := s.userService.GetUserByUsername(ctx, username)
	if err != nil && !errors.Is(err, common.ErrUserNotFound) {
		return nil, err
	}
	// When the identifier looks like an email, resolve the email identity too:
	// a collision between one account's username and a different account's
	// email is rejected instead of silently validating against whichever
	// account the username lookup happened to return.
	if strings.Contains(username, "@") {
		emailUser, emailErr := s.userService.GetUserByEmail(ctx, username)
		switch {
		case errors.Is(emailErr, common.ErrAmbiguousUserEmail):
			slog.WarnContext(ctx, "Rejecting email login: multiple accounts share this email", "email", username)
			return nil, ErrInvalidCredentials
		case emailErr != nil && !errors.Is(emailErr, common.ErrUserNotFound):
			return nil, emailErr
		case emailErr == nil && localUser != nil && emailUser.ID != localUser.ID:
			slog.WarnContext(ctx, "Rejecting login: identifier matches one account's username and a different account's email", "identifier", username)
			return nil, ErrInvalidCredentials
		case emailErr == nil && localUser == nil:
			localUser = emailUser
		}
	}
	if localUser == nil {
		return nil, ErrInvalidCredentials
	}
	if validatePasswordErr := s.userService.ValidatePassword(localUser.PasswordHash, password); validatePasswordErr != nil {
		return nil, ErrInvalidCredentials
	}

	localUser.LastLogin = new(time.Now())
	userCopy := new(*localUser)
	s.runInBackground(ctx, "update_last_login", func(ctx context.Context) error {
		if _, updateUserErr := s.userService.UpdateUser(ctx, userCopy, nil); updateUserErr != nil {
			return fmt.Errorf("failed to update user's last login time: %w", updateUserErr)
		}
		return nil
	})

	return localUser, nil
}

// CompleteLogin creates a session after local credentials are verified.
func (s *AuthService) CompleteLogin(ctx context.Context, localUser *user.User, meta auth.SessionMeta, source string) (*TokenPair, error) {
	if s.SignInMode() != auth.SignInModeLocal {
		return nil, ErrLocalAuthDisabled
	}
	if localUser == nil {
		return nil, common.ErrUserNotFound
	}
	if strings.TrimSpace(source) == "" {
		source = session.UserSessionSourceLocal
	}
	meta.Source = source
	tokenPair, err := s.createSessionAndTokensInternal(ctx, localUser, meta)
	if err != nil {
		return nil, err
	}

	metadata := database.JSON{"action": "login", "method": source}
	if s.eventService != nil {
		logUserID := localUser.ID
		logUsername := localUser.Username
		s.runInBackground(ctx, "log_user_login", func(ctx context.Context) error {
			return s.eventService.LogUserEvent(ctx, event.EventTypeUserLogin, logUserID, logUsername, metadata)
		})
	}

	return tokenPair, nil
}

func (s *AuthService) Login(ctx context.Context, username, password string, meta auth.SessionMeta) (*user.User, *TokenPair, error) {
	localUser, err := s.AuthenticateLocalPrimary(ctx, username, password)
	if err != nil {
		return nil, nil, err
	}
	tokenPair, err := s.CompleteLogin(ctx, localUser, meta, session.UserSessionSourceLocal)
	if err != nil {
		return nil, nil, err
	}
	return localUser, tokenPair, nil
}

func (s *AuthService) LogLogout(ctx context.Context, localUser *user.User) {
	if s.eventService == nil || localUser == nil {
		return
	}

	metadata := database.JSON{
		"action": "logout",
	}

	userID := localUser.ID
	username := localUser.Username
	s.runInBackground(ctx, "log_user_logout", func(ctx context.Context) error {
		return s.eventService.LogUserEvent(ctx, event.EventTypeUserLogout, userID, username, metadata)
	})
}

func (s *AuthService) WithSigningKey(key *mldsa.PrivateKey) *AuthService {
	s.signingKey = key
	return s
}

func (s *AuthService) signingKeyInternal(ctx context.Context) (*mldsa.PrivateKey, error) {
	s.signingKeyMu.Lock()
	defer s.signingKeyMu.Unlock()
	if s.signingKey != nil {
		return s.signingKey, nil
	}
	if s.settingsService == nil {
		return nil, common.Classify(common.ErrUnavailable, errors.New("JWT signing key is not configured"))
	}
	key, err := s.settingsService.EnsureJwtSigningKey(ctx)
	if err != nil {
		return nil, err
	}
	s.signingKey = key
	return key, nil
}

func (s *AuthService) browserSigningKeyInternal(ctx context.Context) ([]byte, error) {
	s.signingKeyMu.Lock()
	defer s.signingKeyMu.Unlock()
	if len(s.browserSigningKey) == browserSessionSigningKeySize {
		return s.browserSigningKey, nil
	}
	if s.settingsService == nil {
		return nil, common.Classify(common.ErrUnavailable, errors.New("browser session signing key is not configured"))
	}
	key, err := s.settingsService.EnsureBrowserSessionSigningKey(ctx)
	if err != nil {
		return nil, err
	}
	if len(key) != browserSessionSigningKeySize {
		return nil, common.Classify(common.ErrUnavailable, errors.New("browser session signing key has invalid length"))
	}
	s.browserSigningKey = key
	return key, nil
}

func (s *AuthService) RefreshToken(ctx context.Context, refreshToken string, meta auth.SessionMeta) (*TokenPair, error) {
	if s.SignInMode() != auth.SignInModeLocal {
		return nil, common.ErrInvalidToken
	}
	signingKey, err := s.signingKeyInternal(ctx)
	if err != nil {
		return nil, err
	}
	claims, err := parseRefreshTokenInternal(ctx, refreshToken, signingKey.PublicKey())
	if err != nil {
		return nil, err
	}

	if claims.AppVersion != "" && claims.AppVersion != config.Version {
		slog.InfoContext(ctx, "Refresh token version mismatch — rotating to current version", "tokenVersion", claims.AppVersion, "currentVersion", config.Version)
	}

	if s.sessionService == nil {
		return nil, common.Classify(common.ErrUnavailable, errors.New("Session service is not configured")) //nolint:staticcheck // Preserve the existing error message.
	}

	userSession, err := s.sessionService.GetSessionByID(ctx, claims.SessionID)
	if err != nil {
		return nil, err
	}
	if userSession.UserID != claims.UserID {
		return nil, common.ErrInvalidToken
	}
	if validateActiveErr := session.ValidateActive(userSession); validateActiveErr != nil {
		return nil, validateActiveErr
	}

	rotatedSession, refreshJTI, err := s.sessionService.RotateRefreshToken(ctx, claims.SessionID, claims.ID, meta)
	if err != nil {
		return nil, err
	}

	localUser, err := s.userService.GetUserByID(ctx, claims.UserID)
	if err != nil {
		return nil, err
	}

	return s.buildTokenPairInternal(ctx, localUser, rotatedSession, refreshJTI)
}

func (s *AuthService) VerifyToken(ctx context.Context, accessToken string) (*user.User, string, error) {
	if s.SignInMode() != auth.SignInModeLocal {
		return nil, "", common.ErrInvalidToken
	}
	tokenHash := kit.SHA256Hex(accessToken)
	if localUser, sessionID, ok := s.cachedVerificationInternal(tokenHash); ok {
		return localUser, sessionID, nil
	}
	signingKey, err := s.signingKeyInternal(ctx)
	if err != nil {
		return nil, "", err
	}
	claims, err := parseAccessTokenInternal(ctx, accessToken, signingKey.PublicKey())
	if err != nil {
		return nil, "", err
	}
	return s.verifyTokenClaimsInternal(ctx, tokenHash, claims)
}

func (s *AuthService) VerifyBrowserToken(ctx context.Context, browserToken string) (*user.User, string, error) {
	if s.SignInMode() != auth.SignInModeLocal {
		return nil, "", common.ErrInvalidToken
	}
	tokenHash := "browser:" + kit.SHA256Hex(browserToken)
	if localUser, sessionID, ok := s.cachedVerificationInternal(tokenHash); ok {
		return localUser, sessionID, nil
	}
	algorithm, ok := signedTokenAlgorithmInternal(browserToken)
	if !ok {
		return nil, "", common.ErrInvalidToken
	}
	switch algorithm {
	case jwa.HS512().String():
		key, err := s.browserSigningKeyInternal(ctx)
		if err != nil {
			return nil, "", err
		}
		claims, err := parseBrowserTokenInternal(ctx, browserToken, key)
		if err != nil {
			return nil, "", err
		}
		return s.verifyTokenClaimsInternal(ctx, tokenHash, claims)
	case jwa.MLDSA87().String():
		return s.VerifyToken(ctx, browserToken)
	default:
		return nil, "", common.ErrInvalidToken
	}
}

func (s *AuthService) verifyTokenClaimsInternal(ctx context.Context, tokenHash string, claims *accessTokenClaims) (*user.User, string, error) {
	gen := s.cacheGen.Load()
	if claims.AppVersion != "" && claims.AppVersion != config.Version {
		slog.InfoContext(ctx, "Token version mismatch detected", "tokenVersion", claims.AppVersion, "currentVersion", config.Version, "user", claims.Username)
		return nil, "", common.ErrTokenVersionMismatch
	}
	if s.sessionService == nil {
		return nil, "", common.Classify(common.ErrUnavailable, errors.New("Session service is not configured")) //nolint:staticcheck // Preserve the existing error message.
	}

	// Verify user exists in DB
	// This ensures that if the database is wiped or user is deleted, the token becomes invalid
	// even if the JWT signature is still valid (e.g. same signing key).
	dbUser, err := s.userService.GetUserByID(ctx, claims.UserID)
	if err != nil {
		return nil, "", kit.Ternary[error](errors.Is(err, common.ErrUserNotFound), common.ErrInvalidToken, err)
	}

	userSession, err := s.sessionService.GetSessionByID(ctx, claims.SessionID)
	if err != nil {
		return nil, "", err
	}
	if userSession.UserID != dbUser.ID {
		return nil, "", common.ErrInvalidToken
	}
	if validateActiveErr := session.ValidateActive(userSession); validateActiveErr != nil {
		return nil, "", validateActiveErr
	}

	s.cacheFillMu.Lock()
	if s.cacheGen.Load() == gen {
		s.tokenCache.Set(tokenHash, verifiedTokenEntry{User: *dbUser, SessionID: userSession.ID, TokenExpiresAt: claims.ExpiresAt, SessionExpiresAt: userSession.ExpiresAt})
	}
	s.cacheFillMu.Unlock()

	return dbUser, userSession.ID, nil
}

func (s *AuthService) ChangePassword(ctx context.Context, userID, currentPassword, newPassword, currentSessionID string) error {
	if s.sessionService == nil {
		return common.Classify(common.ErrUnavailable, errors.New("Session service is not configured")) //nolint:staticcheck // Preserve the existing error message.
	}

	localUser, err := s.userService.GetUserByID(ctx, userID)
	if err != nil {
		return err
	}

	if localUser.PasswordHash != "" {
		if validatePasswordErr := s.userService.ValidatePassword(localUser.PasswordHash, currentPassword); validatePasswordErr != nil {
			return ErrInvalidCredentials
		}
	}

	policy := s.passwordPolicyInternal(ctx)
	if validatePasswordPolicyErr := validation.ValidatePasswordPolicy(newPassword, policy); validatePasswordPolicyErr != nil {
		return common.Classify(common.ErrValidation, validatePasswordPolicyErr)
	}

	if _, err = s.userService.SetPasswordAndRevokeSessionsExcept(ctx, localUser, newPassword, currentSessionID); err != nil {
		return err
	}
	// The current session's cached user still carries the old password flags.
	s.InvalidateUserTokenCache(userID)
	return nil
}

// InvalidateUserTokenCache purges all cached token verifications for a user.
// Call this after admin-initiated role changes, account disable, or user
// deletion so stale verifications cannot grant access for the cache TTL.
func (s *AuthService) InvalidateUserTokenCache(userID string) {
	if s.tokenCache == nil || strings.TrimSpace(userID) == "" {
		return
	}
	s.invalidateTokenCacheInternal(func(entry verifiedTokenEntry) bool { return entry.User.ID == userID })
}

func (s *AuthService) invalidateTokenCacheInternal(matches func(verifiedTokenEntry) bool) {
	s.cacheFillMu.Lock()
	defer s.cacheFillMu.Unlock()
	s.cacheGen.Add(1)
	keys := make([]string, 0)
	s.tokenCache.Range(func(key string, entry verifiedTokenEntry) bool {
		if matches(entry) {
			keys = append(keys, key)
		}
		return true
	})
	s.tokenCache.DeleteMany(keys)
}

func (s *AuthService) RevokeSession(ctx context.Context, sessionID string) error {
	if s.sessionService == nil {
		return nil
	}
	if err := s.sessionService.RevokeSession(ctx, sessionID); err != nil {
		return err
	}
	s.invalidateTokenCacheInternal(func(entry verifiedTokenEntry) bool { return entry.SessionID == sessionID })
	return nil
}

// LogoutAllOtherSessions revokes every active session for userID except
// currentSessionID, so the caller stays signed in on their current device.
func (s *AuthService) LogoutAllOtherSessions(ctx context.Context, userID, currentSessionID string) error {
	if s.sessionService == nil {
		return nil
	}
	if err := s.sessionService.RevokeAllUserSessionsExcept(ctx, userID, currentSessionID); err != nil {
		return err
	}
	s.invalidateTokenCacheInternal(func(entry verifiedTokenEntry) bool {
		return entry.User.ID == userID && entry.SessionID != currentSessionID
	})
	return nil
}

func (s *AuthService) createSessionAndTokensInternal(ctx context.Context, localUser *user.User, meta auth.SessionMeta) (*TokenPair, error) {
	if s.sessionService == nil {
		return nil, common.Classify(common.ErrUnavailable, errors.New("Session service is not configured")) //nolint:staticcheck // Preserve the existing error message.
	}
	refreshExpiry := time.Now().Add(s.refreshExpiry)
	localSession, refreshJTI, err := s.sessionService.CreateSession(ctx, localUser.ID, refreshExpiry, meta)
	if err != nil {
		return nil, err
	}
	return s.buildTokenPairInternal(ctx, localUser, localSession, refreshJTI)
}

func (s *AuthService) buildTokenPairInternal(ctx context.Context, localUser *user.User, localSession *session.UserSession, refreshJTI string) (*TokenPair, error) {
	sessionTimeout, _ := s.GetSessionTimeout(ctx)
	now := time.Now()
	accessTokenExpiry := now.Add(time.Duration(sessionTimeout) * time.Minute)

	signingKey, err := s.signingKeyInternal(ctx)
	if err != nil {
		return nil, err
	}

	accessBuilder := jwt.NewBuilder().
		JwtID(localUser.ID).
		Subject(accessTokenSubject).
		IssuedAt(now).
		Expiration(accessTokenExpiry).
		Claim(claimSessionID, localSession.ID).
		Claim(claimUserID, localUser.ID)
	if localUser.Username != "" {
		accessBuilder.Claim(claimUsername, localUser.Username)
	}
	if localUser.Email != nil && *localUser.Email != "" {
		accessBuilder.Claim(claimEmail, *localUser.Email)
	}
	if localUser.DisplayName != nil && *localUser.DisplayName != "" {
		accessBuilder.Claim(claimDisplayName, *localUser.DisplayName)
	}
	if config.Version != "" {
		accessBuilder.Claim(claimAppVersion, config.Version)
	}
	accessToken, err := accessBuilder.Build()
	if err != nil {
		return nil, err
	}
	accessTokenBytes, err := jwt.Sign(accessToken, jwt.WithKey(jwa.MLDSA87(), signingKey))
	if err != nil {
		return nil, err
	}

	refreshBuilder := jwt.NewBuilder().
		JwtID(refreshJTI).
		Subject(refreshTokenSubject).
		IssuedAt(now).
		Expiration(localSession.ExpiresAt).
		Claim(claimUserID, localUser.ID).
		Claim(claimSessionID, localSession.ID)
	if config.Version != "" {
		refreshBuilder.Claim(claimAppVersion, config.Version)
	}
	refreshToken, err := refreshBuilder.Build()
	if err != nil {
		return nil, err
	}
	refreshTokenBytes, err := jwt.Sign(refreshToken, jwt.WithKey(jwa.MLDSA87(), signingKey))
	if err != nil {
		return nil, err
	}

	browserSigningKey, err := s.browserSigningKeyInternal(ctx)
	if err != nil {
		return nil, err
	}
	browserBuilder := jwt.NewBuilder().
		JwtID(uuid.New().String()).
		Subject(browserTokenSubject).
		IssuedAt(now).
		Expiration(accessTokenExpiry).
		Claim(claimSessionID, localSession.ID).
		Claim(claimUserID, localUser.ID)
	if config.Version != "" {
		browserBuilder.Claim(claimAppVersion, config.Version)
	}
	browserToken, err := browserBuilder.Build()
	if err != nil {
		return nil, err
	}
	browserTokenBytes, err := jwt.Sign(browserToken, jwt.WithKey(jwa.HS512(), browserSigningKey))
	if err != nil {
		return nil, err
	}

	return &TokenPair{
		AccessToken:  string(accessTokenBytes),
		BrowserToken: string(browserTokenBytes),
		RefreshToken: string(refreshTokenBytes),
		ExpiresAt:    accessTokenExpiry,
	}, nil
}

func (s *AuthService) runInBackground(ctx context.Context, name string, fn func(ctx context.Context) error) {
	// Detach context to prevent cancellation when the parent request finishes
	bgCtx := context.WithoutCancel(ctx)

	go func() {
		defer func() {
			if panicErr := utils.PanicToError(recover()); panicErr != nil {
				slog.ErrorContext(bgCtx, "Unhandled error", "error", fmt.Errorf("Background task panicked: %w", panicErr), "task", name) //nolint:staticcheck // Preserve the existing error message.
			}
		}()

		// Set a reasonable timeout for background tasks
		taskCtx, cancel := context.WithTimeout(bgCtx, 1*time.Minute)
		defer cancel()

		if err := fn(taskCtx); err != nil {
			slog.ErrorContext(taskCtx, "Background task failed", "task", name, "error", err)
		}
	}()
}
