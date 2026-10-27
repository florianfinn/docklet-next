// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Registered legacy/async lifecycle boundary with preflush admission.

package activity

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/florianfinn/docklet-next/backend/internal/common"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	activitylib "github.com/florianfinn/docklet-next/backend/pkg/libarcane/activity"
	"github.com/florianfinn/docklet-next/backend/pkg/remenv"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"github.com/florianfinn/docklet-next/types/base"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	projecttypes "github.com/florianfinn/docklet-next/types/project"
)

type runtimeProjectHTTPInput struct {
	EnvironmentID string   `path:"id"`
	ProjectID     string   `path:"projectId"`
	Key           string   `header:"Idempotency-Key" doc:"Opaque UUID for one deliberate runtime intent"`
	Prefer        string   `header:"Prefer" doc:"respond-async returns admission only"`
	Services      []string `query:"services" doc:"Legacy restart service selection"`
	RawBody       []byte
}

type runtimeContainerHTTPInput struct {
	EnvironmentID string `path:"id"`
	ContainerID   string `path:"containerId"`
	Key           string `header:"Idempotency-Key" doc:"Opaque UUID for one deliberate runtime intent"`
	Prefer        string `header:"Prefer" doc:"respond-async returns admission only"`
	RawBody       []byte
}

// RegisterRuntimeLifecycle replaces only the seven selected route handlers.
// The original permission metadata remains the proxy/API authority source.
func RegisterRuntimeLifecycle(api huma.API, service *ActivityService, operation RuntimeOperation, op huma.Operation) {
	op.MaxBodyBytes = activitytypes.RuntimeMaxBodyBytes
	op.BodyReadTimeout = RuntimeAdmissionLimit
	// Legacy commands permit an empty body; RawBody otherwise defaults to required.
	op.RequestBody = &huma.RequestBody{Required: false}
	runtimeHTTPContract(api, operation, &op)
	op.Description += " Runtime commands use durable admission; Prefer: respond-async returns 202 for a new run and 200 for a retained or joined run."
	if strings.HasSuffix(string(operation), "project") {
		middleware.RegisterWithPermission(api, op, runtimePermissionInternal(operation), func(ctx context.Context, input *runtimeProjectHTTPInput) (*huma.StreamResponse, error) {
			options := RuntimeOptions{}
			if operation == RuntimeDeployProject || operation == RuntimeRedeployProject {
				var deploy projecttypes.DeployOptions
				if len(bytes.TrimSpace(input.RawBody)) != 0 && !bytes.Equal(bytes.TrimSpace(input.RawBody), []byte("null")) {
					if err := runtimeDecodeLegacyBody(input.RawBody, &deploy); err != nil {
						return nil, huma.Error400BadRequest("Invalid deployment options")
					}
				}
				options.Deploy = &deploy
			} else {
				if len(bytes.TrimSpace(input.RawBody)) != 0 {
					return nil, huma.Error400BadRequest("Runtime command has no request body")
				}
				if operation == RuntimeRestartProject {
					options.RestartServices = input.Services
				}
			}
			return runtimeLegacyResponse(ctx, service, input.EnvironmentID, input.Key, input.Prefer, operation, RuntimeTarget{Kind: "stack", ID: input.ProjectID}, options)
		})
	} else {
		middleware.RegisterWithPermission(api, op, runtimePermissionInternal(operation), func(ctx context.Context, input *runtimeContainerHTTPInput) (*huma.StreamResponse, error) {
			if len(bytes.TrimSpace(input.RawBody)) != 0 {
				return nil, huma.Error400BadRequest("Runtime command has no request body")
			}
			return runtimeLegacyResponse(ctx, service, input.EnvironmentID, input.Key, input.Prefer, operation, RuntimeTarget{Kind: "container", ID: input.ContainerID}, RuntimeOptions{})
		})
	}
	// Huma marks RawBody required during registration. Preserve the legacy optional body.
	runtimeHTTPBodyContract(api, operation, &op)
}

func runtimeDecodeLegacyBody(body []byte, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return activitytypes.ErrRuntimeWire
	}
	return nil
}

func runtimeJSONResponse(status int, value any) *huma.StreamResponse {
	return &huma.StreamResponse{Body: func(ctx huma.Context) {
		ctx.SetHeader("Content-Type", "application/json")
		ctx.SetStatus(status)
		_ = json.NewEncoder(ctx.BodyWriter()).Encode(value)
	}}
}

