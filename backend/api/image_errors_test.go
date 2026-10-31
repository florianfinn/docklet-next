// Source: docklet-next/docs/design/images-updates.md
// Revision: 361c54cb8d087b7d531ff8bb4dd1acae82f5f534
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Original image and registry safety contract for issue #229.

package api

import (
	"context"
	"encoding/json"
	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humaecho"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
	"github.com/florianfinn/docklet-next/types/containerregistry"
	environmentDTO "github.com/florianfinn/docklet-next/types/environment"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"uuid"
)

func TestPR1ValidationNeverEchoesCredentialInput(t *testing.T) {
	sentinel := uuid.New().String()
	router := echo.New()
	cfg := huma.DefaultConfig("test", "test")
	cfg.Transformers = append(cfg.Transformers, imageErrorTransformer)
	api := humaecho.NewWithGroup(router, router.Group("/api"), cfg)
	called := false
	huma.Register(api, huma.Operation{OperationID: "credential-validation", Method: "POST", Path: "/container-registries"}, func(ctx context.Context, in *struct {
		Body containerregistry.CreateContainerRegistryRequest
	}) (*struct{ Body any }, error) {
		called = true
		return nil, imageDTO.Failure("registry.invalid-config", 400)
	})
	body, _ := json.Marshal(map[string]any{"url": "https://registry.example.com", "token": map[string]string{"private": sentinel}})
	req := httptest.NewRequest(http.MethodPost, "/api/container-registries", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.False(t, called)
	assert.Equal(t, 400, rec.Code)
	require.NotContains(t, rec.Body.String(), sentinel)
	require.Contains(t, rec.Body.String(), "registry.invalid-config")
}

func TestLifecycleValidationIsTypedAndDoesNotEchoInput(t *testing.T) {
	router := echo.New()
	cfg := huma.DefaultConfig("test", "test")
	cfg.Transformers = append(cfg.Transformers, imageErrorTransformer)
	api := humaecho.NewWithGroup(router, router.Group("/api"), cfg)
	called := false
	huma.Register(api, huma.Operation{OperationID: "lifecycle-validation", Method: "POST", Path: "/environments/{id}/updater/projects/{projectId}/update"}, func(context.Context, *struct {
		ID        string `path:"id"`
		ProjectID string `path:"projectId"`
		Body      struct {
			ExpectedRevision string `json:"expectedRevision"`
		}
	}) (*struct{ Body any }, error) {
		called = true
		return nil, imageDTO.Failure("update.invalid-target", 400)
	})
	sentinel := uuid.New().String()
	req := httptest.NewRequest(http.MethodPost, "/api/environments/0/updater/projects/example/update", strings.NewReader(`{"expectedRevision":{"private":"`+sentinel+`"}}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.False(t, called)
	require.Equal(t, 400, rec.Code)
	require.Contains(t, rec.Body.String(), "update.invalid-target")
	require.NotContains(t, rec.Body.String(), sentinel)
}

func TestRetiredUpdateOperationsAdvertiseTypedRefusal(t *testing.T) {
	api := SetupAPIForSpec()
	for _, path := range []string{"/environments/{id}/projects/{projectId}/update-services", "/environments/{id}/containers/{containerId}/update", "/environments/{id}/updater/run"} {
		op := api.OpenAPI().Paths[path].Post
		require.True(t, op.Deprecated, path)
		response := op.Responses["409"]
		require.NotNil(t, response, path)
		body, err := json.Marshal(response)
		require.NoError(t, err)
		require.Contains(t, string(body), "ImageOperationError")
	}
}

func TestImageRuntimeHostProblemSurvivesFinalAPIPass(t *testing.T) {
	for _, code := range []string{"host.restarting", "host.busy", "agent.restart-superseded"} {
		t.Run(code, func(t *testing.T) {
			router := echo.New()
			cfg := huma.DefaultConfig("test", "test")
			cfg.Transformers = append(cfg.Transformers, imageErrorTransformer)
			api := humaecho.NewWithGroup(router, router.Group("/api"), cfg)
			huma.Register(api, huma.Operation{OperationID: "host-refusal", Method: "POST", Path: "/environments/{id}/updater/projects/{projectId}/update"}, func(context.Context, *struct {
				ID        string `path:"id"`
				ProjectID string `path:"projectId"`
			}) (*struct{ Body any }, error) {
				return nil, &environmentDTO.HostProblem{Code: code, Status: 409}
			})
			op := api.OpenAPI().Paths["/environments/{id}/updater/projects/{projectId}/update"].Post
			handlerutil.HostProblemResponses(api, op, "409")
			registerImageSafetyErrors(api)
			schema, err := json.Marshal(op.Responses["409"])
			require.NoError(t, err)
			require.Contains(t, string(schema), "HostProblem")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest("POST", "/api/environments/example/updater/projects/example/update", nil))
			require.Equal(t, 409, rec.Code)
			require.Contains(t, rec.Body.String(), code)
			require.NotContains(t, rec.Body.String(), "update.invalid-target")
		})
	}
}
