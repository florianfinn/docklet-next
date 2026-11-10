// Source: docklet-next/issues/439
// Revision: 910032a54830f5ce1d37070964fa2dc7efb8aa2f
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original Compose-faithful self-update implementation.

package upgrade

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	dto "github.com/florianfinn/docklet-next/types/system"
	"github.com/stretchr/testify/require"
)

func agentTransportRequest(t *testing.T, f *updateTransportFixture, isEdge bool, method, path string, headers map[string]string, body []byte) (int, []byte, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	if isEdge {
		tunnel, ok := edge.GetRegistry().Get(f.remote.ID).Get()
		require.True(t, ok)
		code, _, data, err := edge.ProxyRequest(ctx, tunnel, method, path, "", headers, body)
		return code, data, err
	}
	server := httptest.NewServer(f.handler)
	defer server.Close()
	request, err := http.NewRequestWithContext(ctx, method, server.URL+path, bytes.NewReader(body))
	require.NoError(t, err)
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	response, err := server.Client().Do(request)
	if err != nil {
		return 0, nil, err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	return response.StatusCode, data, err
}
func TestRecoveryAuthenticationAndRemovedRoutesDirectAndEdge(t *testing.T) {
	updateStamp(t)
	for _, isEdge := range []bool{false, true} {
		t.Run(map[bool]string{false: "direct", true: "edge"}[isEdge], func(t *testing.T) {
			f := newUpdateTransport(t, isEdge)
			request := dto.SelfUpdateEnvelope{OpID: "auth-operation", TargetVersion: "1.0.1", TargetReference: repository("agent") + ":1.0.1@sha256:" + testDigest, HubVersion: "1.0.1"}
			body, err := json.Marshal(request)
			require.NoError(t, err)
			for _, mode := range []string{"wrong token", "non-admin"} {
				headers := map[string]string{"Content-Type": "application/json", "X-Arcane-Agent-Token": "example"}
				want := 403
				if mode == "wrong token" {
					headers["X-Arcane-Agent-Token"] = "test-only"
					want = 401
				} else {
					headers["X-Example-Nonadmin"] = "true"
				}
				status, data, err := agentTransportRequest(t, f, isEdge, http.MethodPost, "/api/environments/0/system/upgrade", headers, body)
				require.NoError(t, err)
				require.Equal(t, want, status, "%s: %s", mode, data)
			}
			for _, suffix := range []string{"all", "all/status"} {
				method := http.MethodPost
				if suffix == "all/status" {
					method = http.MethodGet
				}
				path := "/api/environments/0/system/upgrade/" + suffix
				status, _, err := agentTransportRequest(t, f, isEdge, method, path, map[string]string{"X-Arcane-Agent-Token": "example"}, nil)
				if isEdge {
					require.Error(t, err)
					require.False(t, edge.ValidateEdgeCommand("system.upgrade.all", method, path, false))
				} else {
					require.NoError(t, err)
					require.Equal(t, 404, status)
				}
				router := f.hubRouter(t)
				response := httptest.NewRecorder()
				router.ServeHTTP(response, httptest.NewRequest(method, path, nil))
				require.Equal(t, 404, response.Code, response.Body.String())
			}
			f.daemon.mu.Lock()
			mutations := append([]string(nil), f.daemon.mutations...)
			f.daemon.mu.Unlock()
			require.Empty(t, mutations)
		})
	}
}
