// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Actual compatibility aliases, offline held scopes and request lifetimes.

package activity_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func activationRunID(t *testing.T, body []byte) string {
	t.Helper()
	var out struct {
		Data activitytypes.RuntimeAdmission `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &out))
	require.NotEmpty(t, out.Data.ActivityID)
	require.Equal(t, out.Data.ActivityID, out.Data.Activity.ID)
	return out.Data.ActivityID
}

func activationJoinRun(t *testing.T, hub, executor *activationFixture, env, run string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	if env != "0" {
		require.NoError(t, hub.remote.WaitDispatch(ctx, run))
	}
	ref, found, err := executor.services.runtime.Reference(ctx, "0", run)
	require.NoError(t, err)
	require.True(t, found)
	snapshot, err := executor.services.runtime.WaitLocal(ctx, ref)
	require.NoError(t, err)
	require.Equal(t, activitytypes.StatusSuccess, snapshot.Status)
	if env != "0" {
		ref, found, err = hub.services.runtime.Reference(ctx, env, run)
		require.NoError(t, err)
		require.True(t, found)
		_, err = hub.remote.Refresh(ctx, ref)
		require.NoError(t, err)
	}
}

func TestRuntimeActivationLegacyAliasesRetainOfflineBinding(t *testing.T) {
	for _, transport := range []string{"local", "direct", "edge"} {
		for _, alias := range []string{runtimeContainerID[:12], "example_stack-app-1"} {
			t.Run(transport+"/"+alias, func(t *testing.T) {
				hub := activationApp(t, false, "")
				executor := hub
				env := "0"
				if transport != "local" {
					executor = activationApp(t, true, uuid.NewString())
					env = activationRemote(t, hub, executor, transport)
				}
				executor.services.http.onCall = func(_ http.ResponseWriter, r *http.Request) bool {
					if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/containers/"+alias+"/json") {
						r.URL.Path = strings.TrimSuffix(r.URL.Path, "/containers/"+alias+"/json") + "/containers/" + runtimeContainerID + "/json"
					}
					return false
				}
				path := "/api/environments/" + env + "/containers/" + alias + "/start"
				key := uuid.NewString()
				status, body := activationCall(t, hub, path, key, "respond-async", nil)
				require.Equal(t, 202, status, string(body))
				run := activationRunID(t, body)
				activationJoinRun(t, hub, executor, env, run)
				require.NoError(t, hub.services.db.Model(&activity.RuntimeKeyBinding{}).Where("environment_id = ? AND idempotency_key = ?", env, key).UpdateColumn("first_seen_at", time.Now().Add(-11*time.Minute)).Error)
				before := executor.services.http.snapshot()
				executor.services.http.server.Close()
				status, body = activationCall(t, hub, path, key, "respond-async", nil)
				require.Equal(t, 200, status, string(body))
				require.Equal(t, run, activationRunID(t, body))
				require.Equal(t, before, executor.services.http.snapshot(), "retained canonical key must not inspect or execute")
			})
		}
	}
}

func TestRuntimeActivationHeldJoinAndConflictBeforeOfflineInspect(t *testing.T) {
	for _, transport := range []string{"local", "direct", "edge"} {
		t.Run(transport, func(t *testing.T) {
			hub := activationApp(t, false, "")
			executor := hub
			env := "0"
			if transport != "local" {
				executor = activationApp(t, true, uuid.NewString())
				env = activationRemote(t, hub, executor, transport)
			}
			entered, release := make(chan struct{}), make(chan struct{})
			var once sync.Once
			unblock := sync.OnceFunc(func() { close(release) })
			defer unblock()
			var offline atomic.Bool
			executor.services.http.onCall = func(w http.ResponseWriter, r *http.Request) bool {
				if offline.Load() && r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/containers/") {
					http.Error(w, "synthetic inspect unavailable", 503)
					return true
				}
				if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/start") {
					once.Do(func() { close(entered) })
					<-release
					w.WriteHeader(204)
					return true
				}
				return false
			}
			path := "/api/environments/" + env + "/containers/" + runtimeContainerID + "/start"
			status, body := activationCall(t, hub, path, uuid.NewString(), "respond-async", nil)
			require.Equal(t, 202, status, string(body))
			run := activationRunID(t, body)
			select {
			case <-entered:
			case <-time.After(5 * time.Second):
				t.Fatal("real SDK executor did not enter")
			}
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			require.NoError(t, hub.remote.WaitDispatch(ctx, run))
			offline.Store(true)
			before := executor.services.http.snapshot()
			status, body = activationCall(t, hub, path, uuid.NewString(), "respond-async", nil)
			require.Equal(t, 200, status, string(body))
			require.Equal(t, run, activationRunID(t, body))
			status, body = activationCall(t, hub, strings.TrimSuffix(path, "start")+"stop", uuid.NewString(), "respond-async", nil)
			require.Equal(t, 409, status, string(body))
			require.Contains(t, string(body), `"activeRunId":"`+run+`"`)
			require.Equal(t, before, executor.services.http.snapshot(), "held-scope answers must precede fresh inspection")
			unblock()
			activationJoinRun(t, hub, executor, env, run)
			require.Len(t, runtimeEffectEvents(executor.services.http.snapshot()), 1)
		})
	}
}

func TestRuntimeActivationWritableWaitCancellationDetaches(t *testing.T) {
	activationWritableWait(t, false)
}
func TestRuntimeActivationWritableWaitDeadlineDetaches(t *testing.T) { activationWritableWait(t, true) }
func activationWritableWait(t *testing.T, deadline bool) {
	hub := activationApp(t, false, "")
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := sync.OnceFunc(func() { close(release) })
	defer unblock()
	hub.services.http.onCall = func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/start") {
			once.Do(func() { close(entered) })
			<-release
			w.WriteHeader(204)
			return true
		}
		return false
	}
	var ctx context.Context
	var cancel context.CancelFunc
	if deadline {
		ctx, cancel = context.WithTimeout(t.Context(), 5*time.Second)
	} else {
		ctx, cancel = context.WithCancel(t.Context())
	}
	defer cancel()
	path := "/api/environments/0/containers/" + runtimeContainerID + "/start"
	key := uuid.NewString()
	request := httptest.NewRequest(http.MethodPost, path, nil).WithContext(ctx)
	request.Header.Set("Idempotency-Key", key)
	response := httptest.NewRecorder()
	done := make(chan struct{})
	go func() { defer close(done); hub.router.ServeHTTP(response, request) }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("executor did not enter")
	}
	if !deadline {
		cancel()
	} else {
		<-ctx.Done()
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("cancelled waiter did not detach")
	}
	if deadline {
		require.Equal(t, 504, response.Code, response.Body.String())
		require.Contains(t, response.Body.String(), string(activitytypes.CodeRuntimeWaitTimeout))
	} else {
		require.Equal(t, 408, response.Code, response.Body.String())
		require.Contains(t, response.Body.String(), string(activitytypes.CodeRuntimeWaiterDisconnected))
	}
	status, body := activationCall(t, hub, path, key, "respond-async", nil)
	require.Equal(t, 200, status, string(body))
	run := activationRunID(t, body)
	var held activity.RuntimeReceipt
	require.NoError(t, hub.services.db.First(&held, "run_id = ?", run).Error)
	require.Nil(t, held.SettledAt)
	require.False(t, held.CancelRequested, "viewer disconnect must not become cancellation intent")
	status, body = activationCall(t, hub, strings.TrimSuffix(path, "start")+"stop", uuid.NewString(), "respond-async", nil)
	require.Equal(t, 409, status, string(body))
	require.Contains(t, string(body), run)
	unblock()
	activationJoinRun(t, hub, hub, "0", run)
	require.Len(t, runtimeEffectEvents(hub.services.http.snapshot()), 1)
}

func TestRuntimeActivationUnlabelledContainerSDKResponse(t *testing.T) {
	for _, transport := range []string{"local", "direct", "edge"} {
		for _, operation := range []activity.RuntimeOperation{activity.RuntimeStartContainer, activity.RuntimeStopContainer, activity.RuntimeRestartContainer} {
			t.Run(transport+"/"+string(operation), func(t *testing.T) {
				hub := activationApp(t, false, "")
				executor := hub
				env := "0"
				if transport != "local" {
					executor = activationApp(t, true, uuid.NewString())
					env = activationRemote(t, hub, executor, transport)
				}
				executor.services.http.onCall = func(w http.ResponseWriter, r *http.Request) bool {
					if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/containers/"+runtimeContainerID+"/json") {
						w.Header().Set("Content-Type", "application/json")
						require.NoError(t, json.NewEncoder(w).Encode(map[string]any{"Id": runtimeContainerID, "Name": "/example-unlabelled", "Config": map[string]any{"Labels": nil}, "State": map[string]any{"Running": true, "Status": "running"}}))
						return true
					}
					return false
				}
				path := "/api/environments/" + env + "/containers/" + runtimeContainerID + "/" + strings.TrimSuffix(string(operation), "-container")
				status, body := activationCall(t, hub, path, uuid.NewString(), "respond-async", nil)
				require.Equal(t, 202, status, string(body))
				run := activationRunID(t, body)
				activationJoinRun(t, hub, executor, env, run)
				require.Len(t, runtimeEffectEvents(executor.services.http.snapshot()), 1)
			})
		}
	}
}
