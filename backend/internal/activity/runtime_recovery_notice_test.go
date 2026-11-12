// Source: docklet-next/issues/440
// Revision: 6e3afa6a78f4f60ffd74858d9a1eaefbb1b73bda
// Retrieved: 2026-11-12
// License: Apache-2.0
// Deviations: Original terminal recovery notice, startup and suppression-clear integration proofs.

package activity

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/notification/notificationoutbox"
	policy "github.com/florianfinn/docklet-next/backend/internal/updater/children/policies"
	a "github.com/florianfinn/docklet-next/types/activity"
	containerDTO "github.com/florianfinn/docklet-next/types/container"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func noticeCommand(t *testing.T, f *b1FixtureInternal, mode, kind string) RuntimeCommand {
	t.Helper()
	var c RuntimeCommand
	if mode == "scheduled" {
		c, _ = policyCommand(t, f)
	} else {
		c = imageCommand(f.command(), RuntimeUpdateProject)
		c.Options.Lifecycle.Request.AutoRestore = mode == "manual"
	}
	if kind == "container" {
		// Container admission freezes the canonical name independently of Docker ID.
		c.Operation = RuntimeUpdateContainer
		c.Target = RuntimeTarget{Kind: "container", ID: "example-container-id", Name: new("app")}
		c.Options.ContainerImage = &dto.ContainerImageRequest{AutoRestore: mode == "manual"}
		if mode == "scheduled" {
			store := policy.Store{DB: f.store.svc.db.DB}
			p, err := store.Policy(t.Context(), c.Store.EnvironmentID, c.Store.RegistrationID, "app", "container")
			require.NoError(t, err)
			p.Mode, p.ActorID, p.TargetIdentity = "scheduled", c.Actor.ID, "app"
			p.SetWindow(&dto.UpdateWindow{Kind: "daily", Time: "04:00"})
			require.NoError(t, store.SavePolicy(t.Context(), p, 0))
			p.Revision = 1
			child, _, err := store.PrepareChild(t.Context(), p, "container-window")
			require.NoError(t, err)
			c.ProposedRunID, c.Key = child.ID, child.ID
			c.Options.ContainerImage.Policy = &dto.PolicyBinding{PolicyID: p.ID, Revision: 1, JobRunID: child.JobRunID, ChildID: child.ID}
		}
		c.Options.Lifecycle = nil
		_, err := NewRuntimeHTTPService(f.service, RuntimeHTTPPorts{ApplicationContext: t.Context(), Store: func(context.Context, string) (RuntimeStoreBinding, error) { return c.Store, nil }, PrepareTarget: func(_ context.Context, _ string, target RuntimeTarget) (RuntimeResolvedTarget, error) {
			return RuntimeResolvedTarget{Target: target}, nil
		}, GuardContainer: func(context.Context, string, string) error { return nil }})
		require.NoError(t, err)
	}
	return c
}

func noticeRows(t *testing.T, f *b1FixtureInternal) []notificationoutbox.Row {
	t.Helper()
	var rows []notificationoutbox.Row
	require.NoError(t, f.store.svc.db.Order("dedup_key").Find(&rows).Error)
	return rows
}

func completeNotice(t *testing.T, f *b1FixtureInternal, c RuntimeCommand, run string, result runtimeLocalResultInternal) {
	t.Helper()
	ctx := context.WithValue(t.Context(), gateRunContextKey{}, run)
	require.NoError(t, f.service.transactionInternal(ctx, func(tx *gorm.DB, now time.Time) error {
		_, _, err := runtimeCompleteTxInternal(tx, now, b1Identity(t, c, run), result)
		return err
	}))
}

