// Source: docklet-next/issues/440
// Revision: 6e3afa6a78f4f60ffd74858d9a1eaefbb1b73bda
// Retrieved: 2026-11-12
// License: Apache-2.0
// Deviations: Original notice mode, incoming-evidence ordering and startup recovery over real Direct and Edge carriers.

package activity_test

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/notification/notificationoutbox"
	"github.com/florianfinn/docklet-next/backend/internal/project"
	policy "github.com/florianfinn/docklet-next/backend/internal/updater/children/policies"
	a "github.com/florianfinn/docklet-next/types/activity"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func bindNoticeRemote(t *testing.T, f *remoteIntegration, command activity.RuntimeCommand, mode string) activity.RuntimeCommand {
	t.Helper()
	command.Options.Lifecycle = observedRemoteIntent()
	if mode == "unobserved" {
		command.Options.Lifecycle.Request.AutoRestore = false
	}
	if mode != "scheduled" {
		return command
	}
	command.Options.Lifecycle.Request.AutoRestore = false
	require.NoError(t, f.db.Exec("INSERT OR IGNORE INTO environments(id,name,api_url,enabled) VALUES(?,?,?,1)", f.store.EnvironmentID, "Example host", "https://agent.example.com").Error)
	store := policy.Store{DB: f.db.DB}
	one, err := store.Policy(t.Context(), f.store.EnvironmentID, f.store.RegistrationID, command.Target.ID)
	require.NoError(t, err)
	one.Mode, one.ActorID, one.TargetIdentity = "scheduled", command.Actor.ID, command.Options.Lifecycle.TargetIdentity
	one.SetWindow(&dto.UpdateWindow{Kind: "daily", Time: "04:00"})
	require.NoError(t, store.SavePolicy(t.Context(), one, 0))
	one.Revision = 1
	child, _, err := store.PrepareChild(t.Context(), one, uuid.NewString())
	require.NoError(t, err)
	command.Options.Lifecycle.Policy = &dto.PolicyBinding{PolicyID: one.ID, Revision: 1, JobRunID: child.JobRunID, ChildID: child.ID}
	command.ProposedRunID = child.ID
	return command
}

