// Source: docklet-next/issues/68
// Revision: 92e896e926aaaf0189c72dcec6e3b15a52ddbd5d
// Retrieved: 2026-10-16
// License: Apache-2.0
// Deviations: Original single-use startup administrator recovery; M29 recovery also restores local sign-in.

package auth

import (
	"context"
	"errors"
	"log/slog"

	"gorm.io/gorm"

	"github.com/florianfinn/docklet-next/backend/internal/session"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/validation"
	authtypes "github.com/florianfinn/docklet-next/types/auth"
)

func (s *AuthService) recoverAdministrator(ctx context.Context) error {
	s.recoveryOnce.Do(func() {
		password := s.config.AuthResetAdminPassword
		s.config.AuthResetAdminPassword = ""
		if password == "" {
			return
		}
		if err := validation.ValidatePasswordPolicy(password, s.passwordPolicyInternal(ctx)); err != nil {
			s.recoveryErr = errors.New("AUTH_RESET_ADMIN_PASSWORD does not meet the password policy")
			return
		}
		admin, err := s.userService.GlobalAdministrator(ctx)
		if err != nil {
			s.recoveryErr = err
			return
		}
		if _, err := s.userService.SetPasswordAndRevokeSessionsExcept(ctx, admin, password, ""); err != nil {
			s.recoveryErr = errors.New("administrator password recovery failed")
			return
		}
		s.InvalidateAllTokenCache()
		slog.WarnContext(ctx, "Administrator password reset by startup configuration; remove AUTH_RESET_ADMIN_PASSWORD before restarting")
		// A lost password, a wrong proxy setting or an unreadable stored policy all end in stored local sign-in.
		if current := s.CurrentPolicy(); !current.Forced {
			next := &Policy{Mode: authtypes.SignInModeLocal, Header: current.Header, Trusted: current.Trusted}
			if err := s.publishPolicyInternal(ctx, next, func(ctx context.Context, p *Policy) error {
				return s.sessionService.Transaction(ctx, func(tx *gorm.DB) error {
					if err := session.WriteSignInPolicyInDB(tx, p.stored()); err != nil {
						return err
					}
					return session.ActivateSignInModeInDB(tx, p.Mode, true)
				})
			}); err != nil {
				s.recoveryErr = errors.New("sign-in recovery failed")
				return
			}
			slog.WarnContext(ctx, "Sign-in stored as local by AUTH_RESET_ADMIN_PASSWORD; all sessions revoked")
		}
	})
	return s.recoveryErr
}

// Source: getarcaneapp/arcane/backend/internal/auth/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Relocated without behavioural changes.
func (s *AuthService) passwordPolicyInternal(ctx context.Context) string {
	if s.settingsService == nil {
		return validation.PasswordPolicyStrong
	}
	return s.settingsService.GetStringSetting(ctx, "authPasswordPolicy", validation.PasswordPolicyStrong)
}