func TestRecoveryNoticeLocalOutcomesAndMode(t *testing.T) {
	for _, mode := range []string{"scheduled", "manual"} {
		for _, kind := range []string{"stack", "container"} {
			for _, outcome := range []string{"restored", "restore-failed", "unavailable", "not-needed", "interrupted", "candidate-failed", "success", "unknown"} {
				t.Run(mode+"/"+kind+"/"+outcome, func(t *testing.T) {
					f := b1Fixture(t)
					c := noticeCommand(t, f, mode, kind)
					admitted, err := f.service.Admit(t.Context(), c)
					require.NoError(t, err)
					life := &dto.LifecycleResult{RunID: admitted.ActivityID, Code: "update.unstable", HTTPStatus: 500, Recovery: &dto.RecoveryResult{Outcome: outcome}}
					result := runtimeLocalResultInternal{state: "failed", code: a.CodeRuntimeFailed, message: "Update failed", settledAt: f.store.now, imageLifecycle: life}
					want := map[string]string{"restored": "auto-restored", "restore-failed": "auto-restore-failed", "unavailable": "auto-restore-unavailable", "not-needed": "runtime.failed", "interrupted": "update-interrupted"}[outcome]
					switch outcome {
					case "unavailable":
						life.Recovery.Code = "restore.files-changed"
					case "interrupted":
						result.code = a.CodeRuntimeInterrupted
					case "candidate-failed":
						life.Code, life.Recovery.Outcome = "update.candidate-failed", "not-needed"
					case "success":
						life.Code, life.HTTPStatus, life.Verified, life.Recovery.Outcome = "", 0, true, "not-needed"
						result.state, result.code = "success", a.CodeActivitySuccess
						b3Phase(t, f, c, admitted.ActivityID, "executing")
					case "unknown":
						life.Recovery = nil
					}
					result.startedAt = b3Receipt(t, f, admitted.ActivityID).ExecutorStartedAt
					completeNotice(t, f, c, admitted.ActivityID, result)
					if outcome != "unknown" {
						result.startedAt = b3Receipt(t, f, admitted.ActivityID).ExecutorStartedAt
						completeNotice(t, f, c, admitted.ActivityID, result)
					}
					rows := noticeRows(t, f)
					if outcome == "success" || outcome == "unknown" || mode == "scheduled" && outcome == "candidate-failed" || mode == "manual" && outcome == "interrupted" {
						require.Empty(t, rows)
						return
					}
					if mode == "manual" {
						want = "runtime.failed"
					}
					require.Len(t, rows, 1)
					require.Equal(t, "deploy_failed", rows[0].Event)
					require.Equal(t, want, rows[0].Payload["reason"])
					require.Equal(t, "deploy:0:"+admitted.ActivityID, rows[0].DedupKey)
					if mode == "scheduled" {
						require.Equal(t, true, rows[0].Payload["scheduled"])
					} else {
						require.NotContains(t, rows[0].Payload, "scheduled")
					}
					if outcome == "unavailable" && mode == "scheduled" {
						require.Equal(t, "restore.files-changed", rows[0].Payload["recoveryReason"])
					}
				})
			}
		}
	}
}

func TestRecoveryNoticeStartupByAdmissionMode(t *testing.T) {
	for _, mode := range []string{"scheduled", "manual", "unobserved"} {
		for _, kind := range []string{"stack", "container"} {
			t.Run(mode+"/"+kind, func(t *testing.T) {
				f := b1Fixture(t)
				c := noticeCommand(t, f, mode, kind)
				admitted, err := f.service.Admit(t.Context(), c)
				require.NoError(t, err)
				b3Phase(t, f, c, admitted.ActivityID, "executing")
				count, err := f.service.SettleLocalStartup(t.Context(), c.Store)
				require.NoError(t, err)
				require.EqualValues(t, 1, count)
				snapshot, err := f.service.boundSnapshotInternal(t.Context(), RuntimeRunReference{Store: c.Store, RunID: admitted.ActivityID})
				require.NoError(t, err)
				require.Equal(t, a.StatusFailed, snapshot.Status)
				require.Equal(t, "runtime.interrupted", snapshot.MessageCode)
				require.Equal(t, a.RuntimeOutcomeKnown, snapshot.OutcomeState)
				if mode != "unobserved" {
					require.NotNil(t, snapshot.ImageLifecycle)
					require.False(t, snapshot.ImageLifecycle.Verified)
					require.Equal(t, "interrupted", snapshot.ImageLifecycle.Recovery.Outcome)
				}
				rows := noticeRows(t, f)
				if mode == "scheduled" {
					require.Len(t, rows, 1)
					require.Equal(t, "update-interrupted", rows[0].Payload["reason"])
				} else {
					require.Empty(t, rows)
				}
				count, err = f.service.SettleLocalStartup(t.Context(), c.Store)
				require.NoError(t, err)
				require.Zero(t, count)
			})
		}
	}
}

