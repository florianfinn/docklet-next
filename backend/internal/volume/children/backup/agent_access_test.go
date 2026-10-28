// Source: docklet-next/issues/283
// Revision: 62ae23049c387ce625c605279dbbb9ef3457d1f3
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original central backup-route refusal of an outdated Direct or Edge agent.

package backup

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/backuptarget"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/florianfinn/docklet-next/types/contract"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/florianfinn/docklet-next/types/version"
	"github.com/florianfinn/docklet-next/types/volume"
	"github.com/stretchr/testify/require"
)

func TestCentralBackupRoutesRefuseOutdatedAgentWithCompatibilityCode(t *testing.T) {
	for _, mode := range []string{"direct", "edge"} {
		for _, target := range []string{"", "local"} {
			t.Run(mode+"/target="+target, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
				defer cancel()
				f := newRuntimePlan(t)
				require.NoError(t, f.service.deps.DB.AutoMigrate(&environment.Environment{}, &backuptarget.Target{}))
				info, _ := json.Marshal(version.Info{AgentContractVersion: contract.AgentHubVersion - 1, AgentContractFingerprint: "previous", MinimumAgentContractVersion: contract.AgentHubVersion - 1})
				forwarded := 0
				remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/api/app-version" {
						_, _ = w.Write(info)
						return
					}
					if r.Method == http.MethodHead && strings.HasSuffix(r.URL.Path, "/system/health") {
						w.WriteHeader(http.StatusNoContent)
						return
					}
					forwarded++
					_, _ = w.Write([]byte(`{"success":true,"data":{}}`))
				}))
				defer remote.Close()
				id, token, endpoint := "outdated-host", strings.Repeat("o", 32), remote.URL
				if mode == "edge" {
					endpoint = "edge://example"
					conn := &backupIdentityConn{}
					tunnel := edge.NewAgentTunnelWithConn(id, conn)
					tunnel.TokenHash = edge.AgentTokenHash(token)
					conn.send = func(m *edge.TunnelMessage) {
						if m.Type != edge.MessageTypeCommandRequest {
							return
						}
						pending, ok := tunnel.Pending.Load(m.ID)
						require.True(t, ok)
						body := []byte(`{"success":true,"data":{}}`)
						if m.Path == "/api/app-version" {
							body = info
						} else {
							forwarded++
						}
						pending.(*edge.PendingRequest).ResponseCh <- &edge.TunnelMessage{ID: m.ID, Type: edge.MessageTypeResponse, Status: 200, Body: body}
					}
					edge.GetRegistry().Register(id, tunnel)
					defer edge.GetRegistry().Unregister(id)
				}
				require.NoError(t, f.service.deps.DB.Create(&environment.Environment{BaseModel: database.BaseModel{ID: id}, Name: "Example", Enabled: true, IsEdge: mode == "edge", ApiUrl: endpoint, AccessToken: &token}).Error)
				environments := environment.NewEnvironmentService(f.service.deps.DB, remote.Client(), nil, nil, nil, nil)
				handler := &Handler{service: f.service, environmentService: environments, appCtx: t.Context()}
				requestCtx := context.WithValue(ctx, user.CurrentUserContextKey{}, &user.SystemUser)
				_, err := handler.CreateBackup(requestCtx, &CreateBackupInput{EnvironmentID: id, VolumeName: "data", Body: &volume.CreateBackupRequest{TargetID: target}})
				require.Error(t, err)
				status, ok := err.(interface{ GetStatus() int })
				require.True(t, ok, "refusal is a typed HTTP status, not an internal error: %v", err)
				require.Equal(t, http.StatusConflict, status.GetStatus())
				body, e := json.Marshal(err)
				require.NoError(t, e)
				require.JSONEq(t, `{"success":false,"code":"agent.update-required"}`, string(body))
				require.Zero(t, forwarded, "no backup or target request reaches an outdated agent")
			})
		}
	}
}
