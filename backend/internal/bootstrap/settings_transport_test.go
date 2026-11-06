// Source: docklet-next/issues/355
// Revision: b5068aff3da7eac11f208d1f4d8c9068d4a8c073
// Retrieved: 2026-11-06
// License: Apache-2.0
// Deviations: Original real Direct/Edge settings worker, online transition and agent prune evidence.

package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humaecho"
	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/auth"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/internal/event"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/runs"
	francistest "github.com/florianfinn/docklet-next/backend/pkg/utils/francis/testing"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	"github.com/florianfinn/docklet-next/types/contract"
	schedulerDTO "github.com/florianfinn/docklet-next/types/scheduler"
	settingsDTO "github.com/florianfinn/docklet-next/types/settings"
	"github.com/florianfinn/docklet-next/types/version"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func actualSettingsTunnel(t *testing.T, handler http.Handler, credential string) *edge.AgentTunnel {
	t.Helper()
	registry := edge.NewTunnelRegistry()
	previous := edge.GetRegistry()
	edge.SetDefaultRegistry(registry)
	t.Cleanup(func() { edge.SetDefaultRegistry(previous) })
	connected := make(chan struct{})
	var once sync.Once
	server := edge.NewTunnelServerWithRegistry(registry, func(_ context.Context, token string) (string, error) {
		if token != credential {
			return "", errors.New("invalid synthetic token")
		}
		return "remote", nil
	}, func(_ context.Context, _ string, online bool) {
		if online {
			once.Do(func() { close(connected) })
		}
	})
	server.SetSessionKeyResolver(func(context.Context, string, string) (string, error) { return "key", nil })
	router := echo.New()
	router.GET("/api/tunnel/connect", server.HandleConnect)
	manager := httptest.NewServer(router)
	t.Cleanup(manager.Close)
	life, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	tunnelClient := edge.NewRuntimeTunnelClient(&edge.Config{EdgeTransport: edge.EdgeTransportWebSocket, ManagerApiUrl: manager.URL, AgentToken: credential, EdgeReconnectInterval: 1}, handler)
	go func() { defer close(done); tunnelClient.StartWithErrorChan(life, nil) }()
	t.Cleanup(func() {
		cancel()
		registry.Unregister("remote")
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("history Edge client did not join")
		}
	})
	select {
	case <-connected:
	case <-time.After(5 * time.Second):
		t.Fatal("history Edge registration barrier not reached")
	}
	tunnel, ok := registry.Get("remote").Get()
	require.True(t, ok)
	return tunnel
}

type settingsDynamicScheduler struct{ job schedulerDTO.Job }

func (s *settingsDynamicScheduler) AddJob(_ context.Context, j schedulerDTO.Job) error {
	s.job = j
	return nil
}
func (*settingsDynamicScheduler) RemoveJob(context.Context, string) {}
func (*settingsDynamicScheduler) HasJob(string) bool                { return false }
func (*settingsDynamicScheduler) Submit(context.Context, schedulerDTO.Request) (schedulerDTO.Run, error) {
	return schedulerDTO.Run{}, nil
}

type observedSettingsEnvironment struct {
	*environment.EnvironmentService
	attempts    chan error
	transitions atomic.Int32
}

func (s *observedSettingsEnvironment) SubscribeAgentOnline(fn func(context.Context, string)) func() {
	return s.EnvironmentService.SubscribeAgentOnline(func(ctx context.Context, id string) { s.transitions.Add(1); fn(ctx, id) })
}

func (s *observedSettingsEnvironment) ProxyRequest(ctx context.Context, id, method, path string, body []byte) ([]byte, int, error) {
	b, status, err := s.EnvironmentService.ProxyRequest(ctx, id, method, path, body)
	s.attempts <- err
	return b, status, err
}
func TestM19RetentionSyncRealDirectEdgeAndOnlineResend(t *testing.T) {
	for _, isEdge := range []bool{false, true} {
		t.Run(map[bool]string{false: "direct", true: "edge"}[isEdge], func(t *testing.T) {
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
			scheduler.NewEventCleanupJob(event.NewEventService(agentDB, &config.Config{}, nil), activity.NewActivityService(agentDB, nil), agent)
			for i := range 70 {
				require.NoError(t, agentDB.Exec("INSERT INTO events(id,type,title,timestamp) VALUES(?,?,?,?)", fmt.Sprintf("event-%03d", i), "container.start", "Example", time.Now().Add(time.Duration(i)*time.Second)).Error)
			}
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
			router.Any("/api/container-registries/sync", func(c *echo.Context) error { return c.JSON(200, map[string]any{"success": true}) })
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
			// Authentication/version verification of the health job owns the real online transition.
			online.Store(true)
			dynamic := &settingsDynamicScheduler{}
			runtime := francistest.New(t)
			admission := runs.NewAdmission(runtime.Service(), t.Name())
			require.NoError(t, admission.Register(runtime))
			francistest.Start(t, runtime)
			require.NoError(t, env.SetScheduler(t.Context(), dynamic, admission))
			env.RegisterHealthJobsOnStartup(t.Context())
			require.NotNil(t, dynamic.job)
			_, _ = dynamic.job.Run(t.Context())
			require.EqualValues(t, 1, observed.transitions.Load(), "authenticated online transition must wake settings sync")
			select {
			case err := <-observed.attempts:
				require.NoError(t, err)
			case <-time.After(5 * time.Second):
				t.Fatal("online transition did not resend settings")
			}
			// Join accepted effects, including the agent's asynchronous cleanup, before checking their outcome.
			require.NoError(t, agent.Stop(context.Background()))
			require.Equal(t, 50, agent.GetIntSetting(t.Context(), "activityEventLimit", 0))
			require.Equal(t, 10, agent.GetIntSetting(t.Context(), "activityRunLimit", 0))
			var count int64
			require.NoError(t, agentDB.Model(&event.Event{}).Count(&count).Error)
			require.EqualValues(t, 50, count)
			// The actual wire payload retained the complete shared key set.
			encoded, err := json.Marshal(libarcane.AgentSyncedSettingKeys())
			require.NoError(t, err)
			require.Contains(t, string(encoded), "activityEventLimit")
		})
	}
}
