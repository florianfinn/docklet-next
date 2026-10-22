// Source: docklet-next/docs/design/runtime-control.md
// Revision: 230a4a3cb92c59cc15e1f6767abd18f017e05ca4b83835524aa8c20e50a89e46
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Bounded remote runtime implementation, active in production since #125.

package activity_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/auth"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"github.com/florianfinn/docklet-next/types/contract"
	projecttypes "github.com/florianfinn/docklet-next/types/project"
	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
)

type remoteIntegration struct {
	endpoint      string
	agent         *runtimeServicesFixture
	db            *database.DB
	hub           *activity.RuntimeService
	control       *activity.RuntimeRemoteCoordinator
	sender        *activity.RuntimeSender
	store         activity.RuntimeStoreBinding
	token         string
	executeCalls  atomic.Int32
	accessCalls   atomic.Int32
	loseAck       atomic.Bool
	accessMu      sync.Mutex
	executeCancel context.CancelFunc
	tunnels       *edge.TunnelRegistry
}
type remoteDiscardResponse struct {
	http.ResponseWriter
	header http.Header
}

func (w *remoteDiscardResponse) Header() http.Header         { return w.header }
func (w *remoteDiscardResponse) Write(p []byte) (int, error) { return len(p), nil }
func (w *remoteDiscardResponse) WriteHeader(int)             {}
func (w *remoteDiscardResponse) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func newRemoteIntegration(t *testing.T, isEdge bool) *remoteIntegration {
	t.Helper()
	agent := newRuntimeServicesFixture(t)
	f := &remoteIntegration{agent: agent, token: uuid.NewString()}
	receiver, err := activity.NewRuntimeReceiver(agent.runtime)
	require.NoError(t, err)
	receiverHandler, err := activity.NewRuntimeReceiverHTTP(receiver, &auth.AuthService{}, &config.Config{AgentMode: true, AgentToken: f.token}, time.Minute)
	require.NoError(t, err)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Empty(t, r.Header.Get("Idempotency-Key"))
		require.Empty(t, r.Header.Get("Authorization"))
		require.Empty(t, r.Header.Get("Cookie"))
		require.Equal(t, f.token, r.Header.Get("X-Arcane-Agent-Token"))
		execute := r.Method == http.MethodPost && r.URL.Path == "/api/environments/0/runtime-runs"
		if execute {
			f.executeCalls.Add(1)
		}
		if execute && f.loseAck.CompareAndSwap(true, false) {
			receiverHandler.ServeHTTP(&remoteDiscardResponse{ResponseWriter: w, header: make(http.Header)}, r)
			f.accessMu.Lock()
			cancel := f.executeCancel
			f.accessMu.Unlock()
			if cancel != nil {
				cancel()
			}
			return
		}
		receiverHandler.ServeHTTP(w, r)
	})
	direct := httptest.NewServer(handler)
	f.endpoint = direct.URL
	t.Cleanup(direct.Close)
	db, err := database.InitializeHub(t.Context(), sqlitestore.URL(filepath.Join(t.TempDir(), "hub.db")), database.MigrationOptions{})
	require.NoError(t, err)
	f.db = db
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	f.hub = activity.NewRuntimeService(activity.NewActivityService(db, nil), agent.adapter)
	generation, err := f.hub.InitializeGeneration(t.Context())
	require.NoError(t, err)
	f.store = activity.RuntimeStoreBinding{StoreGeneration: generation, ExecutorGeneration: agent.store.ExecutorGeneration, ManagerID: agent.store.ManagerID, RegistrationID: agent.store.RegistrationID, EnvironmentID: "example-remote"}
	registry := edge.NewTunnelRegistry()
	f.tunnels = registry
	if isEdge {
		connected := make(chan struct{})
		var once sync.Once
		server := edge.NewTunnelServerWithRegistry(registry, func(_ context.Context, token string) (string, error) {
			if token != f.token {
				return "", errors.New("invalid synthetic credential")
			}
			return f.store.EnvironmentID, nil
		}, func(_ context.Context, _ string, online bool) {
			if online {
				once.Do(func() { close(connected) })
			}
		})
		server.SetSessionKeyResolver(func(context.Context, string, string) (string, error) { return "example-key", nil })
		router := echo.New()
		router.GET("/api/tunnel/connect", server.HandleConnect)
		manager := httptest.NewServer(router)
		t.Cleanup(manager.Close)
		life, cancel := context.WithCancel(t.Context())
		done := make(chan struct{})
		client := edge.NewRuntimeTunnelClient(&edge.Config{EdgeTransport: edge.EdgeTransportWebSocket, ManagerApiUrl: manager.URL, AgentToken: f.token, EdgeReconnectInterval: 1}, handler)
		go func() { defer close(done); client.StartWithErrorChan(life, nil) }()
		t.Cleanup(func() {
			cancel()
			registry.Unregister(f.store.EnvironmentID)
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Error("Edge client did not join")
			}
		})
		select {
		case <-connected:
		case <-time.After(5 * time.Second):
			t.Fatal("Edge registration barrier not reached")
		}
	}
	// This trusted access port is a test compatibility/session boundary. It does
	// not mock HTTP, Edge, receiver, executor, database or Docker SDK transport.
	access := func(ctx context.Context, id string, token *string, method, path string) (context.Context, context.CancelFunc, error) {
		f.accessCalls.Add(1)
		if id != f.store.EnvironmentID || token == nil || *token != f.token {
			return nil, nil, errors.New("registration mismatch")
		}
		bound := edge.WithSessionCredential(ctx, "example-key", f.token, nil)
		if isEdge {
			tunnel, ok := registry.Get(id).Get()
			if !ok {
				return nil, nil, errors.New("tunnel unavailable")
			}
			bound = edge.WithSessionTunnel(bound, tunnel)
		}
		result, cancel := context.WithCancel(bound)
		if method == http.MethodPost && path == "/api/environments/0/runtime-runs" {
			f.accessMu.Lock()
			f.executeCancel = cancel
			f.accessMu.Unlock()
		}
		return result, cancel, nil
	}
	f.sender, err = activity.NewRuntimeSender(activity.RuntimeRemoteTarget{EnvironmentID: f.store.EnvironmentID, Endpoint: direct.URL, Token: f.token, Edge: isEdge, AgentContractVersion: contract.AgentHubVersion}, access, direct.Client(), registry)
	require.NoError(t, err)
	var elapsed atomic.Int64
	f.control, err = activity.NewRuntimeRemoteCoordinator(f.hub, t.Context(), func(context.Context, activity.RuntimeStoreBinding) (activity.RuntimeRemoteTransport, error) {
		return f.sender, nil
	}, activity.RuntimeReconcileSchedule{Now: func() time.Time { return time.Now().Add(time.Duration(elapsed.Load())) }, Wait: func(ctx context.Context, d time.Duration) error { elapsed.Add(int64(d)); return ctx.Err() }})
	require.NoError(t, err)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		require.NoError(t, f.control.Close(ctx))
	})
	return f
}
func (f *remoteIntegration) command(op activity.RuntimeOperation, target activity.RuntimeTarget) activity.RuntimeCommand {
	c := f.agent.command(op, target)
	c.Store = f.store
	return c
}
func TestRuntimeRemoteActualDirectAndEdgeSevenOperations(t *testing.T) {
	for _, isEdge := range []bool{false, true} {
		for _, op := range []activity.RuntimeOperation{activity.RuntimeDeployProject, activity.RuntimeDownProject, activity.RuntimeRestartProject, activity.RuntimeRedeployProject, activity.RuntimeStartContainer, activity.RuntimeStopContainer, activity.RuntimeRestartContainer} {
			name := string(op)
			if isEdge {
				name = "edge/" + name
			} else {
				name = "direct/" + name
			}
			t.Run(name, func(t *testing.T) {
				f := newRemoteIntegration(t, isEdge)
				target := activity.RuntimeTarget{Kind: "container", ID: runtimeContainerID}
				if strings.Contains(string(op), "project") {
					target = activity.RuntimeTarget{Kind: "stack", ID: "example-project"}
					f.agent.project(target.ID, target.ID, "name: example_stack\nservices:\n  app:\n    image: registry.example.com/app:1\n")
					f.agent.http.setLabels(map[string]string{"com.docker.compose.project": "example_stack", "com.docker.compose.service": "app", "com.docker.compose.container-number": "1", "com.docker.compose.config-hash": "example-hash"})
					if op == activity.RuntimeDownProject || op == activity.RuntimeRestartProject {
						f.agent.http.setComposeContainer(true)
					}
				}
				command := f.command(op, target)
				if op == activity.RuntimeDeployProject || op == activity.RuntimeRedeployProject {
					command.Options.Deploy = &projecttypes.DeployOptions{PullPolicy: "never"}
				}
				admitted, err := f.hub.Admit(t.Context(), command)
				require.NoError(t, err)
				ref := activity.RuntimeRunReference{Store: f.store, RunID: admitted.ActivityID}
				_, err = f.control.Start(t.Context(), ref, command.Options)
				require.NoError(t, err)
				wait, cancel := context.WithTimeout(t.Context(), 10*time.Second)
				defer cancel()
				require.NoError(t, f.control.WaitDispatch(wait, ref.RunID))
				agentResult, err := f.agent.waitLocal(ref.RunID)
				require.NoError(t, err)
				require.Equal(t, activitytypes.StatusSuccess, agentResult.Status, f.agent.debugEvents())
				snapshot, err := f.control.Wait(wait, ref, nil)
				require.NoError(t, err)
				require.Equal(t, activitytypes.StatusSuccess, snapshot.Status)
				require.Equal(t, ref.RunID, snapshot.ID)
				require.Equal(t, f.store.EnvironmentID, snapshot.EnvironmentID)
				require.Equal(t, activitytypes.RuntimeOutcomeKnown, snapshot.OutcomeState)
				effects := runtimeEffectEvents(f.agent.http.snapshot())
				require.NotEmpty(t, effects, f.agent.debugEvents())
				_, err = f.control.Start(t.Context(), ref, command.Options)
				require.NoError(t, err)
				require.NoError(t, f.control.WaitDispatch(wait, ref.RunID))
				require.EqualValues(t, 1, f.executeCalls.Load())
				require.Equal(t, effects, runtimeEffectEvents(f.agent.http.snapshot()))
				var receipt activity.RuntimeReceipt
				require.NoError(t, f.db.First(&receipt, "run_id = ?", ref.RunID).Error)
				id := activitytypes.RuntimeRunIdentity{RuntimeExecutorIdentity: activitytypes.RuntimeExecutorIdentity{ManagerID: f.store.ManagerID, RegistrationID: f.store.RegistrationID, ExecutorGeneration: f.store.ExecutorGeneration}, RunID: ref.RunID, RequestDigest: receipt.RequestDigest}
				outcome, err := f.sender.Query(wait, id)
				require.NoError(t, err)
				require.NoError(t, activitytypes.ValidateRuntimeOutcome(outcome, id))
				var bindings, agentBindings, finals int64
				require.NoError(t, f.db.Table("runtime_key_bindings").Count(&bindings).Error)
				require.EqualValues(t, 1, bindings)
				require.NoError(t, f.agent.db.Table("runtime_key_bindings").Count(&agentBindings).Error)
				require.Zero(t, agentBindings)
				require.NoError(t, f.db.Table("activity_messages").Where("activity_id = ? AND code = 'activity.success'", ref.RunID).Count(&finals).Error)
				require.EqualValues(t, 1, finals)
				require.GreaterOrEqual(t, f.accessCalls.Load(), int32(3))
			})
		}
	}
}
func TestRuntimeRemoteActualLostAcknowledgmentNeverReplays(t *testing.T) {
	for _, isEdge := range []bool{false, true} {
		t.Run(map[bool]string{false: "direct", true: "edge"}[isEdge], func(t *testing.T) {
			f := newRemoteIntegration(t, isEdge)
			f.loseAck.Store(true)
			command := f.command(activity.RuntimeStartContainer, activity.RuntimeTarget{Kind: "container", ID: runtimeContainerID})
			admitted, err := f.hub.Admit(t.Context(), command)
			require.NoError(t, err)
			ref := activity.RuntimeRunReference{Store: f.store, RunID: admitted.ActivityID}
			_, err = f.control.Start(t.Context(), ref, command.Options)
			require.NoError(t, err)
			wait, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			require.NoError(t, f.control.WaitDispatch(wait, ref.RunID))
			_, err = f.agent.waitLocal(ref.RunID)
			require.NoError(t, err)
			snapshot, err := f.control.Wait(wait, ref, nil)
			require.NoError(t, err)
			require.Equal(t, activitytypes.StatusSuccess, snapshot.Status)
			require.EqualValues(t, 1, f.executeCalls.Load())
			require.Len(t, runtimeEffectEvents(f.agent.http.snapshot()), 1)
			_, err = f.control.Start(wait, ref, command.Options)
			require.NoError(t, err)
			require.NoError(t, f.control.WaitDispatch(wait, ref.RunID))
			require.EqualValues(t, 1, f.executeCalls.Load())
		})
	}
}

