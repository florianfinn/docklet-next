// Source: docklet-next/issues/342
// Revision: 89f69980c01d08714ce96c49365fb19065e3d6ba
// Retrieved: 2026-11-05
// License: Apache-2.0
// Deviations: Original public durable update/restore route, header and removed-ID retry proof.

package activity_test

import (
	"net/http"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humaecho"
	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestContainerImagePublicDurableRoutes(t *testing.T) {
	daemon, registry := newContainerImageHTTP(t)
	hub := activationApp(t, false, "", activationSetup{BeforeStart: func(f *runtimeServicesFixture, _ *activity.RuntimeHTTPService, _ *activity.RuntimeReceiver) {
		configureContainerImageFixture(t, f, daemon, registry)
		f.containers.ConfigureOperations(f.db.DB, f.store.RegistrationID, true, nil, nil)
	}})
	api := humaecho.NewWithGroup(hub.router, hub.router.Group("/api"), huma.DefaultConfig("Container image routes", "v14"))
	api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
		ctx = huma.WithValue(ctx, middleware.ContextKeyUserPermissions, authz.SudoPermissionSet())
		ctx = huma.WithValue(ctx, user.CurrentUserContextKey{}, &user.Actor{ID: "example-owner", Username: "Operator"})
		next(ctx)
	})
	for _, spec := range []struct {
		operation activity.RuntimeOperation
		suffix    string
	}{{activity.RuntimeUpdateContainer, "update"}, {activity.RuntimeRestoreContainerImage, "previous-image/restore"}} {
		activity.RegisterRuntimeLifecycle(api, hub.services.activity, spec.operation, huma.Operation{OperationID: string(spec.operation), Method: http.MethodPost, Path: "/environments/{id}/containers/{containerId}/" + spec.suffix})
	}
	path := "/api/environments/0/containers/" + runtimeContainerID + "/update"
	status, body := activationCall(t, hub, path, "", "respond-async", nil)
	require.Equal(t, 400, status, string(body))
	status, body = activationCall(t, hub, path, uuid.NewString(), "", nil)
	require.Equal(t, 400, status, string(body))
	status, body = activationCall(t, hub, path, uuid.NewString(), "respond-async", []byte(`{}`))
	require.Equal(t, 400, status, string(body))
	require.Empty(t, daemon.base.counts())
	require.Zero(t, daemon.pulls)
	key := uuid.NewString()
	status, body = activationCall(t, hub, path, key, "respond-async", nil)
	require.Equal(t, 202, status, string(body))
	run := activationRunID(t, body)
	snapshot, err := hub.services.waitLocal(run)
	require.NoError(t, err)
	require.NotNil(t, snapshot.Result)
	require.NotNil(t, snapshot.ImageLifecycle)
	require.True(t, snapshot.ImageLifecycle.Verified)
	currentID := snapshot.Result.NewID
	status, body = activationCall(t, hub, path, key, "respond-async", nil)
	require.Equal(t, 200, status, string(body))
	require.Equal(t, run, activationRunID(t, body))
	require.Equal(t, 1, daemon.pulls)
	require.Equal(t, 1, daemon.creates)
	status, body = activationCall(t, hub, "/api/environments/0/containers/"+currentID+"/previous-image/restore", uuid.NewString(), "respond-async", nil)
	require.Equal(t, 202, status, string(body))
	snapshot, err = hub.services.waitLocal(activationRunID(t, body))
	require.NoError(t, err)
	require.NotNil(t, snapshot.ImageLifecycle)
	require.True(t, snapshot.ImageLifecycle.Verified)
	require.Equal(t, 1, daemon.tags)
	require.Equal(t, 1, daemon.pulls)
	require.Equal(t, 2, daemon.creates)
}
