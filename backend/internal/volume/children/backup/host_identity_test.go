// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original backup-route Direct/Edge session revocation and late-response proofs.

package backup

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/backuptarget"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	dto "github.com/florianfinn/docklet-next/types/backup"
	"github.com/florianfinn/docklet-next/types/base"
	"github.com/florianfinn/docklet-next/types/contract"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/florianfinn/docklet-next/types/version"
	"github.com/florianfinn/docklet-next/types/volume"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
)

type backupIdentityConn struct {
	mu     sync.Mutex
	closed bool
	send   func(*edge.TunnelMessage)
}

func (c *backupIdentityConn) Send(m *edge.TunnelMessage) error {
	c.mu.Lock()
	closed := c.closed
	c.mu.Unlock()
	if closed {
		return edge.ErrTunnelConnectionClosed
	}
	c.send(m)
	return nil
}
func (c *backupIdentityConn) Receive() (*edge.TunnelMessage, error) { return nil, io.EOF }
func (c *backupIdentityConn) IsExpectedReceiveError(error) bool     { return true }
func (c *backupIdentityConn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	return nil
}
func (c *backupIdentityConn) IsClosed() bool    { c.mu.Lock(); defer c.mu.Unlock(); return c.closed }
func (c *backupIdentityConn) Transport() string { return edge.EdgeTransportWebSocket }

func TestBackupRoutesDropHeldDirectAndEdgeResponsesOnHostRotation(t *testing.T) {
	for _, mode := range []string{"direct", "edge"} {
		for _, route := range []struct {
			method, path string
			status       int
		}{{"GET", "/volumes/backups/overview", 200}, {"POST", "/volumes/data/backups", 202}} {
			t.Run(mode+route.method, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
				defer cancel()
				f := newRuntimePlan(t)
				require.NoError(t, f.service.deps.DB.AutoMigrate(&environment.Environment{}, &backuptarget.Target{}))
				entered := make(chan struct{}, 1)
				release := make(chan struct{})
				info, _ := json.Marshal(version.Info{AgentContractVersion: contract.AgentHubVersion, AgentContractFingerprint: contract.AgentHubFingerprint})
				syncReply := func(body []byte) []byte {
					var set dto.TargetSet
					require.NoError(t, json.Unmarshal(body, &set))
					encoded, e := json.Marshal(base.ApiResponse[dto.TargetSetReceipt]{Success: true, Data: dto.TargetSetReceipt{Authority: set.Authority, SetVersion: set.SetVersion, ContentHash: set.ContentHash, Conflicts: []string{}}})
					require.NoError(t, e)
					return encoded
				}
				oldBody := []byte(`{"success":true,"data":{"activityId":"old-session-admission"}}`)
				remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/api/app-version" {
						_, _ = w.Write(info)
						return
					}
					if r.Method == "PUT" && strings.HasSuffix(r.URL.Path, "/backups/targets") {
						payload, e := io.ReadAll(r.Body)
						require.NoError(t, e)
						_, _ = w.Write(syncReply(payload))
						return
					}
					if r.Method == "HEAD" {
						w.WriteHeader(204)
						return
					}
					entered <- struct{}{}
					<-release
					w.WriteHeader(route.status)
					_, _ = w.Write(oldBody)
				}))
				defer func() { close(release); remote.Close() }()
				id := "identity-host"
				token := strings.Repeat("o", 32)
				endpoint := remote.URL
				var tunnel *edge.AgentTunnel
				if mode == "edge" {
					endpoint = "edge://example"
					conn := &backupIdentityConn{}
					tunnel = edge.NewAgentTunnelWithConn(id, conn)
					tunnel.TokenHash = edge.AgentTokenHash(token)
					conn.send = func(m *edge.TunnelMessage) {
						if m.Type != edge.MessageTypeCommandRequest {
							return
						}
						pending, ok := tunnel.Pending.Load(m.ID)
						require.True(t, ok)
						body := []byte(`{"success":true}`)
						if m.Path == "/api/app-version" {
							body = info
						} else if m.Method == "PUT" && strings.HasSuffix(m.Path, "/backups/targets") {
							body = syncReply(m.Body)
						} else if m.Method != "HEAD" {
							entered <- struct{}{}
							return
						}
						pending.(*edge.PendingRequest).ResponseCh <- &edge.TunnelMessage{ID: m.ID, Type: edge.MessageTypeResponse, Status: 200, Body: body}
					}
					edge.GetRegistry().Register(id, tunnel)
					defer edge.GetRegistry().Unregister(id)
				}
				require.NoError(t, f.service.deps.DB.Create(&environment.Environment{BaseModel: database.BaseModel{ID: id}, Name: "Example", Enabled: true, IsEdge: mode == "edge", ApiUrl: endpoint, AccessToken: &token}).Error)
				environments := environment.NewEnvironmentService(f.service.deps.DB, remote.Client(), nil, nil, nil, nil)
				router := echo.New()
				resolver := func(ctx context.Context, id string) (string, *string, bool, error) {
					e, err := environments.GetEnvironmentByID(ctx, id)
					if err != nil {
						return "", nil, false, err
					}
					return e.ApiUrl, e.AccessToken, e.Enabled, nil
				}
				router.Use(middleware.NewEnvProxyMiddlewareWithParamAndRegistry("0", "id", resolver, nil, nil, edge.GetRegistry(), nil, middleware.EnvironmentProxySecurity{AgentAccess: environments.AgentAccess, HTTPClient: remote.Client()}))
				router.Add(route.method, "/api/environments/:id"+route.path, func(c *echo.Context) error {
					handler := &Handler{service: f.service, environmentService: environments, appCtx: t.Context()}
					requestCtx := context.WithValue(c.Request().Context(), user.CurrentUserContextKey{}, &user.SystemUser)
					output, err := handler.CreateBackup(requestCtx, &CreateBackupInput{EnvironmentID: id, VolumeName: "data", Body: &volume.CreateBackupRequest{TargetID: "local"}})
					if err != nil {
						return c.JSON(502, map[string]string{"error": "revoked"})
					}
					return c.JSON(202, output.Body)
				})
				response := httptest.NewRecorder()
				done := make(chan struct{})
				go func() {
					defer close(done)
					router.ServeHTTP(response, httptest.NewRequest(route.method, "/api/environments/"+id+route.path, strings.NewReader(`{}`)).WithContext(ctx))
				}()
				select {
				case <-entered:
				case <-ctx.Done():
					t.Fatal("route did not reach held remote response")
				}
				_, err := environments.UpdateEnvironment(ctx, id, map[string]any{"access_token": strings.Repeat("n", 32)}, nil, nil)
				require.NoError(t, err)
				select {
				case <-done:
				case <-ctx.Done():
					t.Fatal("old-session request was not revoked")
				}
				require.NotEqual(t, route.status, response.Code)
				require.NotContains(t, response.Body.String(), "old-session-admission")
				if tunnel != nil {
					require.True(t, tunnel.Conn.IsClosed())
					require.Zero(t, countBackupPending(tunnel))
				}
			})
		}
	}
}
func countBackupPending(tunnel *edge.AgentTunnel) int {
	n := 0
	tunnel.Pending.Range(func(_, _ any) bool { n++; return true })
	return n
}
