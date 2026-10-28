// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original volume-backup response bound before transport serialization.

package backuptarget

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	dto "github.com/florianfinn/docklet-next/types/backup"
)

type responseContext huma.Context

type boundedResponse struct {
	responseContext
	headers  http.Header
	status   int
	body     bytes.Buffer
	overflow bool
}

func (r *boundedResponse) SetHeader(name, value string)    { r.headers.Set(name, value) }
func (r *boundedResponse) AppendHeader(name, value string) { r.headers.Add(name, value) }
func (r *boundedResponse) SetStatus(status int)            { r.status = status }
func (r *boundedResponse) Status() int                     { return r.status }
func (r *boundedResponse) BodyWriter() io.Writer           { return r }
func (r *boundedResponse) Unwrap() huma.Context            { return r.responseContext }
func (r *boundedResponse) Write(data []byte) (int, error) {
	if r.overflow || len(data) > dto.MaxVolumeResponseBytes-r.body.Len() {
		r.overflow = true
		return len(data), nil
	}
	return r.body.Write(data)
}

// BudgetAPI scopes buffering to the new backup/target surface. A response is
// committed only after serialization fits; downloads keep their stream contract.
func BudgetAPI(api huma.API) huma.API {
	group := huma.NewGroup(api)
	group.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
		if ctx.Operation().OperationID == "download-volume-backup" {
			next(ctx)
			return
		}
		if ctx.Method() == http.MethodGet {
			bounded, cancel := context.WithTimeout(ctx.Context(), 60*time.Second)
			defer cancel()
			ctx = huma.WithContext(ctx, bounded)
		}
		response := &boundedResponse{responseContext: ctx, headers: http.Header{}, status: http.StatusOK}
		next(response)
		var problem *dto.Problem
		if response.overflow {
			problem = dto.NewProblem(422, "volume.backup.listing-too-large", nil)
		}
		if ctx.Context().Err() == context.DeadlineExceeded && response.status < 400 {
			problem = dto.NewProblem(504, "volume.backup.engine-timeout", nil)
		}
		if problem != nil {
			ctx.SetHeader("Content-Type", "application/problem+json")
			ctx.SetStatus(problem.Status)
			_ = json.NewEncoder(ctx.BodyWriter()).Encode(problem)
			return
		}
		for name, values := range response.headers {
			for _, value := range values {
				ctx.AppendHeader(name, value)
			}
		}
		ctx.SetStatus(response.status)
		_, _ = response.body.WriteTo(ctx.BodyWriter())
	})
	return group
}
