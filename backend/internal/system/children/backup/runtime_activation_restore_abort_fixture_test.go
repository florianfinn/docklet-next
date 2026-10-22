// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Synthetic authenticated HTTP and Edge transport for actual restore fault paths.

package backup

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/auth"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"github.com/florianfinn/docklet-next/types/contract"
	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
)

type restoreExecutorFunc func(context.Context, activity.RuntimeCommand, io.Writer) error

func (f restoreExecutorFunc) ExecuteRuntime(ctx context.Context, c activity.RuntimeCommand, w io.Writer) error {
	return f(ctx, c, w)
}

type restoreAbortResponse struct {
	*httptest.ResponseRecorder
	actual http.ResponseWriter
}

func (w *restoreAbortResponse) Unwrap() http.ResponseWriter { return w.actual }

type restoreAbortRemote struct {
	agent    *restoreArchiveFixture
	sender   *activity.RuntimeSender
	store    activity.RuntimeStoreBinding
	accepted chan int
	release  func()
	executes atomic.Int32
	effects  atomic.Int32
	queries  atomic.Int32
}

func restoreAbortTransport(t *testing.T, ctx context.Context, hub *restoreArchiveFixture, transport string) *restoreAbortRemote {
	t.Helper()
	f := &restoreAbortRemote{agent: restoreArchiveNew(t), accepted: make(chan int, 1)}
	release := make(chan struct{})
	f.release = sync.OnceFunc(func() { close(release) })
	t.Cleanup(f.release)
	f.agent.binding.ManagerID = hub.binding.ManagerID
	f.agent.binding.RegistrationID = "example-remote"
	require.NoError(t, f.agent.runtime.ConfigureLocal(activity.RuntimeLocalConfig{ApplicationContext: ctx, Store: f.agent.binding, Executor: restoreExecutorFunc(func(ctx context.Context, _ activity.RuntimeCommand, _ io.Writer) error {
		if f.effects.Add(1) == 1 {
			select {
			case <-release:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		return nil
	})}))
	receiver, err := activity.NewRuntimeReceiver(f.agent.runtime)
	require.NoError(t, err)
	token := uuid.NewString()
	handler, err := activity.NewRuntimeReceiverHTTP(receiver, &auth.AuthService{}, &config.Config{AgentMode: true, AgentToken: token}, time.Minute)
	require.NoError(t, err)
	wrapped := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/environments/0/runtime-runs" {
			if f.executes.Add(1) == 1 {
				captured := httptest.NewRecorder()
				handler.ServeHTTP(&restoreAbortResponse{ResponseRecorder: captured, actual: w}, r)
				f.accepted <- captured.Code
				if captured.Code != http.StatusOK {
					w.WriteHeader(captured.Code)
					return
				}
				// Lose the acknowledgment only after durable receiver acceptance. Close
				// must cancel this actual request; recovery may query but never resend it.
				<-r.Context().Done()
				return
			}
		}
		if r.Method == http.MethodGet {
			f.queries.Add(1)
		}
		handler.ServeHTTP(w, r)
	})
	direct := httptest.NewServer(wrapped)
	t.Cleanup(direct.Close)
	f.store = activity.RuntimeStoreBinding{EnvironmentID: "example-remote", ManagerID: hub.binding.ManagerID, RegistrationID: f.agent.binding.RegistrationID, StoreGeneration: hub.binding.StoreGeneration, ExecutorGeneration: f.agent.binding.ExecutorGeneration}
	registry := edge.NewTunnelRegistry()
	if transport == "edge" {
		connected := make(chan struct{})
		var once sync.Once
		server := edge.NewTunnelServerWithRegistry(registry, func(_ context.Context, credential string) (string, error) {
			if credential != token {
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
		lifetime, stop := context.WithCancel(ctx)
		done := make(chan struct{})
		client := edge.NewRuntimeTunnelClient(&edge.Config{EdgeTransport: edge.EdgeTransportWebSocket, ManagerApiUrl: manager.URL, AgentToken: token}, wrapped)
		go func() { defer close(done); client.StartWithErrorChan(lifetime, nil) }()
		t.Cleanup(func() {
			stop()
			registry.Unregister(f.store.EnvironmentID)
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Error("Edge client did not join")
			}
		})
		select {
		case <-connected:
		case <-ctx.Done():
			t.Fatal("Edge transport did not connect")
		}
	}
	access := func(ctx context.Context, id string, credential *string, _, _ string) (context.Context, context.CancelFunc, error) {
		if id != f.store.EnvironmentID || credential == nil || *credential != token {
			return nil, nil, errors.New("synthetic registration mismatch")
		}
		bound := edge.WithSessionCredential(ctx, "example-key", token, nil)
		if transport == "edge" {
			tunnel, ok := registry.Get(id).Get()
			if !ok {
				return nil, nil, errors.New("tunnel unavailable")
			}
			bound = edge.WithSessionTunnel(bound, tunnel)
		}
		bound, cancel := context.WithCancel(bound)
		return bound, cancel, nil
	}
	f.sender, err = activity.NewRuntimeSender(activity.RuntimeRemoteTarget{EnvironmentID: f.store.EnvironmentID, Endpoint: direct.URL, Token: token, Edge: transport == "edge", AgentContractVersion: contract.AgentHubVersion}, access, direct.Client(), registry)
	require.NoError(t, err)
	return f
}

func restoreAbortWait(t *testing.T, ctx context.Context, c <-chan struct{}, description string) {
	t.Helper()
	select {
	case <-c:
	case <-ctx.Done():
		t.Fatal(description)
	}
}

func restoreAbortSnapshot(t *testing.T, ctx context.Context, hub *restoreArchiveFixture, remote *activity.RuntimeRemoteCoordinator, c activity.RuntimeCommand, id string) activitytypes.RuntimeSnapshot {
	t.Helper()
	ref := activity.RuntimeRunReference{Store: c.Store, RunID: id}
	require.NoError(t, remote.WaitDispatch(ctx, id))
	snapshot, err := remote.Refresh(ctx, ref)
	require.NoError(t, err)
	return snapshot
}
