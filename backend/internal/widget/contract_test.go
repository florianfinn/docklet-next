// Source: docklet-next/issues/332
// Revision: 14e11ba9c022d57c29bd36c8ed24780763327d0c
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original hub-only module and exported widget contract regression checks.

package widget_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/florianfinn/docklet-next/backend/api"
	"github.com/florianfinn/docklet-next/backend/internal/access"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
)

func TestSW10WidgetModuleHubOnly(t *testing.T) {
	for _, mode := range []struct {
		name    string
		cfg     *config.Config
		present bool
	}{
		{"hub", &config.Config{}, true},
		{"agent", &config.Config{AgentMode: true}, false},
		{"edge", &config.Config{EdgeAgent: true}, false},
	} {
		t.Run(mode.name, func(t *testing.T) {
			e := echo.New()
			a := api.SetupAPI(e, e.Group("/api"), handlerutil.NewActivityAppContext(t.Context()), mode.cfg, api.HandlerDeps{})
			_, exists := a.OpenAPI().Paths["/service-widgets"]
			require.Equal(t, mode.present, exists)
			for path := range a.OpenAPI().Paths {
				if !mode.present {
					require.NotContains(t, path, "/service-widgets")
				}
			}
		})
	}
}

func TestSW11ExportedWidgetContract(t *testing.T) {
	data, err := os.ReadFile("../../../api/openapi.json")
	require.NoError(t, err)
	var spec struct {
		Paths map[string]map[string]struct {
			OperationID string `json:"operationId"`
		} `json:"paths"`
		Components struct {
			Schemas map[string]struct {
				Properties map[string]struct {
					Enum []any `json:"enum"`
				} `json:"properties"`
			} `json:"schemas"`
		} `json:"components"`
	}
	require.NoError(t, json.Unmarshal(data, &spec))
	operations := 0
	for path, methods := range spec.Paths {
		if !strings.HasPrefix(path, "/service-widgets") {
			continue
		}
		for _, op := range methods {
			if op.OperationID == "" {
				continue
			}
			operations++
			_, exists := access.Rules[op.OperationID]
			require.True(t, exists, "every widget operation must have an exact rule: %s", op.OperationID)
		}
	}
	require.Equal(t, 7, operations)
	codes := spec.Components.Schemas["WidgetProblem"].Properties["code"].Enum
	require.Len(t, codes, 11)
	generated, err := os.ReadFile("../../../web/src/api/generated/schema.ts")
	require.NoError(t, err)
	for _, code := range codes {
		require.Contains(t, string(generated), `"`+code.(string)+`"`)
	}
}