func TestRecoveryNoticesDirectEdgeIncomingAndMode(t *testing.T) {
	for _, edge := range []bool{false, true} {
		for _, mode := range []string{"scheduled", "manual", "unobserved"} {
			for _, outcome := range []string{"restored", "restore-failed", "unavailable", "not-needed", "candidate-failed"} {
				if mode == "unobserved" && outcome != "not-needed" {
					continue
				}
				t.Run(fmt.Sprint(edge, "/", mode, "/", outcome), func(t *testing.T) {
					worker := &observedRemoteProjects{result: dto.LifecycleResult{Code: "update.unstable", HTTPStatus: 500, Recovery: &dto.RecoveryResult{Outcome: outcome}}, entered: make(chan struct{}), release: make(chan struct{}), seen: make(chan dto.ObservedExecution, 1)}
					if outcome == "unavailable" {
						worker.result.Recovery.Code = "restore.files-changed"
					}
					if outcome == "candidate-failed" {
						worker.result.Code, worker.result.Recovery.Outcome = "update.candidate-failed", "not-needed"
					}
					if mode == "unobserved" {
						worker.result.Recovery = nil
					}
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
					command := bindNoticeRemote(t, f, f.command(activity.RuntimeUpdateProject, target), mode)
					admitted, err := f.hub.Admit(t.Context(), command)
					require.NoError(t, err)
					ref := activity.RuntimeRunReference{Store: f.store, RunID: admitted.ActivityID}
					// Prove the metadata UPDATE precedes the outbox hook in the same transaction.
					require.NoError(t, f.db.Exec("CREATE TABLE notice_hook_calls (id INTEGER)").Error)
					require.NoError(t, f.db.Exec("CREATE TRIGGER count_notice_hook BEFORE INSERT ON notification_outbox BEGIN INSERT INTO notice_hook_calls VALUES(1); END").Error)
					if mode == "scheduled" && outcome == "restored" {
						require.NoError(t, f.db.Exec(`CREATE TRIGGER notice_metadata_order BEFORE INSERT ON notification_outbox WHEN json_extract((SELECT metadata FROM activities WHERE id = substr(NEW.dedup_key,-36)),'$.imageLifecycle.recovery.outcome') IS NOT 'restored' BEGIN SELECT RAISE(ABORT,'notice precedes checked metadata'); END`).Error)
					}
					_, err = f.control.Start(t.Context(), ref, command.Options)
					require.NoError(t, err)
					require.NoError(t, f.control.WaitDispatch(t.Context(), ref.RunID))
					<-worker.entered
					seen := <-worker.seen
					if mode == "unobserved" {
						require.Empty(t, seen.Mode)
					} else {
						require.Equal(t, mode, seen.Mode)
					}
					require.Empty(t, seen.Suppressed)
					close(worker.release)
					released = true
					_, err = f.agent.waitLocal(ref.RunID)
					require.NoError(t, err)
					snapshot, err := f.control.Refresh(t.Context(), ref)
					require.NoError(t, err)
					require.Equal(t, a.RuntimeOutcomeKnown, snapshot.OutcomeState)
					if mode != "unobserved" {
						var marker activity.RuntimeObservedRun
						require.NoError(t, f.db.First(&marker, "run_id = ?", ref.RunID).Error)
						require.Equal(t, mode, marker.Mode)
					}
					// Repeated refresh does not invoke the terminal notice hook again.
					_, err = f.control.Refresh(t.Context(), ref)
					require.NoError(t, err)
					var rows []notificationoutbox.Row
					require.NoError(t, f.db.Find(&rows).Error)
					expectedCalls := int64(1)
					if outcome == "candidate-failed" && mode == "scheduled" {
						require.Empty(t, rows)
						expectedCalls = 0
					} else {
						require.Len(t, rows, 1)
						reason := "runtime.failed"
						if mode == "scheduled" {
							reason = map[string]string{"restored": "auto-restored", "restore-failed": "auto-restore-failed", "unavailable": "auto-restore-unavailable", "not-needed": "runtime.failed"}[outcome]
						}
						require.Equal(t, reason, rows[0].Payload["reason"])
						require.Equal(t, "deploy:"+f.store.EnvironmentID+":"+ref.RunID, rows[0].DedupKey)
					}
					var hooks int64
					require.NoError(t, f.db.Table("notice_hook_calls").Count(&hooks).Error)
					require.Equal(t, expectedCalls, hooks)
					if mode == "manual" {
						var count int64
						require.NoError(t, f.db.Model(&policy.Child{}).Count(&count).Error)
						require.Zero(t, count)
						require.NoError(t, f.db.Model(&policy.FailedCandidate{}).Count(&count).Error)
						require.Zero(t, count)
					}
				})
			}
		}
	}
}

