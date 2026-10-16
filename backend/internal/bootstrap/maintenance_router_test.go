// Source: docklet-next/docs/design/hub-backup.md
// Revision: 8f8c8f89bc6ebb7261862d0c35a6ddbc1513e9fa
// Retrieved: 2026-10-15
// License: Apache-2.0
// Deviations: Original router-level maintenance regression tests for issue #32.

package bootstrap

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"

	hubbackup "github.com/florianfinn/docklet-next/backend/internal/system/children/backup"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
)

func TestMaintenanceRouterClosesOpenStreamAndEdgeTunnel(t *testing.T) {
	gate := hubbackup.NewMaintenanceGate()
	registry := edge.NewTunnelRegistry()
	gate.OnBegin(registry.CloseActiveForMaintenance)
	router := newEchoInternal()
	router.Use(maintenanceMiddlewareInternal(gate))
	router.GET("/api/stream", func(c *echo.Context) error {
		c.Response().Header().Set("Content-Type", "text/event-stream")
		_, _ = c.Response().Write([]byte("data: ready\n\n"))
		c.Response().(http.Flusher).Flush()
		<-c.Request().Context().Done()
		return nil
	})
	tunnel := edge.NewTunnelServerWithRegistry(registry, func(context.Context, string) (string, error) {
		return "env-example", nil
	}, nil)
	router.GET("/api/tunnel/connect", tunnel.HandleConnect)
	server := httptest.NewServer(router)
	defer server.Close()

	stream, err := server.Client().Get(server.URL + "/api/stream")
	require.NoError(t, err)
	defer stream.Body.Close()
	require.Equal(t, http.StatusOK, stream.StatusCode)

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/tunnel/connect"
	conn, response, err := websocket.Dial(t.Context(), wsURL, nil)
	require.NoError(t, err)
	defer conn.CloseNow()
	require.Equal(t, http.StatusSwitchingProtocols, response.StatusCode)
	require.NoError(t, wsjson.Write(t.Context(), conn, &edge.TunnelMessage{Type: edge.MessageTypeRegister, AgentToken: strings.Join([]string{"example", "token"}, "-")}))
	var registered edge.TunnelMessage
	require.NoError(t, wsjson.Read(t.Context(), conn, &registered))
	require.True(t, registered.Accepted)

	require.NoError(t, gate.Begin("activity-1"))
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	require.NoError(t, gate.Wait(ctx), "restore must proceed while streams were open")
	require.Eventually(t, func() bool {
		active, ok := registry.Get("env-example").Get()
		return !ok || active.Conn.IsClosed()
	}, time.Second, 10*time.Millisecond)
	readCtx, readCancel := context.WithTimeout(t.Context(), time.Second)
	defer readCancel()
	_, _, err = conn.Read(readCtx)
	require.Error(t, err, "agent tunnel must reconnect after maintenance")
}

func TestMaintenanceRouterAdmissionAndDatabaseFreeReads(t *testing.T) {
	gate := hubbackup.NewMaintenanceGate()
	router := newEchoInternal()
	router.Use(maintenanceMiddlewareInternal(gate))
	for _, route := range []struct{ method, path string }{
		{http.MethodGet, "/api/backups/maintenance"},
		{http.MethodGet, "/api/health"},
		{http.MethodGet, "/api/read"},
		{http.MethodPost, "/api/mutate"},
	} {
		router.Add(route.method, route.path, func(c *echo.Context) error { return c.NoContent(http.StatusNoContent) })
	}
	request := func(method, path string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(method, path, http.NoBody))
		return rec
	}
	require.Equal(t, http.StatusNoContent, request(http.MethodGet, "/api/read").Code)
	require.Equal(t, http.StatusNoContent, request(http.MethodPost, "/api/mutate").Code)
	require.NoError(t, gate.Begin("activity-1"))
	status := request(http.MethodGet, "/api/backups/maintenance")
	require.Equal(t, http.StatusOK, status.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(status.Body.Bytes(), &body))
	require.Equal(t, true, body["active"])
	require.Equal(t, "draining", body["phase"])
	require.Equal(t, "activity-1", body["activityId"])
	require.Equal(t, http.StatusServiceUnavailable, request(http.MethodGet, "/api/health").Code)
	require.Equal(t, http.StatusServiceUnavailable, request(http.MethodHead, "/api/health").Code)
	for _, route := range []struct{ method, path string }{{http.MethodGet, "/api/read"}, {http.MethodPost, "/api/mutate"}} {
		response := request(route.method, route.path)
		require.Equal(t, http.StatusServiceUnavailable, response.Code)
		require.Contains(t, response.Header().Get("Content-Type"), "application/problem+json")
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
		require.Equal(t, "backup.maintenance", body["detail"])
		require.Equal(t, float64(http.StatusServiceUnavailable), body["status"])
	}
	gate.SetPhase("applying")
	status = request(http.MethodGet, "/api/backups/maintenance")
	require.NoError(t, json.Unmarshal(status.Body.Bytes(), &body))
	require.Equal(t, "applying", body["phase"])
}

func TestRestoreAdmissionResponseSurvivesBeginningMaintenance(t *testing.T) {
	gate := hubbackup.NewMaintenanceGate()
	router := newEchoInternal()
	router.Use(maintenanceMiddlewareInternal(gate))
	router.POST("/api/backups/:id/restore", func(c *echo.Context) error {
		if err := gate.Begin("activity-1"); err != nil {
			return err
		}
		require.NoError(t, c.Request().Context().Err())
		return c.NoContent(http.StatusAccepted)
	})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/backups/example/restore", http.NoBody))
	require.Equal(t, http.StatusAccepted, rec.Code)
	require.NoError(t, gate.Wait(t.Context()))
}
