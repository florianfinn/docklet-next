// Source: docklet-next/issues/597
// Revision: 6e3c477e97192029f67bcf924da27b3b3849bbd8
// Retrieved: 2026-11-24
// License: Apache-2.0
// Deviations: Original static host-job admission, recovery and activity proofs.

package job

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/access"
	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/internal/role"
	userdomain "github.com/florianfinn/docklet-next/backend/internal/user"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/jobcontext"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	st "github.com/florianfinn/docklet-next/types/scheduler"
	"github.com/stretchr/testify/require"
)

type hostCheckerFunc func(context.Context, string) (st.Outcome, error)

func (f hostCheckerFunc) RunUpdateCheckHost(ctx context.Context, env string) (st.Outcome, error) {
	return f(ctx, env)
}

func hostJobFixture(t *testing.T, env string) *JobService {
	t.Helper()
	db := setupSettingsTestDBInternal(t)
	require.NoError(t, db.AutoMigrate(&environment.Environment{}, &userdomain.User{}, &access.Assignment{}, &role.Role{}, &role.UserRoleAssignment{}))
	require.NoError(t, db.Create(&environment.Environment{BaseModel: database.BaseModel{ID: env}, Name: "Example Host", Enabled: true}).Error)
	require.NoError(t, db.Create(&userdomain.User{Username: "operator", BaseModel: database.BaseModel{ID: "operator"}}).Error)
	require.NoError(t, db.Create(&role.Role{ID: authz.BuiltInRoleAdmin, Name: "Admin", Permissions: database.StringSlice(authz.AllPermissions())}).Error)
	require.NoError(t, db.Create(&role.UserRoleAssignment{UserID: "operator", RoleID: authz.BuiltInRoleAdmin}).Error)
	svc := newJobServiceForTestInternal(t, db, nil, &config.Config{})
	svc.roles = role.NewRoleService(db)
	svc.imagePolicies = hostCheckerFunc(func(ctx context.Context, target string) (st.Outcome, error) {
		require.Equal(t, env, target)
		run, ok := jobcontext.Run(ctx)
		require.True(t, ok)
		require.Equal(t, target, run.EnvironmentID)
		return st.Outcome{Status: st.Succeeded}, nil
	})
	return svc
}

func TestHostJobAdmissionAndRecoveryStayHubOwned(t *testing.T) {
	for _, env := range []string{"0", "example-direct", "example-edge"} {
		t.Run(env, func(t *testing.T) {
			svc := hostJobFixture(t, env)
			h := &JobSchedulesHandler{jobService: svc}
			ctx := context.WithValue(t.Context(), middleware.ContextKeyUserID, "operator")
			out, err := h.RunJob(ctx, &RunJobInput{ID: env, JobID: hostUpdateCheckJob})
			require.NoError(t, err)
			joined, err := h.RunJob(ctx, &RunJobInput{ID: env, JobID: hostUpdateCheckJob})
			require.NoError(t, err)
			require.Equal(t, out.Body.RunID, joined.Body.RunID)
			run, err := svc.GetRun(ctx, env, hostUpdateCheckJob, out.Body.RunID)
			require.NoError(t, err)
			// A nil remote service makes any accidental remote delivery fail immediately.
			svc.remote = nil
			result, err := svc.executeRunInternal(ctx, run)
			require.NoError(t, err)
			require.Equal(t, st.Succeeded, result.Status)
			result, err = svc.reconcileRunInternal(ctx, run)
			require.NoError(t, err)
			require.Equal(t, st.Succeeded, result.Status)
		})
	}
}

func TestHostJobRestartRevalidatesQueuedAndRunningRuns(t *testing.T) {
	for _, status := range []st.RunStatus{st.Queued, st.Running} {
		for _, change := range []string{"unchanged", "removed", "disabled", "reregistered", "revoked"} {
			t.Run(string(status)+"/"+change, func(t *testing.T) {
				svc := hostJobFixture(t, "example-host")
				run, err := svc.Submit(t.Context(), st.Request{EnvironmentID: "example-host", JobID: hostUpdateCheckJob, Trigger: "manual", RequestedBy: "operator"})
				require.NoError(t, err)
				require.NoError(t, svc.runs.UpdateRun(t.Context(), run, func(current *st.Run) error { current.Status = status; return nil }))
				switch change {
				case "removed":
					require.NoError(t, svc.db.Delete(&environment.Environment{}, "id = ?", run.EnvironmentID).Error)
				case "disabled":
					require.NoError(t, svc.db.Model(&environment.Environment{}).Where("id = ?", run.EnvironmentID).Update("enabled", false).Error)
				case "reregistered":
					require.NoError(t, svc.db.Delete(&environment.Environment{}, "id = ?", run.EnvironmentID).Error)
					require.NoError(t, svc.db.Create(&environment.Environment{BaseModel: database.BaseModel{ID: run.EnvironmentID, CreatedAt: run.CreatedAt.Add(time.Second)}, Name: "Replacement", Enabled: true}).Error)
				case "revoked":
					require.NoError(t, svc.db.Delete(&role.UserRoleAssignment{}, "user_id = ?", "operator").Error)
				}
				// A new service reads the durable run and current authority, without an environment forwarder.
				restarted := NewJobService(svc.db, nil, &config.Config{}, svc.runs, role.NewRoleService(svc.db), nil, nil)
				var calls atomic.Int32
				restarted.imagePolicies = hostCheckerFunc(func(context.Context, string) (st.Outcome, error) {
					calls.Add(1)
					return st.Outcome{Status: st.Succeeded}, nil
				})
				require.NoError(t, restarted.runs.Start(t.Context(), t.Context()))
				restarted.runs.Activate()
				t.Cleanup(func() { require.NoError(t, restarted.runs.Stop(context.WithoutCancel(t.Context()))) })
				var persisted st.Run
				require.Eventually(t, func() bool {
					persisted, err = restarted.GetRun(t.Context(), run.EnvironmentID, run.JobID, run.ID)
					return err == nil && persisted.Status.Terminal()
				}, 5*time.Second, 10*time.Millisecond)
				result := persisted.Outcome

				if change == "unchanged" {
					require.NoError(t, err)
					require.Equal(t, st.Succeeded, result.Status)
					require.Equal(t, int32(1), calls.Load())
				} else {
					require.NoError(t, err)
					require.Equal(t, st.Failed, result.Status)
					require.Zero(t, calls.Load())
					expected := "agent.unavailable"
					if change == "reregistered" {
						expected = "update.target-changed"
					}
					if change == "revoked" {
						expected = "update.permission-denied"
					}
					require.Equal(t, expected, result.Code)
				}
			})
		}
	}
}

