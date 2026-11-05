// Source: docklet-next/issues/344
// Revision: 89f69980c01d08714ce96c49365fb19065e3d6ba
// Retrieved: 2026-11-05
// License: Apache-2.0
// Deviations: Original deploy evidence through real Direct and Edge runtime paths across store lifetimes.

package activity_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/auth"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/project"
	life "github.com/florianfinn/docklet-next/backend/internal/project/children/lifecycle"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	activityDTO "github.com/florianfinn/docklet-next/types/activity"
	projectDTO "github.com/florianfinn/docklet-next/types/project"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type deployEvidenceProjects struct {
	*project.ProjectService
	db          *database.DB
	point, code string
}

func (p *deployEvidenceProjects) PrepareDeployPoint(ctx context.Context, id, run, registration string) {
	row := life.Run{ID: run, PointID: p.point}
	if p.code != "" {
		row.PointCode = &p.code
	}
	if err := p.db.WithContext(ctx).Create(&row).Error; err != nil {
		panic(err)
	}
	dto.RecordLifecycleResult(ctx, dto.LifecycleResult{RunID: run, RestorePointID: p.point, PointCode: p.code})
}
func (p *deployEvidenceProjects) DeployProject(ctx context.Context, id string, actor user.Actor, opts *projectDTO.DeployOptions) error {
	run := dto.ExecutionIdentityFromContext(ctx).RunID
	if err := p.db.WithContext(ctx).Model(&life.Run{}).Where("id = ?", run).Update("had_effect", true).Error; err != nil {
		return err
	}
	// This baseline producer deliberately supplies only baseline fields.
	dto.RecordLifecycleResult(ctx, dto.LifecycleResult{RunID: run, Verified: true})
	return nil
}
func (p *deployEvidenceProjects) RedeployProject(ctx context.Context, id string, actor user.Actor, opts *projectDTO.DeployOptions) error {
	return p.DeployProject(ctx, id, actor, opts)
}

func TestRuntimeDeployEvidenceDirectEdgeNormalAndValidation(t *testing.T) {
	for _, isEdge := range []bool{false, true} {
		for _, op := range []activity.RuntimeOperation{activity.RuntimeDeployProject, activity.RuntimeRedeployProject} {
			for _, code := range []string{"", "deploy.point-unavailable", "update.image-binding-unsupported", "update.persistence-failed", "undeclared"} {
				t.Run(map[bool]string{false: "direct", true: "edge"}[isEdge]+"/"+string(op)+"/"+code, func(t *testing.T) {
					point := ""
					if code == "" {
						point = uuid.NewString()
					}
					f := newRemoteIntegration(t, isEdge, func(agent *runtimeServicesFixture) {
						sourceCode := code
						if sourceCode == "undeclared" {
							sourceCode = ""
						}
						agent.adapter = activity.NewRuntimeServiceAdapter(&deployEvidenceProjects{ProjectService: agent.projects, db: agent.db, point: point, code: sourceCode}, agent.containers)
					})
					var injected atomic.Int32
					if code == "undeclared" {
						// Inject only after the real receiver has produced its reply:
						// executor-side normalization cannot prime the hub assertion.
						f.handlerMu.Lock()
						original := f.receiverHandler
						f.receiverHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
							recorded := &remoteCapturedResponse{ResponseWriter: w, header: make(http.Header)}
							original.ServeHTTP(recorded, r)
							var outcome activityDTO.RuntimeExecutionOutcome
							body := recorded.body.Bytes()
							if recorded.status == http.StatusOK && json.Unmarshal(body, &outcome) == nil && outcome.Detail != nil && outcome.Detail.Activity.ImageLifecycle != nil {
								raw := dto.LifecycleResult{RunID: outcome.Identity.RunID, PointCode: "undeclared"}
								outcome.Detail.Activity.ImageLifecycle = &raw
								outcome.Detail.Activity.Metadata["imageLifecycle"] = raw
								body, _ = json.Marshal(outcome)
								injected.Add(1)
							}
							for key, values := range recorded.header {
								w.Header()[key] = values
							}
							w.WriteHeader(recorded.status)
							_, _ = w.Write(body)
						})
						f.handlerMu.Unlock()
					}
					target := activity.RuntimeTarget{Kind: "stack", ID: "example-project"}
					f.agent.project(target.ID, target.ID, "name: example_stack\nservices:\n  app:\n    image: registry.example.com/app:1\n")
					command := f.command(op, target)
					admitted, err := f.hub.Admit(t.Context(), command)
					require.NoError(t, err)
					ref := activity.RuntimeRunReference{Store: f.store, RunID: admitted.ActivityID}
					_, err = f.control.Start(t.Context(), ref, command.Options)
					require.NoError(t, err)
					require.NoError(t, f.control.WaitDispatch(t.Context(), ref.RunID))
					agent, err := f.agent.waitLocal(ref.RunID)
					require.NoError(t, err)
					require.Equal(t, activityDTO.StatusSuccess, agent.Status)
					result, err := f.control.Wait(t.Context(), ref, nil)
					require.NoError(t, err)
					require.NotNil(t, result.ImageLifecycle)
					if code == "undeclared" {
						require.Positive(t, injected.Load(), "hub must receive an undeclared wire value")
						require.Equal(t, "runtime.outcome-unknown", result.ImageLifecycle.Code)
						require.Empty(t, result.ImageLifecycle.PointCode)
					} else {
						require.Equal(t, point, result.ImageLifecycle.RestorePointID)
						require.Equal(t, code, result.ImageLifecycle.PointCode)
						require.True(t, result.ImageLifecycle.Verified, "baseline and point fields are merged")
					}
					var row activity.Activity
					require.NoError(t, f.db.First(&row, "id = ?", ref.RunID).Error)
					if code == "undeclared" {
						body, err := json.Marshal(row.Metadata)
						require.NoError(t, err)
						require.NotContains(t, string(body), "undeclared")
					}
					require.EqualValues(t, 1, f.executeCalls.Load())
				})
			}
		}
	}
}

