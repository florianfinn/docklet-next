// Source: docklet-next/issues/315
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-30
// License: Apache-2.0
// Deviations: Original actual process interruption during compensation and settlement fencing.

package activity_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	containers "github.com/florianfinn/docklet-next/backend/internal/container"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	"github.com/stretchr/testify/require"
)

func TestRecreateCrashDuringCompensationRetainsExclusion(t *testing.T) {
	state, journal := seedRecreateRecovery(t, filepath.Join(t.TempDir(), "crash.db"), "created", true, false, recreateNewID)
	daemon := newRecreateHTTP(t)
	old := daemon.containers[runtimeContainerID]
	old.Name, old.Running = journal.BackupName, false
	daemon.containers[runtimeContainerID] = old
	daemon.containers[recreateNewID] = recreateObserved{ID: recreateNewID, Name: journal.NewName, Labels: map[string]string{"io.docklet.run": journal.RunID}}
	observed, release := make(chan struct{}), make(chan struct{})
	var held atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/rename") && held.CompareAndSwap(false, true) {
			recorder := httptest.NewRecorder()
			require.True(t, daemon.serve(recorder, r))
			close(observed)
			<-release
			for name, values := range recorder.Header() {
				w.Header()[name] = values
			}
			w.WriteHeader(recorder.Code)
			_, _ = w.Write(recorder.Body.Bytes())
			return
		}
		if !daemon.serve(w, r) {
			w.WriteHeader(500)
		}
	}))
	defer server.Close()
	state.Endpoint = server.URL
	body, err := json.Marshal(state)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestRecreateRecoveryProcessChild$", "-test.count=1", "-test.timeout=12s")
	cmd.Env = append(os.Environ(), "DOCKLET_RECREATE_PROCESS="+string(body))
	require.NoError(t, cmd.Start())
	select {
	case <-observed:
	case <-time.After(5 * time.Second):
		close(release)
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		t.Fatal("compensation did not reach observed rename")
	}
	reader, err := sqlitestore.Open(t.Context(), state.Path, false, true)
	require.NoError(t, err)
	var heldCount int
	require.NoError(t, reader.QueryRowContext(t.Context(), "SELECT count(*) FROM runtime_receipts WHERE settled_at IS NULL").Scan(&heldCount))
	require.Equal(t, 1, heldCount)
	var phase string
	require.NoError(t, reader.QueryRowContext(t.Context(), "SELECT phase FROM container_recreate_journal").Scan(&phase))
	require.Equal(t, "comp-rename-old", phase)
	require.NoError(t, reader.Close())
	require.NoError(t, cmd.Process.Kill())
	require.Error(t, cmd.Wait())
	close(release)
	runRecreateRecoveryChild(t, state)
	db, err := database.InitializeHub(t.Context(), sqlitestore.URL(state.Path), database.MigrationOptions{})
	require.NoError(t, err)
	defer db.Close()
	var receipt activity.RuntimeReceipt
	require.NoError(t, db.First(&receipt, "run_id=?", journal.RunID).Error)
	require.Equal(t, "failed", *receipt.ExecutorState)
	require.Equal(t, "container.recreate-recovered", *receipt.ExecutorResultCode)
	require.NotNil(t, receipt.SettledAt)
	counts := daemon.counts()
	require.Equal(t, 1, counts["rename"], "compensation does not rename twice after the crash")
	require.Equal(t, 1, counts["start"])
	require.Zero(t, counts["create"])
	require.Zero(t, counts["pull"])
	restored, ok := daemon.copy(runtimeContainerID)
	require.True(t, ok)
	require.Equal(t, "app", restored.Name)
	require.True(t, restored.Running)
}
func TestRecreateRecoverySettlementFailureKeepsExclusion(t *testing.T) {
	state, journal := seedRecreateRecovery(t, filepath.Join(t.TempDir(), "settlement.db"), "started", true, false, recreateNewID)
	daemon := newRecreateHTTP(t)
	old := daemon.containers[runtimeContainerID]
	old.Name = journal.BackupName
	daemon.containers[runtimeContainerID] = old
	daemon.containers[recreateNewID] = recreateObserved{ID: recreateNewID, Name: journal.NewName, Labels: map[string]string{"io.docklet.run": journal.RunID}, Running: true}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !daemon.serve(w, r) {
			w.WriteHeader(500)
		}
	}))
	defer server.Close()
	state.Endpoint = server.URL
	db, err := database.InitializeHub(t.Context(), sqlitestore.URL(state.Path), database.MigrationOptions{})
	require.NoError(t, err)
	require.NoError(t, db.Exec("CREATE TRIGGER fixture_recreate_settlement BEFORE UPDATE OF executor_settled_at ON runtime_receipts BEGIN SELECT RAISE(ABORT,'synthetic settlement refusal'); END").Error)
	require.NoError(t, db.Close())
	body, err := json.Marshal(state)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestRecreateRecoveryProcessChild$", "-test.count=1", "-test.timeout=12s")
	cmd.Env = append(os.Environ(), "DOCKLET_RECREATE_PROCESS="+string(body))
	output, err := cmd.CombinedOutput()
	require.Error(t, err, string(output))
	require.Contains(t, string(output), "synthetic settlement refusal")
	db, err = database.InitializeHub(t.Context(), sqlitestore.URL(state.Path), database.MigrationOptions{})
	require.NoError(t, err)
	var receipt activity.RuntimeReceipt
	require.NoError(t, db.First(&receipt, "run_id=?", journal.RunID).Error)
	require.Nil(t, receipt.SettledAt)
	var observed containers.RecreateJournal
	require.NoError(t, db.First(&observed, "run_id=?", journal.RunID).Error)
	require.Equal(t, "done", observed.Phase)
	require.NoError(t, db.Exec("DROP TRIGGER fixture_recreate_settlement").Error)
	require.NoError(t, db.Close())
	runRecreateRecoveryChild(t, state)
	require.Equal(t, 1, daemon.counts()["remove"])
	require.Zero(t, daemon.counts()["create"])
	require.Zero(t, daemon.counts()["pull"])
}
