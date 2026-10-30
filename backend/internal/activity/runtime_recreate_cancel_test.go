// Source: docklet-next/issues/315
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-30
// License: Apache-2.0
// Deviations: Original in-process recreate settlement after cancellation mid-effect.

package activity_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	containers "github.com/florianfinn/docklet-next/backend/internal/container"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	usertypes "github.com/florianfinn/docklet-next/types/user"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// Cancelling after the first journal write must not strand a renamed or stopped
// original until a later process start: the detached sequence settles in-process.
func TestRecreateCancelMidEffectSettlesInProcess(t *testing.T) {
	for _, mode := range []string{"local", "direct", "edge"} {
		for _, createFails := range []bool{false, true} {
			name := mode + "/completes"
			if createFails {
				name = mode + "/compensates"
			}
			t.Run(name, func(t *testing.T) {
				hub := activationApp(t, false, "")
				executor, environment := hub, "0"
				if mode != "local" {
					executor = activationApp(t, true, uuid.NewString())
					environment = activationRemote(t, hub, executor, mode)
				}
				daemon := newRecreateHTTP(t)
				daemon.hold = "stop"
				daemon.fail["create"] = createFails
				daemon.entered, daemon.release = make(chan struct{}), make(chan struct{})
				unblock := sync.OnceFunc(func() { close(daemon.release) })
				defer unblock()
				executor.services.http.onCall = daemon.serve
				path := "/api/environments/" + environment + "/containers/" + runtimeContainerID
				status, body := activationCall(t, hub, path+"/recreate", uuid.NewString(), "respond-async", []byte(`{"edit":{}}`))
				require.Equal(t, 202, status, string(body))
				run := activationRunID(t, body)
				select {
				case <-daemon.entered:
				case <-time.After(5 * time.Second):
					t.Fatal("recreate never reached the held stop")
				}
				status, body = activationCall(t, hub, "/api/environments/"+environment+"/activities/"+run+"/cancel", "", "", nil)
				require.Equal(t, 200, status, string(body))
				unblock()
				ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
				defer cancel()
				ref, found, err := executor.services.runtime.Reference(ctx, "0", run)
				require.NoError(t, err)
				require.True(t, found)
				_, err = executor.services.runtime.WaitLocal(ctx, ref)
				require.NoError(t, err)
				var row activity.RuntimeReceipt
				require.NoError(t, executor.services.db.First(&row, "run_id=?", run).Error)
				require.NotNil(t, row.SettledAt, "the executor settles without a process restart")
				old, oldPresent := daemon.copy(runtimeContainerID)
				replacement, replacementPresent := daemon.copy(recreateNewID)
				if createFails {
					require.Equal(t, "failed", *row.ExecutorState)
					require.Equal(t, "container.recreate-recovered", *row.ExecutorResultCode)
					require.Equal(t, "container.recreate-recovered: create failed", *row.ExecutorError)
					require.True(t, oldPresent)
					require.Equal(t, "app", old.Name)
					require.True(t, old.Running)
					require.False(t, replacementPresent)
					return
				}
				require.Equal(t, "success", *row.ExecutorState)
				require.Equal(t, string(activitytypes.CodeActivitySuccess), *row.ExecutorResultCode)
				require.NotNil(t, row.Result)
				require.False(t, oldPresent)
				require.True(t, replacementPresent)
				require.True(t, replacement.Running)
			})
		}
	}
}

// Release would settle the receipt and hide a renamed or stopped original from
// startup recovery, so it waits for the journal to resolve.
func TestRecreateReleaseRefusedWhileJournalUnresolved(t *testing.T) {
	var run string
	setup := activationSetup{BeforeStart: func(f *runtimeServicesFixture, _ *activity.RuntimeHTTPService, _ *activity.RuntimeReceiver) {
		previous := activity.NewRuntimeService(activity.NewActivityService(f.db, f.settings), activity.RuntimeResolverFunc(func(_ context.Context, target activity.RuntimeTarget) (activity.RuntimeResolvedTarget, error) {
			return activity.RuntimeResolvedTarget{Target: target}, nil
		}))
		store := f.store
		store.ExecutorGeneration = uuid.NewString()
		var original struct{ ID string }
		require.NoError(t, f.db.Table("users").Where("username = ?", "arcane").Select("id").Scan(&original).Error)
		command := activity.RuntimeCommand{Store: store, Key: uuid.NewString(), Actor: usertypes.Actor{ID: original.ID, Username: "Historical operator"}, Operation: activity.RuntimeStartContainer, Target: activity.RuntimeTarget{Kind: "container", ID: runtimeContainerID}}
		admitted, err := previous.Admit(t.Context(), command)
		require.NoError(t, err)
		run = admitted.ActivityID
	}}
	hub := activationApp(t, false, "", setup)
	journal := containers.RecreateJournal{RunID: run, OldID: runtimeContainerID, OldName: "app", BackupName: "app-docklet-recreate-" + run, NewName: "app", WasRunning: true, Phase: "renamed", UpdatedAt: time.Now().UTC()}
	require.NoError(t, hub.services.db.Create(&journal).Error)
	releasePath := "/api/environments/0/runtime-runs/" + run + "/release"
	release := []byte(`{"reason":"Executor stopped and Docker state inspected","acknowledged":true}`)
	status, response := activationCall(t, hub, releasePath, "", "", release)
	require.Equal(t, 409, status, string(response))
	var row activity.RuntimeReceipt
	require.NoError(t, hub.services.db.First(&row, "run_id=?", run).Error)
	require.Nil(t, row.SettledAt)
	require.NoError(t, hub.services.db.Model(&containers.RecreateJournal{}).Where("run_id=?", run).Update("phase", "ambiguous").Error)
	status, response = activationCall(t, hub, releasePath, "", "", release)
	require.Equal(t, 200, status, string(response))
}

