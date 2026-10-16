// Source: docklet-next/docs/design/hub-backup.md
// Revision: 8f8c8f89bc6ebb7261862d0c35a6ddbc1513e9fa
// Retrieved: 2026-10-16
// License: Apache-2.0
// Deviations: Original coordinated SQLite backup and paired recovery for issue #32.

package backup

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"sync"

	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	backuptypes "github.com/florianfinn/docklet-next/types/backup"
)

func mustNewestMigration() int64 { version, _ := newestMigration(); return version }

func (s *Service) snapshotArchive(workCtx context.Context, activityID, id string) (string, int64, error) {
	ctx, cancel := context.WithTimeout(workCtx, archiveDeadline)
	defer cancel()
	if s.db == nil || s.db.HubLock() == nil {
		return "", 0, ErrSystemBackupUnavailable
	}
	if err := s.maintenance.Begin(activityID, "backup"); err != nil {
		return "", 0, err
	}
	defer s.maintenance.End()
	if err := s.maintenance.Wait(ctx); err != nil {
		return "", 0, archiveValidationError(ctx)
	}
	if s.coordinator != nil {
		defer s.coordinator.Resume()
		if err := s.coordinator.PauseAndDrain(ctx); err != nil {
			return "", 0, archiveValidationError(ctx)
		}
	}
	if err := s.activityService.CancelAndDrain(ctx, activityID); err != nil {
		return "", 0, archiveValidationError(ctx)
	}
	// Reserving both sole writers drains every transaction, including actor lease
	// renewals. No transaction or callback can write either file until release.
	pool, err := s.db.SQLDB()
	if err != nil {
		return "", 0, ErrSystemBackupUnavailable
	}
	hub, err := pool.Conn(ctx)
	if err != nil {
		return "", 0, archiveValidationError(ctx)
	}
	defer hub.Close()
	var sidecar *sql.Conn
	if s.runtime != nil {
		sidecar, err = s.runtime.ReserveWriter(ctx)
	} else {
		// The offline/test owner already holds the lifetime lock; no actor host exists.
		path := sqlitestore.FrancisPath(s.db.Path())
		if _, statErr := os.Stat(path); statErr != nil {
			return "", 0, ErrSystemBackupUnavailable
		}
		writer, openErr := sqlitestore.Open(ctx, path, false, false)
		if openErr != nil {
			return "", 0, ErrSystemBackupUnavailable
		}
		defer writer.Close()
		sidecar, err = writer.Conn(ctx)
	}
	if err != nil {
		return "", 0, archiveValidationError(ctx)
	}
	defer sidecar.Close()
	// Never allow archive storage to alias the directory holding the live pair.
	dir, err := filepath.EvalSymlinks(s.backupDir())
	if err != nil || dir == filepath.Dir(s.db.Path()) {
		return "", 0, ErrSystemBackupUnavailable
	}
	s.maintenance.SetPhase("applying")
	release := sync.OnceFunc(func() {
		_ = sidecar.Close()
		_ = hub.Close()
		if s.coordinator != nil {
			s.coordinator.Resume()
		}
		s.maintenance.End()
	})
	defer release()
	return createSQLiteArchive(ctx, dir, id, hub, sidecar, backuptypes.MaxSystemArchiveBytes, release)
}
