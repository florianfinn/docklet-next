// Source: docklet-next/issues/676
// Revision: 9590d35e3340294b3d329053d5846656b5791371
// Retrieved: 2026-11-27
// License: Apache-2.0
// Deviations: Original scheduler regression coverage.

package policies

import (
	"context"
	"errors"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/testsqlite"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/jobcontext"
	"github.com/florianfinn/docklet-next/types/scheduler"
	"github.com/florianfinn/docklet-next/types/volume"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"testing"
)

func TestScheduledBackupAdmissionPrecedesProgressAndRemainsHeld(t *testing.T) {
	db, err := gorm.Open(testsqlite.Dialector(t, ":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&VolumeBackupPolicy{}))
	policy := VolumeBackupPolicy{VolumeName: "data", Enabled: true, Schedule: "0 0 2 * * *", LocalEnabled: true}
	require.NoError(t, db.Create(&policy).Error)
	busy := errors.New("busy")
	held := true
	creates := 0
	progress := 0
	s := NewService(Dependencies{DB: &database.DB{DB: db}, AlreadyRunning: busy,
		Acquire: func(ctx context.Context, _ string) (context.Context, func(), error) {
			if held {
				return ctx, nil, busy
			}
			held = true
			return ctx, func() { held = false }, nil
		},
		CreateBackup: func(context.Context, string, string) (*volume.Backup, error) {
			require.True(t, held)
			creates++
			return &volume.Backup{ID: "backup"}, nil
		},
	})
	ctx := jobcontext.WithExecution(t.Context(), scheduler.Run{}, func(scheduler.TargetOutcome) error { progress++; return nil })
	outcome, err := s.runScheduledBackupInternal(ctx, policy.ID)
	require.NoError(t, err)
	require.Equal(t, scheduler.Waiting, outcome.Status)
	require.Equal(t, "run.busy", outcome.Code)
	require.Zero(t, creates)
	require.Zero(t, progress)
	held = false
	outcome, err = s.runScheduledBackupInternal(ctx, policy.ID)
	require.NoError(t, err)
	require.Equal(t, scheduler.Succeeded, outcome.Status)
	require.Equal(t, 1, creates)
	require.False(t, held)
}