type deployNoReplayExecutor struct{ t *testing.T }

func (e deployNoReplayExecutor) ExecuteRuntime(context.Context, activity.RuntimeCommand, io.Writer) error {
	e.t.Error("restarted executor must not replay")
	return nil
}

func TestRuntimeDeployEvidenceDirectEdgeRestartAndLaterQuery(t *testing.T) {
	for _, isEdge := range []bool{false, true} {
		for _, op := range []activity.RuntimeOperation{activity.RuntimeDeployProject, activity.RuntimeRedeployProject} {
			for _, withPoint := range []bool{false, true} {
				t.Run(map[bool]string{false: "direct", true: "edge"}[isEdge]+"/"+string(op)+map[bool]string{false: "/code", true: "/point"}[withPoint], func(t *testing.T) {
					f := newRemoteIntegration(t, isEdge)
					require.NoError(t, f.control.Close(t.Context()))
					target := activity.RuntimeTarget{Kind: "stack", ID: "example-project"}
					f.agent.project(target.ID, target.ID, "name: example_stack\nservices:\n  app:\n    image: registry.example.com/app:1\n")
					command := f.command(op, target)
					command.ProposedRunID = uuid.NewString()
					hubAdmission, err := f.hub.Admit(t.Context(), command)
					require.NoError(t, err)
					// Store-only admission models the durable crash cut without a live owner.
					priorAgent := activity.NewRuntimeService(f.agent.activity, f.agent.adapter)
					localCommand := command
					localCommand.Store = f.agent.store
					localAdmission, err := priorAgent.Admit(t.Context(), localCommand)
					require.NoError(t, err)
					require.Equal(t, hubAdmission.ActivityID, localAdmission.ActivityID)
					point, code := "", "deploy.point-unavailable"
					if withPoint {
						point, code = uuid.NewString(), ""
					}
					row := life.Run{ID: command.ProposedRunID, PointID: point, HadEffect: withPoint}
					if code != "" {
						row.PointCode = &code
					}
					require.NoError(t, f.agent.db.Create(&row).Error)
					require.NoError(t, f.db.Model(&activity.RuntimeReceipt{}).Where("run_id = ?", command.ProposedRunID).Updates(map[string]any{"dispatch_state": "accepted", "dispatched_at": time.Now().UTC()}).Error)
					var paths []struct{ Name, File string }
					require.NoError(t, f.db.Raw("PRAGMA database_list").Scan(&paths).Error)
					hubPath := ""
					for _, path := range paths {
						if path.Name == "main" {
							hubPath = path.File
						}
					}
					require.NotEmpty(t, hubPath)
					require.NoError(t, f.db.Close())
					require.NoError(t, f.agent.db.Close())
					// Both hub and agent get new database handles, services, runtimes and receiver.
					agentDB, err := database.Initialize(t.Context(), sqlitestore.URL(f.agent.dbPath), database.MigrationOptions{})
					require.NoError(t, err)
					defer agentDB.Close()
					agentRuntime := activity.NewRuntimeService(activity.NewActivityService(agentDB, nil), f.agent.adapter, activity.RuntimeLocalConfig{ApplicationContext: t.Context(), Store: f.agent.store, Executor: deployNoReplayExecutor{t}})
					require.NoError(t, agentRuntime.InitializeLocalStartup(t.Context(), f.agent.store))
					var startup activity.Activity
					require.NoError(t, agentDB.First(&startup, "id = ?", row.ID).Error)
					body, err := json.Marshal(startup.Metadata["imageLifecycle"])
					require.NoError(t, err)
					var startupResult dto.LifecycleResult
					require.NoError(t, json.Unmarshal(body, &startupResult))
					require.Equal(t, point, startupResult.RestorePointID)
					require.Equal(t, code, startupResult.PointCode, "startup is asserted before any query")
					// A later receipt query must derive its own point fields, even
					// after the historical Activity evidence was compacted.
					require.NoError(t, agentDB.Model(&activity.Activity{}).Where("id = ?", row.ID).UpdateColumn("metadata", database.JSON{}).Error)

					receiver, err := activity.NewRuntimeReceiver(agentRuntime)
					require.NoError(t, err)
					handler, err := activity.NewRuntimeReceiverHTTP(receiver, &auth.AuthService{}, &config.Config{AgentMode: true, AgentToken: f.token}, time.Minute)
					require.NoError(t, err)
					f.handlerMu.Lock()
					f.receiverHandler = handler
					f.handlerMu.Unlock()
					hubDB, err := database.InitializeHub(t.Context(), sqlitestore.URL(hubPath), database.MigrationOptions{})
					require.NoError(t, err)
					defer hubDB.Close()
					hubRuntime := activity.NewRuntimeService(activity.NewActivityService(hubDB, nil), f.agent.adapter)
					// A direct query is asserted before any hub reconciliation can prime it.
					var receipt activity.RuntimeReceipt
					require.NoError(t, agentDB.First(&receipt, "run_id = ?", row.ID).Error)
					identity := activityDTO.RuntimeRunIdentity{RuntimeExecutorIdentity: activityDTO.RuntimeExecutorIdentity{ManagerID: receipt.ManagerID, RegistrationID: receipt.RegistrationID, ExecutorGeneration: receipt.ExecutorGeneration}, RunID: row.ID, RequestDigest: receipt.RequestDigest}
					queried, err := f.sender.Query(t.Context(), identity)
					require.NoError(t, err)
					require.Equal(t, activityDTO.RuntimeAnswerTerminal, queried.State)
					require.NotNil(t, queried.Detail.Activity.ImageLifecycle)
					require.Equal(t, point, queried.Detail.Activity.ImageLifecycle.RestorePointID)
					require.Equal(t, code, queried.Detail.Activity.ImageLifecycle.PointCode)
					coordinator, err := activity.NewRuntimeRemoteCoordinator(hubRuntime, t.Context(), func(context.Context, activity.RuntimeStoreBinding) (activity.RuntimeRemoteTransport, error) {
						return f.sender, nil
					}, activity.RuntimeReconcileSchedule{})
					require.NoError(t, err)
					defer coordinator.Close(t.Context())
					ref := activity.RuntimeRunReference{Store: f.store, RunID: row.ID}
					result, err := coordinator.Refresh(t.Context(), ref)
					require.NoError(t, err)
					require.Equal(t, activityDTO.StatusFailed, result.Status)
					require.Equal(t, "runtime.interrupted", result.MessageCode)
					require.NotNil(t, result.ImageLifecycle)
					require.Equal(t, point, result.ImageLifecycle.RestorePointID)
					require.Equal(t, code, result.ImageLifecycle.PointCode)
					var settled life.Run
					require.NoError(t, agentDB.First(&settled, "id = ?", row.ID).Error)
					require.True(t, settled.Settled)
					require.Zero(t, f.executeCalls.Load(), "query recovery cannot dispatch")
				})
			}
		}
	}
}
