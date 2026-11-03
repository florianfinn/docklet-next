// Source: docklet-next/issues/335
// Revision: 14e11ba9c022d57c29bd36c8ed24780763327d0c
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original real Direct and Edge history producer/receiver boundary tests.

package history

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humaecho"
	"github.com/florianfinn/docklet-next/backend/internal/auth"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	containerservice "github.com/florianfinn/docklet-next/backend/internal/container"
	"github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	proxyservice "github.com/florianfinn/docklet-next/backend/internal/environment/children/proxy"
	"github.com/florianfinn/docklet-next/backend/internal/event"
	"github.com/florianfinn/docklet-next/backend/internal/project"
	"github.com/florianfinn/docklet-next/backend/internal/runtimeintent"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/florianfinn/docklet-next/backend/pkg/utils"
	"github.com/florianfinn/docklet-next/types/base"
	dto "github.com/florianfinn/docklet-next/types/history"
	userDTO "github.com/florianfinn/docklet-next/types/user"
	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type historyDocker struct {
	mu                 sync.Mutex
	id, state, started string
	exit               int
	failed             bool
	statsCalls         atomic.Int32
}

func (f *historyDocker) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	switch {
	case strings.HasSuffix(r.URL.Path, "/_ping"):
		w.WriteHeader(200)
	case strings.HasSuffix(r.URL.Path, "/containers/json"):
		_ = json.NewEncoder(w).Encode([]any{map[string]any{"Id": f.id, "Names": []string{"/app"}, "State": f.state, "Labels": map[string]string{}}})
	case strings.HasSuffix(r.URL.Path, "/json"):
		_ = json.NewEncoder(w).Encode(map[string]any{"Id": f.id, "Name": "/app", "Config": map[string]any{"Labels": map[string]string{}}, "State": map[string]any{"Status": f.state, "Running": f.state == "running", "StartedAt": f.started, "FinishedAt": "2026-11-03T12:00:00Z", "ExitCode": f.exit}, "RestartCount": 0})
	case strings.HasSuffix(r.URL.Path, "/stats"):
		f.statsCalls.Add(1)
		if r.URL.Query().Get("stream") != "false" || r.URL.Query().Get("one-shot") != "true" {
			http.Error(w, "expected one-shot stats", 400)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"cpu_stats": map[string]any{"cpu_usage": map[string]any{"total_usage": 10}, "system_cpu_usage": 100}, "memory_stats": map[string]any{"usage": 100, "limit": 1000}})
	case strings.HasSuffix(r.URL.Path, "/stop"):
		if f.failed {
			http.Error(w, "stop refused", 500)
			return
		}
		f.state = "exited"
		f.exit = 137
		w.WriteHeader(204)
	default:
		http.Error(w, "unexpected Docker route", 404)
	}
}
func actualHistoryTunnel(t *testing.T, handler http.Handler, credential string) *edge.AgentTunnel {
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
		return "host", nil
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
	tunnelClient := edge.NewRuntimeTunnelClient(&edge.Config{EdgeTransport: edge.EdgeTransportWebSocket, ManagerApiUrl: manager.URL, AgentToken: credential, EdgeReconnectInterval: 1}, handler)
	go func() { defer close(done); tunnelClient.StartWithErrorChan(life, nil) }()
	t.Cleanup(func() {
		cancel()
		registry.Unregister("host")
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
	tunnel, ok := registry.Get("host").Get()
	require.True(t, ok)
	return tunnel
}
func TestHB5HB8HB9ActualDirectAndEdgeSample(t *testing.T) {
	for _, isEdge := range []bool{false, true} {
		t.Run(map[bool]string{false: "direct", true: "edge"}[isEdge], func(t *testing.T) {
			db := historyDB(t)
			credential := uuid.NewString()
			cfg := &config.Config{AgentMode: true, AgentToken: credential, Environment: "test"}
			setting, err := settings.NewSettingsService(t.Context(), db)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, setting.Stop(context.WithoutCancel(t.Context()))) })
			fake := &historyDocker{id: "first", state: "running", started: "2026-11-03T10:00:00Z"}
			daemon := httptest.NewServer(http.HandlerFunc(fake.serve))
			t.Cleanup(daemon.Close)
			cli, err := client.New(client.WithHost(daemon.URL), client.WithAPIVersion("1.41"), client.WithHTTPClient(daemon.Client()))
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, cli.Close()) })
			dockerService := docker.NewDockerClientService(t.Context(), db, cfg, setting).WithClient(cli)
			events := event.NewEventService(db, cfg, daemon.Client())
			projects := project.NewProjectService(db, setting, events, nil, dockerService, nil, nil, cfg, nil, nil)
			containers := containerservice.NewContainerService(events, dockerService, nil, setting, projects)
			producer := NewProducer(db, dockerService, setting)
			producer.hostSample = func(context.Context) (dto.HostSample, error) { return fixtureSample().Host, nil }
			router := echo.New()
			api := humaecho.NewWithGroup(router, router.Group("/api"), huma.DefaultConfig("history", "test"))
			api.UseMiddleware(auth.NewHumaMiddleware(api, &auth.AuthService{}, nil, nil, nil, cfg))
			New(nil, producer).RegisterRoutes(api, true)
			var oversized atomic.Bool
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/stop") {
					if err := containers.StopContainer(r.Context(), "first", userDTO.SystemUser); err != nil {
						http.Error(w, "stop failed", 500)
						return
					}
					w.WriteHeader(204)
					return
				}
				if oversized.Load() {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(200)
					if f, ok := w.(http.Flusher); ok {
						f.Flush()
					}
					_, _ = w.Write([]byte(strings.Repeat("x", dto.SampleLimit+1)))
					return
				}
				router.ServeHTTP(w, r)
			})
			direct := httptest.NewServer(handler)
			t.Cleanup(direct.Close)
			var tunnel *edge.AgentTunnel
			if isEdge {
				tunnel = actualHistoryTunnel(t, handler, credential)
			}
			transport := proxyservice.New(db, direct.Client(), nil, &utils.SyncGate{})
			target, err := proxyservice.NewTarget("host", "Example", direct.URL, isEdge, &credential)
			require.NoError(t, err)
			request := func() (dto.Sample, error) {
				ctx := t.Context()
				if isEdge {
					ctx = edge.WithSessionTunnel(edge.WithSessionCredential(ctx, "example-key", credential, nil), tunnel)
				}
				var response base.ApiResponse[dto.Sample]
				err := transport.JSON(ctx, target, "GET", dto.SamplePath, nil, &response)
				return response.Data, err
			}
			sample, err := request()
			require.NoError(t, err)
			require.Len(t, sample.Containers, 1)
			require.NotNil(t, sample.Containers[0].Stats)
			require.Greater(t, fake.statsCalls.Load(), int32(0))
			require.False(t, sample.Containers[0].StatsMissing)
			stop := func() error {
				ctx := t.Context()
				if isEdge {
					ctx = edge.WithSessionTunnel(edge.WithSessionCredential(ctx, "example-key", credential, nil), tunnel)
				}
				response, err := transport.Execute(ctx, target, "POST", "/api/environments/0/containers/first/stop", nil)
				if err != nil {
					return err
				}
				return response.RequireSuccess()
			}
			require.NoError(t, stop())
			sample, err = request()
			require.NoError(t, err)
			require.True(t, sample.Containers[0].StopIntent)
			state, reason := Classify(sample.Containers[0], 0)
			require.Equal(t, dto.Stopped, state)
			require.Equal(t, "intent", reason)
			fake.mu.Lock()
			fake.state = "running"
			fake.started = time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano)
			fake.mu.Unlock()
			sample, err = request()
			require.NoError(t, err)
			require.False(t, sample.Containers[0].StopIntent)
			var count int64
			require.NoError(t, db.Model(&runtimeintent.Row{}).Count(&count).Error)
			require.Zero(t, count)
			fake.mu.Lock()
			fake.failed = true
			fake.mu.Unlock()
			require.Error(t, stop())
			require.NoError(t, db.Model(&runtimeintent.Row{}).Count(&count).Error)
			require.Zero(t, count)
			fake.mu.Lock()
			fake.failed = false
			fake.started = "2026-11-03T10:00:00Z"
			fake.mu.Unlock()
			require.NoError(t, stop())
			require.NoError(t, db.Model(&runtimeintent.Row{}).Count(&count).Error)
			require.EqualValues(t, 1, count)
			fake.mu.Lock()
			fake.id = "replacement"
			fake.state = "exited"
			fake.exit = 1
			fake.mu.Unlock()
			sample, err = request()
			require.NoError(t, err)
			require.False(t, sample.Containers[0].StopIntent)
			state, _ = Classify(sample.Containers[0], 0)
			require.Equal(t, dto.Down, state)
			oversized.Store(true)
			_, err = request()
			require.Error(t, err)
			require.Contains(t, err.Error(), "history.sample-too-large")
			hub, host, minute := samplerFixture(t)
			hub.fetch = func(context.Context, environment.Environment) (dto.Sample, error) { return request() }
			require.NoError(t, hub.SampleHost(t.Context(), host, minute))
			require.NoError(t, hub.db.Table("history_buckets").Where("kind<>'host'").Count(&count).Error)
			require.Zero(t, count)
		})
	}
}

