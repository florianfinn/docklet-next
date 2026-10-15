//go:build postgres_integration

// Source: getarcaneapp/arcane/backend/internal/volume/children/backup/service_test.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed S3 backup destinations; functional cut: removed SQLite runtime and migrations; functional cut: PostgreSQL integration test fixture.

package backup

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/testpg"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	backuptypes "github.com/florianfinn/docklet-next/types/backup"
	"github.com/florianfinn/docklet-next/types/scheduler"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/florianfinn/docklet-next/types/volume"
	"github.com/stretchr/testify/require"
	"go.getarcane.app/kit/pkg"
	"go.getarcane.app/sys/crypto"
	"gorm.io/gorm"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/backup"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/volume/children/backup/children/policies"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/runs"
	francistest "github.com/florianfinn/docklet-next/backend/pkg/utils/francis/testing"
)

type volumeBackupTestRow struct {
	ID              string `gorm:"primaryKey"`
	VolumeName      string
	Size            int64
	CreatedAt       time.Time
	PolicyID        string
	Status          string
	LocalSnapshotID string
}

func (volumeBackupTestRow) TableName() string { return "volume_backups" }

func applyVolumeBackupMigrationsInternal(t *testing.T, gormDB *gorm.DB) {
	t.Helper()
	require.NoError(t, gormDB.AutoMigrate(&volumeBackupTestRow{}, &policies.VolumeBackupPolicy{}))
}

func newVolumeAdmissionForTestInternal(t testing.TB) *runs.Admission {
	t.Helper()
	runtime := francistest.New(t)
	gate := runs.NewAdmission(runtime.Service(), t.Name())
	require.NoError(t, gate.Register(runtime))
	francistest.Start(t, runtime)
	return gate
}

type volumeBackupPolicySchedulerInternal struct {
	jobs map[string]scheduler.Job
}

func (s *volumeBackupPolicySchedulerInternal) AddJob(_ context.Context, job scheduler.Job) error {
	s.jobs[job.Name()] = job
	return nil
}

func (s *volumeBackupPolicySchedulerInternal) RemoveJob(_ context.Context, name string) {
	delete(s.jobs, name)
}

func (s *volumeBackupPolicySchedulerInternal) HasJob(name string) bool {
	_, ok := s.jobs[name]
	return ok
}

func (s *volumeBackupPolicySchedulerInternal) Submit(_ context.Context, request scheduler.Request) (scheduler.Run, error) {
	return scheduler.Run{ID: request.RunID, JobID: request.JobID, EnvironmentID: request.EnvironmentID, Status: scheduler.Queued}, nil
}

