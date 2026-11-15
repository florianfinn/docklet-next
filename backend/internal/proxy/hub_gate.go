// Source: docklet-next/issues/473
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-15
// License: Apache-2.0
// Deviations: Preserve the hub unknown-route envelope when admission loses switch-off.

package proxy

import (
	"bytes"
	"encoding/json/v2"
	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humaecho"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/types/features"
	moduledto "github.com/florianfinn/docklet-next/types/modules"
	"io"
	"net/http"
)

type admissionBase = huma.Context

type admissionResponse struct {
	admissionBase
	status int
	denial bytes.Buffer
}

func (c *admissionResponse) Unwrap() huma.Context { return c.admissionBase }
func (c *admissionResponse) Status() int {
	if c.status != 0 {
		return c.status
	}
	return c.admissionBase.Status()
}
func (c *admissionResponse) SetStatus(status int) {
	c.status = status
	if status != http.StatusForbidden {
		c.admissionBase.SetStatus(status)
	}
}
func (c *admissionResponse) BodyWriter() io.Writer {
	if c.status == http.StatusForbidden {
		return &c.denial
	}
	return c.admissionBase.BodyWriter()
}

// Only a local disabled admission after the initial gate becomes the canonical
// hub 404. An agent-side refusal while the hub remains on retains its typed 403.
func hubAdmissionResponse(settings middleware.ModuleSettings, agent bool) func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		if agent {
			next(ctx)
			return
		}
		response := &admissionResponse{admissionBase: ctx}
		next(response)
		if response.status != http.StatusForbidden {
			return
		}
		var problem moduledto.Problem
		if json.Unmarshal(response.denial.Bytes(), &problem) == nil && problem.Code == "feature-disabled" && problem.Feature == features.Proxy && settings != nil && !settings.IsFeatureEnabled(ctx.Context(), features.Proxy) {
			c := humaecho.Unwrap(ctx)
			for _, header := range []string{"Content-Type", "Content-Length", "Link"} {
				c.Response().Header().Del(header)
			}
			_ = middleware.APINotFound(c)
			return
		}
		ctx.SetStatus(http.StatusForbidden)
		_, _ = io.Copy(ctx.BodyWriter(), &response.denial)
	}
}