// N1/AUTO-3: a container that vanishes between list and inspect is left out; the host minute still counts.
func TestSampleSkipsContainerGoneBeforeInspect(t *testing.T) {
	db := historyDB(t)
	daemon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/_ping"):
			w.WriteHeader(200)
		case strings.HasSuffix(r.URL.Path, "/containers/json"):
			_ = json.NewEncoder(w).Encode([]any{map[string]any{"Id": "gone", "Names": []string{"/old"}, "State": "running"}, map[string]any{"Id": "kept", "Names": []string{"/app"}, "State": "exited"}})
		case strings.HasSuffix(r.URL.Path, "/gone/json"):
			http.Error(w, `{"message":"No such container: gone"}`, http.StatusNotFound)
		case strings.HasSuffix(r.URL.Path, "/kept/json"):
			_ = json.NewEncoder(w).Encode(map[string]any{"Id": "kept", "Name": "/app", "Config": map[string]any{"Labels": map[string]string{}}, "State": map[string]any{"Status": "exited", "ExitCode": 0, "StartedAt": "2026-11-03T10:00:00Z", "FinishedAt": "2026-11-03T11:00:00Z"}})
		default:
			http.Error(w, "unexpected Docker route", 404)
		}
	}))
	t.Cleanup(daemon.Close)
	cli, err := client.New(client.WithHost(daemon.URL), client.WithAPIVersion("1.41"), client.WithHTTPClient(daemon.Client()))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, cli.Close()) })
	setting, err := settings.NewSettingsService(t.Context(), db)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, setting.Stop(context.WithoutCancel(t.Context()))) })
	dockerService := docker.NewDockerClientService(t.Context(), db, &config.Config{Environment: "test"}, setting).WithClient(cli)
	producer := NewProducer(db, dockerService, setting)
	producer.hostSample = func(context.Context) (dto.HostSample, error) { return fixtureSample().Host, nil }
	sample, err := producer.Sample(t.Context())
	require.NoError(t, err)
	require.Len(t, sample.Containers, 1)
	require.Equal(t, "kept", sample.Containers[0].ID)
}

