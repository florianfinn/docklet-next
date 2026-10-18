// Source: docklet-next/issues/96
// Revision: 9695ef3e6b5a13155348997b86f513eb560efae8
// Retrieved: 2026-10-18
// License: Apache-2.0
// Deviations: Original direct-host token rotation and stale inventory regression tests.

package environment

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/apikey"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/internal/user"
	"github.com/florianfinn/docklet-next/backend/pkg/hoststate"
	"github.com/florianfinn/docklet-next/backend/pkg/remenv"
	"github.com/florianfinn/docklet-next/types/contract"
	"github.com/florianfinn/docklet-next/types/version"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
)

func TestDirectRotationRequiresCurrentAgentToken(t *testing.T) {
	for _, rejection := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		for _, entry := range []string{"compatibility", "health", "heartbeat", "forwarding"} {
			t.Run(fmt.Sprintf("%d/%s", rejection, entry), func(t *testing.T) {
				f := newAgentFixture(t, "direct", version.Info{AgentContractVersion: contract.AgentHubVersion, AgentContractFingerprint: contract.AgentHubFingerprint})
				ctx := t.Context()
				installedToken := f.token
				f.service.httpClient.Transport = agentRoundTrip(func(r *http.Request) (*http.Response, error) {
					body := f.respond(r.Method, r.URL.Path, r.Header)
					status := http.StatusOK
					// Version and public health stay successful even while the agent has the old token.
					if r.URL.Path != "/api/app-version" && r.URL.Path != "/api/health" {
						if r.Header.Get(remenv.HeaderAgentToken) != installedToken || r.Header.Get(remenv.HeaderAPIKey) != installedToken {
							status, body = rejection, []byte(`{"error":"invalid token"}`)
						}
					}
					return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(string(body))), Request: r}, nil
				})
				check := func() error {
					switch entry {
					case "health":
						status, err := f.service.TestConnection(ctx, f.id, nil)
						if err == nil {
							require.Equal(t, "online", status)
						} else {
							require.Equal(t, "token-rejected", status)
						}
						return err
					case "heartbeat":
						return f.service.UpdateEnvironmentHeartbeat(ctx, f.id)
					case "forwarding":
						row, err := f.service.GetEnvironmentByID(ctx, f.id)
						require.NoError(t, err)
						_, cancel, err := f.service.AgentAccess(ctx, f.id, row.AccessToken, http.MethodGet, "/api/environments/0/containers")
						if cancel != nil {
							cancel()
						}
						return err
					default:
						_, err := f.service.CheckAgentCompatibility(ctx, f.id)
						return err
					}
				}
				// A fresh registration still completes using the configured token.
				require.NoError(t, check())
				env, err := f.service.GetEnvironmentByID(ctx, f.id)
				require.NoError(t, err)
				require.Equal(t, "online", env.Status)
				require.NotNil(t, env.LastSeen)

				f.service.apiKeys = apikey.NewApiKeyService(f.service.db, user.NewUserService(f.service.db, nil, nil), nil)
				replacement, err := f.service.RegenerateEnvironmentApiKey(ctx, env, "operator", "operator")
				require.NoError(t, err)
				env, err = f.service.GetEnvironmentByIDCached(ctx, f.id)
				require.NoError(t, err)
				require.Equal(t, "pending", env.Status)
				require.Nil(t, env.LastSeen)
				require.ErrorIs(t, check(), remenv.ErrAgentInvalidToken)
				env, err = f.service.GetEnvironmentByIDCached(ctx, f.id)
				require.NoError(t, err)
				require.Equal(t, "token-rejected", env.Status)
				require.Nil(t, env.LastSeen, "a rejected credential is not successful contact")
				require.Equal(t, 2, f.count("HEAD /api/environments/0/system/health"), "rotation invalidates the successful verdict")

				assertRejectedRotationSnapshots(t, f)
				installedToken = replacement
				require.NoError(t, check(), "a failed probe must be retried once the new token is installed")
				env, err = f.service.GetEnvironmentByIDCached(ctx, f.id)
				require.NoError(t, err)
				require.Equal(t, "online", env.Status)
				require.NotNil(t, env.LastSeen)
				probes := f.count("HEAD /api/environments/0/system/health")
				for range 5 {
					_, err := f.service.ExecuteRemoteRequest(ctx, f.id, http.MethodGet, "/api/environments/0/containers", nil)
					require.NoError(t, err)
				}
				require.Equal(t, probes, f.count("HEAD /api/environments/0/system/health"), "ordinary reads reuse the credential-session verdict")
			})
		}
	}
}

func assertRejectedRotationSnapshots(t *testing.T, f *agentFixture) {
	t.Helper()
	router := echo.New()
	resolver := func(ctx context.Context, id string) (string, *string, bool, error) {
		env, err := f.service.GetEnvironmentByIDCached(ctx, id)
		if err != nil {
			return "", nil, false, err
		}
		return env.ApiUrl, env.AccessToken, env.Enabled, nil
	}
	router.Use(middleware.NewEnvProxyMiddlewareWithParamAndRegistry("0", "id", resolver, nil, nil, nil, nil,
		middleware.EnvironmentProxySecurity{Snapshots: f.service.ReadSnapshots(), InventoryPending: f.service.InventoryPending, AgentAccess: f.service.AgentAccess, HTTPClient: f.service.httpClient}))
	for _, suffix := range []string{"containers", "projects", "images", "volumes", "networks"} {
		snapshot := f.service.ReadSnapshots().Put(hoststate.Key{HostID: f.id, List: "/" + suffix + "?"}, []byte(`{"success":true,"data":[{"id":"saved"}]}`))
		router.GET("/api/environments/:id/"+suffix, func(c *echo.Context) error { return c.NoContent(500) })
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/environments/"+f.id+"/"+suffix, nil))
		require.Equal(t, http.StatusOK, rec.Code, suffix)
		require.Contains(t, rec.Body.String(), "saved", suffix)
		require.Equal(t, "agent.invalid-token", rec.Header().Get("X-Docklet-Host-Reason"), suffix)
		require.Equal(t, snapshot.ObservedAt.Format(time.RFC3339Nano), rec.Header().Get("X-Docklet-Observed-At"), suffix)
		require.Zero(t, f.count("GET /api/environments/0/"+suffix), "rejected credentials must block ordinary forwarding")
	}
}

func TestAgentTokenRejectionCannotOverwriteRotatedSession(t *testing.T) {
	f := newAgentFixture(t, "direct", version.Info{})
	ctx := t.Context()
	env, err := f.service.GetEnvironmentByID(ctx, f.id)
	require.NoError(t, err)
	session, err := f.service.agentSessionFor(ctx, env)
	require.NoError(t, err)
	f.service.apiKeys = apikey.NewApiKeyService(f.service.db, user.NewUserService(f.service.db, nil, nil), nil)
	_, err = f.service.RegenerateEnvironmentApiKey(ctx, env, "operator", "operator")
	require.NoError(t, err)
	err = f.service.agentCompatibilityFailure(ctx, env, session, &remenv.StatusError{StatusCode: http.StatusForbidden})
	require.ErrorIs(t, err, ErrInvalidEnvironmentAccessToken)
	current, err := f.service.GetEnvironmentByID(ctx, f.id)
	require.NoError(t, err)
	require.Equal(t, "pending", current.Status)
	require.Nil(t, current.LastSeen)
}
