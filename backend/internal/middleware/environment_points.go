// Source: docklet-next/docs/design/update-policies.md
// Revision: 839b98c10360da9b7beada6804d70d3da98ac8ed
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Advisory hub point holds and bounded agent-response validation for #229.

package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/florianfinn/docklet-next/backend/pkg/remenv"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"github.com/labstack/echo/v5"
)

type PointHoldReader func(context.Context, string, []string) (map[string]dto.RestorePointHoldState, error)

func pointRoute(suffix string) (list bool, point string) {
	parts := strings.Split(strings.Trim(suffix, "/"), "/")
	if len(parts) >= 2 && parts[0] == "updater" && parts[1] == "restore-points" {
		if len(parts) == 2 {
			return true, ""
		}
		if len(parts) == 3 {
			return false, parts[2]
		}
	}
	if len(parts) >= 4 && parts[0] == "updater" && parts[1] == "project" && parts[3] == "restore-points" {
		if len(parts) == 4 {
			return true, ""
		}
		if len(parts) == 5 {
			return false, parts[4]
		}
	}
	return false, ""
}

func pointFailure(c *echo.Context, code string, status int) error {
	return c.JSON(status, imageDTO.Failure(code, status))
}

func (m *EnvironmentMiddleware) checkPointDelete(c *echo.Context, env, point string) (bool, error) {
	if m.pointHolds == nil {
		return true, pointFailure(c, "update.persistence-failed", 500)
	}
	holds, err := m.pointHolds(c.Request().Context(), env, []string{point})
	if err != nil {
		return true, pointFailure(c, "update.persistence-failed", 500)
	}
	if holds[point] != "" && holds[point] != dto.HoldNone {
		return true, pointFailure(c, "restore.in-use", 409)
	}
	return false, nil
}

func (m *EnvironmentMiddleware) writePointList(c *echo.Context, env string, status int, headers http.Header, reader io.Reader) error {
	body, err := io.ReadAll(io.LimitReader(reader, dto.MaxRestorePointResponseBytes+1))
	if err != nil || len(body) > dto.MaxRestorePointResponseBytes {
		return pointFailure(c, "update.check-unavailable", 503)
	}
	if remenv.AgentAuthRejected(status) {
		return c.JSON(http.StatusBadGateway, map[string]any{"success": false, "code": "agent.invalid-token"})
	}
	if status != http.StatusOK {
		headers.Del("Content-Length")
		edge.CopyResponseHeaders(headers, c.Response().Header(), edge.BuildHopByHopHeaders(headers))
		return c.Blob(status, "application/json", body)
	}
	var envelope struct {
		Success bool               `json:"success"`
		Data    []dto.RestorePoint `json:"data"`
	}
	if json.Unmarshal(body, &envelope) != nil || !envelope.Success || envelope.Data == nil || len(envelope.Data) > dto.MaxRestorePointResponseCount {
		return pointFailure(c, "update.check-unavailable", 503)
	}
	ids := make([]string, len(envelope.Data))
	seen := map[string]bool{}
	for i, point := range envelope.Data {
		// Each point ID binds one row before hub evidence is overlaid.
		if seen[point.ID] {
			return pointFailure(c, "update.check-unavailable", 503)
		}
		seen[point.ID] = true
		ids[i] = point.ID
	}
	if m.pointHolds == nil {
		return pointFailure(c, "update.persistence-failed", 500)
	}
	holds, err := m.pointHolds(c.Request().Context(), env, ids)
	if err != nil {
		return pointFailure(c, "update.persistence-failed", 500)
	}
	for i := range envelope.Data {
		point := &envelope.Data[i]
		point.HoldState = dto.MergePointHold(point.HoldState, holds[point.ID])
	}
	headers.Del("Content-Length")
	headers.Del("Content-Encoding")
	edge.CopyResponseHeaders(headers, c.Response().Header(), edge.BuildHopByHopHeaders(headers))
	return c.JSON(status, envelope)
}

func (m *EnvironmentMiddleware) proxyPointList(c *echo.Context, tunnel *edge.AgentTunnel, env string) error {
	ctx, cancel := context.WithTimeout(c.Request().Context(), edge.DefaultProxyTimeout)
	defer cancel()
	headers := map[string]string{}
	for key, values := range edge.AgentRequestHeaders(c.Request().Header) {
		if len(values) > 0 {
			headers[key] = values[0]
		}
	}
	status, responseHeaders, body, err := edge.ProxyRequest(ctx, tunnel, c.Request().Method, m.buildProxyPath(c, env), c.Request().URL.RawQuery, headers, nil)
	if err != nil {
		return pointFailure(c, "update.check-unavailable", 503)
	}
	h := http.Header{}
	for key, value := range responseHeaders {
		h.Set(key, value)
	}
	return m.writePointList(c, env, status, h, bytes.NewReader(body))
}