func TestRecoveryNoticeTransportHoldThenStartupDirectEdge(t *testing.T) {
	for _, edge := range []bool{false, true} {
		for _, mode := range []string{"scheduled", "manual"} {
			t.Run(fmt.Sprint(edge, "/", mode), func(t *testing.T) {
				worker := &observedRemoteProjects{result: dto.LifecycleResult{Code: "update.unstable", HTTPStatus: 500, Recovery: &dto.RecoveryResult{Outcome: "restored"}}, entered: make(chan struct{}), release: make(chan struct{}), seen: make(chan dto.ObservedExecution, 1)}
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
				command := bindNoticeRemote(t, f, f.command(activity.RuntimeUpdateProject, target), mode)
				admitted, err := f.hub.Admit(t.Context(), command)
				require.NoError(t, err)
				ref := activity.RuntimeRunReference{Store: f.store, RunID: admitted.ActivityID}
				_, err = f.control.Start(t.Context(), ref, command.Options)
				require.NoError(t, err)
				require.NoError(t, f.control.WaitDispatch(t.Context(), ref.RunID))
				<-worker.entered
				// Join execution while preventing its terminal COMMIT. The replacement
				// runtime then has exactly the durable state left by process loss.
				require.NoError(t, f.agent.db.Exec("CREATE TRIGGER lost_terminal BEFORE UPDATE OF executor_state ON runtime_receipts WHEN NEW.executor_state='failed' BEGIN SELECT RAISE(ABORT,'synthetic lost terminal commit'); END").Error)
				close(worker.release)
				released = true
				_, err = f.agent.waitLocal(ref.RunID)
				require.Error(t, err)
				require.NoError(t, f.agent.db.Exec("DROP TRIGGER lost_terminal").Error)
				f.handlerMu.Lock()
				original := f.receiverHandler
				f.receiverHandler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) })
				f.handlerMu.Unlock()
				snapshot, err := f.control.Refresh(t.Context(), ref)
				require.NoError(t, err)
				require.Equal(t, a.RuntimeOutcomeUnknown, snapshot.OutcomeState)
				var rows []notificationoutbox.Row
				require.NoError(t, f.db.Find(&rows).Error)
				require.Empty(t, rows, "transport hold must not consume the notice key")
				require.NoError(t, f.control.Close(t.Context()))
				// No worker is running: the old worker joined above; its in-memory result
				// is intentionally discarded, as on an executor process restart.
				restarted := activity.NewRuntimeService(activity.NewActivityService(f.agent.db, nil), f.agent.adapter)
				count, err := restarted.SettleLocalStartup(t.Context(), f.agent.store)
				require.NoError(t, err)
				require.EqualValues(t, 1, count)
				f.handlerMu.Lock()
				f.receiverHandler = original
				f.handlerMu.Unlock()
				coordinator := f.control
				require.NoError(t, coordinator.Startup(t.Context()))
				snapshot, err = coordinator.Refresh(t.Context(), ref)
				require.NoError(t, err)
				require.Equal(t, a.RuntimeOutcomeKnown, snapshot.OutcomeState)
				require.Equal(t, "runtime.interrupted", snapshot.MessageCode)
				require.False(t, snapshot.ImageLifecycle.Verified)
				require.Equal(t, "interrupted", snapshot.ImageLifecycle.Recovery.Outcome)
				_, err = coordinator.Refresh(t.Context(), ref)
				require.NoError(t, err)
				rows = nil
				require.NoError(t, f.db.Find(&rows).Error)
				if mode == "scheduled" {
					require.Len(t, rows, 1)
					require.Equal(t, "update-interrupted", rows[0].Payload["reason"])
				} else {
					require.Empty(t, rows)
				}
				require.EqualValues(t, 1, f.executeCalls.Load())
			})
		}
	}
}

func TestRecoveryNoticePayloadCannotBindHub(t *testing.T) {
	for _, edge := range []bool{false, true} {
		t.Run(fmt.Sprint(edge), func(t *testing.T) {
			worker := &observedRemoteProjects{result: dto.LifecycleResult{Code: "update.unstable", HTTPStatus: 500, Recovery: &dto.RecoveryResult{Outcome: "restored"}, Applied: []dto.AppliedCandidate{{Service: "app", Repository: "registry.example.com/app", Platform: "linux/amd64", Manifest: "sha256:" + strings.Repeat("a", 64)}}}, seen: make(chan dto.ObservedExecution, 1)}
			f := newRemoteIntegration(t, edge, func(agent *runtimeServicesFixture) {
				worker.ProjectService = agent.projects
				agent.adapter = activity.NewRuntimeServiceAdapter(worker, agent.containers)
			})
			target := activity.RuntimeTarget{Kind: "stack", ID: "example-project"}
			f.agent.project(target.ID, target.ID, "name: example_stack\nservices:\n  app:\n    image: registry.example.com/app:1\n")
			command := bindNoticeRemote(t, f, f.command(activity.RuntimeUpdateProject, target), "manual")
			admitted, err := f.hub.Admit(t.Context(), command)
			require.NoError(t, err)
			// Remove only the hub marker to model a forged bound claim arriving at a
			// hub whose trusted admission was unobserved. No payload can recreate it.
			require.NoError(t, f.db.Delete(&activity.RuntimeObservedRun{}, "run_id = ?", admitted.ActivityID).Error)
			ref := activity.RuntimeRunReference{Store: f.store, RunID: admitted.ActivityID}
			_, err = f.control.Start(t.Context(), ref, command.Options)
			require.NoError(t, err)
			require.NoError(t, f.control.WaitDispatch(t.Context(), ref.RunID))
			_, err = f.agent.waitLocal(ref.RunID)
			require.NoError(t, err)
			snapshot, err := f.control.Refresh(t.Context(), ref)
			require.NoError(t, err)
			require.Equal(t, a.RuntimeOutcomeUnknown, snapshot.OutcomeState)
			var rows []notificationoutbox.Row
			require.NoError(t, f.db.Find(&rows).Error)
			require.Empty(t, rows)
		})
	}
}

