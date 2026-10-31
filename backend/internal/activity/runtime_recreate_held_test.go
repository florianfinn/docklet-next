// Source: docklet-next/issues/315
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-30
// License: Apache-2.0
// Deviations: Original held recreate outcomes through the retained settlement writer.

package activity_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	containers "github.com/florianfinn/docklet-next/backend/internal/container"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func heldRecreateReceipt(t *testing.T, f *activationFixture, run string) activity.RuntimeReceipt {
	t.Helper()
	var row activity.RuntimeReceipt
	require.NoError(t, f.services.db.First(&row, "run_id=?", run).Error)
	return row
}

func outageConsumed(d *recreateHTTP) func() bool {
	return func() bool {
		d.mu.Lock()
		defer d.mu.Unlock()
		return d.outage && d.inspectOutage == 0
	}
}

// A recovery read that fails after a failed effect is no outcome: the run stays
// held as unknown with its journal and its exclusion until startup recovery.
func TestRecreateRecoveryReadFailureStaysHeld(t *testing.T) {
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
			require.Eventually(t, outageConsumed(daemon), 10*time.Second, 10*time.Millisecond)
			require.NoError(t, executor.services.runtime.JoinLocal(t.Context()))
			row := heldRecreateReceipt(t, executor, run)
			require.Nil(t, row.SettledAt, "an unresolved journal keeps the run held")
			require.Equal(t, "unknown", *row.ExecutorState)
			var journal containers.RecreateJournal
			require.NoError(t, executor.services.db.First(&journal, "run_id=?", run).Error)
			require.Equal(t, "intent-create", journal.Phase)
			_, body = activationCall(t, hub, path+"/recreate", uuid.NewString(), "respond-async", []byte(`{"edit":{}}`))
			require.Equal(t, run, activationRunID(t, body), "the held run keeps its container exclusion")
			require.Equal(t, 1, daemon.counts()["create"])
		})
	}
}

// A failed held-evidence write keeps the joined result for the retained retry
// instead of leaving an owner that no path can settle.
func TestRecreateHeldEvidenceWriteFailureIsRetried(t *testing.T) {
	hub := activationApp(t, false, "")
	daemon := newRecreateHTTP(t)
	daemon.fail["create"] = true
	daemon.inspectOutage = 1
	hub.services.http.onCall = daemon.serve
	require.NoError(t, hub.services.db.Exec("CREATE TRIGGER fixture_held_refusal BEFORE UPDATE OF executor_state ON runtime_receipts WHEN NEW.executor_state='unknown' BEGIN SELECT RAISE(ABORT,'synthetic held refusal'); END").Error)
	status, body := activationCall(t, hub, "/api/environments/0/containers/"+runtimeContainerID+"/recreate", uuid.NewString(), "respond-async", []byte(`{"edit":{}}`))
	require.Equal(t, 202, status, string(body))
	run := activationRunID(t, body)
	require.Eventually(t, outageConsumed(daemon), 10*time.Second, 10*time.Millisecond)
	require.ErrorContains(t, hub.services.runtime.JoinLocal(t.Context()), "synthetic held refusal")
	require.Equal(t, "executing", *heldRecreateReceipt(t, hub, run).ExecutorState)
	require.NoError(t, hub.services.db.Exec("DROP TRIGGER fixture_held_refusal").Error)
	require.NoError(t, hub.services.runtime.JoinLocal(t.Context()))
	row := heldRecreateReceipt(t, hub, run)
	require.Equal(t, "unknown", *row.ExecutorState, "the retained retry commits the held evidence")
	require.Nil(t, row.SettledAt)
}

// The journal check runs inside the terminal transaction: an unreadable journal
// retains the joined success instead of discarding it as unknown.
func TestRecreateUnreadableJournalRetainsResolvedOutcome(t *testing.T) {
	hub := activationApp(t, false, "")
	daemon := newRecreateHTTP(t)
	hub.services.http.onCall = daemon.serve
	require.NoError(t, hub.services.db.Exec("CREATE TRIGGER fixture_settle_refusal BEFORE UPDATE OF executor_settled_at ON runtime_receipts WHEN NEW.executor_settled_at IS NOT NULL BEGIN SELECT RAISE(ABORT,'synthetic settlement refusal'); END").Error)
	status, body := activationCall(t, hub, "/api/environments/0/containers/"+runtimeContainerID+"/recreate", uuid.NewString(), "respond-async", []byte(`{"edit":{}}`))
	require.Equal(t, 202, status, string(body))
	run := activationRunID(t, body)
	require.Eventually(t, func() bool {
		var journal containers.RecreateJournal
		return hub.services.db.First(&journal, "run_id=?", run).Error == nil && journal.Phase == "done"
	}, 10*time.Second, 10*time.Millisecond)
	require.ErrorContains(t, hub.services.runtime.JoinLocal(t.Context()), "synthetic settlement refusal")
	require.NoError(t, hub.services.db.Exec("DROP TRIGGER fixture_settle_refusal").Error)
	require.NoError(t, hub.services.db.Exec("ALTER TABLE container_recreate_journal RENAME TO fixture_hidden_journal").Error)
	require.Error(t, hub.services.runtime.JoinLocal(t.Context()), "an unreadable journal is no outcome")
	require.Equal(t, "executing", *heldRecreateReceipt(t, hub, run).ExecutorState)
	require.NoError(t, hub.services.db.Exec("ALTER TABLE fixture_hidden_journal RENAME TO container_recreate_journal").Error)
	require.NoError(t, hub.services.runtime.JoinLocal(t.Context()))
	row := heldRecreateReceipt(t, hub, run)
	require.NotNil(t, row.SettledAt)
	require.Equal(t, "success", *row.ExecutorState)
	require.NotNil(t, row.Result)
	require.Equal(t, 1, daemon.counts()["create"])
}

