// Source: getarcaneapp/arcane/backend/pkg/utils/handlerutil/remote_proxy.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; translate agent authentication failures independently of caller authorization.

package handlerutil

import (
	"context"
	"encoding/json/v2"
	"errors"

	"github.com/danielgtaylor/huma/v2"

	"github.com/florianfinn/docklet-next/backend/pkg/remenv"
)

// RemoteJSONProxy is the remote environment transport operation used by domain handlers.
type RemoteJSONProxy func(context.Context, string, string, string, []byte, any) error

// JSON marshals a request, invokes the environment proxy, and translates transport errors for Huma.
func (p RemoteJSONProxy) JSON[T any](
	ctx context.Context,
	environmentID string,
	method string,
	path string,
	requestBody any,
) (*T, error) {
	body, err := marshalRemoteRequestBodyInternal(requestBody)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to marshal request body: " + err.Error())
	}

	var output T
	if pErr := p(ctx, environmentID, method, path, body, &output); pErr != nil {
		return nil, TranslateRemoteProxyError(pErr)
	}

	return &output, nil
}

func marshalRemoteRequestBodyInternal(requestBody any) ([]byte, error) {
	switch value := requestBody.(type) {
	case nil:
		return nil, nil
	case []byte:
		return value, nil
	default:
		return json.Marshal(value)
	}
}

// TranslateRemoteProxyError converts environment transport failures into Huma errors.
func TranslateRemoteProxyError(err error) error {
	if transportErr, ok := errors.AsType[*remenv.TransportError](err); ok {
		return huma.Error502BadGateway("failed to proxy request to environment: " + transportErr.Error())
	}

	if statusErr, ok := errors.AsType[*remenv.StatusError](err); ok {
		if remenv.AgentAuthRejected(statusErr.StatusCode) {
			return huma.Error502BadGateway("Remote environment rejected the manager's credentials")
		}
		if statusErr.StatusCode < 400 || statusErr.StatusCode > 599 {
			return huma.Error502BadGateway("Remote environment response unavailable")
		}
		return huma.NewError(statusErr.StatusCode, "environment returned error: "+string(statusErr.Body), nil)
	}

	if decodeErr, ok := errors.AsType[*remenv.DecodeError](err); ok {
		return huma.Error500InternalServerError("failed to decode environment response: " + decodeErr.Error())
	}

	return huma.Error500InternalServerError("failed to proxy request to environment: " + err.Error())
}
