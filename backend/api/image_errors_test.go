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
	"github.com/florianfinn/docklet-next/types/containerregistry"
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
