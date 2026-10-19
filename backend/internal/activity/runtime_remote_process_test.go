// Source: docklet-next/docs/design/runtime-control.md
// Revision: 230a4a3cb92c59cc15e1f6767abd18f017e05ca4b83835524aa8c20e50a89e46
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Dormant bounded remote runtime implementation; production activation is separate.

package activity_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	usertypes "github.com/florianfinn/docklet-next/types/user"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type remoteProcessState struct{ Path, Endpoint, Token, Generation, Phase, RunID string }
type remoteProcessResolver struct{}

func (remoteProcessResolver) ResolveRuntimeTarget(_ context.Context, target activity.RuntimeTarget) (activity.RuntimeResolvedTarget, error) {
	return activity.RuntimeResolvedTarget{Target: target}, nil
}
func remoteProcessExit() { fmt.Println("REMOTE_HUB_CHECKPOINT"); _ = os.Stdout.Sync(); os.Exit(0) }
func TestRuntimeRemoteHubProcessChild(t *testing.T) {
	raw := os.Getenv("RUNTIME_REMOTE_PROCESS_STATE")
	if raw == "" {
		return
	}
	var state remoteProcessState
	require.NoError(t, json.Unmarshal([]byte(raw), &state))
	ctx, cancel := context.WithTimeout(t.Context(), 12*time.Second)
	defer cancel()
	db, err := database.InitializeHub(ctx, sqlitestore.URL(state.Path), database.MigrationOptions{})
	require.NoError(t, err)
	runtime := activity.NewRuntimeService(activity.NewActivityService(db, nil), remoteProcessResolver{})
	generation, err := runtime.InitializeGeneration(ctx)
	require.NoError(t, err)
	store := activity.RuntimeStoreBinding{StoreGeneration: generation, ExecutorGeneration: state.Generation, ManagerID: "example-manager", RegistrationID: "example-registration", EnvironmentID: "example-remote"}
	command := activity.RuntimeCommand{Store: store, Key: uuid.NewString(), ProposedRunID: state.RunID, Actor: usertypes.Actor{ID: "example-owner", Username: "Operator"}, Operation: activity.RuntimeStartContainer, Target: activity.RuntimeTarget{Kind: "container", ID: runtimeContainerID}}
	admitted, err := runtime.Admit(ctx, command)
	require.NoError(t, err)
	if state.Phase == "admitted" {
		remoteProcessExit()
	}
	sender, err := activity.NewRuntimeSender(activity.RuntimeRemoteTarget{EnvironmentID: store.EnvironmentID, Endpoint: state.Endpoint, Token: state.Token, AgentContractVersion: 4}, func(ctx context.Context, _ string, _ *string, _ string, _ string) (context.Context, context.CancelFunc, error) {
		c, cancel := context.WithCancel(ctx)
		return c, cancel, nil
	}, nil, nil)
	require.NoError(t, err)
	control, err := activity.NewRuntimeRemoteCoordinator(runtime, ctx, func(context.Context, activity.RuntimeStoreBinding) (activity.RuntimeRemoteTransport, error) {
		if state.Phase == "dispatching" {
			remoteProcessExit()
		}
		return sender, nil
	}, activity.RuntimeReconcileSchedule{})
	require.NoError(t, err)
	ref := activity.RuntimeRunReference{Store: store, RunID: admitted.ActivityID}
	_, err = control.Start(ctx, ref, command.Options)
	require.NoError(t, err)
	require.NoError(t, control.WaitDispatch(ctx, ref.RunID))
	if state.Phase == "accepted" {
		remoteProcessExit()
	}
	snapshot, err := control.Wait(ctx, ref, nil)
	require.NoError(t, err)
	require.Equal(t, activitytypes.StatusSuccess, snapshot.Status)
	remoteProcessExit()
}
func TestRuntimeRemoteHubCrashReopenNeverResends(t *testing.T) {
	for _, phase := range []string{"admitted", "dispatching", "accepted", "terminal"} {
		t.Run(phase, func(t *testing.T) {
			f := newRemoteIntegration(t, false)
			runID := uuid.NewString()
			path := filepath.Join(t.TempDir(), "process-hub.db")
			entered, release := make(chan struct{}), make(chan struct{})
			var once sync.Once
			if phase == "accepted" {
				f.agent.http.mu.Lock()
				f.agent.http.onCall = func(_ http.ResponseWriter, r *http.Request) bool {
					if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/start") {
						return false
					}
					once.Do(func() { close(entered) })
					<-release
					return false
				}
				f.agent.http.mu.Unlock()
			}
			var releaseOnce sync.Once
			unblock := func() { releaseOnce.Do(func() { close(release) }) }
			defer unblock()
			state := remoteProcessState{Path: path, Endpoint: f.senderEndpoint(), Token: f.token, Generation: f.agent.store.ExecutorGeneration, Phase: phase, RunID: runID}
			raw, err := json.Marshal(state)
			require.NoError(t, err)
			ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestRuntimeRemoteHubProcessChild$", "-test.count=1", "-test.timeout=18s")
			cmd.Env = append(os.Environ(), "RUNTIME_REMOTE_PROCESS_STATE="+string(raw))
			output, err := cmd.CombinedOutput()
			require.NoError(t, err, string(output))
			require.Contains(t, string(output), "REMOTE_HUB_CHECKPOINT")
			if phase == "accepted" {
				select {
				case <-entered:
				case <-ctx.Done():
					t.Fatal("accepted SDK barrier missing")
				}
			}
			reopened, err := database.InitializeHub(t.Context(), sqlitestore.URL(path), database.MigrationOptions{})
			require.NoError(t, err)
			defer reopened.Close()
			var receipt activity.RuntimeReceipt
			require.NoError(t, reopened.First(&receipt, "run_id = ?", runID).Error)
			store := activity.RuntimeStoreBinding{StoreGeneration: receipt.StoreGeneration, ExecutorGeneration: receipt.ExecutorGeneration, ManagerID: receipt.ManagerID, RegistrationID: receipt.RegistrationID, EnvironmentID: receipt.EnvironmentID}
			runtime := activity.NewRuntimeService(activity.NewActivityService(reopened, nil), remoteProcessResolver{})
			var elapsed atomic.Int64
			control, err := activity.NewRuntimeRemoteCoordinator(runtime, t.Context(), func(context.Context, activity.RuntimeStoreBinding) (activity.RuntimeRemoteTransport, error) {
				return f.sender, nil
			}, activity.RuntimeReconcileSchedule{Now: func() time.Time { return time.Now().Add(time.Duration(elapsed.Load())) }, Wait: func(ctx context.Context, d time.Duration) error { elapsed.Add(int64(d)); return ctx.Err() }})
			require.NoError(t, err)
			defer func() {
				join, stop := context.WithTimeout(context.Background(), 5*time.Second)
				defer stop()
				require.NoError(t, control.Close(join))
			}()
			require.NoError(t, control.Startup(ctx))
			ref := activity.RuntimeRunReference{Store: store, RunID: runID}
			snapshot, err := control.Refresh(ctx, ref)
			require.NoError(t, err)
			switch phase {
			case "admitted":
				require.Equal(t, activitytypes.StatusFailed, snapshot.Status)
				require.Equal(t, string(activitytypes.CodeRuntimeInterrupted), snapshot.MessageCode)
				require.Zero(t, f.executeCalls.Load())
			case "dispatching":
				require.Equal(t, activitytypes.RuntimeOutcomeUnknown, snapshot.OutcomeState)
				require.Equal(t, activitytypes.StatusRunning, snapshot.Status)
				require.Zero(t, f.executeCalls.Load())
			case "accepted":
				require.Equal(t, activitytypes.StatusRunning, snapshot.Status)
				unblock()
				_, err = f.agent.waitLocal(runID)
				require.NoError(t, err)
				snapshot, err = control.Wait(ctx, ref, nil)
				require.NoError(t, err)
				require.Equal(t, activitytypes.StatusSuccess, snapshot.Status)
				require.EqualValues(t, 1, f.executeCalls.Load())
			case "terminal":
				require.Equal(t, activitytypes.StatusSuccess, snapshot.Status)
				require.EqualValues(t, 1, f.executeCalls.Load())
			}
			before := f.executeCalls.Load()
			_, err = control.Start(ctx, ref, activity.RuntimeOptions{})
			require.NoError(t, err)
			require.Equal(t, before, f.executeCalls.Load())
			if phase == "accepted" || phase == "terminal" {
				require.Len(t, runtimeEffectEvents(f.agent.http.snapshot()), 1)
				var finals int64
				require.NoError(t, reopened.Table("activity_messages").Where("activity_id = ? AND code='activity.success'", runID).Count(&finals).Error)
				require.EqualValues(t, 1, finals)
			}
		})
	}
}
