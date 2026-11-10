// Source: docklet-next/issues/364
// Revision: e683d402f358e87b8eef5b81a71b530fd70cc0fe
// Retrieved: 2026-11-10
// License: Apache-2.0
// Deviations: Original single-run route authorization across actual local, Direct and Edge paths.

package activity_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/access"
	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/apikey"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/session"
	"github.com/florianfinn/docklet-next/backend/internal/user"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	apikeytypes "github.com/florianfinn/docklet-next/types/apikey"
	"github.com/florianfinn/docklet-next/types/base"
	projecttypes "github.com/florianfinn/docklet-next/types/project"
	usertypes "github.com/florianfinn/docklet-next/types/user"
	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
)

func TestSingleActivityReadOwnerAndTargetRealRoutes(t *testing.T) {
	for _, transport := range []string{"local", "direct", "edge"} {
		t.Run(transport, func(t *testing.T) {
			var hub *activationFixture
			controller := &access.Controller{Runtime: func(ctx context.Context, env, id string) ([]access.Container, error) {
				var details projecttypes.Details
				var err error
				if env == "0" {
					details, err = hub.services.projects.GetProjectDetails(ctx, id, projecttypes.DetailsOptions{IncludeRuntimeServices: true})
				} else {
					var response base.ApiResponse[projecttypes.Details]
					err = hub.env.ProxyJSONRequest(ctx, env, http.MethodGet, "/api/environments/0/projects/"+url.PathEscape(id)+"/runtime", nil, &response)
					details = response.Data
				}
				if err != nil {
					return nil, err
				}
				out := []access.Container{}
				for _, item := range details.RuntimeServices {
					out = append(out, access.Container{ID: item.ContainerID, Names: []string{item.ContainerName}})
				}
				return out, nil
			}}
			hub = activationApp(t, false, "", activationSetup{Configure: func(cfg *config.Config) { cfg.AuthMode = "local" }, BeforeRouter: func(router *echo.Echo) {
				router.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
					return func(c *echo.Context) error {
						c.SetRequest(c.Request().WithContext(access.WithController(c.Request().Context(), controller)))
						return next(c)
					}
				})
			}})
			var targetReads atomic.Int32
			executor, env := hub, "0"
			if transport != "local" {
				executor = activationApp(t, true, uuid.NewString(), activationSetup{BeforeRouter: func(router *echo.Echo) {
					router.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
						return func(c *echo.Context) error {
							if c.Request().URL.Path == "/api/environments/0/runtime-target" {
								targetReads.Add(1)
							}
							return next(c)
						}
					})
				}})
				env = activationRemote(t, hub, executor, transport)
				_, err := hub.env.CheckAgentCompatibility(t.Context(), env)
				require.NoError(t, err)
			}
			suffix, body := activationOperation(executor.services, activity.RuntimeRestartProject)
			roles := hub.roles
			users := user.NewUserService(hub.services.db, roles, session.RevokeAllUserSessionsExceptInDB)
			keys := apikey.NewApiKeyService(hub.services.db, users, roles)
			token := func(name, level, project string) (string, string) {
				u := &user.User{Username: name}
				_, err := users.CreateUser(t.Context(), u)
				require.NoError(t, err)
				if level != "" {
					require.NoError(t, users.SetAppAssignments(t.Context(), u.ID, []usertypes.AppAssignment{{EnvironmentID: env, ProjectID: project, Level: level}}))
				}
				key, err := keys.CreatePersonalApiKey(t.Context(), u.ID, apikeytypes.CreateUserApiKey{Name: name})
				require.NoError(t, err)
				return key.Key, u.ID
			}
			owner, ownerID := token("owner", "full", "example-project")
			assigned, _ := token("assigned", "restricted", "example-project")
			unassigned, unassignedID := token("unassigned", "full", "other-project")
			stranger, _ := token("stranger", "", "")
			call := func(method, path, key string, data []byte) (int, string) {
				req, err := http.NewRequestWithContext(t.Context(), method, hub.server.URL+"/api/environments/"+env+path, bytes.NewReader(data))
				require.NoError(t, err)
				req.Header.Set("X-API-Key", key)
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Idempotency-Key", uuid.NewString())
				req.Header.Set("Prefer", "respond-async")
				resp, err := hub.server.Client().Do(req)
				require.NoError(t, err)
				defer resp.Body.Close()
				payload, err := io.ReadAll(resp.Body)
				require.NoError(t, err)
				return resp.StatusCode, string(payload)
			}
			status, payload := call(http.MethodPost, suffix, owner, body)
			require.Equal(t, http.StatusAccepted, status, payload)
			var admitted struct {
				Data activitytypes.RuntimeAdmission `json:"data"`
			}
			require.NoError(t, json.Unmarshal([]byte(payload), &admitted))
			run := admitted.Data.ActivityID
			require.NotEmpty(t, run)
			path := "/activities/" + run
			t.Run("owner", func(t *testing.T) {
				// The persisted actor keeps access even after all assignments are removed.
				require.NoError(t, users.SetAppAssignments(t.Context(), ownerID, nil))
				status, payload := call(http.MethodGet, path, owner, nil)
				require.Equal(t, http.StatusOK, status, payload)
				require.Contains(t, payload, run)
			})
			t.Run("assigned-target", func(t *testing.T) {
				before := targetReads.Load()
				status, payload := call(http.MethodGet, path, assigned, nil)
				require.Equal(t, http.StatusOK, status, payload)
				if transport != "local" {
					require.Greater(t, targetReads.Load(), before, "read must cross the real target resolver transport")
				}
			})
			for name, key := range map[string]string{"other-target": unassigned, "no-rights": stranger} {
				t.Run(name, func(t *testing.T) {
					status, payload := call(http.MethodGet, path, key, nil)
					require.Contains(t, []int{403, 404}, status, payload)
					require.NotContains(t, payload, run)
				})
			}

			t.Run("denials-hide-run-existence", func(t *testing.T) {
				knownStatus, knownBody := call(http.MethodGet, path, unassigned, nil)
				absentStatus, absentBody := call(http.MethodGet, "/activities/"+uuid.NewString(), unassigned, nil)
				require.Equal(t, knownStatus, absentStatus, "a denied reader must not distinguish a persisted run from a missing one")
				require.Equal(t, knownBody, absentBody, "denial bodies must not disclose receipt presence")
			})
			t.Run("projection-cannot-grant-access", func(t *testing.T) {
				// Display projections are not the immutable actor/target authority.
				var stored activity.Activity
				require.NoError(t, hub.services.db.First(&stored, "id = ?", run).Error)
				require.NoError(t, hub.services.db.Model(&activity.Activity{}).Where("id = ?", run).Updates(map[string]any{"started_by_user_id": unassignedID, "resource_id": "other-project"}).Error)
				defer func() {
					require.NoError(t, hub.services.db.Model(&activity.Activity{}).Where("id = ?", run).Updates(map[string]any{"started_by_user_id": stored.StartedByUserID, "resource_id": stored.ResourceID}).Error)
				}()
				status, payload := call(http.MethodGet, path, unassigned, nil)
				require.Contains(t, []int{403, 404}, status, payload)
				status, payload = call(http.MethodGet, path, assigned, nil)
				require.Equal(t, http.StatusOK, status, payload)
			})
			t.Run("missing-run", func(t *testing.T) {
				status, payload := call(http.MethodGet, "/activities/"+uuid.NewString(), assigned, nil)
				require.Contains(t, []int{403, 404}, status, payload)
			})
			t.Run("lists-stay-denied", func(t *testing.T) {
				status, payload := call(http.MethodGet, "/activities?search=owner", assigned, nil)
				require.Equal(t, http.StatusForbidden, status, payload)
			})
			t.Run("restricted-action-stays-denied", func(t *testing.T) {
				status, payload := call(http.MethodPost, "/projects/example-project/redeploy", assigned, nil)
				require.Equal(t, http.StatusForbidden, status, payload)
			})

			for _, operation := range []struct{ name, method, path string }{
				{"cancel-stays-denied", http.MethodPost, path + "/cancel"},
				{"release-stays-denied", http.MethodPost, "/runtime-runs/" + run + "/release"},
				{"retention-stays-denied", http.MethodDelete, "/activities/history"},
			} {
				t.Run(operation.name, func(t *testing.T) {
					status, payload := call(operation.method, operation.path, assigned, nil)
					require.Equal(t, http.StatusForbidden, status, payload)
				})
			}
			t.Run("generic-stays-denied", func(t *testing.T) {
				generic, err := hub.services.activity.StartActivity(t.Context(), activity.StartActivityRequest{EnvironmentID: env, Type: activitytypes.TypeImagePull, Queue: true, DeferSlot: true})
				require.NoError(t, err)
				status, payload := call(http.MethodGet, "/activities/"+generic.ID, assigned, nil)
				require.Equal(t, http.StatusNotFound, status, payload)
			})

			t.Run("container-target", func(t *testing.T) {
				// Synchronize with the real run's settlement before admitting a peer operation.
				require.Eventually(t, func() bool {
					ref, found, err := hub.services.runtime.Reference(t.Context(), env, run)
					if err != nil || !found {
						return false
					}
					detail, err := hub.services.runtime.GetRuntimeHubDetail(t.Context(), ref)
					return err == nil && detail.Activity.Status == activitytypes.StatusSuccess
				}, 10*time.Second, 10*time.Millisecond)
				require.NoError(t, users.SetAppAssignments(t.Context(), ownerID, []usertypes.AppAssignment{{EnvironmentID: env, ProjectID: "example-project", Level: "full"}}))
				status, payload := call(http.MethodPost, "/containers/"+runtimeContainerID+"/restart", owner, nil)
				require.Equal(t, http.StatusAccepted, status, payload)
				var child struct {
					Data activitytypes.RuntimeAdmission `json:"data"`
				}
				require.NoError(t, json.Unmarshal([]byte(payload), &child))
				status, payload = call(http.MethodGet, "/activities/"+child.Data.ActivityID, assigned, nil)
				require.Equal(t, http.StatusOK, status, payload)
				status, payload = call(http.MethodGet, "/activities/"+child.Data.ActivityID, unassigned, nil)
				require.Contains(t, []int{403, 404}, status, payload)
			})
			t.Run("deleted-target", func(t *testing.T) {
				require.NoError(t, executor.services.db.Exec("DELETE FROM projects WHERE id = ?", "example-project").Error)
				status, payload := call(http.MethodGet, path, assigned, nil)
				require.Contains(t, []int{403, 404}, status, payload)
			})
		})
	}
}
