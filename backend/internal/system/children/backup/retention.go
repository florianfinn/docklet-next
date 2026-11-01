// Source: getarcaneapp/arcane/backend/internal/system/children/backup/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Replaced Rustic system recovery with coordinated SQLite archives; retained the separate volume-backup coordinator; notification and Auto Heal contracts and delivery for #296; M16 bind mounts and backup completion.

package backup

import (
	"context"
	"errors"
	backuptypes "github.com/florianfinn/docklet-next/types/backup"
	"log/slog"
	"os"
	"path/filepath"
)

func (s *Service) applyRetention(ctx context.Context, policyID string, keep int) error {
	if keep < 1 {
		return errors.New("backup.invalid_retention")
	}
	ids, err := s.store.ExpiredRunIDs(ctx, policyID, keep)
	if err != nil {
		return err
	}
	for _, id := range ids {
		name, err := backuptypes.ArchiveNameForID(id)
		if err != nil {
			return err
		}
		run, err := s.store.Run(ctx, id)
		if err != nil {
			return err
		}
		if run.Trigger != backuptypes.SystemBackupTriggerScheduled || run.PolicyID != policyID || run.ArchiveName != name {
			continue
		}
		if run.CopyState == backuptypes.SystemBackupCopySucceeded && run.CopyTargetID != "" {
			if copyErr := s.removeArchiveCopy(ctx, run); copyErr != nil {
				slog.WarnContext(ctx, "Hub archive copy retention failed", "backupId", run.ID, "code", "backup.copy.write-failed")
			}
		}
		if err := os.Remove(filepath.Join(s.backupDir(), name)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return errors.New("backup.retention_failed")
		}
		if err := s.store.DeleteRun(ctx, id); err != nil {
			return err
		}
	}
	return nil
}
