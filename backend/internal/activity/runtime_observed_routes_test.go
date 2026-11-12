// Source: docklet-next/issues/440
// Revision: 6e3afa6a78f4f60ffd74858d9a1eaefbb1b73bda
// Retrieved: 2026-11-12
// License: Apache-2.0
// Deviations: Original restore-route automatic recovery rejection before preflight.

package activity_test

import (
	"context"
	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humaecho"
	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestObservedRestoreRouteRefusesAutoRestore(t *testing.T) {
	f := newRemoteIntegration(t, false)
	router := echo.New()
	router.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			ctx := context.WithValue(c.Request().Context(), middleware.ContextKeyUserPermissions, authz.SudoPermissionSet())
			ctx = context.WithValue(ctx, user.CurrentUserContextKey{}, &user.Actor{ID: "operator", Username: "operator"})
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	})
	api := humaecho.NewWithGroup(router, router.Group("/api"), huma.DefaultConfig("recovery", "test"))
	activity.RegisterImageLifecycle(api, f.hubActivity)
	req := httptest.NewRequest(http.MethodPost, "/api/environments/example-remote/updater/project/example-project/restore-points/"+uuid.NewString()+"/restore", strings.NewReader(`{"expectedRevision":"`+strings.Repeat("a", 64)+`","autoRestore":true}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.NewString())
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	require.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
	require.Contains(t, response.Body.String(), "autoRestore")
	require.Zero(t, f.executeCalls.Load())
}

func TestObservedContainerPublicBodyWithoutManifest(t *testing.T) {
	daemon, registry := newContainerImageHTTP(t)
	hub := activationApp(t, false, "", activationSetup{BeforeStart: func(f *runtimeServicesFixture, _ *activity.RuntimeHTTPService, _ *activity.RuntimeReceiver) {
		configureContainerImageFixture(t, f, daemon, registry)
		f.containers.ConfigureOperations(f.db.DB, f.store.RegistrationID, true, nil, nil)
	}})
	api := humaecho.NewWithGroup(hub.router, hub.router.Group("/api"), huma.DefaultConfig("Observed update", "18"))
	api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
		ctx = huma.WithValue(ctx, middleware.ContextKeyUserPermissions, authz.SudoPermissionSet())
		ctx = huma.WithValue(ctx, user.CurrentUserContextKey{}, &user.Actor{ID: "example-owner", Username: "Operator"})
		next(ctx)
	})
	activity.RegisterRuntimeLifecycle(api, hub.services.activity, activity.RuntimeUpdateContainer, huma.Operation{OperationID: string(activity.RuntimeUpdateContainer), Method: http.MethodPost, Path: "/environments/{id}/containers/{containerId}/update"})
	status, body := activationCall(t, hub, "/api/environments/0/containers/"+runtimeContainerID+"/update", uuid.NewString(), "respond-async", []byte(`{"autoRestore":true}`))
	require.Equal(t, http.StatusAccepted, status, string(body))
	run := activationRunID(t, body)
	snapshot, err := hub.services.waitLocal(run)
	require.NoError(t, err)
	require.True(t, snapshot.ImageLifecycle.Verified)
	var rows []activity.RuntimeObservedRun
	require.NoError(t, hub.services.db.Where("run_id = ?", run).Find(&rows).Error)
	require.Len(t, rows, 1)
	require.Equal(t, "manual", rows[0].Mode)
}