func TestHostJobAdmissionRefusesUnavailableAndAgentHosts(t *testing.T) {
	for _, change := range []string{"removed", "disabled", "agent", "edge-agent", "revoked"} {
		t.Run(change, func(t *testing.T) {
			svc := hostJobFixture(t, "example-host")
			switch change {
			case "removed":
				require.NoError(t, svc.db.Delete(&environment.Environment{}, "id = ?", "example-host").Error)
			case "disabled":
				require.NoError(t, svc.db.Model(&environment.Environment{}).Where("id = ?", "example-host").Update("enabled", false).Error)
			case "agent":
				svc.cfg.AgentMode = true
			case "edge-agent":
				svc.cfg.EdgeAgent = true
			case "revoked":
				require.NoError(t, svc.db.Delete(&role.UserRoleAssignment{}, "user_id = ?", "operator").Error)
			}
			_, err := svc.Submit(t.Context(), st.Request{EnvironmentID: "example-host", JobID: hostUpdateCheckJob, Trigger: "manual", RequestedBy: "operator"})
			var problem *imageDTO.OperationError
			require.ErrorAs(t, err, &problem)
			list, err := svc.runs.List(t.Context(), "example-host", hostUpdateCheckJob, 1, 10)
			require.NoError(t, err)
			require.Empty(t, list.Runs)
		})
	}
}

func TestHostJobUnknownRunNeverQueriesAgent(t *testing.T) {
	svc := hostJobFixture(t, "example-host")
	svc.remote = nil
	_, err := svc.GetRun(t.Context(), "example-host", hostUpdateCheckJob, "unknown")
	require.Error(t, err)
	_, err = svc.CancelRun(t.Context(), "example-host", hostUpdateCheckJob, "unknown")
	require.Error(t, err)
}

func TestHostJobActivityProducer(t *testing.T) {
	svc := hostJobFixture(t, "example-host")
	require.NoError(t, svc.db.AutoMigrate(&activity.Activity{}, &activity.ActivityMessage{}, &activity.ActivitySyncReceipt{}))
	svc.activity = activity.NewActivityService(svc.db, nil)
	svc.runs.SetObserver(svc)
	run, err := svc.Submit(t.Context(), st.Request{EnvironmentID: "example-host", JobID: hostUpdateCheckJob, Trigger: "manual", RequestedBy: "operator"})
	require.NoError(t, err)
	detail, err := svc.activity.GetActivityDetail(t.Context(), "example-host", run.ActivityID, 50)
	require.NoError(t, err)
	require.Equal(t, "example-host", detail.Activity.EnvironmentID)
	require.Equal(t, hostUpdateCheckJob, *detail.Activity.ResourceID)
	require.Equal(t, hostUpdateCheckJob, detail.Activity.Metadata["jobId"])
	require.Equal(t, run.ID, detail.Activity.Metadata["runId"])
	row := detail.Activity
	row.ID = "example-host-check"
	row.Metadata["runId"] = "example-run"
	row.BatchID = new("example-run")
	row.Metadata["runUpdatedAt"] = "2026-11-24T00:00:00Z"
	row.CreatedAt = time.Date(2026, 11, 24, 0, 0, 0, 0, time.UTC)
	row.StartedAt = row.CreatedAt
	row.UpdatedAt = nil
	body, err := json.MarshalIndent(row, "", "  ")
	require.NoError(t, err)
	path := filepath.Join("..", "..", "..", "web", "src", "features", "stacks", "testdata")
	// The checked-in web fixture must equal the current producer output;
	// UPDATE_WEB_FIXTURES=1 regenerates it.
	fixturePath := filepath.Join(path, "host-update-activity.json")
	if os.Getenv("UPDATE_WEB_FIXTURES") == "1" {
		require.NoError(t, os.WriteFile(fixturePath, append(body, '\n'), 0644))
	}
	checkedIn, err := os.ReadFile(fixturePath)
	require.NoError(t, err)
	require.JSONEq(t, string(checkedIn), string(body))
}
