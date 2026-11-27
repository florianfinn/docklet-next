// Source: docklet-next/issues/676
// Revision: 9590d35e3340294b3d329053d5846656b5791371
// Retrieved: 2026-11-27
// License: Apache-2.0
// Deviations: Original scheduler regression coverage.

package policies_test

import (
	"context"
	"encoding/json/v2"
	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/internal/job"
	"github.com/florianfinn/docklet-next/backend/internal/job/children/remote"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/backend/internal/volume/children/backup/children/policies"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/runs"
	francistest "github.com/florianfinn/docklet-next/backend/pkg/utils/francis/testing"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	"github.com/florianfinn/docklet-next/types/contract"
	"github.com/florianfinn/docklet-next/types/scheduler"
	"github.com/florianfinn/docklet-next/types/version"
	"github.com/florianfinn/docklet-next/types/volume"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

type policyController struct{ jobs map[string]scheduler.Job }

func (s *policyController) AddJob(_ context.Context, j scheduler.Job) error {
	s.jobs[j.Name()] = j
	return nil
}
func (s *policyController) RemoveJob(_ context.Context, id string) { delete(s.jobs, id) }
func (s *policyController) HasJob(id string) bool                  { _, ok := s.jobs[id]; return ok }
func (s *policyController) Submit(context.Context, scheduler.Request) (scheduler.Run, error) {
	panic("unused")
}
func (s *policyController) GetJob(id string) (scheduler.Job, bool) { j, ok := s.jobs[id]; return j, ok }
func (s *policyController) GetJobRuntimeState(string) (scheduler.JobRuntimeState, bool) {
	return scheduler.JobRuntimeState{}, false
}
func (s *policyController) RescheduleJob(context.Context, scheduler.Job) error { return nil }
func (s *policyController) RunBusWatcherNow(context.Context, string) error     { return nil }
func (s *policyController) ListRegisteredJobs() []scheduler.Job {
	var result []scheduler.Job
	for _, j := range s.jobs {
		result = append(result, j)
	}
	return result
}
func TestRemoteDeliveryObservesAgentPolicyWaitingThenSuccess(t *testing.T) {
	db, err := database.InitializeHub(t.Context(), sqlitestore.URL(filepath.Join(t.TempDir(), "hub.db")), database.MigrationOptions{})
	require.NoError(t, err)
	defer db.Close()
	cfg, err := settings.NewSettingsService(t.Context(), db)
	require.NoError(t, err)
	defer cfg.Stop(t.Context())
	require.NoError(t, cfg.EnsureDefaultSettings(t.Context()))
	require.NoError(t, cfg.LoadDatabaseSettings(t.Context()))
	runtime := francistest.New(t)
	agent := runs.New(runtime.Service(), time.UTC)
	require.NoError(t, agent.Register(runtime))
	francistest.Start(t, runtime)
	gateRuntime := francistest.New(t)
	gate := runs.NewAdmission(gateRuntime.Service(), "agent")
	require.NoError(t, gate.Register(gateRuntime))
	francistest.Start(t, gateRuntime)
	key := scheduler.AdmissionKey{Scope: "volume-backup", ID: "data"}
	held, ok, err := gate.TryAcquire(t.Context(), key)
	require.NoError(t, err)
	require.True(t, ok)
	var creates atomic.Int32
	busy := volumeBusy{}
	svc := policies.NewService(policies.Dependencies{DB: db, AlreadyRunning: busy,
		Acquire: func(ctx context.Context, _ string) (context.Context, func(), error) {
			lease, ok, e := gate.TryAcquire(ctx, key)
			if e != nil {
				return ctx, nil, e
			}
			if !ok {
				return ctx, nil, busy
			}
			return ctx, func() { lease.Release(ctx) }, nil
		},
		CreateBackup: func(context.Context, string, string) (*volume.Backup, error) {
			creates.Add(1)
			return &volume.Backup{ID: "backup"}, nil
		},
	})
	require.NoError(t, db.AutoMigrate(&policies.VolumeBackupPolicy{}))
	p := policies.VolumeBackupPolicy{VolumeName: "data", Enabled: true, Schedule: "0 0 2 * * *", LocalEnabled: true}
	require.NoError(t, db.Create(&p).Error)
	controller := &policyController{jobs: map[string]scheduler.Job{}}
	require.NoError(t, svc.SetScheduler(t.Context(), controller, gate))
	svc.RegisterJobsOnStartup(t.Context())
	jobService := job.NewJobService(db, cfg, &config.Config{AgentMode: true}, agent, nil, nil, nil)
	jobService.SetScheduler(t.Context(), controller)
	router, api := humatest.New(t)
	group := huma.NewGroup(api, "/api")
	group.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
		c := context.WithValue(ctx.Context(), middleware.ContextKeyUserPermissions, authz.SudoPermissionSet())
		c = context.WithValue(c, middleware.ContextKeyUserID, "agent")
		next(huma.WithContext(ctx, c))
	})
	job.New(jobService, nil).RegisterRoutes(group)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/app-version" {
			_ = json.MarshalWrite(w, version.Info{AgentContractVersion: contract.AgentHubVersion, AgentContractFingerprint: contract.AgentHubFingerprint})
			return
		}
		if r.Method == http.MethodHead {
			w.WriteHeader(200)
			return
		}
		router.ServeHTTP(w, r)
	}))
	defer server.Close()
	require.NoError(t, db.Create(&environment.Environment{ID: "remote-agent", Name: "Agent", ApiUrl: server.URL, AccessToken: new("example"), Enabled: true}).Error)
	env := environment.NewEnvironmentService(db, server.Client(), nil, nil, nil, nil)
	managerRuntime := francistest.New(t)
	manager := runs.New(managerRuntime.Service(), time.UTC)
	require.NoError(t, manager.Register(managerRuntime))
	francistest.Start(t, managerRuntime)
	sender := remote.New(manager, nil, env)
	require.NoError(t, agent.Start(t.Context(), t.Context()))
	agent.Activate()
	defer agent.Stop(t.Context())
	run, err := manager.Submit(t.Context(), scheduler.Request{EnvironmentID: "remote-agent", JobID: "volume-backup:" + p.ID, Trigger: "scheduled"})
	require.NoError(t, err)
	first, err := sender.Deliver(t.Context(), run)
	require.NoError(t, err)
	require.Equal(t, scheduler.Waiting, first.Status, "%+v", first)
	require.Eventually(t, func() bool {
		r, e := agent.Get(t.Context(), "0", run.JobID, run.ID)
		return e == nil && r.Status == scheduler.Waiting && r.Outcome.Code == "run.busy"
	}, 5*time.Second, 10*time.Millisecond)
	outcome, err := sender.Deliver(t.Context(), run)
	require.NoError(t, err)
	require.Equal(t, scheduler.Waiting, outcome.Status)
	require.Zero(t, creates.Load())
	held.Release(t.Context())
	require.Eventually(t, func() bool {
		r, e := agent.Get(t.Context(), "0", run.JobID, run.ID)
		return e == nil && r.Status == scheduler.Succeeded
	}, 8*time.Second, 10*time.Millisecond)
	outcome, err = sender.Deliver(t.Context(), run)
	require.NoError(t, err)
	require.Equal(t, scheduler.Succeeded, outcome.Status)
	require.Equal(t, int32(1), creates.Load())
}

type volumeBusy struct{}

func (volumeBusy) Error() string { return "busy" }

func (s *policyController) RegisterJob(j scheduler.Job) error {
	s.jobs[j.Name()] = j
	return nil
}
func (s *policyController) RegisterBusWatcher(scheduler.BusWatcher, bool) error { return nil }
func (s *policyController) StartScheduler(context.Context) error                { return nil }
func (s *policyController) Reinstall(context.Context) error                     { return nil }
func (s *policyController) Stop(context.Context) error                          { return nil }
func (s *policyController) GetLocation() *time.Location                         { return time.UTC }
func (s *policyController) RestartWatcher(context.Context, string) error        { return nil }
func (s *policyController) WatcherHealth(string) (scheduler.WorkerHealth, bool) {
	return scheduler.WorkerHealth{}, false
}