func TestRecoveryNoticeUnknownThenDisposition(t *testing.T) {
	for _, disposition := range []string{"interrupted", "released"} {
		t.Run(disposition, func(t *testing.T) {
			f := b1Fixture(t)
			c := noticeCommand(t, f, "scheduled", "stack")
			admitted, err := f.service.Admit(t.Context(), c)
			require.NoError(t, err)
			completeNotice(t, f, c, admitted.ActivityID, runtimeLocalResultInternal{state: "failed", code: a.CodeRuntimeFailed, message: "Unknown", settledAt: f.store.now, imageLifecycle: &dto.LifecycleResult{RunID: admitted.ActivityID, Code: "runtime.outcome-unknown", HTTPStatus: 503}})
			require.Empty(t, noticeRows(t, f))
			for range 2 {
				_, err = f.service.remoteDispositionInternal(t.Context(), RuntimeRunReference{Store: c.Store, RunID: admitted.ActivityID}, runtimeDispositionInternal{Disposition: disposition, By: "example-admin", Reason: "Target inspected", ReleaseAcknowledged: disposition == "released"})
				require.NoError(t, err)
			}
			rows := noticeRows(t, f)
			require.Len(t, rows, 1)
			require.Equal(t, "update-interrupted", rows[0].Payload["reason"])
			var child policy.Child
			require.NoError(t, f.store.svc.db.First(&child, "id = ?", admitted.ActivityID).Error)
			if disposition == "released" {
				require.Equal(t, "settled", child.Status)
				require.NotNil(t, child.SettledAt)
				require.Equal(t, "runtime.interrupted", child.Code)
			} else {
				require.Equal(t, "admitted", child.Status)
			}
			var count int64
			require.NoError(t, f.store.svc.db.Model(&policy.FailedCandidate{}).Count(&count).Error)
			require.Zero(t, count)
		})
	}
}

func TestRecoveryNoticeRunIdentity(t *testing.T) {
	f := b1Fixture(t)
	for i := 0; i < 2; i++ {
		c := noticeCommand(t, f, "manual", "stack")
		admitted, err := f.service.Admit(t.Context(), c)
		require.NoError(t, err)
		result := runtimeLocalResultInternal{state: "failed", code: a.CodeRuntimeFailed, message: "Failed", settledAt: f.store.now, imageLifecycle: &dto.LifecycleResult{RunID: admitted.ActivityID, Code: "update.unstable", HTTPStatus: 500, Recovery: &dto.RecoveryResult{Outcome: "restored"}}}
		for range 2 {
			result.startedAt = b3Receipt(t, f, admitted.ActivityID).ExecutorStartedAt
			completeNotice(t, f, c, admitted.ActivityID, result)
		}
		require.Len(t, noticeRows(t, f), i+1, fmt.Sprint(i))
	}
}

func TestRecoverySuppressionClearOnlySuccessfulManualUpdate(t *testing.T) {
	for _, kind := range []string{"stack", "container"} {
		for _, mode := range []string{"unobserved", "manual", "scheduled"} {
			for _, ending := range []string{"success", "failed", "restore", "clear-fault"} {
				t.Run(kind+"/"+mode+"/"+ending, func(t *testing.T) {
					f := b1Fixture(t)
					c := noticeCommand(t, f, mode, kind)
					if ending == "restore" {
						if mode != "unobserved" {
							return
						} // Observed restore commands are invalid.
						if kind == "stack" {
							c = imageCommand(c, RuntimeRestoreProject)
						} else {
							c.Operation = RuntimeRestoreContainerImage
						}
					}
					target := c.Target.ID
					if kind == "container" {
						target = *c.Target.Name
					}
					require.NoError(t, f.store.svc.db.Exec("INSERT OR IGNORE INTO environments(id,name,api_url,enabled) VALUES(?,?,?,1)", c.Store.EnvironmentID, "Example", "").Error)
					store := policy.Store{DB: f.store.svc.db.DB}
					one, err := store.Policy(t.Context(), c.Store.EnvironmentID, c.Store.RegistrationID, target, kind)
					require.NoError(t, err)
					if one.Revision == 0 {
						one.Mode, one.TargetIdentity = "check", "historical-identity"
						require.NoError(t, store.SavePolicy(t.Context(), one, 0))
					}
					tuple := policy.FailedCandidate{PolicyID: one.ID, TargetIdentity: "historical-identity", Repository: "registry.example.com/app", Platform: "linux/amd64", Manifest: "sha256:" + strings.Repeat("a", 64), RunID: "previous-run", CreatedAt: f.store.now}
					require.NoError(t, f.store.svc.db.Create(&tuple).Error)
					neighbour, err := store.Policy(t.Context(), c.Store.EnvironmentID, c.Store.RegistrationID, "other-target", kind)
					require.NoError(t, err)
					neighbour.Mode, neighbour.TargetIdentity = "check", "other-identity"
					require.NoError(t, store.SavePolicy(t.Context(), neighbour, 0))
					other := tuple
					other.PolicyID = neighbour.ID
					require.NoError(t, f.store.svc.db.Create(&other).Error)
					admitted, err := f.service.Admit(t.Context(), c)
					require.NoError(t, err)
					b3Phase(t, f, c, admitted.ActivityID, "executing")
					life := &dto.LifecycleResult{RunID: admitted.ActivityID, Verified: true}
					if mode != "unobserved" {
						life.Recovery = &dto.RecoveryResult{Outcome: "not-needed"}
					}
					result := runtimeLocalResultInternal{state: "success", code: a.CodeActivitySuccess, message: "Completed", settledAt: f.store.now, imageLifecycle: life}
					if ending == "failed" {
						result.state, result.code = "failed", a.CodeRuntimeFailed
						life.Verified, life.Code, life.HTTPStatus = false, "update.deploy-failed", 500
					}
					result.startedAt = b3Receipt(t, f, admitted.ActivityID).ExecutorStartedAt
					if ending == "clear-fault" && mode != "scheduled" {
						require.NoError(t, f.store.svc.db.Exec("CREATE TRIGGER clear_failed_fault BEFORE DELETE ON image_policy_failed_candidates BEGIN SELECT RAISE(ABORT,'synthetic clear failure'); END").Error)
						ctx := context.WithValue(t.Context(), gateRunContextKey{}, admitted.ActivityID)
						require.Error(t, f.service.transactionInternal(ctx, func(tx *gorm.DB, now time.Time) error {
							_, _, err := runtimeCompleteTxInternal(tx, now, b1Identity(t, c, admitted.ActivityID), result)
							return err
						}))
						require.Nil(t, b3Receipt(t, f, admitted.ActivityID).SettledAt)
						require.NoError(t, f.store.svc.db.Exec("DROP TRIGGER clear_failed_fault").Error)
					}
					result.startedAt = b3Receipt(t, f, admitted.ActivityID).ExecutorStartedAt
					completeNotice(t, f, c, admitted.ActivityID, result)
					rows, err := store.FailedCandidates(t.Context(), one.ID, "historical-identity")
					require.NoError(t, err)
					if mode != "scheduled" && ending != "failed" && ending != "restore" {
						require.Empty(t, rows)
					} else {
						require.Len(t, rows, 1)
					}
					rows, err = store.FailedCandidates(t.Context(), neighbour.ID, "historical-identity")
					require.NoError(t, err)
					require.Len(t, rows, 1)
					if mode == "manual" {
						var count int64
						require.NoError(t, f.store.svc.db.Model(&policy.Child{}).Count(&count).Error)
						require.Zero(t, count)
					}
				})
			}
		}
	}
}

