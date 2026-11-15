// Source: docklet-next/issues/366
// Revision: e83119924556ce53fea232ceac7a270bd595304b
// Retrieved: 2026-11-13
// License: Apache-2.0
// Deviations: Original replacement hold across real SQLite close, reopen and journal recovery.

package activity_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	"github.com/stretchr/testify/require"
)

func TestReplacementHoldSurvivesReopenAndRecovery(t *testing.T) {
	state, journal := seedRecreateRecovery(t, filepath.Join(t.TempDir(), "replacement.db"), "started", true, false, recreateNewID)
	daemon := newRecreateHTTP(t)
	old := daemon.containers[runtimeContainerID]
	old.Name, old.Running = journal.BackupName, false
	daemon.containers[runtimeContainerID] = old
	daemon.containers[recreateNewID] = recreateObserved{ID: recreateNewID, Name: journal.NewName, Labels: map[string]string{"io.docklet.run": journal.RunID}, Running: true}
	f := newRuntimeServicesFixture(t, false)
	f.http.onCall = daemon.serve
	state.Endpoint = f.http.server.URL
	admit := func(expectBusy bool) {
		db, err := database.InitializeHub(t.Context(), sqlitestore.URL(state.Path), database.MigrationOptions{})
		require.NoError(t, err)
		defer db.Close()
		service := activity.NewRuntimeService(activity.NewActivityService(db, nil), f.adapter)
		_, err = service.InitializeGeneration(t.Context())
		require.NoError(t, err)
		command := f.command(activity.RuntimeStopContainer, activity.RuntimeTarget{Kind: "container", ID: recreateNewID})
		command.Store = activity.RuntimeStoreBinding{StoreGeneration: state.Generation, ExecutorGeneration: state.Generation, EnvironmentID: "0", ManagerID: "manager", RegistrationID: "registration"}
		_, err = service.Admit(context.Background(), command)
		if expectBusy {
			var gate *activity.GateConflict
			require.ErrorAs(t, err, &gate, "reopened store must retain replacement hold")
			require.Equal(t, journal.RunID, gate.RunID)
		} else {
			require.NoError(t, err, "recovery settlement must release the replacement")
		}
	}
	admit(true)
	runRecreateRecoveryChild(t, state)
	admit(false)
	require.Zero(t, daemon.counts()["stop"], "admission proof must not issue a competing stop")
}
