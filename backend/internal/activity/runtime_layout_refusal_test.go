// Source: docklet-next/issues/406
// Revision: 6e3afa6a78f4f60ffd74858d9a1eaefbb1b73bda
// Retrieved: 2026-11-12
// License: Apache-2.0
// Deviations: Original hub-local, Direct and Edge proof of the nested COMPOSE_FILE update refusal.

package activity_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humaecho"
	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	activityDTO "github.com/florianfinn/docklet-next/types/activity"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
)

// The stack update route answers the typed unsupported-layout refusal for a nested
// COMPOSE_FILE base before admission, whether the target resolves on the hub or on
// a Direct or Edge agent.
func TestNestedComposeFileUpdateRefusedBeforeAdmission(t *testing.T) {
	for _, mode := range []string{"local", "direct", "edge"} {
		t.Run(mode, func(t *testing.T) {
			f := newRemoteIntegration(t, mode == "edge")
			target := activity.RuntimeTarget{Kind: "stack", ID: "example-project"}
			path := f.agent.project(target.ID, target.ID, "services:\n  app:\n    image: registry.example.com/app:1\n")
			require.NoError(t, os.Mkdir(filepath.Join(path, "deploy"), 0755))
			require.NoError(t, os.WriteFile(filepath.Join(path, "deploy", "compose.yaml"), []byte("services:\n  app:\n    image: registry.example.com/app:1\n"), 0644))
			require.NoError(t, os.WriteFile(filepath.Join(path, ".env"), []byte("COMPOSE_FILE=deploy/compose.yaml\n"), 0644))
			prepare := func(ctx context.Context, _ string, target activity.RuntimeTarget) (activity.RuntimeResolvedTarget, error) {
				if mode == "local" {
					return f.agent.adapter.PrepareRuntimeTarget(ctx, target)
				}
				meta, err := f.sender.Resolve(ctx, activityDTO.RuntimeTargetRequest{Registration: activityDTO.RuntimeExecutorIdentity{ManagerID: f.store.ManagerID, RegistrationID: f.store.RegistrationID, ExecutorGeneration: f.store.ExecutorGeneration}, Target: target})
				return activity.RuntimeResolvedTarget{Target: meta.Target}, err
			}
			lifecycleCalls := 0
			boundary, err := activity.NewRuntimeHTTPService(f.hub, activity.RuntimeHTTPPorts{ApplicationContext: t.Context(), Store: func(context.Context, string) (activity.RuntimeStoreBinding, error) { return f.store, nil }, PrepareTarget: prepare,
				PrepareLifecycle: func(context.Context, string, activity.RuntimeTarget, activity.RuntimeStoreBinding, dto.LifecycleRequest) (*dto.LifecycleIntent, error) {
					lifecycleCalls++
					return nil, imageDTO.Failure("update.snapshot-unavailable", 409)
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
			req := httptest.NewRequest(http.MethodPost, "/api/environments/example-remote/updater/projects/example-project/update", strings.NewReader(`{"expectedRevision":"`+strings.Repeat("a", 64)+`"}`))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Idempotency-Key", uuid.NewString())
			req.Header.Set("Prefer", "respond-async")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			require.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
			var body imageDTO.OperationError
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
			require.Equal(t, "update.image-binding-unsupported", body.Problem.Code)
			require.Zero(t, lifecycleCalls)
			require.Zero(t, f.executeCalls.Load())
			var count int64
			require.NoError(t, f.db.Table("activities").Count(&count).Error)
			require.Zero(t, count, "refusal must not create an admitted run")
		})
	}
}
