// Source: docklet-next/issues/518
// Revision: 462df7e945082b44e1d90532d8296aa9743c4b6e
// Retrieved: 2026-11-18
// License: Apache-2.0
// Deviations: Original first-run claim and runtime sign-in policy change (docs/design/first-run-setup.md).

package auth

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sync"

	"gorm.io/gorm"

	"github.com/florianfinn/docklet-next/backend/internal/session"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/backend/internal/user"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/validation"
	authtypes "github.com/florianfinn/docklet-next/types/auth"
)

var (
	ErrSetupNotPending   = errors.New(authtypes.CodeSetupNotPending)
	ErrModeForced        = errors.New(authtypes.CodeModeForced)
	ErrProxyUnverified   = errors.New(authtypes.CodeProxyUnverified)
	ErrPasswordRequired  = errors.New(authtypes.CodePasswordRequired)
	ErrInvalidSignInMode = errors.New("invalid sign-in policy")
)

const setupCompletedKey = "setupCompleted"

// setupGuard holds process-local first-run hooks; nothing in it is persisted.
type setupGuard struct {
	mu sync.Mutex
	// afterClaim runs once the claim committed (static admin key reconciliation); failures only log.
	afterClaim func(context.Context) error
}

// PolicyError carries a validation message of a requested policy (400).
type PolicyError struct{ Err error }

func (e *PolicyError) Error() string { return e.Err.Error() }
func (e *PolicyError) Unwrap() error { return ErrInvalidSignInMode }

// SetAfterClaim registers work that must follow a successful first-run claim.
func (s *AuthService) SetAfterClaim(fn func(context.Context) error) {
	s.setup.mu.Lock()
	defer s.setup.mu.Unlock()
	s.setup.afterClaim = fn
}

// startSetupInternal runs once per start while first-run setup is pending.
func (s *AuthService) startSetupInternal(ctx context.Context) {
	if s.config.AuthResetAdminPassword != "" {
		s.config.AuthResetAdminPassword = ""
		slog.WarnContext(ctx, "AUTH_RESET_ADMIN_PASSWORD is ignored until setup has created the first administrator")
	}
	slog.InfoContext(ctx, "First-run setup is pending: open "+s.config.GetAppURL()+" to create the first administrator")
}

// requestedPolicyInternal builds the next policy; a forced policy only accepts its own mode.
func requestedPolicyInternal(current *Policy, in authtypes.SignInPolicyInput) (*Policy, error) {
	if current.Forced {
		if in.Mode != "" && in.Mode != current.Mode {
			return nil, ErrModeForced
		}
		next := *current
		next.Pending = false
		return &next, nil
	}
	next, err := newPolicyInternal(session.StoredSignInPolicy{Mode: in.Mode, ProxyHeader: strings.TrimSpace(in.ProxyHeader), TrustedProxies: strings.TrimSpace(in.TrustedProxies)})
	if err != nil {
		return nil, &PolicyError{Err: err}
	}
	return next, nil
}

// verifyClaimProxyInternal requires that this request, under the next policy, resolves to the administrator
// being claimed. The claimed account is the hub's global administrator after commit (only administrator on a
// fresh hub; the taken-over oldest one on an upgraded hub), so a listed or unlisted-but-same-name identity
// maps to it; the claim cannot resolve through the database before its transaction commits.
func (s *AuthService) verifyClaimProxyInternal(req *http.Request, next *Policy, adminUsername string) error {
	identity, err := proxyIdentityInternal(req, next)
	if err != nil {
		return ErrProxyUnverified
	}
	if s.config.AuthProxyAdminUsers == nil || strings.TrimSpace(*s.config.AuthProxyAdminUsers) == "" {
		return nil
	}
	for _, name := range strings.Split(*s.config.AuthProxyAdminUsers, ",") {
		if strings.TrimSpace(name) == identity {
			return nil
		}
	}
	if identity == adminUsername {
		return nil
	}
	return ErrProxyUnverified
}

func (s *AuthService) validatePasswordInternal(ctx context.Context, password string) error {
	if err := validation.ValidatePasswordPolicy(password, s.passwordPolicyInternal(ctx)); err != nil {
		return &PolicyError{Err: err}
	}
	return nil
}

// ClaimResult is the administrator created by setup; Tokens is set in local mode.
type ClaimResult struct {
	User   *user.User
	Tokens *TokenPair
}

