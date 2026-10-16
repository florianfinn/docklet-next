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
	"sync"
	"time"

	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	backuptypes "github.com/florianfinn/docklet-next/types/backup"
)

const snapshotDeadline = 30 * time.Second

func mustNewestMigration() int64 { version, _ := newestMigration(); return version }

func (s *Service) snapshotArchive(workCtx context.Context, activityID, id string) (string, int64, error) {
	ctx, cancel := context.WithTimeout(workCtx, archiveDeadline)
	defer cancel()
	if s.db == nil || s.db.HubLock() == nil {
		return "", 0, ErrSystemBackupUnavailable
	}
	if err := s.databaseProviderError(); err != nil {
		return "", 0, err
	}
	if err := s.maintenance.Begin(activityID, "backup"); err != nil {
		return "", 0, err
	}
	var hub, sidecar *sql.Conn
	var writer *sql.DB
	paused := false
	release := sync.OnceFunc(func() {
		if sidecar != nil {
			_ = sidecar.Close()
		}
		if hub != nil {
			_ = hub.Close()
		}
		if writer != nil {
			_ = writer.Close()
		}
		if paused {
			s.coordinator.Resume()
		}
		s.maintenance.End()
	})
	defer release()
	if err := s.maintenance.Wait(ctx); err != nil {
		return "", 0, archiveValidationError(ctx)
	}
	if s.coordinator != nil {
		paused = true
		if err := s.coordinator.PauseAndDrain(ctx); err != nil {
			return "", 0, archiveValidationError(ctx)
		}
	}
	if err := s.activityService.CancelAndDrain(ctx, activityID); err != nil {
		return "", 0, archiveValidationError(ctx)
	}
	// Reserve Hub then Francis under one deadline, including time waiting for
	// transactions. Both remain reserved until the two durable images exist.
	snapshotCtx, stopSnapshot := context.WithTimeout(ctx, snapshotDeadline)
	defer stopSnapshot()
	pool, err := s.db.SQLDB()
	if err != nil {
		return "", 0, ErrSystemBackupUnavailable
	}
	hub, err = pool.Conn(snapshotCtx)
	if err != nil {
		return "", 0, archiveValidationError(snapshotCtx)
	}
	if s.runtime != nil {
		sidecar, err = s.runtime.ReserveWriter(snapshotCtx)
	} else {
		// The offline/test owner already holds the lifetime lock; no actor host exists.
		path := sqlitestore.FrancisPath(s.db.Path())
		if _, statErr := os.Stat(path); statErr != nil {
			return "", 0, ErrSystemBackupUnavailable
		}
		writer, err = sqlitestore.Open(snapshotCtx, path, false, false)
		if err != nil {
			return "", 0, archiveValidationError(snapshotCtx)
		}
		sidecar, err = writer.Conn(snapshotCtx)
	}
	if err != nil {
		return "", 0, archiveValidationError(snapshotCtx)
	}
	s.maintenance.SetPhase("applying")
	return createSQLiteArchive(ctx, snapshotCtx, s.backupDir(), id, hub, sidecar, backuptypes.MaxSystemArchiveBytes, release)
}
