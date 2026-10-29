// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original per-actor durable and workspace contention and scheduler projection proofs.

package backup

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/florianfinn/docklet-next/backend/internal/backup"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/internal/upload"
	"github.com/florianfinn/docklet-next/backend/internal/volume/children/backup/children/policies"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/types/scheduler"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/florianfinn/docklet-next/types/volume"
	"github.com/stretchr/testify/require"
)

type backupSchedulerProof struct {
	jobs map[string]scheduler.Job
	next time.Time
}

func (s *backupSchedulerProof) AddJob(_ context.Context, j scheduler.Job) error {
	s.jobs[j.Name()] = j
	return nil
}
func (s *backupSchedulerProof) RemoveJob(_ context.Context, name string) { delete(s.jobs, name) }
func (s *backupSchedulerProof) HasJob(name string) bool                  { _, ok := s.jobs[name]; return ok }
func (s *backupSchedulerProof) Submit(context.Context, scheduler.Request) (scheduler.Run, error) {
	return scheduler.Run{}, nil
}
func (s *backupSchedulerProof) GetJobRuntimeState(string) (scheduler.JobRuntimeState, bool) {
	return scheduler.JobRuntimeState{Scheduled: true, NextRun: &s.next}, true
}

func TestEveryBackupActorRefusesBothVolumeOwnershipBoundaries(t *testing.T) {
	for _, mode := range []string{"hub", "direct", "edge"} {
		for _, boundary := range []string{"durable", "workspace"} {
			t.Run(mode+"/"+boundary, func(t *testing.T) {
				t.Setenv("TMPDIR", t.TempDir())
				f := newRuntimePlan(t)
				f.volumeState = map[string]map[string]string{"data": {}}
				require.NoError(t, f.service.deps.DB.AutoMigrate(&policies.VolumeBackupPolicy{}))
				f.service.deps.AlreadyRunning = errors.New("backup is busy")
				f.service.deps.Store.Run = func(context.Context, string) (*volume.Backup, error) {
					return &volume.Backup{ID: "row", VolumeName: "data", Format: "rustic", LocalSnapshotID: strings.Repeat("a", 64), Size: 1}, nil
				}
				f.service.deps.Store.Latest = func(context.Context, string) (*volume.Backup, error) { return nil, nil }
				f.service = NewService(f.service.deps)
				jobs := &backupSchedulerProof{jobs: map[string]scheduler.Job{}}
				require.NoError(t, f.service.SetScheduler(t.Context(), jobs, newVolumeAdmissionForTestInternal(t)))
				_, err := f.service.policies.UpdateBackupPolicies(t.Context(), "data", []volume.UpdateBackupPolicy{{Enabled: true, LocalEnabled: true, Schedule: "0 0 2 * * *"}})
				require.NoError(t, err)
				if boundary == "durable" {
					lease, ok, err := f.service.deps.Engine.TryAcquireRun(t.Context(), backup.VolumeAdmissionScope, "data")
					require.NoError(t, err)
					require.True(t, ok)
					defer lease.Release(t.Context())
				} else {
					defer f.service.deps.Locks.Lock("data")()
				}
				uploads := upload.NewUploadService(nil)
				router, rawAPI := humatest.New(t)
				api := huma.NewGroup(rawAPI, "/api")
				api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
					request := context.WithValue(ctx.Context(), middleware.ContextKeyUserPermissions, authz.SudoPermissionSet())
					request = context.WithValue(request, user.CurrentUserContextKey{}, &user.SystemUser)
					next(huma.WithContext(ctx, request))
				})
				RegisterRoutes(api, NewHandler(f.service, nil, nil, uploads, t.Context(), &config.Config{AgentMode: mode == "direct", EdgeAgent: mode == "edge"}))
				call := backupRouteTransport(t, mode, router)
				uploadID := portableRestoreInput(t, uploads)
				for _, request := range []struct{ method, path, body string }{
					{"POST", "/api/environments/0/volumes/data/backups", `{}`},
					{"POST", "/api/environments/0/volumes/data/backups/row/restore", `{}`},
					{"POST", "/api/environments/0/volumes/data/backups/row/restore-files", `{"paths":["file.txt"]}`},
					{"POST", "/api/environments/0/volumes/data/backups/upload", `{"uploadId":"` + uploadID + `"}`},
					{"DELETE", "/api/environments/0/volumes/backups/row", `{}`},
					{"GET", "/api/environments/0/volumes/backups/row/download", ``},
				} {
					status, _, body := call(t.Context(), request.method, request.path, []byte(request.body))
					require.Equal(t, 409, status, string(body))
					require.Contains(t, string(body), "volume.backup.busy")
				}
				require.Len(t, jobs.jobs, 1)
				for _, job := range jobs.jobs {
					outcome, err := job.Run(t.Context())
					require.NoError(t, err)
					require.Equal(t, scheduler.Skipped, outcome.Status)
				}
				require.Empty(t, f.creates, "refused work cannot start an engine or helper")
				var count int64
				require.NoError(t, f.service.deps.DB.Table("volume_backups").Count(&count).Error)
				require.Zero(t, count)
				require.NoError(t, f.service.deps.DB.Table("volume_restore_runs").Count(&count).Error)
				require.Zero(t, count)
			})
		}
	}
}
