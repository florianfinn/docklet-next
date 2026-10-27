// Source: docklet-next/docs/design/images-updates.md
// Revision: 5a7a7ad39c1817a3d222bc84c0cf91d433d48f0d
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Hub-only stack lifecycle admission through existing durable runtime.

package activity

import (
	"context"
	"github.com/danielgtaylor/huma/v2"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	updaterDTO "github.com/florianfinn/docklet-next/types/updater"
	"net/http"
	"reflect"
)

type runtimeImageInput struct {
	EnvironmentID string `path:"id"`
	ProjectID     string `path:"projectId"`
	Key           string `header:"Idempotency-Key" format:"uuid" required:"true"`
	Prefer        string `header:"Prefer"`
	Body          updaterDTO.LifecycleRequest
}
type runtimeRestoreInput struct {
	EnvironmentID string `path:"id"`
	Kind          string `path:"kind"`
	TargetID      string `path:"targetId"`
	PointID       string `path:"pointId" format:"uuid"`
	Key           string `header:"Idempotency-Key" format:"uuid" required:"true"`
	Prefer        string `header:"Prefer"`
	Body          updaterDTO.LifecycleRequest
}

func RegisterImageLifecycle(api huma.API, service *ActivityService) {
	registerRestorePointSchema(api)
	middleware.RegisterWithPermission(api, imageAdmissionOperation(api, RuntimeUpdateProject, "update-project-images", "/environments/{id}/updater/projects/{projectId}/update"), authz.PermProjectsDeploy,
		func(ctx context.Context, in *runtimeImageInput) (*huma.StreamResponse, error) {
			if in.Body.PointID != "" {
				return nil, huma.Error400BadRequest("Invalid update intent")
			}
			return runtimeLegacyResponse(ctx, service, in.EnvironmentID, in.Key, in.Prefer, RuntimeUpdateProject, RuntimeTarget{Kind: "stack", ID: in.ProjectID}, RuntimeOptions{Lifecycle: &updaterDTO.LifecycleIntent{Request: in.Body}})
		})
	middleware.RegisterWithPermission(api, imageAdmissionOperation(api, RuntimeRestoreProject, "restore-update", "/environments/{id}/updater/{kind}/{targetId}/restore-points/{pointId}/restore"), authz.PermProjectsDeploy,
		func(ctx context.Context, in *runtimeRestoreInput) (*huma.StreamResponse, error) {
			if in.Kind != "project" || in.Body.PointID != "" && in.Body.PointID != in.PointID {
				return nil, imageDTO.Failure("update.invalid-target", 400)
			}
			in.Body.PointID = in.PointID
			return runtimeLegacyResponse(ctx, service, in.EnvironmentID, in.Key, in.Prefer, RuntimeRestoreProject, RuntimeTarget{Kind: "stack", ID: in.TargetID}, RuntimeOptions{Lifecycle: &updaterDTO.LifecycleIntent{Request: in.Body}})
		})
}

func imageAdmissionOperation(api huma.API, operation RuntimeOperation, id, path string) huma.Operation {
	op := huma.Operation{OperationID: id, Method: http.MethodPost, Path: path, Tags: []string{"Updater"}, Summary: "Admit stack image lifecycle; volume data is excluded", MaxBodyBytes: 64 << 10}
	runtimeHTTPContract(api, operation, &op)
	problem := api.OpenAPI().Components.Schemas.Schema(reflect.TypeFor[imageDTO.OperationError](), true, "")
	for _, status := range []string{"400", "403", "404", "409", "500", "502", "503"} {
		body := &huma.Schema{OneOf: []*huma.Schema{problem}}
		if prior := op.Responses[status]; prior != nil {
			if content := prior.Content["application/json"]; content != nil {
				body.OneOf = append(body.OneOf, content.Schema)
			}
		}
		op.Responses[status] = &huma.Response{Description: "Typed refusal or admitted failure with durable action identity", Content: map[string]*huma.MediaType{"application/json": {Schema: body}}}
	}
	return op
}
