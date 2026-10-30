// Source: docklet-next/issues/318
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-30
// License: Apache-2.0
// Deviations: Original typed connection result and user-activity tests.

package environment

import (
	"context"
	"encoding/json"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	activitylib "github.com/florianfinn/docklet-next/backend/pkg/libarcane/activity"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
	dto "github.com/florianfinn/docklet-next/types/activity"
	"github.com/florianfinn/docklet-next/types/contract"
	hostDTO "github.com/florianfinn/docklet-next/types/environment"
	"github.com/florianfinn/docklet-next/types/version"
	"github.com/stretchr/testify/require"
)

type connectionActivity struct {
	onStart           func()
	started, finished int
	status            dto.Status
}

func (a *connectionActivity) StartActivity(_ context.Context, request activitylib.StartRequest) (*dto.Activity, error) {
	a.started++
	if a.onStart != nil {
		a.onStart()
	}
	return &dto.Activity{ID: "example-check", Type: request.Type}, nil
}
func (a *connectionActivity) CompleteActivity(_ context.Context, _ string, status dto.Status, _ string, _ *string, _ ...string) (*dto.Activity, error) {
	a.finished++
	a.status = status
	return &dto.Activity{ID: "example-check", Status: status}, nil
}

func TestConnectionTypedResultsAndActivity(t *testing.T) {
	for _, state := range []string{"online", "offline", "invalid-token", "ui-disabled", "update-required"} {
		t.Run(state, func(t *testing.T) {
			f := newAgentFixture(t, "direct", version.Info{})
			agent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if state == "ui-disabled" && r.Method == "GET" && r.URL.Path == "/api/health" {
					w.WriteHeader(403)
					_, _ = w.Write([]byte(`{"code":"settings.ui-configuration-disabled"}`))
					return
				}
				if state == "offline" {
					w.WriteHeader(503)
					return
				}
				if r.URL.Path == "/api/environments/0/system/health" {
					switch state {
					case "invalid-token":
						w.WriteHeader(401)
						return
					case "ui-disabled":
						w.WriteHeader(403)
						_, _ = w.Write([]byte(`{"code":"settings.ui-configuration-disabled"}`))
						return
					}
					require.Equal(t, "example", r.Header.Get("X-Arcane-Agent-Token"))
					w.WriteHeader(200)
					return
				}
				if r.URL.Path == "/api/app-version" {
					info := version.Info{AgentContractVersion: contract.AgentHubVersion, AgentContractFingerprint: contract.AgentHubFingerprint}
					if state == "update-required" {
						info.AgentContractVersion--
					}
					require.NoError(t, json.NewEncoder(w).Encode(info))
					return
				}
				_, _ = w.Write([]byte(`{"status":"UP"}`))
			}))
			defer agent.Close()
			require.NoError(t, f.service.db.Model(&Environment{}).Where("id=?", f.id).Update("api_url", agent.URL).Error)
			f.service.httpClient.Transport = http.DefaultTransport
			activity := &connectionActivity{}
			handler := &EnvironmentHandler{environmentService: f.service, activityService: activity}
			out, err := handler.TestConnection(t.Context(), &TestConnectionInput{ID: f.id})
			require.Equal(t, 1, activity.started)
			require.Equal(t, 1, activity.finished)
			if state == "online" {
				require.NoError(t, err)
				require.Equal(t, "online", out.Body.Data.Status)
				require.Equal(t, dto.StatusSuccess, activity.status)
				return
			}
			require.Error(t, err)
			require.Nil(t, out)
			require.Equal(t, dto.StatusFailed, activity.status)
			if state == "offline" {
				var p *hostDTO.HostProblem
				require.ErrorAs(t, err, &p)
				require.Equal(t, "host.unreachable", p.Code)
			} else {
				var p *handlerutil.AgentAccessError
				require.ErrorAs(t, err, &p)
				expected := "agent.invalid-token"
				if state == "ui-disabled" {
					expected = "agent.ui-configuration-disabled"
				}
				if state == "update-required" {
					expected = "agent.update-required"
				}
				require.Equal(t, expected, p.Code)
			}
		})
	}
}

func TestConnectionStandbyOpenedResult(t *testing.T) {
	f := newAgentFixture(t, "edge", version.Info{AgentContractVersion: contract.AgentHubVersion, AgentContractFingerprint: contract.AgentHubFingerprint})
	edge.GetRegistry().Unregister(f.id)
	edge.GetPollRuntimeRegistry().Update(f.id, time.Hour, time.Now())
	a := &connectionActivity{onStart: func() {
		f.conn.mu.Lock()
		f.conn.closed = false
		f.conn.mu.Unlock()
		f.tunnel = edge.NewAgentTunnelWithConn(f.id, f.conn)
		f.tunnel.APIKeyID = "example"
		f.tunnel.TokenHash = edge.AgentTokenHash(f.token)
		edge.GetRegistry().Register(f.id, f.tunnel)
	}}
	h := &EnvironmentHandler{environmentService: f.service, activityService: a}
	out, err := h.TestConnection(t.Context(), &TestConnectionInput{ID: f.id})
	require.NoError(t, err)
	require.Equal(t, "standby-opened", out.Body.Data.Status)
	require.Equal(t, dto.StatusSuccess, a.status)
}

func TestConnectionCustomURLDoesNotClaimStandbyOpened(t *testing.T) {
	f := newAgentFixture(t, "edge", version.Info{})
	edge.GetRegistry().Unregister(f.id)
	edge.GetPollRuntimeRegistry().Update(f.id, time.Hour, time.Now())
	a := &connectionActivity{}
	h := &EnvironmentHandler{environmentService: f.service, activityService: a}
	ctx := context.WithValue(t.Context(), middleware.ContextKeyUserPermissions, authz.SudoPermissionSet())
	out, err := h.TestConnection(ctx, &TestConnectionInput{ID: f.id, Body: &hostDTO.TestConnectionRequest{ApiUrl: new("https://agent.example.com")}})
	require.NoError(t, err)
	require.Equal(t, "online", out.Body.Data.Status)
	require.False(t, edge.HasActiveTunnel(f.id), "a custom URL probe does not open the saved Edge tunnel")
	require.Equal(t, 1, a.started)
	require.Equal(t, dto.StatusSuccess, a.status)
}
