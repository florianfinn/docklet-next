// Source: docklet-next/issues/341
// Revision: 89f69980c01d08714ce96c49365fb19065e3d6ba
// Retrieved: 2026-11-04
// License: Apache-2.0
// Deviations: Original real image producer, Direct sender/receiver and Edge command-path notification proofs.

package updater

import (
	"context"
	"crypto/sha256"
	"fmt"
	"github.com/florianfinn/docklet-next/backend/internal/auth"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/internal/role"
	"github.com/florianfinn/docklet-next/backend/internal/user"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humaecho"
	"github.com/florianfinn/docklet-next/backend/internal/apikey"
	containers "github.com/florianfinn/docklet-next/backend/internal/container"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/internal/imageupdate"
	"github.com/florianfinn/docklet-next/backend/internal/notification"
	"github.com/florianfinn/docklet-next/backend/internal/registry"
	policy "github.com/florianfinn/docklet-next/backend/internal/updater/children/policies"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/notifications"
	"github.com/florianfinn/docklet-next/types/base"
	"github.com/florianfinn/docklet-next/types/contract"
	projectDTO "github.com/florianfinn/docklet-next/types/project"
	versionDTO "github.com/florianfinn/docklet-next/types/version"
	"github.com/google/go-containerregistry/pkg/name"
	registryHTTP "github.com/google/go-containerregistry/pkg/registry"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
)

