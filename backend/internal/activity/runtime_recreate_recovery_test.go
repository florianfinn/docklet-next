// Source: docklet-next/issues/315
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-30
// License: Apache-2.0
// Deviations: Original separate-process recovery decision table and exclusion evidence.

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
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	containers "github.com/florianfinn/docklet-next/backend/internal/container"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	dto "github.com/florianfinn/docklet-next/types/activity"
	"github.com/google/uuid"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/require"
)

type recreateProcessState struct{ Path, Endpoint, Generation string }

func TestRecreateRecoveryProcessChild(t *testing.T) {
	raw := os.Getenv("DOCKLET_RECREATE_PROCESS")
	if raw == "" {
		return
	}
	var state recreateProcessState
	require.NoError(t, json.Unmarshal([]byte(raw), &state))
	db, err := database.InitializeHub(t.Context(), sqlitestore.URL(state.Path), database.MigrationOptions{})
	require.NoError(t, err)
	defer db.Close()
	client, err := client.New(client.WithHost(state.Endpoint), client.WithAPIVersion("1.41"))
	require.NoError(t, err)
	defer client.Close()
	service := containers.NewContainerService(nil, docker.NewDockerClientService(t.Context(), nil, nil, nil).WithClient(client), nil, nil, nil)
	service.ConfigureOperations(db.DB, "registration", true, nil, nil)
	adapter := activity.NewRuntimeServiceAdapter(nil, service)
	runtime := activity.NewRuntimeService(activity.NewActivityService(db, nil), adapter)
	generation, err := runtime.InitializeGeneration(t.Context())
	require.NoError(t, err)
	require.Equal(t, state.Generation, generation)
	store := activity.RuntimeStoreBinding{StoreGeneration: generation, ExecutorGeneration: generation, EnvironmentID: "0", ManagerID: "manager", RegistrationID: "registration"}
	require.NoError(t, runtime.ConfigureLocal(activity.RuntimeLocalConfig{ApplicationContext: t.Context(), Store: store, Executor: adapter}))
	require.NoError(t, runtime.InitializeLocalStartup(t.Context(), store))
	var n int64
	require.NoError(t, db.Table("runtime_receipts").Where("settled_at IS NULL").Count(&n).Error)
	require.Zero(t, n, "recovery settlement releases the receipt exclusion")
}
func seedRecreateRecovery(t *testing.T, path, phase string, wasRunning, autoRemove bool, newID string) (recreateProcessState, containers.RecreateJournal) {
	t.Helper()
	db, err := database.InitializeHub(t.Context(), sqlitestore.URL(path), database.MigrationOptions{})
	require.NoError(t, err)
	generation := uuid.NewString()
	require.NoError(t, db.Create(&activity.RuntimeStoreGeneration{ID: 1, Generation: generation, CreatedAt: time.Now().UTC()}).Error)
	run := uuid.NewString()
	stamp := time.Now().UTC().Add(-time.Minute)
	row := activity.RuntimeReceipt{RunID: run, Kind: "receipt", StoreGeneration: generation, ManagerID: "manager", RegistrationID: "registration", EnvironmentID: "0", ExecutorGeneration: generation, RequestDigest: strings.Repeat("a", 64), CallerID: new("operator"), CallerName: new("Operator"), OperationID: new("recreate-container"), TargetKind: new("container"), TargetID: new(runtimeContainerID), TargetName: new("app"), GateScope: new("container:" + runtimeContainerID), GateKey: new("recreate"), AdmittedAt: &stamp, DispatchState: new("pending"), ExecutorState: new("executing"), ExecutorStartedAt: &stamp, FinalWriteAt: stamp, OutputNextSeq: 1}
	require.NoError(t, db.Create(&row).Error)
	model := activity.Activity{EnvironmentID: "0", Type: dto.TypeContainerEdit, Status: dto.StatusRunning, ResourceType: row.TargetKind, ResourceID: row.TargetID, ResourceName: row.TargetName, StartedByUserID: row.CallerID, StartedByUsername: row.CallerName, StartedAt: stamp, GateScope: *row.GateScope, GateKey: *row.GateKey}
	model.ID = run
	require.NoError(t, db.Create(&model).Error)
	journal := containers.RecreateJournal{RunID: run, OldID: runtimeContainerID, OldName: "app", BackupName: "app-docklet-recreate-" + run, NewID: newID, NewName: "replacement", WasRunning: wasRunning, AutoRemove: autoRemove, Phase: phase, UpdatedAt: stamp}
	require.NoError(t, db.Create(&journal).Error)
	require.NoError(t, db.Close())
	return recreateProcessState{Path: path, Generation: generation}, journal
}
func runRecreateRecoveryChild(t *testing.T, state recreateProcessState) []byte {
	t.Helper()
	body, err := json.Marshal(state)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestRecreateRecoveryProcessChild$", "-test.count=1", "-test.timeout=12s")
	cmd.Env = append(os.Environ(), "DOCKLET_RECREATE_PROCESS="+string(body))
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, string(output))
	return output
}
func TestRecreateRecoveryDecisionTableAcrossProcesses(t *testing.T) {
	for _, tc := range []struct {
		name, phase, oldName                                           string
		old, new, running, wasRunning, autoRemove, find, foreign, fail bool
		code                                                           string
	}{
		{name: "intent-before-rename", phase: "intent-rename", old: true, oldName: "app", wasRunning: true, code: "container.recreate-recovered"},
		{name: "rename-before-observation", phase: "intent-rename", old: true, oldName: "backup", wasRunning: true, code: "container.recreate-recovered"},
		{name: "renamed", phase: "renamed", old: true, oldName: "backup", wasRunning: true, code: "container.recreate-recovered"},
		{name: "create-not-issued", phase: "intent-create", old: true, oldName: "backup", wasRunning: true, code: "container.recreate-recovered"},
		{name: "create-before-observation", phase: "intent-create", old: true, oldName: "backup", new: true, find: true, wasRunning: true, code: "container.recreate-recovered"},
		{name: "created-stopped", phase: "created", old: true, oldName: "backup", new: true, wasRunning: true, code: "container.recreate-recovered"},
		{name: "intent-start-not-running", phase: "intent-start", old: true, oldName: "backup", new: true, wasRunning: true, code: "container.recreate-recovered"},
		{name: "intent-start-running", phase: "intent-start", old: true, oldName: "backup", new: true, running: true, wasRunning: true},
		{name: "started", phase: "started", old: true, oldName: "backup", new: true, running: true, wasRunning: true},
		{name: "intent-remove-old", phase: "intent-remove-old", old: true, oldName: "backup", new: true, running: true, wasRunning: true},
		{name: "old-removed", phase: "intent-remove-old", new: true, running: true, wasRunning: true},
		{name: "deliberately-stopped", phase: "created", old: true, oldName: "backup", new: true, wasRunning: false},
		{name: "auto-remove-missing", phase: "intent-start", new: true, running: true, wasRunning: true, autoRemove: true, code: "container.recreate-ambiguous"},
		{name: "both-missing-precedence", phase: "intent-remove-old", wasRunning: false, code: "container.recreate-ambiguous"},
		{name: "replacement-missing", phase: "started", old: true, oldName: "backup", wasRunning: true, code: "container.recreate-ambiguous"},
		{name: "foreign-name", phase: "started", old: true, oldName: "backup", new: true, running: true, wasRunning: true, foreign: true, code: "container.recreate-ambiguous"},
		{name: "compensation-remove", phase: "comp-remove-new", old: true, oldName: "backup", new: true, wasRunning: true, code: "container.recreate-recovered"},
		{name: "compensation-rename", phase: "comp-rename-old", old: true, oldName: "backup", wasRunning: true, code: "container.recreate-recovered"},
		{name: "compensation-start", phase: "comp-start-old", old: true, oldName: "app", wasRunning: true, code: "container.recreate-recovered"},
		{name: "failing-compensation", phase: "comp-rename-old", old: true, oldName: "backup", wasRunning: true, fail: true, code: "container.recreate-ambiguous"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			newID := ""
			if tc.new && !tc.find || tc.name == "replacement-missing" {
				newID = recreateNewID
			}
			state, journal := seedRecreateRecovery(t, filepath.Join(t.TempDir(), "recovery.db"), tc.phase, tc.wasRunning, tc.autoRemove, newID)
			daemon := newRecreateHTTP(t)
			daemon.containers = map[string]recreateObserved{}
			if tc.old {
				name := tc.oldName
				if name == "backup" {
					name = journal.BackupName
				}
				daemon.containers[runtimeContainerID] = recreateObserved{ID: runtimeContainerID, Name: name, Labels: map[string]string{}, Running: tc.phase == "intent-rename" && name == "app"}
			}
			if tc.new {
				daemon.containers[recreateNewID] = recreateObserved{ID: recreateNewID, Name: journal.NewName, Labels: map[string]string{"io.docklet.run": journal.RunID}, Running: tc.running, Created: time.Now().UTC().Format(time.RFC3339Nano)}
			}
			if tc.foreign {
				daemon.containers[runtimeSecondContainerID] = recreateObserved{ID: runtimeSecondContainerID, Name: journal.OldName, Running: true}
			}
			if tc.fail {
				daemon.fail["rename"] = true
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !daemon.serve(w, r) {
					w.WriteHeader(500)
				}
			}))
			defer server.Close()
			state.Endpoint = server.URL
			runRecreateRecoveryChild(t, state)
			db, err := database.InitializeHub(t.Context(), sqlitestore.URL(state.Path), database.MigrationOptions{})
			require.NoError(t, err)
			defer db.Close()
			var row activity.RuntimeReceipt
			require.NoError(t, db.First(&row, "run_id=?", journal.RunID).Error)
			require.NotNil(t, row.SettledAt)
			var got containers.RecreateJournal
			require.NoError(t, db.First(&got, "run_id=?", journal.RunID).Error)
			counts := daemon.counts()
			require.Zero(t, counts["create"])
			require.Zero(t, counts["pull"])
			old, oldPresent := daemon.copy(runtimeContainerID)
			replacement, replacementPresent := daemon.copy(recreateNewID)
			if tc.code == "" {
				require.Equal(t, "success", *row.ExecutorState)
				require.NotNil(t, row.Result)
				require.Equal(t, "done", got.Phase)
				require.False(t, oldPresent)
				require.True(t, replacementPresent)
				require.Equal(t, tc.wasRunning, replacement.Running)
			} else {
				require.Equal(t, "failed", *row.ExecutorState)
				require.Equal(t, tc.code, *row.ExecutorResultCode)
				if tc.code == "container.recreate-recovered" {
					require.True(t, oldPresent)
					require.Equal(t, journal.OldName, old.Name)
					require.Equal(t, tc.wasRunning, old.Running)
					require.False(t, replacementPresent)
					require.Equal(t, "rolled-back", got.Phase)
				} else {
					require.Equal(t, "ambiguous", got.Phase)
					if !tc.fail {
						require.Empty(t, counts, "ambiguous identities must not be touched")
					}
				}
			}
		})
	}
}
