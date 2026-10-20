// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Actual registered machine authentication and Direct/Edge compatibility fences.

package activity_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/pkg/remenv"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"github.com/florianfinn/docklet-next/types/contract"
	versiontypes "github.com/florianfinn/docklet-next/types/version"
	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
)

func TestRuntimeActivationV3AndMismatchBeforeDirectAndEdgeCommands(t *testing.T) {
	for _, transport := range []string{"direct", "edge"} {
		for _, mode := range []string{"v3", "mismatch"} {
			t.Run(transport+"/"+mode, func(t *testing.T) {
				hub := activationApp(t, false, "")
				agent := activationApp(t, true, uuid.NewString())
				var ordinary atomic.Int32
				agent.router.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
					return func(c *echo.Context) error {
						if c.Request().URL.Path == "/api/app-version" {
							info := versiontypes.Info{AgentContractVersion: contract.AgentHubVersion, AgentContractFingerprint: "different-fingerprint", MinimumAgentContractVersion: contract.MinimumAgentVersion}
							if mode == "v3" {
								info.AgentContractVersion = 3
								info.AgentContractFingerprint = "39fc19066cb96d807cba48cb01ccf58ed9cfc45f15dbafb8ce2d2bccd7d4015e"
								info.MinimumAgentContractVersion = 3
							}
							return c.JSON(200, info)
						}
						if strings.Contains(c.Request().URL.Path, "/runtime-") || strings.Contains(c.Request().URL.Path, "/containers/") {
							ordinary.Add(1)
						}
						return next(c)
					}
				})
				env := activationRemote(t, hub, agent, transport)
				_, err := hub.env.CheckAgentCompatibility(t.Context(), env)
				require.ErrorIs(t, err, remenv.ErrAgentUpdateRequired)
				path := "/api/environments/" + env + "/containers/" + runtimeContainerID + "/start"
				status, body := activationCall(t, hub, path, uuid.NewString(), "respond-async", nil)
				require.Equal(t, 503, status, string(body))
				require.Contains(t, string(body), "agent.update-required")
				require.NotContains(t, string(body), "activityId")
				token := agent.cfg.AgentToken
				for _, query := range []string{"/api/environments/0/containers", "/api/environments/0/runtime-runs/" + uuid.NewString()} {
					_, cleanup, err := hub.env.AgentAccess(t.Context(), env, &token, "GET", query)
					require.ErrorIs(t, err, remenv.ErrAgentUpdateRequired)
					require.Nil(t, cleanup)
				}
				for _, route := range []struct{ method, path string }{{"GET", "/api/app-version"}, {"HEAD", "/api/environments/0/system/health"}, {"POST", "/api/environments/0/system/upgrade"}} {
					_, cleanup, err := hub.env.AgentAccess(t.Context(), env, &token, route.method, route.path)
					require.NoError(t, err)
					require.NotNil(t, cleanup)
					cleanup()
				}
				var receipts int64
				require.NoError(t, hub.services.db.Model(&activity.RuntimeReceipt{}).Count(&receipts).Error)
				require.Zero(t, receipts)
				require.Zero(t, ordinary.Load())
				require.Empty(t, runtimeEffectEvents(agent.services.http.snapshot()))
			})
		}
	}
}

func activationMetadataCall(t *testing.T, f *activationFixture, path string, headers map[string]string, body []byte) (int, []byte) {
	t.Helper()
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, f.server.URL+path, bytes.NewReader(body))
	require.NoError(t, err)
	request.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	response, err := f.server.Client().Do(request)
	require.NoError(t, err)
	defer response.Body.Close()
	result, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	return response.StatusCode, result
}

func TestRuntimeActivationRegisteredReceiverRequiresMachineCredential(t *testing.T) {
	token := uuid.NewString()
	bootstrap := uuid.NewString()
	agent := activationApp(t, true, token, activationSetup{Configure: func(cfg *config.Config) { cfg.AdminStaticAPIKey = bootstrap }})
	body := []byte(`{"managerId":"example-manager","registrationId":"example-registration"}`)
	for _, headers := range []map[string]string{nil, {"Authorization": "Bearer " + uuid.NewString()}, {"X-Api-Key": bootstrap}, {remenv.HeaderAgentToken: uuid.NewString()}} {
		status, response := activationMetadataCall(t, agent, "/api/environments/0/runtime-registration", headers, body)
		require.True(t, status == 401 || status == 403, string(response))
		var count int64
		require.NoError(t, agent.services.db.Model(&activity.RuntimeExecutorBinding{}).Count(&count).Error)
		require.Zero(t, count)
	}
	status, response := activationMetadataCall(t, agent, "/api/environments/0/runtime-registration", map[string]string{remenv.HeaderAgentToken: token}, body)
	require.Equal(t, 200, status, string(response))
	var identity activitytypes.RuntimeExecutorIdentity
	require.NoError(t, json.Unmarshal(response, &identity))
	require.Equal(t, "example-manager", identity.ManagerID)
	require.Equal(t, "example-registration", identity.RegistrationID)
	require.Equal(t, agent.services.runtime.LocalStore().ExecutorGeneration, identity.ExecutorGeneration)
	var receipts int64
	require.NoError(t, agent.services.db.Model(&activity.RuntimeReceipt{}).Count(&receipts).Error)
	require.Zero(t, receipts)
	require.Empty(t, runtimeEffectEvents(agent.services.http.snapshot()))
}

func TestRuntimeActivationRegisteredBindingCannotReplaceLiveExecutor(t *testing.T) {
	hub := activationApp(t, false, "")
	agent := activationApp(t, true, uuid.NewString())
	env := activationRemote(t, hub, agent, "direct")
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := sync.OnceFunc(func() { close(release) })
	defer unblock()
	agent.services.http.onCall = func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/start") {
			once.Do(func() { close(entered) })
			<-release
			w.WriteHeader(204)
			return true
		}
		return false
	}
	status, body := activationCall(t, hub, "/api/environments/"+env+"/containers/"+runtimeContainerID+"/start", uuid.NewString(), "respond-async", nil)
	require.Equal(t, 202, status, string(body))
	run := activationRunID(t, body)
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("actual SDK executor did not start")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	require.NoError(t, hub.remote.WaitDispatch(ctx, run))
	original := agent.services.runtime.LocalStore()
	for _, changed := range []bool{false, true} {
		registration := activitytypes.RuntimeRegistration{ManagerID: original.ManagerID, RegistrationID: original.RegistrationID}
		if changed {
			registration.RegistrationID = "example-replacement"
		}
		encoded, err := json.Marshal(registration)
		require.NoError(t, err)
		status, response := activationMetadataCall(t, agent, "/api/environments/0/runtime-registration", map[string]string{remenv.HeaderAgentToken: agent.cfg.AgentToken}, encoded)
		if changed {
			require.Equal(t, 409, status, string(response))
		} else {
			require.Equal(t, 200, status, string(response))
		}
		require.Equal(t, original, agent.services.runtime.LocalStore())
	}
	unblock()
	activationJoinRun(t, hub, agent, env, run)
	require.Len(t, runtimeEffectEvents(agent.services.http.snapshot()), 1)
}
