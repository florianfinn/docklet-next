// Source: docklet-next/docs/design/hub-backup.md
// Revision: 8f8c8f89bc6ebb7261862d0c35a6ddbc1513e9fa
// Retrieved: 2026-10-15
// License: Apache-2.0
// Deviations: Original startup storage reconciliation for issue #32.

package backup

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/florianfinn/docklet-next/backend/pkg/pagination"
	backuptypes "github.com/florianfinn/docklet-next/types/backup"
	"gorm.io/gorm"
)

// ReconcileStorage removes incomplete files and records left by a crashed process.
func (s *Service) ReconcileStorage(ctx context.Context) error {
	dir := s.backupDir()
	if err := ensureArchiveDir(dir); err != nil {
		return ErrSystemBackupUnavailable
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ErrSystemBackupUnavailable
	}
	for _, entry := range entries {
		if !entry.Type().IsRegular() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, ".partial-") {
			if err := os.Remove(filepath.Join(dir, name)); err != nil {
				return ErrSystemBackupUnavailable
			}
			continue
		}
		if !strings.HasSuffix(name, ".dump") {
			continue
		}
		id := strings.TrimSuffix(name, ".dump")
		expected, err := backuptypes.ArchiveNameForID(id)
		if err != nil || expected != name {
			continue
		}
		run, err := s.store.Run(ctx, id)
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if errors.Is(err, gorm.ErrRecordNotFound) || (run != nil && (run.Status != backuptypes.SystemBackupStatusSucceeded || run.ArchiveName != name)) {
			if removeErr := os.Remove(filepath.Join(dir, name)); removeErr != nil {
				return ErrSystemBackupUnavailable
			}
		}
	}
	// A deletion interrupted after unlink leaves a metadata row without a file.
	if s.store.ListRuns != nil {
		var completed []backuptypes.SystemBackupRun
		for start := 0; ; start += 100 {
			rows, _, err := s.store.ListRuns(ctx, pagination.QueryParams{
				SortParams: pagination.SortParams{Sort: "createdAt", Order: pagination.SortDesc},
				Params:     pagination.Params{Start: start, Limit: 100},
			})
			if err != nil {
				return err
			}
			completed = append(completed, rows...)
			if len(rows) < 100 {
				break
			}
		}
		for _, run := range completed {
			if run.Status != backuptypes.SystemBackupStatusSucceeded || run.ArchiveName == "" {
				continue
			}
			name, err := backuptypes.ArchiveNameForID(run.ID)
			if err != nil || name != run.ArchiveName {
				slog.WarnContext(ctx, "Hub backup archive is inconsistent", "code", "backup.invalid_archive", "backupId", run.ID)
				if err := s.store.FailRun(ctx, run.ID, "backup.invalid_archive"); err != nil {
					return err
				}
				continue
			}
			info, err := os.Lstat(filepath.Join(dir, name))
			if errors.Is(err, os.ErrNotExist) {
				if err := s.store.DeleteRun(ctx, run.ID); err != nil {
					return err
				}
				continue
			}
			if err != nil {
				return ErrSystemBackupUnavailable
			}
			if !info.Mode().IsRegular() || info.Size() != run.Size {
				slog.WarnContext(ctx, "Hub backup archive is inconsistent", "code", "backup.invalid_archive", "backupId", run.ID)
				if err := s.store.FailRun(ctx, run.ID, "backup.invalid_archive"); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
