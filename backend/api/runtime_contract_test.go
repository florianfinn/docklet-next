// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Current registered hub/agent graphs and v4 fixture/schema parity.

package api

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/pkg/remenv"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"github.com/stretchr/testify/require"
)

func TestRuntimeActivationCurrentRegisteredContract(t *testing.T) {
	body, err := os.ReadFile("../../types/contract/testdata/agent-hub-v4.json")
	require.NoError(t, err)
	var fixture struct {
		RuntimeRoutes  []string `json:"runtimeRoutes"`
		HubRoutes      []string `json:"hubRuntimeRoutes"`
		Authentication struct {
			Header string `json:"requestHeader"`
		} `json:"authentication"`
	}
	require.NoError(t, json.Unmarshal(body, &fixture))
	find := func(api huma.API, route string) *huma.Operation {
		t.Helper()
		method, path, ok := strings.Cut(route, " ")
		require.True(t, ok)
		path = strings.TrimPrefix(path, "/api")
		item := api.OpenAPI().Paths[path]
		require.NotNil(t, item, route)
		var op *huma.Operation
		switch method {
		case "GET":
			op = item.Get
		case "POST":
			op = item.Post
		}
		require.NotNil(t, op, route)
		return op
	}
	for _, cfg := range []*config.Config{{AgentMode: true}, {EdgeAgent: true}} {
		agent := setupAPIForSpecMode(cfg)
		scheme := agent.OpenAPI().Components.SecuritySchemes["AgentCredential"]
		require.NotNil(t, scheme)
		require.Equal(t, remenv.HeaderAgentToken, fixture.Authentication.Header)
		require.Equal(t, fixture.Authentication.Header, scheme.Name)
		require.Equal(t, "header", scheme.In)
		for _, route := range fixture.RuntimeRoutes {
			op := find(agent, route)
			require.Equal(t, []map[string][]string{{"AgentCredential": {}}}, op.Security)
			require.EqualValues(t, activitytypes.RuntimeMaxBodyBytes, op.MaxBodyBytes)
			require.NotNil(t, op.Responses["200"])
			require.NotNil(t, op.Responses["413"])
		}
		for _, route := range fixture.HubRoutes {
			_, path, _ := strings.Cut(route, " ")
			require.Nil(t, agent.OpenAPI().Paths[strings.TrimPrefix(path, "/api")])
		}
	}
	hub := setupAPIForSpecMode(&config.Config{})
	for _, route := range fixture.HubRoutes {
		find(hub, route)
	}
	current := SetupAPIForSpec()
	for _, route := range append(fixture.RuntimeRoutes, fixture.HubRoutes...) {
		find(current, route)
	}
	for _, path := range []string{"/projects/{projectId}/up", "/projects/{projectId}/down", "/projects/{projectId}/restart", "/projects/{projectId}/redeploy", "/containers/{containerId}/start", "/containers/{containerId}/stop", "/containers/{containerId}/restart"} {
		op := find(current, "POST /api/environments/{id}"+path)
		require.Contains(t, op.Responses["202"].Description, "admission only")
		require.NotEmpty(t, op.RequestBody.Content, "OpenAPI requestBody content is required even when an optional body is disallowed")
		for _, status := range []string{"200", "202", "409", "408", "500", "503", "504"} {
			require.NotNil(t, op.Responses[status], path+" "+status)
		}
	}
	registry := current.OpenAPI().Components.Schemas
	found := map[reflect.Type]bool{}
	for name, schema := range registry.Map() {
		typeOf := registry.TypeFromRef("#/components/schemas/" + name)
		if typeOf != reflect.TypeFor[activitytypes.Activity]() && typeOf != reflect.TypeFor[activitytypes.RuntimeSnapshot]() {
			continue
		}
		found[typeOf] = true
		if typeOf == reflect.TypeFor[activitytypes.Activity]() {
			require.NotContains(t, schema.Required, "outcomeState")
			require.NotContains(t, schema.Required, "cancelRequested")
			require.Equal(t, []string{"cancelRequested"}, schema.DependentRequired["outcomeState"])
		} else {
			require.Contains(t, schema.Required, "outcomeState")
			require.Contains(t, schema.Required, "cancelRequested")
		}
	}
	require.True(t, found[reflect.TypeFor[activitytypes.Activity]()])
	require.True(t, found[reflect.TypeFor[activitytypes.RuntimeSnapshot]()])
}
