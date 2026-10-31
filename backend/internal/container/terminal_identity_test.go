// Source: docklet-next/issues/312
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original credential and removal replacement-session identity evidence.

package container

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/containershell"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	wshub "github.com/florianfinn/docklet-next/backend/pkg/libarcane/ws"
	"github.com/florianfinn/docklet-next/backend/pkg/sessionlife"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	activityDTO "github.com/florianfinn/docklet-next/types/activity"
	"github.com/florianfinn/docklet-next/types/contract"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/florianfinn/docklet-next/types/version"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
)

type shellObservedActivity struct {
	*activity.ActivityService
	completed chan string
	mu        sync.Mutex
	counts    map[string]int
}

func (a *shellObservedActivity) CompleteActivity(ctx context.Context, id string, status activityDTO.Status, message string, errorMessage *string, step ...string) (*activityDTO.Activity, error) {
	a.mu.Lock()
	a.counts[id]++
	a.mu.Unlock()
	result, err := a.ActivityService.CompleteActivity(ctx, id, status, message, errorMessage, step...)
	a.completed <- id
	return result, err
}
func TestShellReplacementIdentityRealDirectAndEdge(t *testing.T) {
	for _, transport := range []string{"direct", "edge"} {
		for _, change := range []string{"credential", "remove-and-register"} {
			t.Run(transport+"/"+change, func(t *testing.T) {
				db, err := database.Initialize(t.Context(), sqlitestore.URL(filepath.Join(t.TempDir(), "shell.db")), database.MigrationOptions{})
				require.NoError(t, err)
				t.Cleanup(func() { _ = db.Close() })
				require.NoError(t, db.Exec("INSERT INTO api_keys(id,name,key_hash,key_prefix,kind) VALUES(?,?,?,?,?)", "example-key", "Example", "test-only", "example", "environment").Error)
				lateOutput, lateAttempted := make(chan struct{}), make(chan struct{})
				var lateOnce sync.Once
				var oldOutputStarted atomic.Bool
				t.Cleanup(func() {
					lateOnce.Do(func() { close(lateOutput) })
					if oldOutputStarted.Load() {
						<-lateAttempted
					}
				})
				var connectionNumber atomic.Int32
				var credential atomic.Value
				credential.Store("test-only")
				agent := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/api/app-version" {
						_ = json.MarshalWrite(w, version.Info{AgentContractVersion: contract.AgentHubVersion, AgentContractFingerprint: contract.AgentHubFingerprint})
						return
					}
					if r.Header.Get("X-Arcane-Agent-Token") != credential.Load().(string) && r.Header.Get("X-API-Key") != credential.Load().(string) {
						w.WriteHeader(401)
						return
					}
					if r.URL.Path == "/api/environments/0/system/health" {
						w.WriteHeader(204)
						return
					}
					conn, err := wshub.Accept(w, r, nil)
					if err != nil {
						return
					}
					defer conn.CloseNow()
					_ = conn.Write(r.Context(), websocket.MessageBinary, []byte("ready"))
					if connectionNumber.Add(1) == 1 {
						oldOutputStarted.Store(true)
						go func() {
							<-lateOutput
							writeCtx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), time.Second)
							defer cancel()
							_ = conn.Write(writeCtx, websocket.MessageBinary, []byte("stale-output"))
							close(lateAttempted)
						}()
					}
					for {
						kind, data, err := conn.Read(r.Context())
						if err != nil {
							return
						}
						if wshub.IsTerminalClose(kind, data) {
							code := 0
							wshub.WriteTerminalExit(r.Context(), conn, &code, "")
							_ = conn.Close(websocket.StatusNormalClosure, "")
							return
						}
						if conn.Write(r.Context(), kind, data) != nil {
							return
						}
					}
				})
				agentHTTP := httptest.NewServer(agent)
				t.Cleanup(agentHTTP.Close)
				registry := edge.NewTunnelRegistry()
				previous := edge.GetRegistry()
				edge.SetDefaultRegistry(registry)
				t.Cleanup(func() { edge.SetDefaultRegistry(previous) })
				endpoint := agentHTTP.URL
				envService := environment.NewEnvironmentService(db, http.DefaultClient, nil, nil, nil, nil)
				var reconnect func(string)
				if transport == "edge" {
					connected := make(chan struct{}, 4)
					server := edge.NewTunnelServerWithRegistry(registry, envService.ResolveEdgeEnvironmentByToken, func(_ context.Context, _ string, online bool) {
						if online {
							connected <- struct{}{}
						}
					})
					server.SetSessionKeyResolver(envService.ResolveEdgeSessionKey)
					router := echo.New()
					router.GET("/api/tunnel/connect", server.HandleConnect)
					manager := httptest.NewServer(router)
					t.Cleanup(manager.Close)
					address, err := url.Parse(agentHTTP.URL)
					require.NoError(t, err)
					var stops []context.CancelFunc
					var joins []chan struct{}
					defer func() {
						for _, stop := range stops {
							stop()
						}
						registry.Unregister("example-host")
						for _, joined := range joins {
							<-joined
						}
					}()
					reconnect = func(value string) {
						// A fresh agent process re-registers with the current credential.
						for _, stop := range stops {
							stop()
						}
						for _, joined := range joins {
							<-joined
						}
						stops = nil
						joins = nil
						life, cancel := context.WithCancel(t.Context())
						joined := make(chan struct{})
						stops = append(stops, cancel)
						joins = append(joins, joined)
						client := edge.NewTunnelClient(&edge.Config{EdgeTransport: edge.EdgeTransportWebSocket, ManagerApiUrl: manager.URL, AgentToken: value, Listen: address.Hostname(), Port: address.Port()}, agent)
						go func() { defer close(joined); client.StartWithErrorChan(life, nil) }()
						select {
						case <-connected:
						case <-time.After(5 * time.Second):
							t.Fatal("replacement Edge registration missing")
						}
					}
					endpoint = "edge://example-host"
				}
				createHost := func() {
					require.NoError(t, db.Create(&environment.Environment{ID: "example-host", Name: "Example", ApiUrl: endpoint, IsEdge: transport == "edge", Enabled: true, AccessToken: new(credential.Load().(string)), ApiKeyID: new("example-key")}).Error)
				}
				createHost()
				if reconnect != nil {
					reconnect(credential.Load().(string))
				}
				audit := &shellObservedActivity{ActivityService: activity.NewActivityService(db, nil), completed: make(chan string, 4), counts: map[string]int{}}
				sessions := containershell.New(audit)
				held, release := make(chan struct{}), make(chan struct{})
				var releaseOnce sync.Once
				defer releaseOnce.Do(func() { close(release) })
				var admissions, identities atomic.Int32
				hub := echo.New()
				hub.Use(middleware.NewEnvProxyMiddlewareWithParamAndRegistry("0", "id", func(ctx context.Context, id string) (string, *string, bool, error) {
					env, err := envService.GetEnvironmentByID(ctx, id)
					if err != nil {
						return "", nil, false, err
					}
					return env.ApiUrl, env.AccessToken, env.Enabled, nil
				}, func(_ context.Context, c *echo.Context) (*authz.PermissionSet, *user.Actor, bool) {
					identity := "example-new"
					if identities.Add(1) == 1 {
						identity = "example-old"
					}
					c.SetRequest(c.Request().WithContext(sessionlife.WithSession(c.Request().Context(), identity, "example-user", func(context.Context) error { return nil })))
					return authz.SudoPermissionSet(), &user.Actor{ID: "example-user", Username: "operator"}, true
				}, authz.NewPermissionMatcher(), registry, func(r *http.Request) bool { return r.Header.Get("Origin") == "https://hub.example.com" }, middleware.EnvironmentProxySecurity{ShellSessions: sessions, AgentAccess: func(ctx context.Context, id string, token *string, method, path string) (context.Context, context.CancelFunc, error) {
					ctx, cleanup, err := envService.AgentAccess(ctx, id, token, method, path)
					if err != nil {
						return nil, nil, err
					}
					admission := admissions.Add(1)
					return ctx, func() {
						cleanup()
						if admission == 1 {
							close(held)
							<-release
						}
					}, nil
				}}))
				hub.GET("/api/environments/:id/ws/containers/:containerId/terminal", func(c *echo.Context) error { return c.NoContent(418) })
				hubHTTP := httptest.NewServer(hub)
				t.Cleanup(func() { releaseOnce.Do(func() { close(release) }); hubHTTP.Close() })
				old := dialShellHub(t, hubHTTP)
				_, data := readShell(t, old)
				require.Equal(t, "ready", string(data))
				require.Equal(t, 1, sessions.ActiveCount())
				if change == "credential" {
					credential.Store("example")
					_, err = envService.UpdateEnvironment(t.Context(), "example-host", map[string]any{"access_token": "example"}, nil, nil)
					require.NoError(t, err)
				} else {
					require.NoError(t, envService.DeleteEnvironment(t.Context(), "example-host", nil, nil))
					createHost()
				}
				exit := readShellExit(t, old)
				require.Empty(t, exit.Reason)
				_, _, err = old.Read(t.Context())
				require.Contains(t, []websocket.StatusCode{websocket.StatusNormalClosure, websocket.StatusPolicyViolation}, websocket.CloseStatus(err))
				<-held
				if reconnect != nil {
					reconnect(credential.Load().(string))
				}
				replacement := dialShellHub(t, hubHTTP)
				_, data = readShell(t, replacement)
				require.Equal(t, "ready", string(data))
				require.Equal(t, 2, sessions.ActiveCount())
				lateOnce.Do(func() { close(lateOutput) })
				<-lateAttempted
				sessionlife.Revoke("example-old", "example-user", "")
				releaseOnce.Do(func() { close(release) })
				oldID := <-audit.completed
				require.Equal(t, 1, sessions.ActiveCount(), "delayed old cleanup removed the replacement")
				require.NoError(t, replacement.Write(t.Context(), websocket.MessageBinary, []byte("still-live")))
				_, data = readShell(t, replacement)
				require.Equal(t, "still-live", string(data))
				sessions.CloseByUser("example-user")
				exit = readShellExit(t, replacement)
				require.Empty(t, exit.Reason)
				_, _, err = replacement.Read(t.Context())
				require.Equal(t, websocket.StatusNormalClosure, websocket.CloseStatus(err))
				newID := <-audit.completed
				require.NotEqual(t, oldID, newID)
				require.Zero(t, sessions.ActiveCount())
				audit.mu.Lock()
				require.Equal(t, map[string]int{oldID: 1, newID: 1}, audit.counts)
				audit.mu.Unlock()
				var entries []activity.Activity
				require.NoError(t, db.Order("started_at").Find(&entries).Error)
				require.Len(t, entries, 2)
				for _, entry := range entries {
					require.NotNil(t, entry.EndedAt)
					require.Equal(t, "example-user", *entry.StartedByUserID)
					require.Equal(t, "example-host", entry.EnvironmentID)
					require.Equal(t, "example", *entry.ResourceID)
					require.Empty(t, entry.Output)
					require.Equal(t, "Shell ended", entry.LatestMessage)
					require.NotContains(t, fmt.Sprint(entry.Metadata), "still-live")
				}
			})
		}
	}
}
