// Source: docklet-next/docs/design/hub-backup.md
// Revision: 8f8c8f89bc6ebb7261862d0c35a6ddbc1513e9fa
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original real-alarm startup and policy-update regression coverage for issue #78.

package bootstrap

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humaecho"
	"github.com/labstack/echo/v5"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/require"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/internal/job"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/internal/system"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/runs"
	francistest "github.com/florianfinn/docklet-next/backend/pkg/utils/francis/testing"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	backuptypes "github.com/florianfinn/docklet-next/types/backup"
	schedulertypes "github.com/florianfinn/docklet-next/types/scheduler"
)

func TestHubBackupSchedulerTicksThroughPolicyRegistration(t *testing.T) {
	for _, zone := range []string{"UTC", "Europe/Berlin"} {
		for _, path := range []string{"startup", "create", "update"} {
			t.Run(zone+"/"+path, func(t *testing.T) {
				cfg := &config.Config{DatabaseURL: sqlitestore.URL(filepath.Join(t.TempDir(), "hub.db")), BackupDir: t.TempDir(), Timezone: zone}
				db, err := database.InitializeHub(t.Context(), cfg.DatabaseURL, database.MigrationOptions{})
				require.NoError(t, err)
				t.Cleanup(func() { require.NoError(t, db.Close()) })
				runtime := francistest.New(t, cfg.DatabaseURL)
				coordinator := runs.New(runtime.Service(), cfg.GetLocation())
				require.NoError(t, coordinator.Register(runtime))
				admission := runs.NewAdmission(runtime.Service(), t.Name())
				require.NoError(t, admission.Register(runtime))
				activities := activity.NewActivityService(db, nil)
				var dockerProbes atomic.Int32
				dockerAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					dockerProbes.Add(1)
					http.Error(w, "Docker unavailable", http.StatusServiceUnavailable)
				}))
				t.Cleanup(dockerAPI.Close)
				dockerClient, err := client.New(client.WithHost("tcp://"+strings.TrimPrefix(dockerAPI.URL, "http://")), client.WithHTTPClient(dockerAPI.Client()), client.WithAPIVersion("1.53"))
				require.NoError(t, err)
				t.Cleanup(func() { require.NoError(t, dockerClient.Close()) })
				dockerService := &docker.DockerClientService{Client: dockerClient}
				environments := environment.NewEnvironmentService(db, nil, dockerService, nil, nil, nil)
				require.NoError(t, db.Create(&environment.Environment{BaseModel: database.BaseModel{ID: "0"}, Name: "Local", Enabled: true, Status: "offline"}).Error)
				jobs := job.NewJobService(db, nil, cfg, coordinator, nil, environments, activities)
				clock, err := scheduler.NewJobScheduler(t.Context(), coordinator, cfg.GetLocation())
				require.NoError(t, err)
				jobs.SetScheduler(t.Context(), clock)
				hub := system.NewSystemService(db, dockerService, nil, nil, nil, nil, nil, activities, nil, nil, nil, nil, nil, cfg, nil, nil, coordinator, nil, runtime, nil)
				router := echo.New()
				apiConfig := huma.DefaultConfig("test", "1.0.0")
				apiConfig.Components.Schemas = huma.NewMapRegistry("#/components/schemas/", func(typ reflect.Type, hint string) string {
					return strings.NewReplacer("/", "_", ".", "_", "-", "_").Replace(typ.PkgPath()) + "_" + huma.DefaultSchemaNamer(typ, hint)
				})
				api := humaecho.NewWithGroup(router, router.Group("/api"), apiConfig)
				api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
					next(huma.WithContext(ctx, context.WithValue(ctx.Context(), middleware.ContextKeyUserPermissions, authz.SudoPermissionSet())))
				})
				system.New(hub, nil, nil, activities, cfg).RegisterRoutes(api, handlerutil.NewActivityAppContext(t.Context()))
				due := time.Now().In(cfg.GetLocation()).Add(3 * time.Second).Truncate(time.Second)
				expression := fmt.Sprintf("%d %d %d * * *", due.Second(), due.Minute(), due.Hour())
				policyID := "daily"
				if path != "create" {
					saved := expression
					if path == "update" {
						saved = "0 0 0 1 1 *"
					}
					require.NoError(t, db.Create(&system.SystemBackupPolicy{BaseModel: database.BaseModel{ID: policyID}, Enabled: true, Schedule: saved, RetentionCount: 2}).Error)
				}
				francistest.Start(t, runtime)
				require.NoError(t, registerDynamicJobs(dynamicJobsParams{AppCtx: t.Context(), Config: cfg, Scheduler: clock, System: hub, Admission: admission}))
				require.NoError(t, coordinator.Start(t.Context(), t.Context()))
				require.NoError(t, clock.StartScheduler(t.Context()))
				coordinator.Activate()
				t.Cleanup(func() {
					ctx := context.WithoutCancel(t.Context())
					require.NoError(t, coordinator.Stop(ctx))
					require.NoError(t, clock.Stop(ctx))
				})
				if path != "startup" {
					id := policyID
					if path == "create" {
						id = ""
					}
					body, err := json.Marshal(backuptypes.UpdateSystemBackupPolicies{Policies: []backuptypes.UpdateSystemBackupPolicy{{ID: id, Enabled: true, Schedule: expression, RetentionCount: 2}}})
					require.NoError(t, err)
					request := httptest.NewRequest(http.MethodPut, "/api/backups/policies", bytes.NewReader(body))
					request.Header.Set("Content-Type", "application/json")
					response := httptest.NewRecorder()
					router.ServeHTTP(response, request)
					require.Equal(t, http.StatusOK, response.Code, response.Body.String())
					var policies backuptypes.SystemBackupPolicyCollection
					require.NoError(t, json.Unmarshal(response.Body.Bytes(), &policies))
					require.Len(t, policies.Policies, 1)
					policyID = policies.Policies[0].ID
				}
				state, ok := clock.GetJobRuntimeState("system-backup:" + policyID)
				require.True(t, ok)
				require.True(t, state.Scheduled)
				require.WithinDuration(t, due, *state.NextRun, 0)
				var backup system.SystemBackupRun
				t.Cleanup(func() {
					record, err := coordinator.ScheduleState(context.WithoutCancel(t.Context()), "system-backup:"+policyID)
					if err == nil && t.Failed() {
						t.Logf("scheduler runs: %+v", record.Runs)
					}
				})
				require.Eventually(t, func() bool {
					var backups []system.SystemBackupRun
					err := db.Where("policy_id = ? AND status = ?", policyID, backuptypes.SystemBackupStatusSucceeded).Find(&backups).Error
					if err != nil || len(backups) == 0 {
						return false
					}
					backup = backups[0]
					return true
				}, 8*time.Second, 50*time.Millisecond)
				require.EqualValues(t, backuptypes.SystemBackupTriggerScheduled, backup.Trigger)
				require.WithinDuration(t, due, backup.CreatedAt, 2*time.Second)
				require.Positive(t, backup.Size)
				require.Zero(t, dockerProbes.Load(), "Hub SQLite archives must not probe Docker")
				scheduled, err := coordinator.List(t.Context(), "0", "system-backup:"+policyID, 1, 20)
				require.NoError(t, err)
				require.Len(t, scheduled.Runs, 1)
				require.Equal(t, "scheduled", scheduled.Runs[0].Trigger)
				require.Eventually(t, func() bool {
					run, err := coordinator.Get(t.Context(), "0", scheduled.Runs[0].JobID, scheduled.Runs[0].ID)
					return err == nil && run.Status == schedulertypes.Succeeded
				}, 3*time.Second, 20*time.Millisecond)
			})
		}
	}
}
