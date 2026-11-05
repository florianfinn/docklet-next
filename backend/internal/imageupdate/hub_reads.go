// Source: docklet-next/issues/341
// Revision: 89f69980c01d08714ce96c49365fb19065e3d6ba
// Retrieved: 2026-11-04
// License: Apache-2.0
// Deviations: Original hub handoff for every public image check source.

package imageupdate

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/florianfinn/docklet-next/types/base"
	checkDTO "github.com/florianfinn/docklet-next/types/imageupdate"
	"log/slog"
)

var errObservation = errors.New("incomplete update response")

// ConfigureHubReads is wired once, before route serving, only on the hub.
func (s *ImageUpdateService) ConfigureHubReads(proxy func(context.Context, string, string, string, []byte, any) error, observe func(context.Context, string) (func(checkDTO.BatchResponse) error, error)) {
	s.hubProxy, s.hubObserve = proxy, observe
}

func readHost[T any](ctx context.Context, s *ImageUpdateService, env, method, path string, body any, local func() (T, error)) (T, error) {
	var result T
	var err error
	var observe func(checkDTO.BatchResponse) error
	if s.hubObserve != nil {
		var e error
		observe, e = s.hubObserve(ctx, env)
		if e != nil {
			slog.WarnContext(ctx, "Hub image observation fence unavailable")
		}
	}
	if env != "0" && s.hubProxy != nil {
		var payload []byte
		if body != nil {
			payload, err = json.Marshal(body)
			if err != nil {
				return result, err
			}
		}
		var response base.ApiResponse[T]
		err = s.hubProxy(ctx, env, method, path, payload, &response)
		if err == nil && !response.Success {
			err = errObservation
		}
		result = response.Data
	} else {
		result, err = local()
	}
	if err == nil && observe != nil {
		batch := checkDTO.BatchResponse{}
		switch v := any(result).(type) {
		case *checkDTO.Response:
			if v != nil {
				batch[v.ImageRef] = v
			}
		case checkDTO.BatchResponse:
			batch = v
		}
		if e := observe(batch); e != nil {
			slog.WarnContext(ctx, "Hub image observation bookkeeping failed")
		}
	}
	return result, err
}
