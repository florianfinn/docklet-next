// Source: docklet-next/issues/438
// Revision: 57e835f7cce5778fd3d164298e5b488e11abb71d
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original public verified container body validation and retained-key proof.

package activity_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humaecho"
	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestVerifiedContainerPublicBodyAndRetainedKey(t *testing.T) {
	daemon, registry := newContainerImageHTTP(t)
	hub := activationApp(t, false, "", activationSetup{BeforeStart: func(f *runtimeServicesFixture, _ *activity.RuntimeHTTPService, _ *activity.RuntimeReceiver) {
		configureContainerImageFixture(t, f, daemon, registry)
		f.containers.ConfigureOperations(f.db.DB, f.store.RegistrationID, true, nil, nil)
	}})
	api := humaecho.NewWithGroup(hub.router, hub.router.Group("/api"), huma.DefaultConfig("Verified container update", "17"))
	api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
		ctx = huma.WithValue(ctx, middleware.ContextKeyUserPermissions, authz.SudoPermissionSet())
		ctx = huma.WithValue(ctx, user.CurrentUserContextKey{}, &user.Actor{ID: "example-owner", Username: "Operator"})
		next(ctx)
	})
	activity.RegisterRuntimeLifecycle(api, hub.services.activity, activity.RuntimeUpdateContainer, huma.Operation{OperationID: string(activity.RuntimeUpdateContainer), Method: http.MethodPost, Path: "/environments/{id}/containers/{containerId}/update"})
	path := "/api/environments/0/containers/" + runtimeContainerID + "/update"
	for _, body := range []string{`{}`, `{"expectedManifest":"sha256:invalid"}`, `{"expectedManifest":"sha256:` + strings.Repeat("a", 64) + `","policy":{}}`} {
		status, response := activationCall(t, hub, path, uuid.NewString(), "respond-async", []byte(body))
		require.Equal(t, 400, status, string(response))
	}
	tag, err := name.NewTag(daemon.ref, name.Insecure)
	require.NoError(t, err)
	head, err := remote.Head(tag)
	require.NoError(t, err)
	body, err := json.Marshal(map[string]string{"expectedManifest": head.Digest.String()})
	require.NoError(t, err)
	key := uuid.NewString()
	status, response := activationCall(t, hub, path, key, "respond-async", body)
	require.Equal(t, 202, status, string(response))
	snapshot, err := hub.services.waitLocal(activationRunID(t, response))
	require.NoError(t, err)
	require.NotNil(t, snapshot.ImageLifecycle)
	require.True(t, snapshot.ImageLifecycle.Verified)
	status, response = activationCall(t, hub, path, key, "respond-async", []byte(`{"expectedManifest":"sha256:`+strings.Repeat("d", 64)+`"}`))
	require.Equal(t, 409, status, string(response))
	require.Equal(t, 1, daemon.pulls)
}