func TestRuntimeRemoteActualPrecommitCancellationFencesWithoutEffects(t *testing.T) {
	for _, isEdge := range []bool{false, true} {
		t.Run(map[bool]string{false: "direct", true: "edge"}[isEdge], func(t *testing.T) {
			f := newRemoteIntegration(t, isEdge)
			command := f.command(activity.RuntimeStartContainer, activity.RuntimeTarget{Kind: "container", ID: runtimeContainerID})
			admitted, err := f.hub.Admit(t.Context(), command)
			require.NoError(t, err)
			ref := activity.RuntimeRunReference{Store: f.store, RunID: admitted.ActivityID}
			entered, joined := make(chan struct{}), make(chan struct{})
			var once sync.Once
			f.agent.http.mu.Lock()
			f.agent.http.onCall = func(w http.ResponseWriter, r *http.Request) bool {
				if r.Method != http.MethodGet || !strings.HasSuffix(r.URL.Path, "/containers/"+runtimeContainerID+"/json") {
					return false
				}
				once.Do(func() { close(entered) })
				<-r.Context().Done()
				close(joined)
				return true
			}
			f.agent.http.mu.Unlock()
			_, err = f.control.Start(t.Context(), ref, command.Options)
			require.NoError(t, err)
			select {
			case <-entered:
			case <-time.After(5 * time.Second):
				t.Fatal("actual receiver SDK resolver barrier not reached")
			}
			f.accessMu.Lock()
			cancel := f.executeCancel
			f.accessMu.Unlock()
			require.NotNil(t, cancel)
			cancel()
			select {
			case <-joined:
			case <-time.After(5 * time.Second):
				t.Fatal("cancel did not join actual receiver SDK request")
			}
			wait, stop := context.WithTimeout(t.Context(), 10*time.Second)
			defer stop()
			require.NoError(t, f.control.WaitDispatch(wait, ref.RunID))
			require.Empty(t, runtimeEffectEvents(f.agent.http.snapshot()))
			require.EqualValues(t, 1, f.executeCalls.Load())
			var accepted int64
			require.NoError(t, f.agent.db.Table("runtime_receipts").Where("kind='receipt'").Count(&accepted).Error)
			require.Zero(t, accepted)
			var tombstones int64
			require.NoError(t, f.agent.db.Table("runtime_receipts").Where("kind='not_received'").Count(&tombstones).Error)
			require.EqualValues(t, 1, tombstones)
		})
	}
}

func (f *remoteIntegration) senderEndpoint() string { return f.endpoint }
