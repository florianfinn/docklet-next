// Source: docklet-next/issues/68
// Revision: 92e896e926aaaf0189c72dcec6e3b15a52ddbd5d
// Retrieved: 2026-10-16
// License: Apache-2.0
// Deviations: Original single-use startup administrator recovery.

package auth

import (
	"context"
	"errors"
	"log/slog"

	"github.com/florianfinn/docklet-next/backend/pkg/utils/validation"
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
	})
	return s.recoveryErr
}

func (s *AuthService) passwordPolicyInternal(ctx context.Context) string {
	if s.settingsService == nil {
		return validation.PasswordPolicyStrong
	}
	return s.settingsService.GetStringSetting(ctx, "authPasswordPolicy", validation.PasswordPolicyStrong)
}
