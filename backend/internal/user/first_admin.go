// Source: docklet-next/issues/518
// Revision: 462df7e945082b44e1d90532d8296aa9743c4b6e
// Retrieved: 2026-11-18
// License: Apache-2.0
// Deviations: Original first-run administrator claim (docs/design/first-run-setup.md).

package user

import (
	"context"
	"errors"
	"strings"

	"gorm.io/gorm"

	"github.com/florianfinn/docklet-next/backend/internal/role"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	authtypes "github.com/florianfinn/docklet-next/types/auth"
)

// FirstAdministrator is the account chosen in first-run setup. An empty Password stores no password.
type FirstAdministrator struct {
	Username    string
	DisplayName *string
	Password    string
	AllowAt     bool // proxy identities may contain @
}

// ClaimFirstAdministratorInDB creates the first administrator or, on an upgraded hub, takes over the
// oldest global administrator (enabled first, else the oldest disabled one, which is re-enabled), keeping
// its ID so sessions, keys and activity stay attributed. It runs inside the caller's claim transaction.
func (s *UserService) ClaimFirstAdministratorInDB(ctx context.Context, tx *gorm.DB, in FirstAdministrator) (*User, error) {
	if s.roleService == nil {
		return nil, errors.New("administrator service unavailable")
	}
	username, err := normalizeUsernameInternal(in.Username)
	if err != nil {
		return nil, err
	}
	if !in.AllowAt && strings.Contains(username, "@") {
		return nil, errors.New("Local usernames cannot contain @")
	}
	hash := ""
	if in.Password != "" {
		if hash, err = s.HashPassword(in.Password); err != nil {
			return nil, err
		}
	}

	var users []User
	if err := tx.WithContext(ctx).Order("created_at, id").Find(&users).Error; err != nil {
		return nil, err
	}
	var target *User
	for _, enabledPass := range []bool{true, false} {
		for i := range users {
			if (users[i].DisabledAt == nil) != enabledPass {
				continue
			}
			ps, err := s.roleService.ResolveUserPermissionsInDB(ctx, tx, users[i].ID)
			if err != nil {
				return nil, err
			}
			if ps != nil && ps.IsGlobalAdmin() {
				target = &users[i]
				break
			}
		}
		if target != nil {
			break
		}
	}

	targetID := ""
	if target != nil {
		targetID = target.ID
	}
	var taken int64
	if err := tx.WithContext(ctx).Model(&User{}).Where("username = ? AND id <> ?", username, targetID).Count(&taken).Error; err != nil {
		return nil, err
	}
	if taken > 0 {
		return nil, ErrUsernameTaken
	}

	if target == nil {
		target = &User{Username: username, DisplayName: in.DisplayName, PasswordHash: hash}
		if err := tx.WithContext(ctx).Create(target).Error; err != nil {
			return nil, err
		}
	} else {
		target.Username = username
		if in.DisplayName != nil {
			target.DisplayName = in.DisplayName
		}
		target.PasswordHash = hash
		target.RequiresPasswordChange = false
		target.DisabledAt = nil
		if err := tx.WithContext(ctx).Model(&User{}).Where("id = ?", target.ID).Updates(map[string]any{
			"username":                 target.Username,
			"display_name":             target.DisplayName,
			"password_hash":            target.PasswordHash,
			"requires_password_change": false,
			"disabled_at":              nil,
		}).Error; err != nil {
			return nil, err
		}
	}

	ps, err := s.roleService.ResolveUserPermissionsInDB(ctx, tx, target.ID)
	if err != nil {
		return nil, err
	}
	if ps == nil || !ps.IsGlobalAdmin() {
		assignment := &role.UserRoleAssignment{UserID: target.ID, RoleID: authz.BuiltInRoleAdmin, Source: role.RoleAssignmentSourceManual}
		if err := tx.WithContext(ctx).Create(assignment).Error; err != nil {
			return nil, err
		}
	}
	return target, nil
}

// AfterFirstAdministratorClaim drops cached permissions once the claim transaction committed.
func (s *UserService) AfterFirstAdministratorClaim(userID string) {
	if s.roleService != nil {
		s.roleService.InvalidateUser(userID)
	}
}

// SetSignInModeSource lets user handlers follow runtime sign-in changes.
func (s *UserService) SetSignInModeSource(fn func() authtypes.SignInMode) {
	s.signInMode = fn
}

func (h *UserHandler) signInModeInternal() authtypes.SignInMode {
	if h.userService != nil && h.userService.signInMode != nil {
		return h.userService.signInMode()
	}
	if h.cfg == nil {
		return authtypes.SignInModeLocal
	}
	return h.cfg.SignInMode()
}