// ClaimSetup stores the sign-in policy and creates or takes over the first administrator in one transaction.
func (s *AuthService) ClaimSetup(ctx context.Context, req *http.Request, in authtypes.SetupIdentityRequest, meta authtypes.SessionMeta) (*ClaimResult, error) {
	current := s.CurrentPolicy()
	if !current.Pending {
		return nil, ErrSetupNotPending
	}
	next, err := requestedPolicyInternal(current, in.SignInPolicyInput)
	if err != nil {
		return nil, err
	}
	password := ""
	if in.Password != nil {
		password = *in.Password
	}
	if next.Mode == authtypes.SignInModeLocal && password == "" {
		return nil, ErrPasswordRequired
	}
	if password != "" {
		if err := s.validatePasswordInternal(ctx, password); err != nil {
			return nil, err
		}
	}
	if next.Mode == authtypes.SignInModeProxy {
		if err := s.verifyClaimProxyInternal(req, next, strings.TrimSpace(in.Username)); err != nil {
			return nil, err
		}
	}

	var claimed *user.User
	err = s.publishPolicyInternal(ctx, next, func(ctx context.Context, p *Policy) error {
		return s.sessionService.Transaction(ctx, func(tx *gorm.DB) error {
			var marker settings.SettingVariable
			if err := tx.Where("key = ?", session.SettingSetupIdentity).First(&marker).Error; err == nil && marker.Value == session.SetupIdentityDone {
				return ErrSetupNotPending
			} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			u, err := s.userService.ClaimFirstAdministratorInDB(ctx, tx, user.FirstAdministrator{Username: in.Username, DisplayName: in.DisplayName, Password: password, AllowAt: p.Mode == authtypes.SignInModeProxy})
			if err != nil {
				return err
			}
			if err := session.WriteSignInPolicyInDB(tx, p.stored()); err != nil {
				return err
			}
			for key, value := range map[string]string{session.SettingSetupIdentity: session.SetupIdentityDone, setupCompletedKey: "false"} {
				if err := tx.Save(&settings.SettingVariable{Key: key, Value: value}).Error; err != nil {
					return err
				}
			}
			if err := session.ActivateSignInModeInDB(tx, p.Mode, true); err != nil {
				return err
			}
			claimed = u
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	s.userService.AfterFirstAdministratorClaim(claimed.ID)
	s.setup.mu.Lock()
	afterClaim := s.setup.afterClaim
	s.setup.mu.Unlock()
	if s.settingsService != nil {
		if err := s.settingsService.RefreshAfterRestore(ctx); err != nil {
			slog.ErrorContext(ctx, "Failed to refresh settings after setup claim", "error", err)
		}
	}
	if afterClaim != nil {
		if err := afterClaim(ctx); err != nil {
			slog.ErrorContext(ctx, "Post-setup reconciliation failed; it is retried at the next start", "error", err)
		}
	}
	slog.InfoContext(ctx, "First administrator created by setup", "username", claimed.Username, "mode", next.Mode)
	result := &ClaimResult{User: claimed}
	if next.Mode == authtypes.SignInModeLocal {
		if result.Tokens, err = s.CompleteLogin(ctx, claimed, meta, session.UserSessionSourceLocal); err != nil {
			return nil, err
		}
	}
	return result, nil
}

// UpdateSignIn changes the stored policy at runtime; every session is revoked and, in local mode,
// the caller receives a new one. The change applies only if the caller stays signed in afterwards.
func (s *AuthService) UpdateSignIn(ctx context.Context, req *http.Request, callerID string, in authtypes.SignInUpdateRequest, meta authtypes.SessionMeta) (*ClaimResult, error) {
	current := s.CurrentPolicy()
	if current.Pending {
		return nil, ErrSetupPending
	}
	if current.Forced {
		return nil, ErrModeForced
	}
	next, err := requestedPolicyInternal(current, in.SignInPolicyInput)
	if err != nil {
		return nil, err
	}
	caller, err := s.userService.GetUserByID(ctx, callerID)
	if err != nil {
		return nil, err
	}
	password := ""
	if in.Password != nil {
		password = *in.Password
	}
	if password != "" {
		if err := s.validatePasswordInternal(ctx, password); err != nil {
			return nil, err
		}
	}
	if next.Mode == authtypes.SignInModeLocal && password == "" && caller.PasswordHash == "" {
		return nil, ErrPasswordRequired
	}
	if next.Mode == authtypes.SignInModeProxy {
		// Resolve the principal exactly as requests will after the change; it must be the caller.
		principal, err := s.modeRequestUserInternal(ctx, req, next)
		if err != nil || principal == nil || principal.ID != caller.ID {
			return nil, ErrProxyUnverified
		}
	}
	hash := ""
	if password != "" {
		if hash, err = s.userService.HashPassword(password); err != nil {
			return nil, err
		}
	}
	err = s.publishPolicyInternal(ctx, next, func(ctx context.Context, p *Policy) error {
		return s.sessionService.Transaction(ctx, func(tx *gorm.DB) error {
			if hash != "" {
				if err := tx.Model(&user.User{}).Where("id = ?", caller.ID).Updates(map[string]any{"password_hash": hash, "requires_password_change": false}).Error; err != nil {
					return err
				}
			}
			if err := session.WriteSignInPolicyInDB(tx, p.stored()); err != nil {
				return err
			}
			return session.ActivateSignInModeInDB(tx, p.Mode, true)
		})
	})
	if err != nil {
		return nil, err
	}
	slog.InfoContext(ctx, "Sign-in policy changed in settings; all sessions were revoked", "mode", next.Mode)
	if hash != "" {
		caller.PasswordHash = hash
		caller.RequiresPasswordChange = false
	}
	result := &ClaimResult{User: caller}
	if next.Mode == authtypes.SignInModeLocal {
		if result.Tokens, err = s.CompleteLogin(ctx, caller, meta, session.UserSessionSourceLocal); err != nil {
			return nil, err
		}
	}
	return result, nil
}
