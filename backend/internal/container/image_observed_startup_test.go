// Source: docklet-next/issues/440
// Revision: 45ee2635d68e71671b571cbd8970425ddd194b59
// Retrieved: 2026-11-12
// License: Apache-2.0
// Deviations: Original startup coverage of observed container journals through the real startup writer.

package container

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	activityDTO "github.com/florianfinn/docklet-next/types/activity"
	dto "github.com/florianfinn/docklet-next/types/container"
	updaterDTO "github.com/florianfinn/docklet-next/types/updater"
	"github.com/google/uuid"
	dc "github.com/moby/moby/api/types/container"
	"github.com/stretchr/testify/require"
)

// K-1 / K-3 / M-5: process loss during an observed update is never certified
// from journal identity. Roll-forward keeps the live replacement as R; the run
// ends interrupted with Verified=false for either observed mode.
func TestObservedContainerStartupIsInterrupted(t *testing.T) {
	for _, tc := range []struct {
		name, mode, phase string
		replacement       bool
	}{
		{"scheduled started", "scheduled", "started", true},
		{"manual started", "manual", "started", true},
		{"scheduled done", "scheduled", "done", true},
		{"scheduled compensation", "scheduled", "comp-rename-old", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, d, _, db := updateFixture(t)
			adapter := activity.NewRuntimeServiceAdapter(nil, s)
			rt := activity.NewRuntimeService(activity.NewActivityService(db, nil), adapter)
			generation, err := rt.InitializeGeneration(t.Context())
			require.NoError(t, err)
			store := activity.RuntimeStoreBinding{StoreGeneration: generation, ExecutorGeneration: generation, EnvironmentID: "0", RegistrationID: "registration", ManagerID: "manager"}
			require.NoError(t, rt.ConfigureLocal(activity.RuntimeLocalConfig{ApplicationContext: t.Context(), Store: store, Executor: adapter}))
			run := uuid.NewString()
			stamp := time.Now().UTC().Add(-time.Minute)
			row := activity.RuntimeReceipt{RunID: run, Kind: "receipt", StoreGeneration: generation, ExecutorGeneration: generation, EnvironmentID: "0", RegistrationID: "registration", ManagerID: "manager", RequestDigest: strings.Repeat("a", 64), CallerID: new("operator"), CallerName: new("Operator"), OperationID: new("update-container"), TargetKind: new("container"), TargetID: new(updateOldID), TargetName: new("app"), GateScope: new("container:" + updateOldID), GateKey: new("update-container"), AdmittedAt: &stamp, DispatchState: new("pending"), ExecutorState: new("executing"), ExecutorStartedAt: &stamp, FinalWriteAt: stamp, OutputNextSeq: 1}
			require.NoError(t, db.Create(&row).Error)
			require.NoError(t, db.Create(&activity.RuntimeObservedRun{RunID: run, Kind: "container", Mode: tc.mode, CreatedAt: stamp}).Error)
			model := activity.Activity{EnvironmentID: "0", Type: activityDTO.TypeContainerRedeploy, Status: activityDTO.StatusRunning, ResourceType: row.TargetKind, ResourceID: row.TargetID, ResourceName: row.TargetName, StartedByUserID: row.CallerID, StartedByUsername: row.CallerName, StartedAt: stamp, GateScope: *row.GateScope, GateKey: *row.GateKey}
			model.ID = run
			require.NoError(t, db.Create(&model).Error)
			require.NoError(t, s.replacePreviousImage(t.Context(), "app", updateOldImage, updateRef, updateOldID, run))
			journal := RecreateJournal{RunID: run, OldID: updateOldID, OldName: "app", BackupName: "app-docklet-recreate-" + run, NewName: "app", Phase: tc.phase, WasRunning: true, UpdatedAt: stamp, ExpectedImageID: new(updateNewImage)}
			old := d.containers[updateOldID]
			old.Name = "/" + journal.BackupName
			old.State.Running = false
			d.containers[updateOldID] = old
			if tc.replacement {
				journal.NewID = updateNewID
				d.containers[updateNewID] = dc.InspectResponse{ID: updateNewID, Name: "/app", Image: updateNewImage, Config: &dc.Config{Image: updateRef, Labels: map[string]string{"io.docklet.run": run}}, State: &dc.State{Running: true}}
			}
			if tc.phase == "done" {
				delete(d.containers, updateOldID)
			}
			require.NoError(t, db.Create(&journal).Error)

			require.NoError(t, rt.InitializeLocalStartup(t.Context(), store))
			var settled activity.RuntimeReceipt
			require.NoError(t, db.First(&settled, "run_id=?", run).Error)
			require.NotNil(t, settled.SettledAt)
			require.Equal(t, "failed", *settled.ExecutorState)
			var evidence activity.Activity
			require.NoError(t, db.First(&evidence, "id=?", run).Error)
			body, err := json.Marshal(evidence.Metadata["imageLifecycle"])
			require.NoError(t, err)
			var result updaterDTO.LifecycleResult
			require.NoError(t, json.Unmarshal(body, &result))
			require.Equal(t, "runtime.interrupted", result.Code)
			require.False(t, result.Verified)
			require.NotNil(t, result.Recovery)
			require.Equal(t, "interrupted", result.Recovery.Outcome)
			var observed activity.RuntimeObservedRun
			require.NoError(t, db.First(&observed, "run_id=?", run).Error)
			require.Equal(t, "interrupted", observed.Outcome)
			if tc.replacement {
				require.NotNil(t, settled.Result)
				var replacement dto.RecreateResult
				require.NoError(t, json.Unmarshal([]byte(*settled.Result), &replacement))
				require.Equal(t, updateNewID, replacement.NewID)
				require.NotContains(t, d.containers, updateOldID, "roll-forward keeps the live replacement")
			} else {
				require.Nil(t, settled.Result)
				require.Equal(t, "/app", d.containers[updateOldID].Name)
			}
		})
	}
}
