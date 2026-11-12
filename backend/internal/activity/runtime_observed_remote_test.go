// Source: docklet-next/issues/440
// Revision: 6e3afa6a78f4f60ffd74858d9a1eaefbb1b73bda
// Retrieved: 2026-11-12
// License: Apache-2.0
// Deviations: Original recovery evidence over actual Direct and Edge execution and query seams. M24 recovery evidence, validation and reason contracts.

package activity_test

import (
	"context"
	"fmt"
	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/project"
	policy "github.com/florianfinn/docklet-next/backend/internal/updater/children/policies"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	a "github.com/florianfinn/docklet-next/types/activity"
	image "github.com/florianfinn/docklet-next/types/image"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

type observedRemoteProjects struct {
	*project.ProjectService
	result           dto.LifecycleResult
	entered, release chan struct{}
	seen             chan dto.ObservedExecution
}

func (p *observedRemoteProjects) ExecuteImageLifecycle(ctx context.Context, _, run, _ string, _ *dto.LifecycleIntent, _ user.Actor) (*dto.LifecycleResult, error) {
	p.seen <- dto.ObservedExecutionFromContext(ctx)
	if p.entered != nil {
		close(p.entered)
		<-p.release
	}
	result := p.result
	result.RunID = run
	dto.RecordLifecycleResult(ctx, result)
	return &result, image.Failure(result.Code, 500)
}
func observedRemoteIntent() *dto.LifecycleIntent {
	revision := strings.Repeat("a", 64)
	return &dto.LifecycleIntent{Request: dto.LifecycleRequest{ExpectedRevision: revision, AutoRestore: true}, TargetIdentity: revision, SavedRevision: revision, ObservedRevision: revision, BaselineRevision: revision, Selections: []dto.ImageSelection{}}
}
func TestObservedRecoveryDirectEdgeHistoryAndHubRestart(t *testing.T) {
	for _, mode := range []string{"manual", "scheduled"} {
		for _, edge := range []bool{false, true} {
			t.Run(mode+"/"+fmt.Sprint(edge), func(t *testing.T) {
				worker := &observedRemoteProjects{result: dto.LifecycleResult{Code: "update.unstable", HTTPStatus: 500, BaselineCode: "update.baseline-unavailable", PointCode: "deploy.point-unavailable", RestorePointID: uuid.NewString(), Recovery: &dto.RecoveryResult{Outcome: "restored"}, Applied: []dto.AppliedCandidate{{Service: "app", Repository: "registry.example.com/app", Platform: "linux/amd64", Manifest: "sha256:" + strings.Repeat("f", 64)}}}, entered: make(chan struct{}), release: make(chan struct{}), seen: make(chan dto.ObservedExecution, 1)}
				released := false
				defer func() {
					if !released {
						close(worker.release)
					}
				}()
				f := newRemoteIntegration(t, edge, func(agent *runtimeServicesFixture) {
					worker.ProjectService = agent.projects
					agent.adapter = activity.NewRuntimeServiceAdapter(worker, agent.containers)
				})
				target := activity.RuntimeTarget{Kind: "stack", ID: "example-project"}
				f.agent.project(target.ID, target.ID, "name: example_stack\nservices:\n  app:\n    image: registry.example.com/app:1\n")
				command := f.command(activity.RuntimeUpdateProject, target)
				command.Options.Lifecycle = observedRemoteIntent()
				expectedContext := dto.ObservedExecution{Mode: mode}
				if mode == "scheduled" {
					command.Options.Lifecycle.Request.AutoRestore = false
					require.NoError(t, f.db.Exec("INSERT OR IGNORE INTO environments(id,name,api_url,enabled) VALUES(?,?,?,1)", f.store.EnvironmentID, "Example", "https://agent.example.com").Error)
					store := policy.Store{DB: f.db.DB}
					current, err := store.Policy(t.Context(), f.store.EnvironmentID, f.store.RegistrationID, target.ID)
					require.NoError(t, err)
					current.Mode = "scheduled"
					current.ActorID = command.Actor.ID
					current.TargetIdentity = command.Options.Lifecycle.TargetIdentity
					current.SetWindow(&dto.UpdateWindow{Kind: "daily", Time: "04:00"})
					require.NoError(t, store.SavePolicy(t.Context(), current, 0))
					current.Revision = 1
					child, _, err := store.PrepareChild(t.Context(), current, "window")
					require.NoError(t, err)
					command.Options.Lifecycle.Policy = &dto.PolicyBinding{PolicyID: current.ID, Revision: 1, JobRunID: "window", ChildID: child.ID}
					command.ProposedRunID = child.ID
					expectedContext.PolicyID = current.ID
					expectedContext.ChildID = child.ID
				}

				admitted, err := f.hub.Admit(t.Context(), command)
				require.NoError(t, err)
				ref := activity.RuntimeRunReference{Store: f.store, RunID: admitted.ActivityID}
				_, err = f.control.Start(t.Context(), ref, command.Options)
				require.NoError(t, err)
				require.NoError(t, f.control.WaitDispatch(t.Context(), ref.RunID))
				<-worker.entered
				require.Equal(t, expectedContext, <-worker.seen)
				for _, db := range []*database.DB{f.db, f.agent.db} {
					var rows []activity.RuntimeObservedRun
					require.NoError(t, db.Where("run_id = ?", ref.RunID).Find(&rows).Error)
					require.Len(t, rows, 1)
					require.Equal(t, mode, rows[0].Mode)
				}
				require.NoError(t, f.control.Close(t.Context()))
				// Restart the hub store while the real agent owner continues executing.
				var databases []struct{ Name, File string }
				require.NoError(t, f.db.Raw("PRAGMA database_list").Scan(&databases).Error)
				path := ""
				for _, db := range databases {
					if db.Name == "main" {
						path = db.File
					}
				}
				require.NotEmpty(t, path)
				require.NoError(t, f.db.Close())
				hubDB, err := database.InitializeHub(t.Context(), sqlitestore.URL(path), database.MigrationOptions{})
				require.NoError(t, err)
				defer hubDB.Close()
				hubActivity := activity.NewActivityService(hubDB, nil)
				hub := activity.NewRuntimeService(hubActivity, f.agent.adapter)
				close(worker.release)
				released = true
				local, err := f.agent.waitLocal(ref.RunID)
				require.NoError(t, err)
				require.Equal(t, a.StatusFailed, local.Status)
				require.False(t, local.ImageLifecycle.Verified)
				expected := worker.result
				expected.RunID = ref.RunID
				require.Equal(t, &expected, local.ImageLifecycle)
				// Prune executor history before the reconnected hub's first query.
				_, err = f.agent.activity.DeleteHistory(t.Context(), "0")
				require.NoError(t, err)
				var receipt activity.RuntimeReceipt
				require.NoError(t, f.agent.db.First(&receipt, "run_id = ?", ref.RunID).Error)
				identity := a.RuntimeRunIdentity{RuntimeExecutorIdentity: a.RuntimeExecutorIdentity{ManagerID: receipt.ManagerID, RegistrationID: receipt.RegistrationID, ExecutorGeneration: receipt.ExecutorGeneration}, RunID: ref.RunID, RequestDigest: receipt.RequestDigest}
				queried, err := f.sender.Query(t.Context(), identity)
				require.NoError(t, err)
				require.Equal(t, a.RuntimeAnswerTerminal, queried.State)
				require.Equal(t, &expected, queried.Detail.Activity.ImageLifecycle)
				coordinator, err := activity.NewRuntimeRemoteCoordinator(hub, t.Context(), func(context.Context, activity.RuntimeStoreBinding) (activity.RuntimeRemoteTransport, error) {
					return f.sender, nil
				}, activity.RuntimeReconcileSchedule{})
				require.NoError(t, err)
				defer coordinator.Close(t.Context())
				snapshot, err := coordinator.Refresh(t.Context(), ref)
				require.NoError(t, err)
				require.Equal(t, &expected, snapshot.ImageLifecycle)
				_, err = hubActivity.DeleteHistory(t.Context(), f.store.EnvironmentID)
				require.NoError(t, err)
				snapshot, err = coordinator.Snapshot(t.Context(), ref)
				require.NoError(t, err)
				require.Equal(t, &expected, snapshot.ImageLifecycle)
				require.Equal(t, a.RuntimeOutcomeKnown, snapshot.OutcomeState)
				require.EqualValues(t, 1, f.executeCalls.Load())
			})
		}
	}
}
