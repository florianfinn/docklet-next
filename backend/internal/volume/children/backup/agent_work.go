// Source: docklet-next/issues/368
// Revision: 32e24a1b5260c98b6ce490ce2c1f6f50413c1e83
// Retrieved: 2026-11-07
// License: Apache-2.0
// Deviations: Original read-only busy check for an agent self restart.

package backup

import (
	"context"
	"errors"
)

// WorkRunning reports local backup or restore work an agent restart would
// interrupt: running backup (including bind snapshot) and restore rows, which
// startup keeps while writer discovery is unconfirmed, and active writer fences.
func (s *Service) WorkRunning(ctx context.Context) (bool, error) {
	s.fenceMu.Lock()
	fenced := false
	for _, active := range s.writerFences {
		fenced = fenced || active
	}
	s.fenceMu.Unlock()
	if fenced {
		return true, nil
	}
	if s.deps.DB == nil {
		return true, errors.New("volume backup store unavailable")
	}
	var count int64
	if err := s.deps.DB.WithContext(ctx).Table("volume_backups").Where("status='running'").Count(&count).Error; err != nil || count > 0 {
		return true, err
	}
	if err := s.deps.DB.WithContext(ctx).Table("volume_restore_runs").Where("status='running'").Count(&count).Error; err != nil || count > 0 {
		return true, err
	}
	return false, nil
}
