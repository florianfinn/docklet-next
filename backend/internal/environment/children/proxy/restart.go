// Source: docklet-next/issues/318
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original status-first Direct restart acknowledgement transport.

package proxy

import (
	"context"
	"errors"
	"github.com/florianfinn/docklet-next/backend/pkg/remenv"
	"io"
	"net/http"
)

// ExecuteRestart acknowledges a received 202 before reading optional body evidence.
// Edge delivers one complete response frame, so its normal transport owns that boundary.
func (s *Service) ExecuteRestart(ctx context.Context, target *Target) (*remenv.Response, error) {
	if target == nil {
		return nil, errors.New("remote environment target is required")
	}
	const path = "/api/system/agent/restart"
	if target.isEdge {
		return s.Execute(ctx, target, http.MethodPost, path, nil)
	}
	if s.accessCheck != nil {
		checked, cancel, err := s.accessCheck(ctx, target.id, target.accessToken, http.MethodPost, path)
		if err != nil {
			return nil, err
		}
		defer cancel()
		ctx = checked
	}
	request, err := buildRemoteRequestInternal(target, http.MethodPost, path, nil, nil)
	if err != nil {
		return nil, err
	}
	outbound, err := http.NewRequestWithContext(ctx, http.MethodPost, request.URL, http.NoBody)
	if err != nil {
		return nil, err
	}
	for key, value := range request.Headers {
		outbound.Header.Set(key, value)
	}
	client := s.httpClient
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(outbound)
	if err != nil {
		return nil, &remenv.TransportError{Err: err}
	}
	defer response.Body.Close()
	result := &remenv.Response{StatusCode: response.StatusCode}
	if response.StatusCode == http.StatusAccepted {
		return result, nil
	}
	result.Body, err = io.ReadAll(io.LimitReader(response.Body, (16<<10)+1))
	if err != nil {
		return result, &remenv.TransportError{Err: err}
	}
	return result, nil
}