func TestDefaultChecksAndHubNotificationsDirectEdge(t *testing.T) {
	for _, isEdge := range []bool{false, true} {
		t.Run(map[bool]string{false: "direct", true: "edge"}[isEdge], func(t *testing.T) {
			agent := newPolicyFixture(t)
			hub := newPolicyFixture(t)
			registryServer := httptest.NewServer(registryHTTP.New(registryHTTP.Logger(log.New(io.Discard, "", 0))))
			t.Cleanup(registryServer.Close)
			repository := strings.TrimPrefix(registryServer.URL, "http://") + "/app"
			makeImage := func(version string) v1.Image {
				img, e := mutate.ConfigFile(empty.Image, &v1.ConfigFile{OS: "linux", Architecture: "amd64", Config: v1.Config{Labels: map[string]string{"org.opencontainers.image.version": version}}})
				require.NoError(t, e)
				return img
			}
			before, after := makeImage("1.0.0"), makeImage("1.1.0")
			configID, e := before.ConfigName()
			require.NoError(t, e)
			manifestID, e := before.Digest()
			require.NoError(t, e)
			ref := repository + ":1.0.0"
			tag, e := name.NewTag(ref, name.Insecure)
			require.NoError(t, e)
			require.NoError(t, remote.Write(tag, after, remote.WithContext(t.Context())))
			var effects, executorSends atomic.Int32
			daemon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					effects.Add(1)
					w.WriteHeader(500)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				switch {
				case strings.HasSuffix(r.URL.Path, "/info"):
					_, _ = w.Write([]byte(`{"OSType":"linux","Architecture":"amd64"}`))
				case strings.HasSuffix(r.URL.Path, "/containers/json"):
					_, _ = fmt.Fprintf(w, `[{"Id":"app-a","Names":["/app-a"],"Image":%q,"ImageID":%q,"State":"running","Labels":{}},{"Id":"app-b","Names":["/app-b"],"Image":%q,"ImageID":%q,"State":"running","Labels":{}}]`, ref, configID.String(), ref, configID.String())
				case strings.Contains(r.URL.Path, "/images/"):
					_, _ = fmt.Fprintf(w, `{"Id":%q,"RepoDigests":[%q],"Os":"linux","Architecture":"amd64","Config":{"Labels":{"org.opencontainers.image.version":"1.0.0"}}}`, configID.String(), repository+"@"+manifestID.String())
				case strings.Contains(r.URL.Path, "/containers/"):
					parts := strings.Split(r.URL.Path, "/")
					id := parts[len(parts)-2]
					_, _ = fmt.Fprintf(w, `{"Id":%q,"Name":%q,"Image":%q,"Config":{"Image":%q,"Labels":{}}}`, id, "/"+id, configID.String(), ref)
				default:
					http.NotFound(w, r)
				}
			}))
			t.Cleanup(daemon.Close)
			dock := docker.NewDockerClientService(t.Context(), nil, nil, nil).WithClient(newTestDockerClientInternal(t, daemon))
			require.NoError(t, agent.db.Create(&registry.ContainerRegistry{URL: registryServer.URL, Enabled: true, Insecure: true, RegistryType: "generic"}).Error)
			reg := registry.NewContainerRegistryService(agent.db, nil, nil, nil, registryServer.Client())
			sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { executorSends.Add(1); w.WriteHeader(200) }))
			t.Cleanup(sink.Close)
			require.NoError(t, agent.db.Create(&notification.NotificationSettings{Provider: notifications.NotificationProviderGeneric, Enabled: true, Config: database.JSON{"webhookUrl": sink.URL, "method": "POST", "contentType": "application/json"}}).Error)
			notify := notification.NewNotificationService(agent.db, nil, nil, nil)
			checks := imageupdate.NewImageUpdateService(agent.db, agent.s.deps.Settings, reg, dock, nil, notify, nil)
			inventory := containers.NewContainerService(nil, dock, nil, nil, nil)
			router := echo.New()
			apiConfig := huma.DefaultConfig("agent", "v14")
			apiConfig.Components.Schemas = huma.NewMapRegistry("#/components/schemas/", func(t reflect.Type, hint string) string {
				sum := sha256.Sum256([]byte(t.String()))
				return fmt.Sprintf("%s%x", huma.DefaultSchemaNamer(t, hint), sum[:4])
			})
			api := humaecho.NewWithGroup(router, router.Group("/api"), apiConfig)
			agentConfig := &config.Config{AgentMode: true}
			api.UseMiddleware(auth.NewHumaMiddleware(api, &auth.AuthService{}, nil, nil, nil, agentConfig))
			app := handlerutil.NewActivityAppContext(t.Context())
			imageupdate.RegisterImageUpdates(api, checks, nil, app)
			containers.New(inventory, nil, nil, nil).RegisterRoutes(api, app)
			router.HEAD("/api/environments/0/system/health", func(c *echo.Context) error { return c.NoContent(200) })
			router.GET("/api/app-version", func(c *echo.Context) error {
				return c.JSON(200, versionDTO.Info{AgentContractVersion: contract.AgentHubVersion, AgentContractFingerprint: contract.AgentHubFingerprint})
			})
			router.GET("/api/environments/0/projects", func(c *echo.Context) error {
				return c.JSON(200, base.Paginated[projectDTO.Details]{Success: true, Data: []projectDTO.Details{{ID: "stack", Name: "Stack"}}, Pagination: base.PaginationResponse{CurrentPage: 1, TotalPages: 1, TotalItems: 1}})
			})
			router.POST("/api/environments/0/updater/projects/stack/check", func(c *echo.Context) error {
				result, e := checks.ReadUpdateInfo(c.Request().Context(), ref)
				if e != nil {
					return e
				}
				return c.JSON(200, base.ApiResponse[*projectDTO.UpdateInfo]{Success: true, Data: &projectDTO.UpdateInfo{Status: "has_update", HasUpdate: result.HasUpdate, ServiceUpdates: map[string]projectDTO.ServiceUpdateInfo{"app": {ImageRef: ref, UpdateInfo: imageCheckInfo(result)}}}})
			})
			endpoint := httptest.NewServer(router)
			t.Cleanup(endpoint.Close)
			env := "remote-example"
			require.NoError(t, hub.db.Create(&environment.Environment{BaseModel: database.BaseModel{ID: env}, Name: "Remote", ApiUrl: endpoint.URL, Enabled: true}).Error)
			keys := apikey.NewApiKeyService(hub.db, user.NewUserService(hub.db, role.NewRoleService(hub.db), nil), role.NewRoleService(hub.db))
			key, e := keys.CreateEnvironmentApiKey(t.Context(), env)
			require.NoError(t, e)
			token := key.Key
			agentConfig.AgentToken = token
			tunnels := edge.NewTunnelRegistry()
			prior := edge.GetRegistry()
			edge.SetDefaultRegistry(tunnels)
			t.Cleanup(func() { edge.SetDefaultRegistry(prior) })
			address := endpoint.URL
			if isEdge {
				connected := make(chan struct{})
				var once sync.Once
				server := edge.NewTunnelServerWithRegistry(tunnels, func(context.Context, string) (string, error) { return env, nil }, func(_ context.Context, _ string, online bool) {
					if online {
						once.Do(func() { close(connected) })
					}
				})
				server.SetSessionKeyResolver(func(context.Context, string, string) (string, error) { return key.ID, nil })
				managerRouter := echo.New()
				managerRouter.GET("/api/tunnel/connect", server.HandleConnect)
				manager := httptest.NewServer(managerRouter)
				t.Cleanup(manager.Close)
				life, cancel := context.WithCancel(t.Context())
				done := make(chan struct{})
				client := edge.NewRuntimeTunnelClient(&edge.Config{EdgeTransport: edge.EdgeTransportWebSocket, ManagerApiUrl: manager.URL, AgentToken: token, EdgeReconnectInterval: 1}, router)
				go func() { defer close(done); client.StartWithErrorChan(life, nil) }()
				t.Cleanup(func() {
					cancel()
					tunnels.Unregister(env)
					select {
					case <-done:
					case <-time.After(5 * time.Second):
						t.Error("Edge client did not stop")
					}
				})
				select {
				case <-connected:
				case <-time.After(5 * time.Second):
					t.Fatal("Edge connection missing")
				}
				address = "edge://example"
			}
			require.NoError(t, hub.db.Model(&environment.Environment{}).Where("id=?", env).Updates(map[string]any{"api_url": address, "is_edge": isEdge, "access_token": token, "api_key_id": key.ID}).Error)
			require.NoError(t, hub.db.Exec("UPDATE environments SET enabled=0 WHERE id='0'").Error)
			configure := func() *environment.EnvironmentService {
				keys = apikey.NewApiKeyService(hub.db, user.NewUserService(hub.db, role.NewRoleService(hub.db), nil), role.NewRoleService(hub.db))
				envs := environment.NewEnvironmentService(hub.db, endpoint.Client(), nil, nil, hub.s.deps.Settings, keys)
				hub.p.ports.targets = hub.s.policyTargets(envs, inventory)
				hub.p.ports.checkContainer = hub.s.policyContainerCheck(envs)
				hub.p.ports.check = func(ctx context.Context, env, target string) (*projectDTO.UpdateInfo, error) {
					var response base.ApiResponse[*projectDTO.UpdateInfo]
					e := envs.ProxyJSONRequest(ctx, env, http.MethodPost, "/api/environments/0/updater/projects/"+target+"/check", nil, &response)
					return response.Data, e
				}
				return envs
			}
			envs := configure()
			_, inventoryError := hub.p.ports.targets(t.Context(), env)
			require.NoError(t, inventoryError)
			require.NoError(t, hub.s.deps.Settings.UpdateSetting(t.Context(), "updateNotify", "false"))
			outcome, e := hub.s.RunUpdateCheck(t.Context())
			require.NoError(t, e)
			require.Len(t, outcome.Targets, 3)
			require.Equal(t, "succeeded", string(outcome.Status))
			require.Zero(t, hub.actors)
			var n int64
			require.NoError(t, hub.db.Model(&policy.Policy{}).Count(&n).Error)
			require.EqualValues(t, 3, n)
			require.NoError(t, hub.db.Model(&policy.Notice{}).Count(&n).Error)
			require.Zero(t, n)
			require.NoError(t, hub.s.deps.Settings.UpdateSetting(t.Context(), "updateNotify", "true"))
			manual := imageupdate.NewImageUpdateService(nil, nil, nil, nil, nil, nil, nil)
			manual.ConfigureHubReads(envs.ProxyJSONRequest, hub.s.observeManualImages)
			hubRouter := echo.New()
			hubAPI := humaecho.NewWithGroup(hubRouter, hubRouter.Group("/api"), huma.DefaultConfig("hub", "v14"))
			hubAPI.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
				next(huma.WithContext(ctx, context.WithValue(ctx.Context(), middleware.ContextKeyUserPermissions, authz.SudoPermissionSet())))
			})
			imageupdate.RegisterImageUpdates(hubAPI, manual, nil, app)
			// Manual image checks run before any notifying scheduled check, so only the real
			// hub read hook can produce the two container entries; repeats stay deduplicated.
			for i := 0; i < 2; i++ {
				request := httptest.NewRequest(http.MethodGet, "/api/environments/"+env+"/image-updates/check?imageRef="+url.QueryEscape(ref), nil)
				w := httptest.NewRecorder()
				hubRouter.ServeHTTP(w, request)
				require.Equal(t, 200, w.Code, w.Body.String())
				require.Equal(t, 2, hub.notifications)
			}
			_, e = hub.s.RunUpdateCheck(t.Context())
			require.NoError(t, e)
			require.Equal(t, 3, hub.notifications)
			hub.reopen(t)
			configure()
			_, e = hub.s.RunUpdateCheck(t.Context())
			require.NoError(t, e)
			require.Equal(t, 3, hub.notifications)
			require.Zero(t, executorSends.Load())
			require.Zero(t, effects.Load())
			policies, e := hub.s.ListImagePolicies(t.Context(), env)
			require.NoError(t, e)
			require.Len(t, policies, 3)
		})
	}
}
