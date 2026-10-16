// Source: docklet-next/docs/design/hub-backup.md
// Revision: 73e62f9e35cd5ae59d1bb79802831f8204490866
// Retrieved: 2026-10-16
// License: Apache-2.0
// Deviations: Original live restore regression with pinned Hub and Francis WAL readers.

package system

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	backupservice "github.com/florianfinn/docklet-next/backend/internal/system/children/backup"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	backuptypes "github.com/florianfinn/docklet-next/types/backup"
	usertypes "github.com/florianfinn/docklet-next/types/user"
	"github.com/stretchr/testify/require"
)

func TestHubLiveRestoreBlocksOnUncheckpointedWAL(t *testing.T) {
	for _, hub := range []bool{true, false} {
		name := "francis"
		if hub {
			name = "hub"
		}
		t.Run(name, func(t *testing.T) {
			db, databaseURL := newMigratedHubBackupDBWithURL(t)
			path := db.Path()
			writer, err := db.SQLDB()
			require.NoError(t, err)
			if !hub {
				path = sqlitestore.FrancisPath(path)
				writer, err = sqlitestore.Open(t.Context(), path, false, false)
				require.NoError(t, err)
				defer writer.Close()
			}
			_, err = writer.Exec("CREATE TABLE restore_wal_probe(value TEXT)")
			require.NoError(t, err)
			system := &SystemService{db: db}
			service := backupservice.NewService(backupservice.Dependencies{
				Store: system.backupStoreInternal(), DB: db, Activity: activity.NewActivityService(db, nil),
				Config: &config.Config{DatabaseURL: databaseURL, BackupDir: t.TempDir()},
			})
			run, err := service.CreateBackup(t.Context(), usertypes.SystemUser, backuptypes.SystemBackupTriggerManual, backuptypes.CreateSystemBackupRequest{})
			require.NoError(t, err)

			// A second reader pins the pre-write snapshot, preventing TRUNCATE even
			// after the Hub and Francis writer pools have closed.
			reader, err := sqlitestore.Open(t.Context(), path, hub, true)
			require.NoError(t, err)
			defer reader.Close()
			tx, err := reader.BeginTx(t.Context(), &sql.TxOptions{ReadOnly: true})
			require.NoError(t, err)
			defer tx.Rollback()
			var count int
			require.NoError(t, tx.QueryRow("SELECT count(*) FROM restore_wal_probe").Scan(&count))
			_, err = writer.Exec("INSERT INTO restore_wal_probe VALUES('committed-after-archive')")
			require.NoError(t, err)
			if !hub {
				require.NoError(t, writer.Close())
			}
			wal, err := os.Stat(path + "-wal")
			require.NoError(t, err)
			require.Positive(t, wal.Size())
			blocked, restored := make(chan struct{}), make(chan struct{})
			service.SetExitUnready(func() { close(blocked) })
			service.SetRestoreSchedulerRefresh(func(context.Context) error { close(restored); return nil })
			_, err = service.StartRestore(t.Context(), usertypes.Actor{ID: "example-user"}, "", run.ID, run.ID)
			require.NoError(t, err)
			select {
			case <-blocked:
			case <-restored:
				t.Fatal("restore replaced files despite a pinned reader and committed WAL data")
			case <-time.After(15 * time.Second):
				t.Fatal("restore did not fail closed")
			}
			require.Equal(t, "blocked", service.MaintenanceGate().Status().Phase)
			_, err = os.Stat(db.Path() + ".restore/restore-state.json")
			require.True(t, os.IsNotExist(err), "replacement must not have started")
			stages, err := filepath.Glob(db.Path() + ".restore-*")
			require.NoError(t, err)
			require.Empty(t, stages)
			wal, err = os.Stat(path + "-wal")
			require.NoError(t, err)
			require.Positive(t, wal.Size())
			require.NoError(t, tx.Rollback())
			require.NoError(t, reader.QueryRow("SELECT count(*) FROM restore_wal_probe WHERE value='committed-after-archive'").Scan(&count))
			require.Equal(t, 1, count)
		})
	}
}
