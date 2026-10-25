// Source: docklet-next/docs/design/images-updates.md
// Revision: 48915fde5d95ed00c757116410f76617e23e8a50
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Actual Direct/Edge typed preflight refusals through the registered hub admission HTTP route.

package activity_test

import (
	"context"
	"encoding/json"
	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humaecho"
	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/internal/project"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	activityDTO "github.com/florianfinn/docklet-next/types/activity"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type lifecycleRefusalProjects struct {
	*project.ProjectService
	code string
}

func (p *lifecycleRefusalProjects) PrepareImageLifecycle(context.Context, string, string, dto.LifecycleRequest) (*dto.LifecycleIntent, error) {
	problem := imageDTO.Failure(p.code, 409)
	problem.Problem.Message = "synthetic-private agent diagnostic"
	return nil, problem
}

func TestRuntimeImageTypedRefusalThroughDirectAndEdgeHTTP(t *testing.T) {
	for _, isEdge := range []bool{false, true} {
		mode := "direct"
		if isEdge {
			mode = "edge"
		}
		for _, code := range []string{"update.config-drift", "restore.shared-reference"} {
			t.Run(mode+"/"+code, func(t *testing.T) {
				f := newRemoteIntegration(t, isEdge, func(agent *runtimeServicesFixture) {
					agent.adapter = activity.NewRuntimeServiceAdapter(&lifecycleRefusalProjects{ProjectService: agent.projects, code: code}, agent.containers)
				})
				target := activity.RuntimeTarget{Kind: "stack", ID: "example-project"}
				f.agent.project(target.ID, target.ID, "services:\n  app:\n    image: registry.example.com/app:1\n")
				boundary, err := activity.NewRuntimeHTTPService(f.hub, activity.RuntimeHTTPPorts{ApplicationContext: t.Context(), Store: func(context.Context, string) (activity.RuntimeStoreBinding, error) { return f.store, nil }, PrepareTarget: func(context.Context, string, activity.RuntimeTarget) (activity.RuntimeResolvedTarget, error) {
					return activity.RuntimeResolvedTarget{Target: target}, nil
				}, PrepareLifecycle: func(ctx context.Context, _ string, target activity.RuntimeTarget, store activity.RuntimeStoreBinding, request dto.LifecycleRequest) (*dto.LifecycleIntent, error) {
					meta, err := f.sender.Resolve(ctx, activityDTO.RuntimeTargetRequest{Registration: activityDTO.RuntimeExecutorIdentity{ManagerID: store.ManagerID, RegistrationID: store.RegistrationID, ExecutorGeneration: store.ExecutorGeneration}, Target: target, Lifecycle: &request})
					return meta.Lifecycle, err
				}})
				require.NoError(t, err)
				boundary.SetReady(true)
				router := echo.New()
				router.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
					return func(c *echo.Context) error {
						ctx := context.WithValue(c.Request().Context(), middleware.ContextKeyUserPermissions, authz.SudoPermissionSet())
						ctx = context.WithValue(ctx, user.CurrentUserContextKey{}, &user.Actor{ID: "operator", Username: "operator"})
						c.SetRequest(c.Request().WithContext(ctx))
						return next(c)
					}
				})
				api := humaecho.NewWithGroup(router, router.Group("/api"), huma.DefaultConfig("lifecycle", "test"))
				activity.RegisterImageLifecycle(api, f.hubActivity)
				path := "/api/environments/example-remote/updater/projects/example-project/update"
				if code == "restore.shared-reference" {
					path = "/api/environments/example-remote/updater/project/example-project/restore-points/" + uuid.NewString() + "/restore"
				}
				req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"expectedRevision":"`+strings.Repeat("a", 64)+`"}`))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Idempotency-Key", uuid.NewString())
				req.Header.Set("Prefer", "respond-async")
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, req)
				require.Zero(t, f.executeCalls.Load(), "refusal must precede runtime execution")
				require.Equal(t, 409, rec.Code, rec.Body.String())
				var body imageDTO.OperationError
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
				require.Equal(t, code, body.Problem.Code)
				require.NotContains(t, rec.Body.String(), "synthetic-private")
				var count int64
				require.NoError(t, f.db.Table("activities").Count(&count).Error)
				require.Zero(t, count, "preflight refusal must not create an admitted run")
			})
		}
	}
}