func TestRecoveryNoticeContainerStartupRecoveryPort(t *testing.T) {
	for _, mode := range []string{"scheduled", "manual"} {
		for _, ending := range []string{"done", "rolled-back"} {
			t.Run(mode+"/"+ending, func(t *testing.T) {
				f := b1Fixture(t)
				c := noticeCommand(t, f, mode, "container")
				admitted, err := f.service.Admit(t.Context(), c)
				require.NoError(t, err)
				b3Phase(t, f, c, admitted.ActivityID, "executing")
				var marker RuntimeObservedRun
				require.NoError(t, f.store.svc.db.First(&marker, "run_id = ?", admitted.ActivityID).Error)
				require.Empty(t, marker.Evidence)
				recovered := containerDTO.RecreateRecovery{RunID: admitted.ActivityID}
				if ending == "done" {
					recovered.Result = &containerDTO.RecreateResult{OldID: c.Target.ID, NewID: strings.Repeat("b", 64), OldName: "app", NewName: "app"}
				} else {
					recovered.Code = "container.recreate-recovered"
				}
				adapter := NewRuntimeServiceAdapter(nil, &observedAdapterContainer{recovered: recovered})
				require.NoError(t, f.service.ConfigureLocal(RuntimeLocalConfig{ApplicationContext: t.Context(), Store: c.Store, Executor: adapter}))
				require.NoError(t, f.service.InitializeLocalStartup(t.Context(), c.Store))
				snapshot, err := f.service.boundSnapshotInternal(t.Context(), RuntimeRunReference{Store: c.Store, RunID: admitted.ActivityID})
				require.NoError(t, err)
				require.Equal(t, a.StatusFailed, snapshot.Status)
				require.Equal(t, "runtime.interrupted", snapshot.MessageCode)
				require.False(t, snapshot.ImageLifecycle.Verified)
				require.Equal(t, "interrupted", snapshot.ImageLifecycle.Recovery.Outcome)
				require.Equal(t, recovered.Result, snapshot.Result)
				rows := noticeRows(t, f)
				if mode == "scheduled" {
					require.Len(t, rows, 1)
					require.Equal(t, "update-interrupted", rows[0].Payload["reason"])
				} else {
					require.Empty(t, rows)
				}
			})
		}
	}
}
