// Source: docklet-next/issues/355
// Revision: 8e133434769733113609882186fb6ba6e79a8c67
// Retrieved: 2026-11-06
// License: Apache-2.0
// Deviations: Original manual reconnect and Edge heartbeat settings resend test.

package bootstrap

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humaecho"
	"github.com/florianfinn/docklet-next/backend/internal/auth"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	"github.com/florianfinn/docklet-next/types/contract"
	settingsDTO "github.com/florianfinn/docklet-next/types/settings"
	"github.com/florianfinn/docklet-next/types/version"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
)

// An agent offline during a retention save gets the values when it is next seen online through a manual
// connection test (Direct) or an Edge heartbeat, not only through the scheduled health job.
func TestM19ManualReconnectAndEdgeHeartbeatResendSettings(t *testing.T) {
	for _, isEdge := range []bool{false, true} {
		t.Run(map[bool]string{false: "direct-manual-test", true: "edge-heartbeat"}[isEdge], func(t *testing.T) {
			makeDB := func(name string) *database.DB {
				db, err := database.Initialize(t.Context(), sqlitestore.URL(filepath.Join(t.TempDir(), name)), database.MigrationOptions{})
				require.NoError(t, err)
				t.Cleanup(func() { require.NoError(t, db.Close()) })
				return db
			}
			hubDB, agentDB := makeDB("hub.db"), makeDB("agent.db")
			hub, err := settings.NewSettingsService(t.Context(), hubDB)
			require.NoError(t, err)
			defer hub.Stop(context.Background())
			agent, err := settings.NewSettingsService(t.Context(), agentDB)
			require.NoError(t, err)
			defer agent.Stop(context.Background())
			cfg := &config.Config{AgentMode: true, AgentToken: "example"}
			router := echo.New()
			api := humaecho.NewWithGroup(router, router.Group("/api"), huma.DefaultConfig("settings", "test"))
			api.UseMiddleware(auth.NewHumaMiddleware(api, &auth.AuthService{}, nil, nil, nil, cfg))
			settings.RegisterSettings(api, agent, nil, cfg)
			huma.Get(api, "/app-version", func(context.Context, *struct{}) (*struct{ Body version.Info }, error) {
				return &struct{ Body version.Info }{Body: version.Info{AgentContractVersion: contract.AgentHubVersion, AgentContractFingerprint: contract.AgentHubFingerprint}}, nil
			})
			huma.Head(api, "/environments/{id}/system/health", func(context.Context, *struct {
				ID string `path:"id"`
			}) (*struct{}, error) {
				return nil, nil
			})
			router.Any("/api/*", func(c *echo.Context) error { return c.JSON(200, map[string]any{"success": true}) })
			var online atomic.Bool
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !online.Load() {
					w.WriteHeader(503)
					return
				}
				router.ServeHTTP(w, r)
			})
			server := httptest.NewServer(handler)
			defer server.Close()
			if isEdge {
				_ = actualSettingsTunnel(t, handler, "example")
			}
			require.NoError(t, hubDB.Exec("INSERT INTO api_keys(id,name,key_hash,key_prefix,kind) VALUES('key','Example','example','example','environment')").Error)
			host := environment.Environment{ID: "remote", Name: "Example", Enabled: true, Status: "offline", ApiUrl: server.URL, IsEdge: isEdge, ApiKeyID: new("key"), AccessToken: new("example")}
			require.NoError(t, hubDB.Create(&host).Error)
			env := environment.NewEnvironmentService(hubDB, http.DefaultClient, nil, nil, hub, nil)
			observed := &observedSettingsEnvironment{EnvironmentService: env, attempts: make(chan error, 8)}
			subscribe := func(keys []string, fn func([]libarcane.SettingUpdate)) { hub.SubscribeSettingsChanges(keys, fn) }
			worker, cancel := setupTimeoutSettingsSubscriptionInternal(settingsSubscriptionsParams{LifecycleCtx: t.Context(), Config: &config.Config{}, Settings: hub, Environment: observed}, subscribe)
			defer cancel()
			defer worker.Stop(context.Background())

			_, err = hub.UpdateSettings(t.Context(), settingsDTO.Update{ActivityEventLimit: new("50"), ActivityRunLimit: new("10")})
			require.NoError(t, err)
			select {
			case <-observed.attempts:
			case <-time.After(5 * time.Second):
				t.Fatal("offline save not attempted")
			}
			require.Equal(t, 500, agent.GetIntSetting(t.Context(), "activityEventLimit", 0))

			online.Store(true)
			if isEdge {
				require.NoError(t, env.UpdateEnvironmentHeartbeat(t.Context(), "remote"))
			} else {
				status, err := env.TestConnection(t.Context(), "remote", nil)
				require.NoError(t, err)
				require.Equal(t, "online", status)
			}
			require.EqualValues(t, 1, observed.transitions.Load(), "the authenticated online write announces the agent")
			select {
			case err := <-observed.attempts:
				require.NoError(t, err)
			case <-time.After(5 * time.Second):
				t.Fatal("coming online did not resend the settings")
			}
			require.NoError(t, agent.Stop(context.Background()))
			require.Equal(t, 50, agent.GetIntSetting(t.Context(), "activityEventLimit", 0))
			require.Equal(t, 10, agent.GetIntSetting(t.Context(), "activityRunLimit", 0))

			// A second check of an agent already online announces nothing.
			if isEdge {
				require.NoError(t, env.UpdateEnvironmentHeartbeat(t.Context(), "remote"))
			} else {
				_, err = env.TestConnection(t.Context(), "remote", nil)
				require.NoError(t, err)
			}
			require.EqualValues(t, 1, observed.transitions.Load())
		})
	}
}