// A recovery read that fails after a failed effect is no outcome: the run stays
// held as unknown with its journal, and the application-owned retry resolves it.
func TestRecreateRecoveryReadFailureStaysHeldUntilRetry(t *testing.T) {
	for _, mode := range []string{"local", "direct", "edge"} {
		t.Run(mode, func(t *testing.T) {
			hub := activationApp(t, false, "")
			executor, environment := hub, "0"
			if mode != "local" {
				executor = activationApp(t, true, uuid.NewString())
				environment = activationRemote(t, hub, executor, mode)
			}
			daemon := newRecreateHTTP(t)
			daemon.fail["create"] = true
			daemon.inspectOutage = 1
			executor.services.http.onCall = daemon.serve
			path := "/api/environments/" + environment + "/containers/" + runtimeContainerID
			status, body := activationCall(t, hub, path+"/recreate", uuid.NewString(), "respond-async", []byte(`{"edit":{}}`))
			require.Equal(t, 202, status, string(body))
			run := activationRunID(t, body)
			var row activity.RuntimeReceipt
			require.Eventually(t, func() bool {
				row = activity.RuntimeReceipt{}
				if executor.services.db.First(&row, "run_id=?", run).Error != nil {
					return false
				}
				return row.SettledAt != nil || (row.ExecutorState != nil && *row.ExecutorState == "unknown")
			}, 10*time.Second, 20*time.Millisecond)
			require.Nil(t, row.SettledAt, "an unresolved journal keeps the run held")
			var journal containers.RecreateJournal
			require.NoError(t, executor.services.db.First(&journal, "run_id=?", run).Error)
			require.Equal(t, "intent-create", journal.Phase)
			old, _ := daemon.copy(runtimeContainerID)
			require.False(t, old.Running)

			require.NoError(t, executor.services.runtime.RetryRecreateRecovery(t.Context()))
			row = activity.RuntimeReceipt{}
			require.NoError(t, executor.services.db.First(&row, "run_id=?", run).Error)
			require.NotNil(t, row.SettledAt)
			require.Equal(t, "failed", *row.ExecutorState)
			require.Equal(t, "container.recreate-recovered", *row.ExecutorResultCode)
			old, _ = daemon.copy(runtimeContainerID)
			require.Equal(t, "app", old.Name)
			require.True(t, old.Running)
			_, replacementPresent := daemon.copy(recreateNewID)
			require.False(t, replacementPresent)
			require.Equal(t, 1, daemon.counts()["create"], "recovery never replays create")
		})
	}
}

// The maintenance retry never touches a journal whose live attempt still owns it.
func TestRecreateRecoveryRetrySkipsLiveAttempt(t *testing.T) {
	hub := activationApp(t, false, "")
	daemon := newRecreateHTTP(t)
	daemon.hold = "stop"
	daemon.entered, daemon.release = make(chan struct{}), make(chan struct{})
	unblock := sync.OnceFunc(func() { close(daemon.release) })
	defer unblock()
	hub.services.http.onCall = daemon.serve
	status, body := activationCall(t, hub, "/api/environments/0/containers/"+runtimeContainerID+"/recreate", uuid.NewString(), "respond-async", []byte(`{"edit":{}}`))
	require.Equal(t, 202, status, string(body))
	run := activationRunID(t, body)
	select {
	case <-daemon.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("recreate never reached the held stop")
	}
	before := daemon.counts()
	require.NoError(t, hub.services.runtime.RetryRecreateRecovery(t.Context()))
	require.Equal(t, before, daemon.counts(), "a live attempt's containers are untouched")
	var journal containers.RecreateJournal
	require.NoError(t, hub.services.db.First(&journal, "run_id=?", run).Error)
	require.Equal(t, "intent-create", journal.Phase)
	unblock()
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	ref, found, err := hub.services.runtime.Reference(ctx, "0", run)
	require.NoError(t, err)
	require.True(t, found)
	_, err = hub.services.runtime.WaitLocal(ctx, ref)
	require.NoError(t, err)
	var row activity.RuntimeReceipt
	require.NoError(t, hub.services.db.First(&row, "run_id=?", run).Error)
	require.Equal(t, "success", *row.ExecutorState)
}
