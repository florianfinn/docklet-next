// Source: docklet-next/issues/669
// Revision: 99c1b0b8c4dfe56182c72749e092b08202436fa5
// Retrieved: 2026-11-27
// License: Apache-2.0
// Deviations: Original shared transactional administrator guard extracted from the role service.

package role

import (
	"context"
	"errors"

	"github.com/florianfinn/docklet-next/backend/internal/common"
	"gorm.io/gorm"
)

// AssertGlobalAdminExists requires an enabled global administrator after role reconciliation.
func (s *RoleService) AssertGlobalAdminExists(ctx context.Context) error {
	return s.assertGlobalAdminExistsInDB(ctx, s.db.WithContext(ctx))
}

func (s *RoleService) assertGlobalAdminExistsInDB(ctx context.Context, tx *gorm.DB) error {
	count, err := s.CountGlobalAdminsExcludingUserInDB(ctx, tx, "")
	if err != nil {
		return err
	}
	if count == 0 {
		return common.Classify(common.ErrNoGlobalAdminRemains, errors.New("At least one user must retain a global Admin role assignment")) //nolint:staticcheck // Preserve the existing error message.
	}
	return nil
}
