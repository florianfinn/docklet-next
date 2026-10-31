// Source: docklet-next/issues/315
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-30
// License: Apache-2.0
// Deviations: Original recreate options, results and receipt retention through real transports.

package activity_test

import (
	"encoding/json"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/container/children/origin"
	dto "github.com/florianfinn/docklet-next/types/activity"
	containerDTO "github.com/florianfinn/docklet-next/types/container"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestRecreateRealTransportsResultAndRemovedIDRetry(t *testing.T) {
	for _, mode := range []string{"local", "direct", "edge"} {
		t.Run(mode, func(t *testing.T) {
			hub := activationApp(t, false, "")
			executor, env := hub, "0"
			if mode != "local" {
				executor = activationApp(t, true, uuid.NewString())
				env = activationRemote(t, hub, executor, mode)
			}
			daemon := newRecreateHTTP(t)
			daemon.containers[runtimeContainerID] = recreateObserved{ID: runtimeContainerID, Name: "app", Labels: map[string]string{"io.kubernetes.pod.name": "app"}, Running: true}
			executor.services.http.onCall = daemon.serve
			registration := env
			if env == "0" {
				registration = hub.services.store.RegistrationID
			}
			require.NoError(t, hub.services.db.Create(&origin.Override{RegistrationID: registration, ContainerName: "app", DockerID: runtimeContainerID, Origin: "standalone", AssignedBy: "operator"}).Error)
			key := uuid.NewString()
			path := "/api/environments/" + env + "/containers/" + runtimeContainerID + "/recreate"
			body := []byte(`{"edit":{"name":"renamed","environment":["EXAMPLE=retained"]}}`)
			status, response := activationCall(t, hub, path, key, "respond-async", body)
			require.Equal(t, 202, status, string(response))
			run := activationRunID(t, response)
			activationJoinRun(t, hub, executor, env, run)
			var receipt activity.RuntimeReceipt
			require.NoError(t, hub.services.db.First(&receipt, "run_id=?", run).Error)
			require.Equal(t, "success", *receipt.ExecutorState)
			require.NotNil(t, receipt.Result)
			var result containerDTO.RecreateResult
			require.NoError(t, json.Unmarshal([]byte(*receipt.Result), &result))
			require.Equal(t, containerDTO.RecreateResult{OldID: runtimeContainerID, NewID: recreateNewID, OldName: "app", NewName: "renamed"}, result)
			var override origin.Override
			require.NoError(t, hub.services.db.First(&override, "registration_id=?", registration).Error)
			require.Equal(t, recreateNewID, override.DockerID)
			require.Equal(t, "renamed", override.ContainerName)
			var history activity.Activity
			require.NoError(t, hub.services.db.First(&history, "id=?", run).Error)
			metadata, err := json.Marshal(history.Metadata)
			require.NoError(t, err)
			require.Contains(t, string(metadata), "environment")
			require.Contains(t, string(metadata), "name")
			require.NotContains(t, string(metadata), "EXAMPLE=retained")
			require.NoError(t, hub.services.db.Exec("DELETE FROM activity_messages").Error)
			require.NoError(t, hub.services.db.Exec("DELETE FROM activities").Error)
			if executor != hub {
				require.NoError(t, executor.services.db.Exec("DELETE FROM activity_messages").Error)
				require.NoError(t, executor.services.db.Exec("DELETE FROM activities").Error)
			}
			before := daemon.counts()
			status, response = activationCall(t, hub, path, key, "respond-async", body)
			require.Equal(t, 200, status, string(response))
			var retained struct{ Data dto.RuntimeAdmission }
			require.NoError(t, json.Unmarshal(response, &retained))
			require.Equal(t, run, retained.Data.ActivityID)
			require.Equal(t, &result, retained.Data.Activity.Result)
			status, response = activationCall(t, hub, path, key, "respond-async", []byte(`{"edit":{"name":"different"}}`))
			require.Equal(t, 409, status, string(response))
			require.Contains(t, string(response), "runtime.idempotency-mismatch")
			require.Equal(t, before, daemon.counts())
			require.Equal(t, 1, before["create"])
			var row activity.RuntimeReceipt
			require.NoError(t, executor.services.db.First(&row, "run_id=?", run).Error)
			require.NotNil(t, row.Result)
			require.NotContains(t, string(response), "EXAMPLE=")
		})
	}
}

// A present-empty section replaces the inspected value on every transport and
// keeps its own digest; absent preserves it.
func TestRecreatePresentEmptySectionCrossesTransports(t *testing.T) {
	for _, mode := range []string{"local", "direct", "edge"} {
		t.Run(mode, func(t *testing.T) {
			hub := activationApp(t, false, "")
			executor, env := hub, "0"
			if mode != "local" {
				executor = activationApp(t, true, uuid.NewString())
				env = activationRemote(t, hub, executor, mode)
			}
			daemon := newRecreateHTTP(t)
			current := daemon.containers[runtimeContainerID]
			current.Environment = []string{"EXAMPLE=old"}
			daemon.containers[runtimeContainerID] = current
			executor.services.http.onCall = daemon.serve
			key := uuid.NewString()
			path := "/api/environments/" + env + "/containers/" + runtimeContainerID + "/recreate"
			status, response := activationCall(t, hub, path, key, "respond-async", []byte(`{"edit":{"environment":[]}}`))
			require.Equal(t, 202, status, string(response))
			run := activationRunID(t, response)
			activationJoinRun(t, hub, executor, env, run)
			replacement, ok := daemon.copy(recreateNewID)
			require.True(t, ok)
			require.Empty(t, replacement.Environment, "a present-empty section must clear the inspected value")
			status, response = activationCall(t, hub, path, key, "respond-async", []byte(`{"edit":{}}`))
			require.Equal(t, 409, status, string(response))
			require.Contains(t, string(response), "runtime.idempotency-mismatch")
			require.Equal(t, 1, daemon.counts()["create"])
		})
	}
}
