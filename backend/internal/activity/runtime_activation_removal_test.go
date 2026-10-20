// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Actual environment handler deletion shares durable runtime exclusion and tombstones.

package activity_test

import (
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestRuntimeActivationEnvironmentRemovalIsAtomicAndOffline(t *testing.T) {
	hub, executor := activationApp(t, false, ""), activationApp(t, true, uuid.NewString())
	env := activationRemote(t, hub, executor, "direct")
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := sync.OnceFunc(func() { close(release) })
	defer unblock()
	executor.services.http.onCall = func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/start") {
			once.Do(func() { close(entered) })
			<-release
			w.WriteHeader(204)
			return true
		}
		return false
	}
	path, key := "/api/environments/"+env+"/containers/"+runtimeContainerID+"/start", uuid.NewString()
	status, body := activationCall(t, hub, path, key, "respond-async", nil)
	require.Equal(t, 202, status, string(body))
	run := activationRunID(t, body)
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("actual executor never started")
	}
	remove := func() (int, string) {
		request, err := http.NewRequest("DELETE", hub.server.URL+"/api/environments/"+env, nil)
		require.NoError(t, err)
		response, err := hub.server.Client().Do(request)
		require.NoError(t, err)
		defer response.Body.Close()
		body, err := io.ReadAll(response.Body)
		require.NoError(t, err)
		return response.StatusCode, string(body)
	}
	status, text := remove()
	require.Equal(t, 409, status, text)
	require.NotContains(t, text, run, "removal refusal does not disclose a bound run")
	var count int64
	require.NoError(t, hub.services.db.Model(&activity.RuntimeRemovedTarget{}).Count(&count).Error)
	require.Zero(t, count)
	unblock()
	activationJoinRun(t, hub, executor, env, run)
	before := executor.services.http.snapshot()
	executor.server.Close()
	require.NoError(t, hub.services.db.Exec("CREATE TRIGGER fixture_activation_delete BEFORE DELETE ON environments BEGIN SELECT RAISE(ABORT,'synthetic removal refusal'); END").Error)
	status, text = remove()
	require.Equal(t, 500, status, text)
	require.NoError(t, hub.services.db.Model(&activity.RuntimeRemovedTarget{}).Count(&count).Error)
	require.Zero(t, count, "failed row deletion rolls back tombstones")
	_, err := hub.env.GetEnvironmentByID(t.Context(), env)
	require.NoError(t, err)
	require.NoError(t, hub.services.db.Exec("DROP TRIGGER fixture_activation_delete").Error)
	status, text = remove()
	require.Equal(t, 200, status, text)
	_, err = hub.env.GetEnvironmentByID(t.Context(), env)
	require.ErrorIs(t, err, environment.ErrEnvironmentNotFound)
	require.NoError(t, hub.services.db.Model(&activity.RuntimeRemovedTarget{}).Where("registration_id = ?", env).Count(&count).Error)
	require.EqualValues(t, 1, count)
	status, body = activationCall(t, hub, path, key, "respond-async", nil)
	require.Equal(t, 410, status, string(body))
	require.Contains(t, string(body), "runtime.target-removed")
	require.Equal(t, before, executor.services.http.snapshot(), "offline removal and retry never consult the old executor")
}
