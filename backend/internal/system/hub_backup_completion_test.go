// Source: docklet-next/issues/467
// Revision: b4f894ff7f35186d4c39be1fc9fccd01b4b5934a
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Original hub backup completion ordering and write-failure proofs.

package system

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strconv"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	backupservice "github.com/florianfinn/docklet-next/backend/internal/system/children/backup"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	dto "github.com/florianfinn/docklet-next/types/backup"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestHubBackupSuccessFollowsGateRelease(t *testing.T) {
	for _, operation := range []string{"create", "upload"} {
		for _, completionFails := range []bool{false, true} {
			t.Run(operation+"/completion-fails="+strconv.FormatBool(completionFails), func(t *testing.T) {
				db, dsn := newMigratedHubBackupDBWithURL(t)
				owner := &SystemService{db: db}
				activities := activity.NewActivityService(db, nil)
				store := owner.backupStoreInternal()
				deps := backupservice.Dependencies{DB: db, Store: store, Activity: activities, Config: &config.Config{DatabaseURL: dsn, BackupDir: t.TempDir()}}
				service := backupservice.NewService(deps)
				var archive []byte
				if operation == "upload" {
					seed, err := service.CreateBackup(t.Context(), user.SystemUser, dto.SystemBackupTriggerManual, dto.CreateSystemBackupRequest{})
					require.NoError(t, err)
					reader, _, _, err := service.OpenArchive(t.Context(), seed.ID)
					require.NoError(t, err)
					archive, err = io.ReadAll(reader)
					require.NoError(t, err)
					require.NoError(t, reader.Close())
				}
				if completionFails {
					require.NoError(t, db.Callback().Update().Before("gorm:update").Register("test:refuse-completion", func(tx *gorm.DB) {
						fields, ok := tx.Statement.Dest.(map[string]any)
						if tx.Statement.Table == "activities" && ok && fields["ended_at"] != nil {
							tx.AddError(errors.New("controlled terminal write failure"))
						}
					}))
				}
				successWrites := 0
				save := store.SaveRun
				store.SaveRun = func(ctx context.Context, run *dto.SystemBackupRun) error {
					if err := save(ctx, run); err != nil {
						return err
					}
					if run.Status != dto.SystemBackupStatusSucceeded {
						return nil
					}
					successWrites++
					if completionFails {
						return nil
					}
					gate, err := activities.StartGatedActivity(t.Context(), activity.GateRequest{EnvironmentID: "0", Operation: "next:example", Scope: "system-backup", Type: activitytypes.TypeResourceAction})
					require.NoError(t, err, "a succeeded backup must allow the next gated action immediately")
					_, err = activities.CompleteActivity(gate.WorkContext, gate.Run.ID, activitytypes.StatusSuccess, "done", nil)
					require.NoError(t, err)
					return nil
				}
				deps.Store = store
				service = backupservice.NewService(deps)
				var err error
				if operation == "create" {
					_, err = service.CreateBackup(t.Context(), user.SystemUser, dto.SystemBackupTriggerManual, dto.CreateSystemBackupRequest{})
				} else {
					_, err = service.UploadArchive(t.Context(), user.SystemUser, bytes.NewReader(archive))
				}
				if completionFails {
					require.Error(t, err)
					require.Zero(t, successWrites, "failed completion must never persist a succeeded backup")
				} else {
					require.NoError(t, err)
					require.Equal(t, 1, successWrites)
				}
			})
		}
	}
}
