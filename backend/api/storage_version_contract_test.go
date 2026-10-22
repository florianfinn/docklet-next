// Source: docklet-next/docs/design/storage-workspace.md
// Revision: 60d2471b62b12e1af64c54060a50102a2e39d1c1
// Retrieved: 2026-10-22
// License: Apache-2.0
// Deviations: Original v5 fixture parity with registered producers for issue #179.
package api

import (
	"encoding/json"
	"os"
	"reflect"
	"slices"
	"testing"

	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/volumehelper"
	"github.com/florianfinn/docklet-next/types/volume"
	"github.com/stretchr/testify/require"
)

func TestStorageV5FixtureMatchesRegisteredProducer(t *testing.T) {
	body, err := os.ReadFile("../../types/contract/testdata/agent-hub-v5.json")
	require.NoError(t, err)
	var fixture struct {
		Storage struct {
			Routes                []struct{ Method, Path, OperationID string }
			RequiredFields        map[string][]string `json:"requiredFields"`
			HelperUnavailableCode string              `json:"helperUnavailableCode"`
			HelperErrorCode       string              `json:"helperErrorCode"`
			HelperImageSetting    string              `json:"helperImageSetting"`
		} `json:"storage"`
	}
	require.NoError(t, json.Unmarshal(body, &fixture))
	api := SetupAPIForSpec()
	for _, route := range fixture.Storage.Routes {
		item := api.OpenAPI().Paths[route.Path]
		require.NotNil(t, item, route.OperationID)
		operation := storageOperations(item)[route.Method]
		require.NotNil(t, operation, route.OperationID)
		require.Equal(t, route.OperationID, operation.OperationID)
	}
	require.Len(t, fixture.Storage.Routes, 30)
	for name, fields := range fixture.Storage.RequiredFields {
		schema := api.OpenAPI().Components.Schemas.Map()[name]
		require.NotNil(t, schema, name)
		actual := slices.Clone(schema.Required)
		slices.Sort(actual)
		require.Equal(t, fields, actual, name)
	}
	problem := volumehelper.IncompatibleHelper(nil)
	require.Equal(t, fixture.Storage.HelperErrorCode, problem.Code)
	require.Equal(t, fixture.Storage.HelperUnavailableCode, volumehelper.UnavailableHelper(nil).Code)
	require.Equal(t, "toolsImage", fixture.Storage.HelperImageSetting)
	for _, op := range storageOperations(api.OpenAPI().Paths["/environments/{id}/volumes/prune"]) {
		if op != nil {
			require.Equal(t, authz.PermVolumesPrune, op.Metadata[authz.MetaRequiredPermission])
		}
	}
	for _, path := range []string{"/environments/{id}/volumes/{volumeName}/workspace", "/environments/{id}/volumes/{volumeName}/workspace/file", "/environments/{id}/volumes/{volumeName}/workspace/file/download"} {
		for _, op := range storageOperations(api.OpenAPI().Paths[path]) {
			if op == nil {
				continue
			}
			response := op.Responses["503"]
			require.NotNil(t, response)
			schemas := response.Content["application/problem+json"].Schema.OneOf
			require.Len(t, schemas, 2)
			require.Equal(t, reflect.TypeFor[volume.HelperIncompatibleError](), api.OpenAPI().Components.Schemas.TypeFromRef(schemas[0].Ref))
			require.Equal(t, reflect.TypeFor[volume.HelperUnavailableError](), api.OpenAPI().Components.Schemas.TypeFromRef(schemas[1].Ref))
		}
	}
}
