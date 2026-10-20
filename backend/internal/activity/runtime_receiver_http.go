// Source: docklet-next/docs/design/runtime-control.md
// Revision: 230a4a3cb92c59cc15e1f6767abd18f017e05ca4b83835524aa8c20e50a89e46
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Dormant bounded remote runtime implementation; production activation is separate.

package activity

import (
	"context"
	"encoding/json/v2"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/florianfinn/docklet-next/backend/internal/auth"
	"github.com/florianfinn/docklet-next/backend/internal/common"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
)

// NewRuntimeReceiverHTTP builds a standalone agent-only Huma adapter. Production
// registration, allowlists, compatibility and API advertising remain separate.
func NewRuntimeReceiverHTTP(receiver *RuntimeReceiver, authentication *auth.AuthService, cfg *config.Config, budget time.Duration) (http.Handler, error) {
	if receiver == nil || authentication == nil || cfg == nil || !cfg.AgentMode || strings.TrimSpace(cfg.AgentToken) == "" {
		return nil, errRuntimeInvalidInternal
	}
	frozen := *cfg
	mux := http.NewServeMux()
	api := humago.New(mux, huma.DefaultConfig("Runtime receiver", "0.0.0"))
	api.UseMiddleware(auth.NewHumaMiddleware(api, authentication, nil, nil, nil, &frozen))
	for _, route := range []struct{ method, path, operation string }{
		{http.MethodPost, "/api/environments/0/runtime-runs", "execute-runtime-run"},
		{http.MethodGet, "/api/environments/0/runtime-runs/{runId}", "get-runtime-run-outcome"},
		{http.MethodPost, "/api/environments/0/runtime-runs/{runId}/cancel", "cancel-runtime-run"},
	} {
		op := &huma.Operation{OperationID: route.operation, Method: route.method, Path: route.path, Security: []map[string][]string{{"ApiKeyAuth": {}}}}
		api.Adapter().Handle(op, api.Middlewares().Handler(func(ctx huma.Context) { runtimeReceiverHTTPInternal(receiver, ctx) }))
	}
	return RuntimeReceiverIngress(mux, budget), nil
}
func runtimeWriteAPIError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.MarshalWrite(w, common.NewAPIError(message, common.APIErrorCode(statusCodeInternal(status)), status))
}
func statusCodeInternal(status int) string {
	switch status {
	case 400, 413:
		return string(common.APIErrorCodeBadRequest)
	case 401:
		return string(common.APIErrorCodeUnauthorized)
	case 403:
		return string(common.APIErrorCodeForbidden)
	case 409:
		return string(common.APIErrorCodeConflict)
	case 408:
		return string(common.APIErrorCodeTimeout)
	default:
		return string(activitytypes.CodeRuntimeOutcomeUnknown)
	}
}
func runtimeHTTPErrorInternal(ctx huma.Context, status int, message string) {
	ctx.SetHeader("Content-Type", "application/json")
	ctx.SetStatus(status)
	_ = json.MarshalWrite(ctx.BodyWriter(), common.NewAPIError(message, common.APIErrorCode(statusCodeInternal(status)), status))
}
func runtimeReceiverHTTPInternal(receiver *RuntimeReceiver, ctx huma.Context) {
	if !middleware.HasAgentCredential(ctx.Context()) {
		runtimeHTTPErrorInternal(ctx, 403, "Agent credential required")
		return
	}
	if ok, _ := ctx.Context().Value(runtimeIngressKey{}).(bool); !ok {
		runtimeHTTPErrorInternal(ctx, 503, "Runtime ingress unavailable")
		return
	}
	if ctx.Context().Err() != nil {
		runtimeHTTPErrorInternal(ctx, 408, "Runtime admission interrupted")
		return
	}
	var result RuntimeReceiverResult
	var err error
	execute := ctx.Operation().OperationID == "execute-runtime-run"
	if ctx.Method() == http.MethodGet {
		body, readErr := io.ReadAll(io.LimitReader(ctx.BodyReader(), 1))
		if readErr != nil || len(body) != 0 {
			runtimeHTTPErrorInternal(ctx, 400, "Invalid runtime request")
			return
		}
		id, parseErr := activitytypes.ParseRuntimeIdentityQuery(ctx.Param("runId"), ctx.URL().RawQuery)
		if parseErr != nil {
			runtimeHTTPErrorInternal(ctx, 400, "Invalid runtime request")
			return
		}
		result, err = receiver.Query(ctx.Context(), id)
	} else {
		body, readErr := io.ReadAll(io.LimitReader(ctx.BodyReader(), activitytypes.RuntimeMaxBodyBytes+1))
		var limit *http.MaxBytesError
		if len(body) > activitytypes.RuntimeMaxBodyBytes || errors.As(readErr, &limit) {
			runtimeHTTPErrorInternal(ctx, 413, "Runtime request too large")
			return
		}
		if readErr != nil {
			if ctx.Context().Err() != nil {
				runtimeHTTPErrorInternal(ctx, 408, "Runtime admission interrupted")
			} else {
				runtimeHTTPErrorInternal(ctx, 400, "Invalid runtime request")
			}
			return
		}
		if execute {
			var envelope activitytypes.RuntimeExecutionEnvelope
			if ctx.URL().RawQuery != "" || activitytypes.DecodeRuntimeJSON(body, &envelope) != nil {
				runtimeHTTPErrorInternal(ctx, 400, "Invalid runtime request")
				return
			}
			result, err = receiver.Execute(ctx.Context(), envelope)
		} else {
			var request activitytypes.RuntimeCancellation
			if ctx.URL().RawQuery != "" || activitytypes.DecodeRuntimeJSON(body, &request) != nil || request.Identity.RunID != ctx.Param("runId") {
				runtimeHTTPErrorInternal(ctx, 400, "Invalid runtime request")
				return
			}
			result, err = receiver.Cancel(ctx.Context(), request.Identity, request.Attribution)
		}
	}
	if err != nil {
		var gate *GateConflict
		var conflict *runtimeConflictInternal
		var store *RuntimeStoreError
		switch {
		case execute && errors.As(err, &gate):
			refusal := activitytypes.RuntimeExecutionRefusal{Code: activitytypes.Code(gate.Code)}
			if refusal.Safe() {
				ctx.SetHeader("Content-Type", "application/json")
				ctx.SetStatus(409)
				_ = json.MarshalWrite(ctx.BodyWriter(), refusal)
				return
			}
		case errors.As(err, &conflict):
			runtimeHTTPErrorInternal(ctx, 409, "Runtime identity conflict")
			return
		case errors.Is(err, errRuntimeInvalidInternal):
			runtimeHTTPErrorInternal(ctx, 400, "Invalid runtime request")
			return
		case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
			runtimeHTTPErrorInternal(ctx, 408, "Runtime admission interrupted")
			return
		case errors.As(err, &store):
			runtimeHTTPErrorInternal(ctx, 503, "Runtime outcome unavailable")
			return
		}
		runtimeHTTPErrorInternal(ctx, 503, "Runtime outcome unavailable")
		return
	}
	outcome := activitytypes.RuntimeExecutionOutcome{Identity: result.Identity, State: result.State, Detail: result.Detail}
	if activitytypes.ValidateRuntimeOutcome(outcome, result.Identity) != nil {
		runtimeHTTPErrorInternal(ctx, 503, "Runtime detail unavailable")
		return
	}
	ctx.SetHeader("Content-Type", "application/json")
	ctx.SetStatus(200)
	_ = json.MarshalWrite(ctx.BodyWriter(), outcome)
}