func runtimeLegacyResponse(ctx context.Context, service *ActivityService, environmentID, key, prefer string, operation RuntimeOperation, target RuntimeTarget, options RuntimeOptions) (*huma.StreamResponse, error) {
	if service == nil || service.runtimeHTTP == nil {
		return nil, huma.Error503ServiceUnavailable("Runtime is not ready")
	}
	h := service.runtimeHTTP
	admitted, ref, err := h.admit(ctx, environmentID, key, operation, target, options)
	if err != nil {
		var imageErr *imageDTO.OperationError
		var gate *GateConflict
		var mismatch *runtimeConflictInternal
		var targetErr *RuntimeTargetError
		switch {
		case errors.As(err, &imageErr):
			return runtimeJSONResponse(imageErr.Status, imageErr), nil
		case errors.As(err, &gate):
			return runtimeJSONResponse(409, activitytypes.RuntimeConflictResponse{Data: activitytypes.RuntimeConflict{Code: activitytypes.Code(gate.Code), ActiveRunID: gate.RunID}}), nil
		case errors.As(err, &mismatch):
			return runtimeJSONResponse(409, activitytypes.RuntimeConflictResponse{Data: activitytypes.RuntimeConflict{Code: activitytypes.CodeRunRequestMismatch}}), nil
		case errors.As(err, &targetErr):
			return runtimeJSONResponse(409, activitytypes.RuntimeConflictResponse{Data: activitytypes.RuntimeConflict{Code: activitytypes.Code(targetErr.Code)}}), nil
		case errors.Is(err, remenv.ErrAgentInvalidToken):
			return runtimeJSONResponse(502, common.NewAPIError("Agent credential rejected", "agent.invalid-token", 502)), nil
		case errors.Is(err, ErrRuntimeTransport):
			return runtimeJSONResponse(503, common.NewAPIError("Remote runtime outcome unavailable", common.APIErrorCode(activitytypes.CodeRuntimeOutcomeUnknown), 503)), nil
		case errors.Is(err, remenv.ErrAgentUpdateRequired):
			return runtimeJSONResponse(503, common.NewAPIError("Agent update required", "agent.update-required", 503)), nil
		case errors.Is(err, ErrRuntimeTargetRemoved):
			return runtimeJSONResponse(410, common.NewAPIError("Runtime target removed", common.APIErrorCode(activitytypes.CodeRuntimeTargetRemoved), 410)), nil
		default:
			var status huma.StatusError
			if errors.As(err, &status) {
				return nil, err
			}
			return nil, huma.Error503ServiceUnavailable("Runtime admission unavailable")
		}
	}
	async := false
	for _, token := range strings.Split(prefer, ",") {
		if strings.EqualFold(strings.TrimSpace(token), "respond-async") {
			async = true
		}
	}
	if async {
		status := 200
		if admitted.Code == activitytypes.CodeRunStarted {
			status = 202
		}
		return runtimeJSONResponse(status, base.ApiResponse[activitytypes.RuntimeAdmission]{Success: true, Data: admitted.RuntimeAdmission}), nil
	}
	stream := operation == RuntimeDeployProject || operation == RuntimeRedeployProject
	return &huma.StreamResponse{Body: func(humaCtx huma.Context) {
		waitCtx, cancel := context.WithTimeout(humaCtx.Context(), 30*time.Minute)
		defer cancel()
		writer := humaCtx.BodyWriter()
		var emit func(activitytypes.OutputLine) error
		if stream {
			humaCtx.SetHeader("Content-Type", "application/x-ndjson")
			humaCtx.SetHeader("Cache-Control", "no-cache")
			humaCtx.SetStatus(200)
			activitylib.WriteStartedLine(writer, ref.RunID)
			if flush, ok := writer.(http.Flusher); ok {
				flush.Flush()
			}
			emit = func(line activitytypes.OutputLine) error {
				// Existing output objects keep their exact payload. Relay loss markers
				// and plain text use the legacy log frame, never invalid NDJSON.
				text := bytes.TrimSpace([]byte(line.Text))
				var err error
				if len(text) == 0 || text[0] != '{' || !json.Valid(text) {
					err = json.NewEncoder(writer).Encode(map[string]string{"log": line.Text})
				} else {
					_, err = io.WriteString(writer, line.Text+"\n")
				}
				if flush, ok := writer.(http.Flusher); ok {
					flush.Flush()
				}
				return err
			}
		}
		snapshot, err := h.wait(waitCtx, ref, emit)
		status, code := runtimeWaitFailure(snapshot, err)
		if stream {
			if status == 200 {
				activitylib.WriteDoneLine(writer)
			} else {
				_ = json.NewEncoder(writer).Encode(activitytypes.RuntimeStreamFailure{Error: string(code), Code: code, ActivityID: ref.RunID})
			}
			return
		}
		humaCtx.SetHeader("Content-Type", "application/json")
		humaCtx.SetStatus(status)
		if status == 200 {
			_ = json.NewEncoder(writer).Encode(base.ApiResponse[base.MessageResponse]{Success: true, Data: base.MessageResponse{Message: runtimeSuccessMessage(operation), ActivityID: &ref.RunID}})
		} else {
			_ = json.NewEncoder(writer).Encode(activitytypes.RuntimeFailureResponse{Error: activitytypes.RuntimeHTTPError{Code: code, Message: string(code)}, Data: activitytypes.RuntimeID{ActivityID: ref.RunID}})
		}
	}}, nil
}

func runtimeSuccessMessage(operation RuntimeOperation) string {
	switch operation {
	case RuntimeDownProject:
		return "Project brought down successfully"
	case RuntimeRestartProject:
		return "Project restarted successfully"
	case RuntimeStartContainer:
		return "Container started successfully"
	case RuntimeStopContainer:
		return "Container stopped successfully"
	case RuntimeRestartContainer:
		return "Container restarted successfully"
	}
	return "Runtime workflow completed"
}
