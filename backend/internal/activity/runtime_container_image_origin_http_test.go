// Source: docklet-next/issues/342
// Revision: 6568cd71578eea183abdd086f1216e64e5fa2310
// Retrieved: 2026-11-05
// License: Apache-2.0
// Deviations: A locked container's update refusal keeps its exact public code.

package activity_test

import (
	"encoding/json"
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

func TestLockedContainerUpdateAnswersOriginLocked(t *testing.T) {
	daemon, registry := newContainerImageHTTP(t)
	hub := activationApp(t, false, "", activationSetup{BeforeStart: func(f *runtimeServicesFixture, _ *activity.RuntimeHTTPService, _ *activity.RuntimeReceiver) {
		configureContainerImageFixture(t, f, daemon, registry)
		f.containers.ConfigureOperations(f.db.DB, f.store.RegistrationID, true, nil, nil)
	}})
	observed := daemon.base.containers[runtimeContainerID]
	observed.Labels = map[string]string{"net.unraid.docker.managed": ""}
	daemon.base.containers[runtimeContainerID] = observed
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
		status, body := activationCall(t, hub, "/api/environments/0/containers/"+runtimeContainerID+"/"+spec.suffix, uuid.NewString(), "respond-async", nil)
		require.Equal(t, 409, status, string(body))
		var envelope struct {
			Success bool
			Error   struct{ Code string }
		}
		require.NoError(t, json.Unmarshal(body, &envelope))
		require.Equal(t, "container.origin-locked", envelope.Error.Code, string(body))
	}
	require.Zero(t, daemon.pulls)
}
