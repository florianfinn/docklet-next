// Source: docklet-next/issues/85
// Revision: 0927d517e029fffe31805211b3c02e49c4a93af5
// Retrieved: 2026-10-18
// License: Apache-2.0
// Deviations: Original pending inventory regression through the credential lifecycle.

package environment

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/apikey"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/internal/user"
	"github.com/florianfinn/docklet-next/backend/pkg/hoststate"
	"github.com/florianfinn/docklet-next/types/contract"
	environmenttypes "github.com/florianfinn/docklet-next/types/environment"
	"github.com/florianfinn/docklet-next/types/version"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
)

func TestAB2InventoryAfterRegistrationRotationAndReconnect(t *testing.T) {
	for _, mode := range []string{"direct", "edge"} {
		for _, transition := range []string{"registration", "rotation", "reconnect"} {
			if mode == "direct" && transition == "reconnect" {
				continue
			}
			t.Run(mode+"/"+transition, func(t *testing.T) {
				f := newAgentFixture(t, mode, version.Info{AgentContractVersion: contract.AgentHubVersion, AgentContractFingerprint: contract.AgentHubFingerprint, OnboardingChecks: &version.OnboardingChecks{ProjectDirectory: "/srv/projects"}})
				ctx := t.Context()
				suffixes := []string{"containers", "projects", "images", "volumes", "networks", "projects/app", "volumes/data", "networks/front"}
				observed := map[string]string{}
				if transition != "registration" {
					_, err := f.service.CheckAgentCompatibility(ctx, f.id)
					require.NoError(t, err)
					pending, err := f.service.InventoryPending(ctx, f.id)
					require.NoError(t, err)
					require.False(t, pending)
					for _, suffix := range suffixes {
						value := f.service.ReadSnapshots().Put(hoststate.Key{HostID: f.id, List: "/" + suffix + "?"}, []byte(`{"success":true,"data":[{"id":"saved"}]}`))
						observed[suffix] = value.ObservedAt.Format(time.RFC3339Nano)
					}
					if transition == "rotation" {
						f.service.apiKeys = apikey.NewApiKeyService(f.service.db, user.NewUserService(f.service.db, nil, nil), nil)
						env, err := f.service.GetEnvironmentByID(ctx, f.id)
						require.NoError(t, err)
						_, err = f.service.RegenerateEnvironmentApiKey(ctx, env, "operator", "operator")
						require.NoError(t, err)
					} else {
						require.NoError(t, f.service.UpdateEnvironmentConnectionState(ctx, f.id, true))
					}
				}
				env, err := f.service.GetEnvironmentByID(ctx, f.id)
				require.NoError(t, err)
				require.Equal(t, "pending", env.Status)
				if transition == "registration" {
					require.Nil(t, env.LastSeen)
					dto := environmenttypes.Environment{ID: f.id, Status: env.Status, IsEdge: mode == "edge"}
					f.service.ApplyHostRuntimeState(ctx, &dto)
					require.Nil(t, dto.AgentReport)
				}
				if transition == "rotation" {
					require.NotNil(t, env.LastSeen, "rotation retains the last successful contact")
					dto := environmenttypes.Environment{ID: f.id, Status: env.Status, IsEdge: mode == "edge"}
					f.service.ApplyHostRuntimeState(ctx, &dto)
					require.NotNil(t, dto.AgentReport, "rotation retains the historical report")
					require.Equal(t, "/srv/projects", dto.AgentReport.Version.OnboardingChecks.ProjectDirectory)
					require.Equal(t, "pending", dto.Status)
				}
				pending, err := f.service.InventoryPending(ctx, f.id)
				require.NoError(t, err)
				require.True(t, pending, "cached online state must be invalidated")
				calls := len(f.calls)
				router := echo.New()
				router.Use(middleware.NewEnvProxyMiddlewareWithParamAndRegistry("0", "id",
					func(ctx context.Context, id string) (string, *string, bool, error) {
						row, err := f.service.GetEnvironmentByIDCached(ctx, id)
						if err != nil {
							return "", nil, false, err
						}
						return row.ApiUrl, row.AccessToken, row.Enabled, nil
					}, nil, nil, nil, nil, middleware.EnvironmentProxySecurity{Snapshots: f.service.ReadSnapshots(), InventoryPending: f.service.InventoryPending, AgentAccess: f.service.AgentAccess, HTTPClient: f.service.httpClient}))
				for _, suffix := range suffixes {
					router.GET("/api/environments/:id/"+suffix, func(c *echo.Context) error { return c.NoContent(500) })
					rec := httptest.NewRecorder()
					router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/environments/"+f.id+"/"+suffix, nil))
					require.Equal(t, "agent.unavailable", rec.Header().Get("X-Docklet-Host-Reason"), suffix)
					if transition == "registration" {
						require.Equal(t, http.StatusBadGateway, rec.Code, suffix)
						require.Empty(t, rec.Header().Get("X-Docklet-Observed-At"), suffix)
					} else {
						require.Equal(t, http.StatusOK, rec.Code, suffix)
						require.Contains(t, rec.Body.String(), "saved", suffix)
						require.Equal(t, observed[suffix], rec.Header().Get("X-Docklet-Observed-At"), suffix)
					}
				}
				require.Len(t, f.calls, calls, "pending hosts must receive no version or inventory I/O")
			})
		}
	}
}
