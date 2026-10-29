// Source: docklet-next/issues/296
// Revision: a910be6a64e97a4ce3a422f12451a5301b115f7a
// Retrieved: 2026-10-29
// License: Apache-2.0
// Deviations: Original notification and Auto Heal foundation.

package backup

import (
	"context"
	"errors"
)

// HeldForAutoHeal reads every local volume ownership source without waiting for a writer.
func (s *Service) HeldForAutoHeal(ctx context.Context, names []string) (bool, error) {
	for _, name := range names {
		if s.WriterFenced(name) {
			return true, nil
		}
		if s.deps.DB == nil || s.deps.Locks == nil {
			return true, errors.New("volume ownership unavailable")
		}
		held, err := s.deps.Engine.HeldForVolume(ctx, name)
		if err != nil || held {
			return true, err
		}
		unlock, ok := s.deps.Locks.TryLock(name)
		if !ok {
			return true, nil
		}
		unlock()
		var count int64
		if err := s.deps.DB.WithContext(ctx).Table("volume_backups").Where("volume_name=? AND status='running'", name).Count(&count).Error; err != nil {
			return true, err
		}
		if count > 0 {
			return true, nil
		}
		if err := s.deps.DB.WithContext(ctx).Table("volume_restore_runs").Where("status='running' AND (volume_name=? OR new_volume_name=?)", name, name).Count(&count).Error; err != nil {
			return true, err
		}
		if count > 0 {
			return true, nil
		}
	}
	return false, nil
}