type noticeSuccessfulProjects struct{ *project.ProjectService }

func (p *noticeSuccessfulProjects) ExecuteImageLifecycle(ctx context.Context, _, run, _ string, _ *dto.LifecycleIntent, _ user.Actor) (*dto.LifecycleResult, error) {
	result := dto.LifecycleResult{RunID: run, Verified: true}
	if dto.ObservedExecutionFromContext(ctx).Mode != "" {
		result.Recovery = &dto.RecoveryResult{Outcome: "not-needed"}
	}
	dto.RecordLifecycleResult(ctx, result)
	return &result, nil
}
func TestRecoverySuppressionRemoteManualSuccessClears(t *testing.T) {
	for _, edge := range []bool{false, true} {
		for _, mode := range []string{"unobserved", "manual"} {
			t.Run(fmt.Sprint(edge, "/", mode), func(t *testing.T) {
				f := newRemoteIntegration(t, edge, func(agent *runtimeServicesFixture) {
					agent.adapter = activity.NewRuntimeServiceAdapter(&noticeSuccessfulProjects{agent.projects}, agent.containers)
				})
				target := activity.RuntimeTarget{Kind: "stack", ID: "example-project"}
				f.agent.project(target.ID, target.ID, "name: example_stack\nservices:\n  app:\n    image: registry.example.com/app:1\n")
				command := bindNoticeRemote(t, f, f.command(activity.RuntimeUpdateProject, target), mode)
				require.NoError(t, f.db.Exec("INSERT OR IGNORE INTO environments(id,name,api_url,enabled) VALUES(?,?,?,1)", f.store.EnvironmentID, "Example", "https://agent.example.com").Error)
				store := policy.Store{DB: f.db.DB}
				one, err := store.Policy(t.Context(), f.store.EnvironmentID, f.store.RegistrationID, target.ID)
				require.NoError(t, err)
				one.Mode = "check"
				require.NoError(t, store.SavePolicy(t.Context(), one, 0))
				require.NoError(t, f.db.Create(&policy.FailedCandidate{PolicyID: one.ID, TargetIdentity: "historical-identity", Repository: "registry.example.com/app", Platform: "linux/amd64", Manifest: "sha256:" + strings.Repeat("a", 64), RunID: "previous-run", CreatedAt: time.Now().UTC()}).Error)
				admitted, err := f.hub.Admit(t.Context(), command)
				require.NoError(t, err)
				ref := activity.RuntimeRunReference{Store: f.store, RunID: admitted.ActivityID}
				_, err = f.control.Start(t.Context(), ref, command.Options)
				require.NoError(t, err)
				require.NoError(t, f.control.WaitDispatch(t.Context(), ref.RunID))
				_, err = f.agent.waitLocal(ref.RunID)
				require.NoError(t, err)
				snapshot, err := f.control.Refresh(t.Context(), ref)
				require.NoError(t, err)
				require.Equal(t, a.StatusSuccess, snapshot.Status)
				rows, err := store.FailedCandidates(t.Context(), one.ID, "historical-identity")
				require.NoError(t, err)
				require.Empty(t, rows)
				var notices []notificationoutbox.Row
				require.NoError(t, f.db.Find(&notices).Error)
				require.Empty(t, notices)
			})
		}
	}
}