// A refused first journal intent ran no effect: the run fails ordinarily and
// releases its exclusion instead of being held without a recovery source.
func TestRecreateFirstJournalIntentFailureIsOrdinary(t *testing.T) {
	for _, operation := range []string{"recreate", "redeploy"} {
		for _, mode := range []string{"local", "direct", "edge"} {
			t.Run(operation+"/"+mode, func(t *testing.T) {
				hub := activationApp(t, false, "")
				executor, environment := hub, "0"
				if mode != "local" {
					executor = activationApp(t, true, uuid.NewString())
					environment = activationRemote(t, hub, executor, mode)
				}
				daemon := newRecreateHTTP(t)
				executor.services.http.onCall = daemon.serve
				require.NoError(t, executor.services.db.Exec("CREATE TRIGGER fixture_journal_refusal BEFORE INSERT ON container_recreate_journal BEGIN SELECT RAISE(ABORT,'synthetic journal refusal'); END").Error)
				path := "/api/environments/" + environment + "/containers/" + runtimeContainerID + "/" + operation
				var request []byte
				if operation == "recreate" {
					request = []byte(`{"edit":{}}`)
				}
				status, body := activationCall(t, hub, path, uuid.NewString(), "respond-async", request)
				require.Equal(t, 202, status, string(body))
				run := activationRunID(t, body)
				ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
				defer cancel()
				if mode != "local" {
					require.NoError(t, hub.remote.WaitDispatch(ctx, run))
				}
				require.NoError(t, executor.services.runtime.JoinLocal(ctx))
				row := heldRecreateReceipt(t, executor, run)
				require.NotNil(t, row.SettledAt, "a run without a journal is never held")
				require.Equal(t, "failed", *row.ExecutorState)
				counts := daemon.counts()
				for _, effect := range []string{"rename", "stop", "create", "start", "remove"} {
					require.Zero(t, counts[effect], effect)
				}
				if mode != "local" {
					ref, found, err := hub.services.runtime.Reference(ctx, environment, run)
					require.NoError(t, err)
					require.True(t, found)
					_, err = hub.remote.Refresh(ctx, ref)
					require.NoError(t, err)
				}
				require.NoError(t, executor.services.db.Exec("DROP TRIGGER fixture_journal_refusal").Error)
				status, body = activationCall(t, hub, path, uuid.NewString(), "respond-async", request)
				require.Equal(t, 202, status, string(body))
				next := activationRunID(t, body)
				require.NotEqual(t, run, next, "the exclusion was released")
				activationJoinRun(t, hub, executor, environment, next)
			})
		}
	}
}

// A receipt held unknown by a live attempt is settled by the next process start
// before readiness, from the journal and without a create replay.
func TestRecreateHeldReceiptRecoveredAtStartup(t *testing.T) {
	state, journal := seedRecreateRecovery(t, filepath.Join(t.TempDir(), "held.db"), "intent-create", true, false, "")
	db, err := database.InitializeHub(t.Context(), sqlitestore.URL(state.Path), database.MigrationOptions{})
	require.NoError(t, err)
	require.NoError(t, db.Model(&activity.RuntimeReceipt{}).Where("run_id=?", journal.RunID).Updates(map[string]any{"executor_state": "unknown", "unknown_reason": "runtime.outcome-unknown"}).Error)
	require.NoError(t, db.Close())
	daemon := newRecreateHTTP(t)
	old := daemon.containers[runtimeContainerID]
	old.Name, old.Running = journal.BackupName, false
	daemon.containers[runtimeContainerID] = old
	state.Endpoint = recreateServer(t, daemon)
	runRecreateRecoveryChild(t, state)
	db, err = database.InitializeHub(t.Context(), sqlitestore.URL(state.Path), database.MigrationOptions{})
	require.NoError(t, err)
	defer db.Close()
	var row activity.RuntimeReceipt
	require.NoError(t, db.First(&row, "run_id=?", journal.RunID).Error)
	require.NotNil(t, row.SettledAt)
	require.Equal(t, "container.recreate-recovered", *row.ExecutorResultCode)
	old, _ = daemon.copy(runtimeContainerID)
	require.Equal(t, "app", old.Name)
	require.True(t, old.Running)
	require.Zero(t, daemon.counts()["create"])
}
func recreateServer(t *testing.T, daemon *recreateHTTP) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !daemon.serve(w, r) {
			w.WriteHeader(500)
		}
	}))
	t.Cleanup(server.Close)
	return server.URL
}
