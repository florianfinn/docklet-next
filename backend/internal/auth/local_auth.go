// Source: getarcaneapp/arcane/backend/internal/auth/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed federated credentials; functional cut: removed OIDC and passkeys; M15 simple-user rights and identity contracts.

package auth

import (
	"context"
	"errors"
	"fmt"
	"github.com/florianfinn/docklet-next/backend/internal/common"
	"github.com/florianfinn/docklet-next/backend/internal/user"
	"log/slog"
	"strings"
	"time"
)

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
	if localUser == nil || localUser.DisabledAt != nil {
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