// An inspect that misses its deadline marks the container uninspected and keeps its stop intent; a container
// gone since the list loses it; any other inspect failure fails the host sample.
func TestSampleInspectFailureKinds(t *testing.T) {
	for _, tc := range []struct {
		name      string
		slow      int
		wantError bool
	}{{"deadline", http.StatusOK, false}, {"daemon error", http.StatusInternalServerError, true}} {
		t.Run(tc.name, func(t *testing.T) {
			db := historyDB(t)
			daemon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case strings.HasSuffix(r.URL.Path, "/_ping"):
					w.WriteHeader(200)
				case strings.HasSuffix(r.URL.Path, "/containers/json"):
					_ = json.NewEncoder(w).Encode([]any{map[string]any{"Id": "gone", "Names": []string{"/old"}, "State": "running"}, map[string]any{"Id": "slow", "Names": []string{"/db"}, "State": "exited"}, map[string]any{"Id": "kept", "Names": []string{"/app"}, "State": "exited"}})
				case strings.HasSuffix(r.URL.Path, "/gone/json"):
					http.Error(w, `{"message":"No such container: gone"}`, http.StatusNotFound)
				case strings.HasSuffix(r.URL.Path, "/slow/json"):
					if tc.slow != http.StatusOK {
						http.Error(w, `{"message":"daemon unavailable"}`, tc.slow)
						return
					}
					select {
					case <-r.Context().Done():
					case <-time.After(5 * time.Second):
					}
				case strings.HasSuffix(r.URL.Path, "/kept/json"):
					_ = json.NewEncoder(w).Encode(map[string]any{"Id": "kept", "Name": "/app", "Config": map[string]any{"Labels": map[string]string{}}, "State": map[string]any{"Status": "exited", "ExitCode": 0, "StartedAt": "2026-11-03T10:00:00Z", "FinishedAt": "2026-11-03T11:00:00Z"}})
				default:
					http.Error(w, "unexpected Docker route", 404)
				}
			}))
			t.Cleanup(daemon.Close)
			cli, err := client.New(client.WithHost(daemon.URL), client.WithAPIVersion("1.41"), client.WithHTTPClient(daemon.Client()))
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, cli.Close()) })
			setting, err := settings.NewSettingsService(t.Context(), db)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, setting.Stop(context.WithoutCancel(t.Context()))) })
			recorded := time.Date(2026, 11, 3, 10, 30, 0, 0, time.UTC)
			for _, id := range []string{"gone", "slow"} {
				require.NoError(t, db.Create(&runtimeintent.Row{ContainerID: id, ContainerName: id, RecordedAt: recorded}).Error)
			}
			producer := NewProducer(db, docker.NewDockerClientService(t.Context(), db, &config.Config{Environment: "test"}, setting).WithClient(cli), setting)
			producer.inspectTimeout = 200 * time.Millisecond
			producer.hostSample = func(context.Context) (dto.HostSample, error) { return fixtureSample().Host, nil }
			sample, err := producer.Sample(t.Context())
			if tc.wantError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, []string{"kept"}, []string{sample.Containers[0].ID})
			require.Len(t, sample.Containers, 1)
			require.Equal(t, []string{"slow"}, sample.Uninspected)
			var kept []string
			require.NoError(t, db.Model(&runtimeintent.Row{}).Order("container_id").Pluck("container_id", &kept).Error)
			require.Equal(t, []string{"slow"}, kept)
		})
	}
}
