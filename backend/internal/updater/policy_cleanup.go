// Source: docklet-next/issues/242
// Revision: 89f69980c01d08714ce96c49365fb19065e3d6ba
// Retrieved: 2026-11-04
// License: Apache-2.0
// Deviations: Original hub deletion handoff and window refresh.

package updater

import (
	"context"
	policy "github.com/florianfinn/docklet-next/backend/internal/updater/children/policies"
	"gorm.io/gorm"
)

func (s *UpdaterService) StackDestroyed(ctx context.Context, env, target string) error {
	p, err := s.hubPolicies()
	if err != nil {
		return err
	}
	if err = p.store.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error { return policy.DeleteStack(tx, env, target) }); err != nil {
		return err
	}
	return s.RefreshUpdateWindows(ctx)
}
