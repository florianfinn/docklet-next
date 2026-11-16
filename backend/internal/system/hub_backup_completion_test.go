// Source: docklet-next/issues/467
// Revision: b4f894ff7f35186d4c39be1fc9fccd01b4b5934a
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Original hub backup completion ordering, write-failure, retention and restart proofs.

package system

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	backupservice "github.com/florianfinn/docklet-next/backend/internal/system/children/backup"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	dto "github.com/florianfinn/docklet-next/types/backup"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type hubBackupRig struct {
	db         *database.DB
	system     *SystemService
	activities *activity.ActivityService
	store      backupservice.Store
	deps       backupservice.Dependencies
	dir        string
}

func newHubBackupRig(t *testing.T) *hubBackupRig {
	db, dsn := newMigratedHubBackupDBWithURL(t)
	system := &SystemService{db: db}
	activities := activity.NewActivityService(db, nil)
	store := system.backupStoreInternal()
	dir := t.TempDir()
	deps := backupservice.Dependencies{DB: db, Store: store, Activity: activities, Config: &config.Config{DatabaseURL: dsn, BackupDir: dir}}
	return &hubBackupRig{db: db, system: system, activities: activities, store: store, deps: deps, dir: dir}
}

func (r *hubBackupRig) service() *backupservice.Service {
	r.deps.Store = r.store
	return backupservice.NewService(r.deps)
}

func (r *hubBackupRig) refuseTerminalWrite(t *testing.T) {
	require.NoError(t, r.db.Callback().Update().Before("gorm:update").Register("test:refuse-completion", func(tx *gorm.DB) {
		fields, ok := tx.Statement.Dest.(map[string]any)
		if tx.Statement.Table == "activities" && ok && fields["ended_at"] != nil {
			tx.AddError(errors.New("controlled terminal write failure"))
		}
	}))
}

// onSucceededWrite calls observe after every write that persists a succeeded backup row.
func (r *hubBackupRig) onSucceededWrite(t *testing.T, observe func()) {
	callback := func(tx *gorm.DB) {
		if run, ok := tx.Statement.Dest.(*SystemBackupRun); ok && tx.Error == nil && run.Status == SystemBackupStatusSucceeded {
			observe()
		}
	}
	require.NoError(t, r.db.Callback().Update().After("gorm:update").Register("test:observe-success-update", callback))
	require.NoError(t, r.db.Callback().Create().After("gorm:create").Register("test:observe-success-create", callback))
}

func (r *hubBackupRig) uploadArchive(t *testing.T) []byte {
	seed, err := r.service().CreateBackup(t.Context(), user.SystemUser, dto.SystemBackupTriggerManual, dto.CreateSystemBackupRequest{})
	require.NoError(t, err)
	reader, _, _, err := r.service().OpenArchive(t.Context(), seed.ID)
	require.NoError(t, err)
	archive, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.NoError(t, reader.Close())
	return archive
}

// run performs the operation; upload needs an archive prepared before observers are registered.
func (r *hubBackupRig) run(t *testing.T, operation string, archive []byte) (*dto.SystemBackupRun, error) {
	switch operation {
	case "create":
		return r.service().CreateBackup(t.Context(), user.SystemUser, dto.SystemBackupTriggerManual, dto.CreateSystemBackupRequest{})
	case "scheduled":
		return r.service().CreateBackup(t.Context(), user.SystemUser, dto.SystemBackupTriggerScheduled, dto.CreateSystemBackupRequest{PolicyID: "daily"})
	default:
		answer, err := r.service().UploadArchive(t.Context(), user.SystemUser, bytes.NewReader(archive))
		if answer == nil {
			return nil, err
		}
		return &dto.SystemBackupRun{ID: answer.BackupID, ActivityID: answer.ActivityID}, err
	}
}

// A client that sees the succeeded row and starts the next gated action at once must be admitted.
func TestHubBackupSuccessFollowsGateRelease(t *testing.T) {
	for _, operation := range []string{"create", "upload"} {
		t.Run(operation, func(t *testing.T) {
			r := newHubBackupRig(t)
			var archive []byte
			if operation == "upload" {
				archive = r.uploadArchive(t)
			}
			var mu sync.Mutex
			var admissions sync.WaitGroup
			var results []error
			r.onSucceededWrite(t, func() {
				done := make(chan struct{})
				admissions.Add(1)
				go func() {
					defer admissions.Done()
					defer close(done)
					gate, err := r.activities.StartGatedActivity(t.Context(), activity.GateRequest{EnvironmentID: "0", Operation: "next:example", Scope: "system-backup", Type: activitytypes.TypeResourceAction})
					if err == nil {
						_, err = r.activities.CompleteActivity(gate.WorkContext, gate.Run.ID, activitytypes.StatusSuccess, "done", nil)
					}
					mu.Lock()
					results = append(results, err)
					mu.Unlock()
				}()
				// Give an admission that does not wait for the backup gate time to answer before the backup proceeds.
				select {
				case <-done:
				case <-time.After(200 * time.Millisecond):
				}
			})
			_, err := r.run(t, operation, archive)
			require.NoError(t, err)
			admissions.Wait()
			require.NotEmpty(t, results, "no succeeded backup write observed")
			for _, err := range results {
				require.NoError(t, err, "a succeeded backup must admit the next gated action immediately")
			}
		})
	}
}

