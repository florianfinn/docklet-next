// Source: docklet-next/issues/82
// Revision: 579ca3a12d93da7c7a6e68686d20425a642423fc
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original host inventory read implementation.

package middleware

import (
	"bytes"
	"encoding/json/v2"
	"net/http"
	"strings"
	"time"

	"github.com/florianfinn/docklet-next/backend/pkg/hoststate"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/labstack/echo/v5"
)

// A cold inventory read waits at most one second for an Edge agent to reconnect.
func (m *EnvironmentMiddleware) recoverInventoryTunnel(c *echo.Context, hostID, suffix string) bool {
	edge.TouchTunnelDemand(hostID, edge.DefaultTunnelDemandTTL)
	key := hoststate.Key{HostID: hostID, List: suffix + "?" + c.Request().URL.Query().Encode()}
	if _, cached := m.snapshots.Get(key); cached {
		return false
	}
	_, recovered := m.waitForActiveEdgeTunnelInternal(c.Request().Context(), hostID, time.Second).Get()
	return recovered
}

// Inventory reads retain only successful JSON responses, after caller authorization.
func (m *EnvironmentMiddleware) inventoryRead(c *echo.Context, hostID string, forward func() error) error {
	suffix := m.buildResourceSuffix(c.Request().URL.Path, hostID)
	parts := strings.Split(strings.Trim(suffix, "/"), "/")
	if m.snapshots == nil || c.Request().Method != http.MethodGet || !inventorySuffix(suffix, parts) {
		return forward()
	}
	c.Request().Header.Set("Accept-Encoding", "identity")
	key := hoststate.Key{HostID: hostID, List: suffix + "?" + c.Request().URL.Query().Encode()}
	original := c.Response()
	capture := &inventoryResponse{header: make(http.Header)}
	c.SetResponse(capture)
	err := forward()
	c.SetResponse(original)
	if err != nil {
		return err
	}
	var envelope struct {
		Success bool   `json:"success"`
		Code    string `json:"code"`
	}
	valid := json.Unmarshal(capture.body.Bytes(), &envelope) == nil
	observed := ""
	reason := ""
	if capture.status == http.StatusOK && valid && envelope.Success {
		value := m.snapshots.Put(key, capture.body.Bytes())
		observed = value.ObservedAt.Format(time.RFC3339Nano)
	} else if valid && (envelope.Code == "agent.unavailable" || envelope.Code == "agent.invalid-token" || envelope.Code == "agent.update-required") {
		reason = envelope.Code
		if value, ok := m.snapshots.Get(key); ok {
			capture.status = http.StatusOK
			capture.body.Reset()
			capture.body.Write(value.Data)
			capture.header = make(http.Header)
			observed = value.ObservedAt.Format(time.RFC3339Nano)
		}
	}
	edge.CopyResponseHeaders(capture.header, original.Header(), nil)
	original.Header().Del("Content-Length")
	original.Header().Set("Content-Type", "application/json")
	original.Header().Set("Cache-Control", "no-store")
	if observed != "" {
		original.Header().Set("X-Docklet-Observed-At", observed)
	}
	if reason != "" {
		original.Header().Set("X-Docklet-Host-Reason", reason)
	}
	original.WriteHeader(capture.status)
	_, err = original.Write(capture.body.Bytes())
	return err
}

type inventoryResponse struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func (w *inventoryResponse) Header() http.Header { return w.header }
func (w *inventoryResponse) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
}
func (w *inventoryResponse) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.body.Write(data)
}

func inventorySuffix(suffix string, parts []string) bool {
	switch suffix {
	case "/containers", "/projects", "/images", "/volumes", "/networks", "/ports", "/networks/topology", "/dashboard":
		return true
	}
	return len(parts) == 2 && (parts[0] == "projects" || parts[0] == "networks" || parts[0] == "volumes")
}
