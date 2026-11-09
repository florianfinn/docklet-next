// Source: docklet-next/issues/439
// Revision: 910032a54830f5ce1d37070964fa2dc7efb8aa2f
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original Compose-faithful self-update implementation.

package upgrade

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	core "github.com/florianfinn/docklet-next/backend/internal/selfupdate"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/stretchr/testify/require"
)

func TestRestartAndUpdateExcludeEachOther(t *testing.T) {
	updateStamp(t)
	f := newUpdateTransport(t, false)
	now := time.Now().UTC()
	restart := environment.AgentRestartRequest{RunID: "active-restart", EnvironmentID: f.remote.ID, RegistrationID: f.remote.ID, CredentialRevision: f.remote.CredentialRevision, State: "awaiting", CreatedAt: now, UpdatedAt: now, Deadline: now.Add(time.Minute)}
	require.NoError(t, f.db.Create(&restart).Error)
	op := core.Operation{OpID: "host-update", Role: "agent", EnvironmentID: f.remote.ID, CredentialRevision: f.remote.CredentialRevision, TargetRef: repository("agent") + ":1.0.1@sha256:" + testDigest, TargetVersion: "1.0.1", Deadline: now.Add(core.OperationTimeout)}
	_, _, err := f.hub.reserveRun(t.Context(), f.hub.remoteStore(f.remote.ID), op, "operator", "operator")
	require.ErrorContains(t, err, "host.busy")
	require.NoError(t, f.db.Model(&restart).Updates(map[string]any{"settled_at": now, "state": "connected"}).Error)
	_, _, err = f.hub.reserveRun(t.Context(), f.hub.remoteStore(f.remote.ID), op, "operator", "operator")
	require.NoError(t, err)
	require.NoError(t, f.hub.environments.StartAgentRestarts(t.Context()))
	t.Cleanup(f.hub.environments.StopAgentRestarts)
	mux := http.NewServeMux()
	api := humago.New(mux, huma.DefaultConfig("Host gate", "1.0.1"))
	api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
		next(huma.WithContext(ctx, context.WithValue(ctx.Context(), middleware.ContextKeyUserPermissions, authz.SudoPermissionSet())))
	})
	environment.RegisterEnvironments(api, environment.NewHandler(f.hub.environments, nil, nil, nil, nil, nil))
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/environments/"+f.remote.ID+"/agent/restart", nil))
	require.Equal(t, 409, response.Code, response.Body.String())
	require.Contains(t, response.Body.String(), "host.busy")
	// A deploy admitted on another resource still shares the host exclusion.
	activities := activity.NewActivityService(f.db, nil)
	_, err = activities.StartGatedActivity(t.Context(), activity.GateRequest{EnvironmentID: f.remote.ID, Operation: "deploy", Scope: "project:example", Type: "project_deploy"})
	require.ErrorContains(t, err, "host.busy")
}