// A run whose success does not commit never persists success, keeps no archive and costs no retained archive.
func TestHubBackupFailedCompletionKeepsRetention(t *testing.T) {
	for _, failure := range []string{"terminal-write", "commit-hook", "none"} {
		t.Run(failure, func(t *testing.T) {
			r := newHubBackupRig(t)
			require.NoError(t, r.db.Create(&SystemBackupPolicy{BaseModel: database.BaseModel{ID: "daily"}, Schedule: "0 0 3 * * *", RetentionCount: 1}).Error)
			old := "old-example"
			require.NoError(t, r.db.Create(&SystemBackupRun{BaseModel: database.BaseModel{ID: old}, CreatedAt: time.Now().UTC().Add(-time.Hour),
				Status: SystemBackupStatusSucceeded, Trigger: SystemBackupTriggerScheduled, PolicyID: "daily",
				ArchiveName: new(old + ".tar"), CopyState: dto.SystemBackupCopyNone}).Error)
			require.NoError(t, os.WriteFile(filepath.Join(r.dir, old+".tar"), []byte("archive"), 0o600))
			successes := 0
			r.onSucceededWrite(t, func() { successes++ })
			switch failure {
			case "terminal-write":
				r.refuseTerminalWrite(t)
			case "commit-hook":
				commit := r.store.CommitRunTx
				r.store.CommitRunTx = func(tx *gorm.DB, run *dto.SystemBackupRun, expired []string) error {
					if run.Status == dto.SystemBackupStatusSucceeded {
						return errors.New("controlled commit failure")
					}
					return commit(tx, run, expired)
				}
			}
			run, err := r.run(t, "scheduled", nil)
			require.NotNil(t, run)
			stored, loadErr := r.store.Run(t.Context(), run.ID)
			require.NoError(t, loadErr)
			_, oldRowErr := r.store.Run(t.Context(), old)
			_, oldFileErr := os.Stat(filepath.Join(r.dir, old+".tar"))
			if failure == "none" {
				require.NoError(t, err)
				require.Equal(t, 1, successes)
				require.Equal(t, dto.SystemBackupStatusSucceeded, stored.Status)
				require.ErrorIs(t, oldRowErr, gorm.ErrRecordNotFound, "success prunes the expired row in its commit")
				require.ErrorIs(t, oldFileErr, os.ErrNotExist)
				return
			}
			require.Error(t, err)
			require.Zero(t, successes, "a run whose success did not commit must never persist succeeded")
			require.Equal(t, dto.SystemBackupStatusFailed, stored.Status)
			require.NoError(t, oldRowErr, "a failed completion must keep the retained row")
			require.NoError(t, oldFileErr, "a failed completion must keep the retained archive")
			_, currentErr := os.Stat(filepath.Join(r.dir, stored.ArchiveName))
			require.ErrorIs(t, currentErr, os.ErrNotExist, "the failed run must not keep its archive")
			if failure == "commit-hook" {
				detail, detailErr := r.activities.GetActivityDetail(t.Context(), "0", run.ActivityID, 20)
				require.NoError(t, detailErr)
				require.Equal(t, activitytypes.StatusFailed, detail.Activity.Status)
			}
		})
	}
}

// A process that stops after validating an archive keeps that archive across the restart.
func TestHubBackupRestartKeepsValidatedArchive(t *testing.T) {
	r := newHubBackupRig(t)
	r.system.backup = r.service()
	validated, pending := "validated-example", "pending-example"
	for _, row := range []struct {
		id      string
		archive *string
	}{{validated, new(validated + ".tar")}, {pending, nil}} {
		require.NoError(t, r.db.Create(&SystemBackupRun{BaseModel: database.BaseModel{ID: row.id}, CreatedAt: time.Now().UTC(),
			Status: SystemBackupStatusRunning, Trigger: SystemBackupTriggerManual, ArchiveName: row.archive, CopyState: dto.SystemBackupCopyPending}).Error)
	}
	content := []byte("validated archive")
	require.NoError(t, os.WriteFile(filepath.Join(r.dir, validated+".tar"), content, 0o600))
	require.NoError(t, r.db.Model(&SystemBackupRun{}).Where("id = ?", validated).Update("size", len(content)).Error)
	require.NoError(t, r.system.ReconcileInterruptedBackups(t.Context()))
	kept, err := r.store.Run(t.Context(), validated)
	require.NoError(t, err)
	require.Equal(t, dto.SystemBackupStatusSucceeded, kept.Status)
	require.Equal(t, dto.SystemBackupCopyFailed, kept.CopyState)
	_, err = os.Stat(filepath.Join(r.dir, validated+".tar"))
	require.NoError(t, err, "startup must keep a validated archive")
	failed, err := r.store.Run(t.Context(), pending)
	require.NoError(t, err)
	require.Equal(t, dto.SystemBackupStatusFailed, failed.Status)
	require.Equal(t, dto.SystemBackupErrorInterrupted, failed.ErrorCode)
}
