// Source: docklet-next/issues/440
// Revision: 6e3afa6a78f4f60ffd74858d9a1eaefbb1b73bda
// Retrieved: 2026-11-12
// License: Apache-2.0
// Deviations: Original failed replacement settlement over real Direct and Edge carriers. M24 recovery evidence, validation and reason contracts.

package activity_test

import (
	"context"
	"fmt"
	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/container"
	"github.com/florianfinn/docklet-next/backend/internal/container/children/origin"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	a "github.com/florianfinn/docklet-next/types/activity"
	c "github.com/florianfinn/docklet-next/types/container"
	image "github.com/florianfinn/docklet-next/types/image"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"testing"
	"time"
)

type observedReplacementContainer struct {
	*container.ContainerService
	recovery c.RecreateRecovery
}

func (p *observedReplacementContainer) ExecuteContainerImage(ctx context.Context, _, _ string, _ bool, _ user.Actor) error {
	run := dto.ExecutionIdentityFromContext(ctx).RunID
	c.RecordRecreateResult(ctx, c.RecreateResult{OldID: runtimeContainerID, NewID: recreateNewID, OldName: "app", NewName: "app"})
	dto.RecordLifecycleResult(ctx, dto.LifecycleResult{RunID: run, Code: "update.target-changed", HTTPStatus: 409})
	return image.Failure("update.target-changed", 409)
}
func (p *observedReplacementContainer) RecoverRecreates(context.Context) ([]c.RecreateRecovery, []string, error) {
	return []c.RecreateRecovery{p.recovery}, nil, nil
}
func (p *observedReplacementContainer) PruneRecreateJournals(context.Context) error { return nil }
func TestObservedFailedReplacementDirectEdge(t *testing.T) {
	for _, edge := range []bool{false, true} {
		for _, startup := range []bool{false, true} {
			t.Run(fmt.Sprint(edge, "/", startup), func(t *testing.T) {
				var port *observedReplacementContainer
				daemon := newRecreateHTTP(t)
				f := newRemoteIntegration(t, edge, func(agent *runtimeServicesFixture) {
					port = &observedReplacementContainer{ContainerService: agent.containers}
					agent.adapter = activity.NewRuntimeServiceAdapter(agent.projects, port)
					agent.http.onCall = daemon.serve
				})
				boundary, err := activity.NewRuntimeHTTPService(f.hub, activity.RuntimeHTTPPorts{ApplicationContext: t.Context(), Store: func(context.Context, string) (activity.RuntimeStoreBinding, error) { return f.store, nil }, PrepareTarget: func(ctx context.Context, _ string, target activity.RuntimeTarget) (activity.RuntimeResolvedTarget, error) {
					return f.agent.adapter.ResolveRuntimeTarget(ctx, target)
				}, GuardContainer: func(context.Context, string, string) error { return nil }})
				require.NoError(t, err)
				boundary.SetReady(true)
				command := f.command(activity.RuntimeUpdateContainer, activity.RuntimeTarget{Kind: "container", ID: runtimeContainerID})
				if startup {
					command.Options.ContainerImage = &dto.ContainerImageRequest{AutoRestore: true}
				}
				admitted, err := f.hub.Admit(t.Context(), command)
				require.NoError(t, err)
				ref := activity.RuntimeRunReference{Store: f.store, RunID: admitted.ActivityID}
				assigned := origin.Override{RegistrationID: f.store.RegistrationID, ContainerName: "app", DockerID: runtimeContainerID, Origin: "standalone", AssignedBy: "operator", AssignedAt: time.Now()}
				require.NoError(t, f.db.Create(&assigned).Error)
				if startup {
					// Seed the committed receipt cut without starting another owner. The startup
					// adapter then settles the journal result through the production writer.
					var row activity.RuntimeReceipt
					require.NoError(t, f.db.First(&row, "run_id = ?", ref.RunID).Error)
					row.EnvironmentID = "0"
					row.StoreGeneration = f.agent.store.StoreGeneration
					row.DispatchState = new("pending")
					row.ExecutorState = new("accepted")
					require.NoError(t, f.agent.db.Transaction(func(tx *gorm.DB) error {
						if err := tx.Create(&row).Error; err != nil {
							return err
						}
						if err := tx.Create(&activity.RuntimeObservedRun{RunID: row.RunID, Kind: "container", Mode: "manual", CreatedAt: time.Now()}).Error; err != nil {
							return err
						}
						model := activity.Activity{BaseModel: database.BaseModel{ID: row.RunID, CreatedAt: time.Now()}, EnvironmentID: "0", Type: a.TypeContainerRedeploy, Status: a.StatusRunning, ResourceID: row.TargetID, ResourceType: row.TargetKind, ResourceName: row.TargetName, StartedByUserID: row.CallerID, StartedByUsername: row.CallerName, StartedAt: time.Now(), GateScope: *row.GateScope, GateKey: *row.GateKey}
						return tx.Create(&model).Error
					}))
					port.recovery = c.RecreateRecovery{RunID: row.RunID, Code: "runtime.interrupted", Result: &c.RecreateResult{OldID: runtimeContainerID, NewID: recreateNewID, OldName: "app", NewName: "app"}}
					require.NoError(t, f.agent.runtime.InitializeLocalStartup(t.Context(), f.agent.store))
					require.NoError(t, f.db.Model(&activity.RuntimeReceipt{}).Where("run_id = ?", ref.RunID).Updates(map[string]any{"dispatch_state": "accepted", "dispatched_at": time.Now()}).Error)
				} else {
					_, err = f.control.Start(t.Context(), ref, command.Options)
					require.NoError(t, err)
					require.NoError(t, f.control.WaitDispatch(t.Context(), ref.RunID))
					_, err = f.agent.waitLocal(ref.RunID)
					require.NoError(t, err)
				}
				snapshot, err := f.control.Refresh(t.Context(), ref)
				require.NoError(t, err)
				require.Equal(t, a.StatusFailed, snapshot.Status)
				require.NotNil(t, snapshot.Result)
				require.Equal(t, recreateNewID, snapshot.Result.NewID)
				require.False(t, snapshot.ImageLifecycle.Verified)
				want := "update.target-changed"
				if startup {
					want = "runtime.interrupted"
				}
				require.Equal(t, want, snapshot.ImageLifecycle.Code)
				require.NoError(t, f.db.First(&assigned, "registration_id = ? AND container_name = ?", assigned.RegistrationID, "app").Error)
				require.Equal(t, recreateNewID, assigned.DockerID)
			})
		}
	}
}
