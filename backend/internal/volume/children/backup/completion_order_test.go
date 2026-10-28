// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original asynchronous worker release ordering at actual activity terminal persistence.

package backup

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/backup"
	"github.com/florianfinn/docklet-next/backend/internal/event"
	"github.com/florianfinn/docklet-next/backend/internal/volume/children/backup/children/policies"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/florianfinn/docklet-next/types/volume"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestAsyncWorkersReleaseAdmissionBeforeActivityTerminalCommit(t *testing.T) {
	for _, kind := range []string{"capture", "repository-check"} {
		t.Run(kind, func(t *testing.T) {
			f := newRuntimePlan(t)
			db := f.service.deps.DB
			require.NoError(t, db.AutoMigrate(activity.Models()...))
			require.NoError(t, db.AutoMigrate(&event.Event{}, &policies.VolumeBackupPolicy{}))
			f.service.deps.Activity = activity.NewActivityService(db, nil)
			f.service.deps.Events = event.NewEventService(db, nil, nil)
			f.service.deps.Store.Create = func(ctx context.Context, row *volume.Backup) error {
				return db.WithContext(ctx).Table("volume_backups").Create(row).Error
			}
			if kind == "capture" {
				f.stdout = `{"id":"` + strings.Repeat("a", 64) + `"}`
			} else {
				f.service.deps.Exec = func(context.Context, string, string, []string) (string, string, error) { return "empty", "", nil }
				f.service.deps.HelperImage = func(context.Context, *client.Client) (string, error) { return "example/tools:1.0.0", nil }
			}
			f.service = NewService(f.service.deps)
			defer f.service.deps.Activity.CancelAndDrain(t.Context(), "")
			observed := make(chan bool, 1)
			require.NoError(t, db.Callback().Update().Before("gorm:update").Register("test:release-before-terminal", func(tx *gorm.DB) {
				if tx.Statement.Table != "activities" {
					return
				}
				fields, ok := tx.Statement.Dest.(map[string]any)
				if !ok || fields["ended_at"] == nil {
					return
				}
				scope := "data"
				unlocked := true
				if kind == "capture" {
					release, ok := f.service.deps.Locks.TryLock(scope)
					unlocked = ok
					if ok {
						release()
					}
				} else {
					scope = "repository:local"
				}
				lease, acquired, err := f.service.deps.Engine.TryAcquireRun(t.Context(), backup.VolumeAdmissionScope, scope)
				if acquired {
					lease.Release(t.Context())
				}
				observed <- unlocked && acquired && err == nil
			}))
			ctx := context.WithValue(t.Context(), user.CurrentUserContextKey{}, &user.SystemUser)
			if kind == "capture" {
				_, err := f.service.StartBackup(ctx, "0", "data", user.SystemUser, volume.CreateBackupRequest{})
				require.NoError(t, err)
			} else {
				_, err := TargetHandler(f.service, nil, nil, t.Context()).Test(ctx, "local", "0")
				require.NoError(t, err)
			}
			select {
			case released := <-observed:
				require.True(t, released, "terminal activity cannot precede workspace/durable release")
			case <-time.After(3 * time.Second):
				t.Fatal("worker did not commit terminal activity")
			}
			require.NoError(t, f.service.deps.Activity.CancelAndDrain(t.Context(), ""))
		})
	}
}
