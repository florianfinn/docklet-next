// Source: docklet-next/issues/342
// Revision: 89f69980c01d08714ce96c49365fb19065e3d6ba
// Retrieved: 2026-11-05
// License: Apache-2.0
// Deviations: Original slot/journal/runtime startup crash cuts and truthful image settlement.

package container

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	activityDTO "github.com/florianfinn/docklet-next/types/activity"
	updaterDTO "github.com/florianfinn/docklet-next/types/updater"
	"github.com/google/uuid"
	dc "github.com/moby/moby/api/types/container"
	"github.com/stretchr/testify/require"
)

func TestContainerImageStartupCrashCuts(t *testing.T) {
	for _, tc := range []struct {
		cut, phase                           string
		slot, replacement, removed, mismatch bool
	}{
		{cut: "before-slot"},
		{cut: "after-slot", slot: true},
		{cut: "ambiguous", phase: "ambiguous", slot: true},
		{cut: "after-rename", phase: "renamed", slot: true},
		{cut: "after-start", phase: "started", slot: true, replacement: true},
		{cut: "after-done", phase: "done", slot: true, replacement: true, removed: true},
		{cut: "tag-moved-before-normal-verification", phase: "started", slot: true, replacement: true, mismatch: true},
	} {
		t.Run(tc.cut, func(t *testing.T) {
			s, d, imagePort, db := updateFixture(t)
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
			model := activity.Activity{EnvironmentID: "0", Type: activityDTO.TypeContainerRedeploy, Status: activityDTO.StatusRunning, ResourceType: row.TargetKind, ResourceID: row.TargetID, ResourceName: row.TargetName, StartedByUserID: row.CallerID, StartedByUsername: row.CallerName, StartedAt: stamp, GateScope: *row.GateScope, GateKey: *row.GateKey}
			model.ID = run
			require.NoError(t, db.Create(&model).Error)
			if tc.slot {
				require.NoError(t, s.replacePreviousImage(t.Context(), "app", updateOldImage, updateRef, updateOldID, run))
			}
			if tc.phase != "" {
				journal := RecreateJournal{RunID: run, OldID: updateOldID, OldName: "app", BackupName: "app-docklet-recreate-" + run, NewName: "app", Phase: tc.phase, WasRunning: true, UpdatedAt: stamp, ExpectedImageID: new(updateNewImage)}
				old := d.containers[updateOldID]
				old.Name = "/" + journal.BackupName
				old.State.Running = tc.phase == "renamed"
				d.containers[updateOldID] = old
				if tc.replacement {
					journal.NewID = updateNewID
					id := updateNewImage
					if tc.mismatch {
						id = updateOtherImage
					}
					d.containers[updateNewID] = dc.InspectResponse{ID: updateNewID, Name: "/app", Image: id, Config: &dc.Config{Image: updateRef, Labels: map[string]string{"io.docklet.run": run}}, State: &dc.State{Running: true}}
				}
				if tc.removed {
					delete(d.containers, updateOldID)
				}
				require.NoError(t, db.Create(&journal).Error)
			}
			// Real startup invokes RecoverRecreates before generic interrupted settlement.
			require.NoError(t, rt.InitializeLocalStartup(t.Context(), store))
			var settled activity.RuntimeReceipt
			require.NoError(t, db.First(&settled, "run_id=?", run).Error)
			pins, pinErr := imagePort.g.Reasons(t.Context(), updateOldImage)
			require.NoError(t, pinErr)
			if tc.slot {
				require.Len(t, pins, 1)
			} else {
				require.Empty(t, pins)
			}
			if tc.cut == "ambiguous" {
				require.Nil(t, settled.SettledAt)
				require.Equal(t, "unknown", *settled.ExecutorState)
				require.NoError(t, rt.InitializeLocalStartup(t.Context(), store))
				require.NoError(t, db.First(&settled, "run_id=?", run).Error)
				require.Nil(t, settled.SettledAt)
				require.Empty(t, d.counts)
				return
			}
			require.NotNil(t, settled.SettledAt)
			require.NotNil(t, settled.ExecutorSettledAt)
			var evidence activity.Activity
			require.NoError(t, db.First(&evidence, "id=?", run).Error)
			if tc.replacement && !tc.mismatch {
				require.Equal(t, "success", *settled.ExecutorState)
				require.NotNil(t, evidence.Metadata["imageLifecycle"])
				slot, e := s.previousImage(t.Context(), "app")
				require.NoError(t, e)
				require.Equal(t, updateNewID, slot.OwnerContainerID)
				require.Equal(t, updateNewImage, d.containers[updateNewID].Image)
			} else {
				require.Equal(t, "failed", *settled.ExecutorState)
				if tc.mismatch || tc.phase != "" {
					body, err := json.Marshal(evidence.Metadata["imageLifecycle"])
					require.NoError(t, err)
					var result updaterDTO.LifecycleResult
					require.NoError(t, json.Unmarshal(body, &result))
					code := "update.deploy-failed"
					if tc.mismatch {
						code = "update.target-changed"
					}
					require.Equal(t, code, result.Code)
					require.False(t, result.Verified)
				}
				if tc.mismatch {
					require.Equal(t, updateOtherImage, d.containers[updateNewID].Image)
					require.NotContains(t, d.containers, updateOldID)
				}
				if tc.phase == "renamed" {
					require.Equal(t, "/app", d.containers[updateOldID].Name)
					require.True(t, d.containers[updateOldID].State.Running)
				}
				if tc.slot {
					slot, e := s.previousImage(t.Context(), "app")
					require.NoError(t, e)
					require.Equal(t, updateOldID, slot.OwnerContainerID)
				}
			}
			require.Zero(t, d.counts["pull"])
			require.Zero(t, d.counts["create"])
			first := *settled.SettledAt
			var terminals int64
			require.NoError(t, db.Table("activity_messages").Where("activity_id=?", run).Count(&terminals).Error)
			require.NoError(t, rt.InitializeLocalStartup(t.Context(), store))
			require.NoError(t, db.First(&settled, "run_id=?", run).Error)
			require.True(t, first.Equal(*settled.SettledAt))
			var repeated int64
			require.NoError(t, db.Table("activity_messages").Where("activity_id=?", run).Count(&repeated).Error)
			require.Equal(t, terminals, repeated)
			require.Zero(t, d.counts["pull"])
			require.Zero(t, d.counts["create"])
		})
	}
}
