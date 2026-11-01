// Source: docklet-next/issues/327
// Revision: b9b59ae92f1b09eb828940860971002eab9267b6
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original helper problem schema compatibility regression; M16 bind mounts and backup completion.

package backuptarget

import (
	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/stretchr/testify/require"
	"net/http"
	"testing"
)

func TestProblemSchemasPreserveWorkspaceHelperFailures(t *testing.T) {
	_, api := humatest.New(t)
	helper := &huma.Schema{OneOf: []*huma.Schema{{Ref: "#/components/schemas/HelperIncompatibleError"}, {Ref: "#/components/schemas/HelperUnavailableError"}}}
	op := &huma.Operation{OperationID: "get-volume-workspace", Method: http.MethodGet, Path: "/environments/{id}/volumes/{volumeName}/workspace", Responses: map[string]*huma.Response{"503": {Content: map[string]*huma.MediaType{"application/problem+json": {Schema: helper}}}}}
	api.OpenAPI().AddOperation(op)
	RegisterProblemSchemas(api)
	require.Equal(t, helper, op.Responses["503"].Content["application/problem+json"].Schema)
	first := op.Responses["500"].Content["application/problem+json"].Schema
	RegisterProblemSchemas(api)
	require.Equal(t, first, op.Responses["500"].Content["application/problem+json"].Schema, "registration must be idempotent")
}