func TestVolumeBackupPolicy_RetentionIgnoresFailedRuns(t *testing.T) {
	gormDB, err := gorm.Open(testpg.Dialector(t, "file:volume-backup-retention-failed?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	applyVolumeBackupMigrationsInternal(t, gormDB)

	policyID := "policy-1"
	require.NoError(t, gormDB.Exec(
		"INSERT INTO volume_backups (id, volume_name, size, created_at, policy_id, status, local_snapshot_id) VALUES (?, ?, ?, ?, ?, ?, ?)",
		"succeeded-run", "app-data", 0, time.Now().Add(-time.Hour), policyID, backuptypes.VolumeBackupStatusSucceeded, "snapshot-1",
	).Error)
	require.NoError(t, gormDB.Exec(
		"INSERT INTO volume_backups (id, volume_name, size, created_at, policy_id, status) VALUES (?, ?, ?, ?, ?, ?)",
		"failed-run", "app-data", 0, time.Now(), policyID, backuptypes.VolumeBackupStatusFailed,
	).Error)

	service := NewService(Dependencies{DB: &database.DB{DB: gormDB}})
	require.NoError(t, service.applyVolumeBackupRetentionInternal(t.Context(), policyID, 1))

	var backups []volume.Backup
	require.NoError(t, gormDB.Table("volume_backups").Order("created_at ASC").Find(&backups).Error)
	require.Len(t, backups, 2)
	require.Equal(t, "snapshot-1", backups[0].LocalSnapshotID)
	require.Equal(t, backuptypes.VolumeBackupStatusFailed, backups[1].Status)
}

func TestVolumeBackupPolicy_ScheduledRunCreatesActivity(t *testing.T) {
	gormDB, err := gorm.Open(testpg.Dialector(t, "file:volume-backup-scheduled-activity?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, gormDB.AutoMigrate(&policies.VolumeBackupPolicy{}))
	require.NoError(t, gormDB.AutoMigrate(activity.Models()...))
	db := &database.DB{DB: gormDB}
	policy := &policies.VolumeBackupPolicy{
		VolumeName:     "app-data",
		Enabled:        true,
		Schedule:       "0 0 2 * * *",
		RetentionCount: 7,
		LocalEnabled:   true,
	}
	require.NoError(t, gormDB.Create(policy).Error)
	gate := newVolumeAdmissionForTestInternal(t)
	engine := backup.NewEngine(t.Context(), gate, nil)
	t.Cleanup(func() { require.NoError(t, engine.Stop(context.WithoutCancel(t.Context()))) })
	errAlreadyRunning := errors.New("a backup is already running for this volume")
	service := NewService(Dependencies{
		DB:             db,
		Activity:       activity.NewActivityService(db, nil),
		Engine:         engine,
		AlreadyRunning: errAlreadyRunning,
	})
	jobScheduler := &volumeBackupPolicySchedulerInternal{jobs: make(map[string]scheduler.Job)}
	require.NoError(t, service.SetScheduler(t.Context(), jobScheduler, gate))
	service.RegisterJobsOnStartup(t.Context())
	job, registered := jobScheduler.jobs["volume-backup:"+policy.ID]
	require.True(t, registered)
	// Holding the volume's admission lease makes the scheduled run fail with
	// "already running", which still must record a failed activity.
	lease, admitted, err := engine.TryAcquireRun(t.Context(), backup.VolumeAdmissionScope, policy.VolumeName)
	require.NoError(t, err)
	require.True(t, admitted)
	defer lease.Release(t.Context())

	outcome, scheduledBackupErr := job.Run(t.Context())
	require.NoError(t, scheduledBackupErr)
	require.Equal(t, scheduler.Skipped, outcome.Status)

	var backupActivity activity.Activity
	require.NoError(t, gormDB.Where("resource_type = ?", "volume_backup").First(&backupActivity).Error)
	require.Equal(t, activitytypes.StatusFailed, backupActivity.Status)
	require.Equal(t, "scheduled_volume_backup", backupActivity.Metadata["action"])
	require.Equal(t, policy.Schedule, backupActivity.Metadata["schedule"])
}

func TestVolumeBackup_ValidationErrors(t *testing.T) {
	gormDB, err := gorm.Open(testpg.Dialector(t, "file:volume-backup-validation?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, gormDB.AutoMigrate(&policies.VolumeBackupPolicy{}))
	service := NewService(Dependencies{DB: &database.DB{DB: gormDB}})
	ctx := t.Context()

	_, err = service.policies.UpdateBackupPolicies(ctx, "app-data", []volume.UpdateBackupPolicy{{Schedule: "not a cron", LocalEnabled: true}})
	require.ErrorContains(t, err, "invalid volume backup schedule")

	_, err = service.policies.UpdateBackupPolicies(ctx, "app-data", []volume.UpdateBackupPolicy{{Schedule: "0 0 2 * * *", RetentionCount: 7}})
	require.ErrorContains(t, err, "select a local volume backup destination")

	_, err = service.StartBackup(ctx, "0", "app-data", user.Actor{}, volume.CreateBackupRequest{Destination: volume.BackupDestination("invalid")})
	require.EqualError(t, err, "invalid volume backup destination")

}

func TestVolumeBackupPasswordPrefersRecoveryKey(t *testing.T) {
	gormDB, err := gorm.Open(testpg.Dialector(t, "file:volume-backup-password?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, gormDB.AutoMigrate(&backup.SystemBackupRecoveryConfig{}))
	crypto.InitEncryption(&crypto.Config{EncryptionKey: "volume-backup-password-test-key-32bytes", Environment: "test"})
	recoveryKeys := backup.NewRecoveryKeyStore(&database.DB{DB: gormDB})
	service := NewService(Dependencies{
		DB:            &database.DB{DB: gormDB},
		EncryptionKey: "legacy-instance-secret",
		RecoveryKeys:  recoveryKeys,
	})

	password, err := service.volumeBackupPasswordInternal(t.Context(), nil)
	require.NoError(t, err)
	require.Equal(t, kit.SHA256Hex(legacyVolumePasswordSaltInternal+service.deps.EncryptionKey), password)

	recoveryKey := "QWERTY-ABCDEF-234567-GHIJKL-MNOPQR-STUVWX-YZ2345-ZXCVBN"
	require.NoError(t, recoveryKeys.Set(t.Context(), recoveryKey))
	password, err = service.volumeBackupPasswordInternal(t.Context(), nil)
	require.NoError(t, err)
	require.Equal(t, recoveryKey, password)
}
