// Source: docklet-next/issues/322
// Revision: 6365cc5cebd04f199d15fe5f53c006a127d0a669
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original simple-user resource authorization.

package user

import (
	"context"
	"errors"
	"github.com/florianfinn/docklet-next/backend/internal/access"
	"github.com/florianfinn/docklet-next/backend/internal/role"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/dbutil"
	dto "github.com/florianfinn/docklet-next/types/user"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"time"
)

func (s *UserService) SetRightsInvalidator(fn func(string)) {
	if s.roleService != nil {
		s.roleService.UserChanged = fn
	}
}

// RightsChanged purges cached rights and closes every open stream of id. It is
// for mutations that target one account explicitly (disable, reset, delete,
// administrator change); administrators acting on their own data use UserRightsChanged.
func (s *UserService) RightsChanged(id string) {
	if s.roleService != nil {
		s.roleService.InvalidateUser(id)
	}
	access.Revoke(id)
}

// UserRightsChanged purges cached rights and closes streams only when id is a
// user, so administrator streams survive key deletion and assignment cleanup.
func (s *UserService) UserRightsChanged(ctx context.Context, id string) {
	if s.roleService == nil {
		access.Revoke(id)
		return
	}
	s.roleService.InvalidateUser(id)
	if ps, err := s.roleService.ResolvePermissions(ctx, id); err != nil || !ps.IsGlobalAdmin() {
		access.Revoke(id)
	}
}
func (s *UserService) AppAssignments(ctx context.Context, id string) ([]dto.AppAssignment, error) {
	return access.Assignments(ctx, s.db.DB, id)
}
func (s *UserService) guardLastAdmin(ctx context.Context, tx *gorm.DB, id string) error {
	if s.roleService == nil {
		return nil
	}
	ps, err := s.roleService.ResolveUserPermissionsInDB(ctx, tx, id)
	if err != nil {
		return err
	}
	if ps.IsGlobalAdmin() {
		n, err := s.roleService.CountGlobalAdminsExcludingUserInDB(ctx, tx, id)
		if err != nil {
			return err
		}
		if n == 0 {
			return ErrCannotRemoveLastAdmin
		}
	}
	return nil
}
func (s *UserService) SetDisabled(ctx context.Context, id string, disabled bool) error {
	err := dbutil.WithTx(ctx, s.db.DB, func(tx *gorm.DB) error {
		var u User
		if err := tx.First(&u, "id = ?", id).Error; err != nil {
			return err
		}
		var at *time.Time
		if disabled {
			if err := s.guardLastAdmin(ctx, tx, id); err != nil {
				return err
			}
			at = new(time.Now())
		}
		if err := tx.Model(&User{}).Where("id = ?", id).Update("disabled_at", at).Error; err != nil {
			return err
		}
		if disabled && s.revokeSessionsExcept != nil {
			return s.revokeSessionsExcept(ctx, tx, id, "")
		}
		return nil
	})
	if err == nil {
		s.RightsChanged(id)
	}
	return err
}
func (s *UserService) ResetInitialPassword(ctx context.Context, id, password string) error {
	hash, err := s.HashPassword(password)
	if err != nil {
		return err
	}
	err = dbutil.WithTx(ctx, s.db.DB, func(tx *gorm.DB) error {
		var u User
		if err := tx.First(&u, "id = ?", id).Error; err != nil {
			return err
		}
		if err := tx.Model(&User{}).Where("id = ?", id).Updates(map[string]any{"password_hash": hash, "requires_password_change": true}).Error; err != nil {
			return err
		}
		if s.revokeSessionsExcept != nil {
			return s.revokeSessionsExcept(ctx, tx, id, "")
		}
		return nil
	})
	if err == nil {
		s.RightsChanged(id)
	}
	return err
}
func (s *UserService) SetAdministrator(ctx context.Context, actor, id string, admin bool) error {
	if actor == id {
		return errors.New("cannot change your own administrator status")
	}
	err := dbutil.WithTx(ctx, s.db.DB, func(tx *gorm.DB) error {
		var u User
		if err := tx.First(&u, "id = ?", id).Error; err != nil {
			return err
		}
		if !admin {
			if err := s.guardLastAdmin(ctx, tx, id); err != nil {
				return err
			}
		}
		if err := tx.Where("user_id = ? AND role_id = ? AND environment_id IS NULL", id, authz.BuiltInRoleAdmin).Delete(&role.UserRoleAssignment{}).Error; err != nil {
			return err
		}
		if admin {
			return tx.Create(&role.UserRoleAssignment{UserID: id, RoleID: authz.BuiltInRoleAdmin, Source: role.RoleAssignmentSourceManual}).Error
		}
		return nil
	})
	if err == nil {
		s.RightsChanged(id)
	}
	return err
}
func (s *UserService) SetAppAssignments(ctx context.Context, id string, rows []dto.AppAssignment) error {
	err := dbutil.WithTx(ctx, s.db.DB, func(tx *gorm.DB) error {
		var u User
		if err := tx.First(&u, "id = ?", id).Error; err != nil {
			return err
		}
		for _, a := range rows {
			if a.EnvironmentID == "" || a.ProjectID == "" || (a.Level != "restricted" && a.Level != "full") {
				return errors.New("invalid application assignment")
			}
		}
		if err := tx.Table("user_app_assignments").Where("user_id = ?", id).Delete(&access.Assignment{}).Error; err != nil {
			return err
		}
		for _, a := range rows {
			if err := tx.Table("user_app_assignments").Create(map[string]any{"id": uuid.NewString(), "user_id": id, "environment_id": a.EnvironmentID, "project_id": a.ProjectID, "level": a.Level, "created_at": time.Now()}).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err == nil {
		s.UserRightsChanged(ctx, id)
	}
	return err
}
