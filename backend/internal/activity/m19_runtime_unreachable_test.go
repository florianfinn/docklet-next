// Source: docklet-next/issues/349
// Revision: 4aa230d8e013057514050fef6048f1559d6c5a34
// Retrieved: 2026-11-06
// License: Apache-2.0
// Deviations: Original unreachable-agent admission refusal test.

package activity

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"syscall"
	"testing"

	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/remenv"
	userDTO "github.com/florianfinn/docklet-next/types/user"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// A stopped Direct agent or a disconnected Edge agent refuses a new command with agent.unavailable before admission,
// so a bulk summary can name the reason; nothing is admitted.
func TestM19UnreachableAgentRefusesAdmissionWithItsCode(t *testing.T) {
	for name, cause := range map[string]error{
		"direct-dial-refused": &remenv.TransportError{Err: &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED}},
		"edge-disconnected":   remenv.ErrEnvironmentUnavailable,
	} {
		t.Run(name, func(t *testing.T) {
			f := b1Fixture(t)
			c := f.command()
			mux, path := activationLegacyMux(t, f, c, activationLegacyPermission(c))
			f.service.activity.runtimeHTTP.ports.Store = func(context.Context, string) (RuntimeStoreBinding, error) {
				return RuntimeStoreBinding{}, cause
			}
			f.service.activity.runtimeHTTP.ports.PrepareTarget = func(context.Context, string, RuntimeTarget) (RuntimeResolvedTarget, error) {
				t.Fatal("target prepared for an unreachable agent")
				return RuntimeResolvedTarget{}, nil
			}
			req := httptest.NewRequest(http.MethodPost, path, nil)
			req.Header.Set("Idempotency-Key", uuid.NewString())
			req.Header.Set("Prefer", "respond-async")
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, req)
			require.Equal(t, http.StatusServiceUnavailable, response.Code, response.Body.String())
			require.Contains(t, response.Body.String(), `"code":"agent.unavailable"`)
			require.Empty(t, runtimeBoundarySnapshot(t, f.store).Receipts)
		})
	}
}

// Each route's refusal body fits a 503 schema the checked-in OpenAPI declares for that route (image routes declare only
// the image error envelope).
func TestM19UnreachableRefusalMatchesTheDeclared503(t *testing.T) {
	data, err := os.ReadFile("../../../api/openapi.json")
	require.NoError(t, err)
	var spec struct {
		Paths map[string]map[string]struct {
			Responses map[string]struct {
				Content map[string]struct {
					Schema struct {
						OneOf []struct {
							Ref string `json:"$ref"`
						} `json:"oneOf"`
					} `json:"schema"`
				} `json:"content"`
			} `json:"responses"`
		} `json:"paths"`
		Components struct {
			Schemas map[string]struct {
				Properties map[string]any `json:"properties"`
				Required   []string       `json:"required"`
			} `json:"schemas"`
		} `json:"components"`
	}
	require.NoError(t, json.Unmarshal(data, &spec))
	for op, path := range map[RuntimeOperation]string{
		RuntimeDeployProject:         "/environments/{id}/projects/{projectId}/up",
		RuntimeUpdateContainer:       "/environments/{id}/containers/{containerId}/update",
		RuntimeUpdateProject:         "/environments/{id}/updater/projects/{projectId}/update",
		RuntimeRestoreProject:        "/environments/{id}/updater/{kind}/{targetId}/restore-points/{pointId}/restore",
		RuntimeRestoreContainerImage: "/environments/{id}/containers/{containerId}/previous-image/restore",
	} {
		t.Run(string(op), func(t *testing.T) {
			f := b1Fixture(t)
			c := f.command()
			c.Operation = op
			if op == RuntimeUpdateProject || op == RuntimeRestoreProject {
				c = imageCommand(c, op)
			}
			if runtimeContainerImageOperation(op) {
				c.Target = RuntimeTarget{Kind: "container", ID: "example-container"}
			}
			ps := activationLegacyPermission(c)
			ps.AddEnv(c.Store.EnvironmentID, authz.PermProjectsUpdate, authz.PermImagesPull, authz.PermImageUpdatesCheck)
			_, _ = activationLegacyMux(t, f, c, ps)
			f.service.activity.runtimeHTTP.ports.Store = func(context.Context, string) (RuntimeStoreBinding, error) {
				return RuntimeStoreBinding{}, remenv.ErrEnvironmentUnavailable
			}
			ctx := context.WithValue(t.Context(), middleware.ContextKeyUserPermissions, ps)
			ctx = context.WithValue(ctx, userDTO.CurrentUserContextKey{}, &c.Actor)
			out, err := runtimeLegacyResponse(ctx, f.store.svc, c.Store.EnvironmentID, c.Key, "respond-async", op, c.Target, c.Options)
			require.NoError(t, err)
			response := httptest.NewRecorder()
			out.Body(humago.NewContext(nil, httptest.NewRequest(http.MethodPost, "/", nil), response))
			require.Equal(t, http.StatusServiceUnavailable, response.Code)
			require.Contains(t, response.Body.String(), `"code":"agent.unavailable"`)
			var body map[string]any
			require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
			declared, ok := spec.Paths[path]["post"].Responses["503"].Content["application/json"]
			require.True(t, ok, path)
			fits := false
			for _, alternative := range declared.Schema.OneOf {
				schema := spec.Components.Schemas[strings.TrimPrefix(alternative.Ref, "#/components/schemas/")]
				match := len(schema.Properties) > 0
				for key := range body {
					_, known := schema.Properties[key]
					match = match && known
				}
				for _, key := range schema.Required {
					_, present := body[key]
					match = match && present
				}
				fits = fits || match
			}
			require.True(t, fits, "%s: %s fits no declared 503 schema", path, response.Body.String())
		})
	}
}
