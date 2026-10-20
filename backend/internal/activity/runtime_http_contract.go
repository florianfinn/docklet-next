// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Registered compatibility routes advertise their actual response variants.

package activity

import (
	"github.com/danielgtaylor/huma/v2"
	"github.com/florianfinn/docklet-next/backend/internal/common"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"github.com/florianfinn/docklet-next/types/base"
	projecttypes "github.com/florianfinn/docklet-next/types/project"
	"reflect"
	"strconv"
)

func runtimeHTTPContract(api huma.API, operation RuntimeOperation, op *huma.Operation) {
	registry := api.OpenAPI().Components.Schemas
	schema := func(t reflect.Type) *huma.Schema { return registry.Schema(t, true, "") }
	admission := schema(reflect.TypeFor[base.ApiResponse[activitytypes.RuntimeAdmission]]())
	success := schema(reflect.TypeFor[base.ApiResponse[base.MessageResponse]]())
	content := map[string]*huma.MediaType{"application/json": {Schema: &huma.Schema{OneOf: []*huma.Schema{success, admission}}}}
	if operation == RuntimeDeployProject || operation == RuntimeRedeployProject {
		content = map[string]*huma.MediaType{"application/json": {Schema: admission}, "application/x-ndjson": {Schema: &huma.Schema{Type: "string", Description: "Legacy activity started frame, unchanged output lines and a terminal done or coded error frame with activityId"}}}
	}
	op.Responses = map[string]*huma.Response{
		"200": {Description: "Joined admission or confirmed legacy workflow success; NDJSON ends with done only after success", Content: content},
		"202": {Description: "New admission only; activityId equals activity.id", Content: map[string]*huma.MediaType{"application/json": {Schema: admission}}},
		"409": {Description: "Pre-admission minimal authorized conflict or request mismatch; admitted interrupted/cancelled legacy wait carries activityId", Content: map[string]*huma.MediaType{"application/json": {Schema: &huma.Schema{OneOf: []*huma.Schema{schema(reflect.TypeFor[activitytypes.RuntimeConflictResponse]()), schema(reflect.TypeFor[activitytypes.RuntimeFailureResponse]())}}}}},
	}
	op.Responses["410"] = &huma.Response{Description: "Removed registration; runtime.target-removed never dispatches", Content: map[string]*huma.MediaType{"application/json": {Schema: schema(reflect.TypeFor[common.APIError]())}}}
	for _, status := range []int{408, 500, 503, 504} {
		op.Responses[strconv.Itoa(status)] = &huma.Response{Description: "Admitted non-success carries stable activityId and runtime code; pre-admission unavailability carries no fabricated run", Content: map[string]*huma.MediaType{"application/json": {Schema: &huma.Schema{OneOf: []*huma.Schema{schema(reflect.TypeFor[activitytypes.RuntimeFailureResponse]()), schema(reflect.TypeFor[huma.ErrorModel]()), schema(reflect.TypeFor[common.APIError]())}}}}}
	}
}

// Huma's raw decoder was bound before this documentation-only content update;
// semantic options are validated by the shared admission boundary.
func runtimeHTTPBodyContract(api huma.API, operation RuntimeOperation, op *huma.Operation) {
	op.RequestBody.Required = false
	if operation == RuntimeDeployProject || operation == RuntimeRedeployProject {
		op.RequestBody.Content = map[string]*huma.MediaType{"application/json": {Schema: api.OpenAPI().Components.Schemas.Schema(reflect.TypeFor[projecttypes.DeployOptions](), true, "")}}
	} else {
		op.RequestBody.Content = map[string]*huma.MediaType{"application/json": {Schema: &huma.Schema{Not: &huma.Schema{}}}}
	}
}
