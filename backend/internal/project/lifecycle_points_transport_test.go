// Source: docklet-next/docs/design/update-policies.md
// Revision: 839b98c10360da9b7beada6804d70d3da98ac8ed
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Producer and transport regression proofs for #229 point projection.

package project

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	life "github.com/florianfinn/docklet-next/backend/internal/project/children/lifecycle"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
)

type pointOrderConn struct {
	tunnel  *edge.AgentTunnel
	handler http.Handler
	lost    *bool
}

func (p *pointOrderConn) Send(msg *edge.TunnelMessage) error {
	if msg.Type != edge.MessageTypeCommandRequest {
		return nil
	}
	if !edge.ValidateEdgeCommand(msg.Command, msg.Method, msg.Path, false) {
		return errors.New("invalid command")
	}
	rec := httptest.NewRecorder()
	p.handler.ServeHTTP(rec, httptest.NewRequest(msg.Method, msg.Path, bytes.NewReader(msg.Body)))
	if *p.lost && msg.Method == http.MethodDelete {
		return context.DeadlineExceeded
	}
	pending, _ := p.tunnel.Pending.Load(msg.ID)
	pending.(*edge.PendingRequest).ResponseCh <- &edge.TunnelMessage{ID: msg.ID, Type: edge.MessageTypeResponse, Status: rec.Code, Body: rec.Body.Bytes(), Headers: map[string]string{"Content-Type": "application/json"}}
	return nil
}
func (*pointOrderConn) Receive() (*edge.TunnelMessage, error) { return nil, io.EOF }
func (*pointOrderConn) IsExpectedReceiveError(error) bool     { return true }
func (*pointOrderConn) Close() error                          { return nil }
func (*pointOrderConn) IsClosed() bool                        { return false }
func (*pointOrderConn) Transport() string                     { return "test" }

func TestPointDeleteOrdersTransport(t *testing.T) {
	for _, transport := range []string{"direct", "edge"} {
		for _, order := range []string{"delete-first", "admitted-not-dispatched", "late-delete", "running", "unknown-delete"} {
			t.Run(transport+"/"+order, func(t *testing.T) {
				f := newLifecycleFixture(t)
				snapshot := f.establishBaseline()
				store := f.service.lifecycleStore()
				point, err := store.Prepare(t.Context(), snapshot, uuid.NewString(), "", "settled")
				require.NoError(t, err)
				require.NoError(t, store.Available(t.Context(), point.ID))
				require.NoError(t, f.db.Model(&life.Run{}).Where("point_id = ?", point.ID).Update("settled", true).Error)
				effects, deletes := 0, 0
				hold := dto.HoldNone
				lost := false
				selected, resume := make(chan struct{}), make(chan struct{})
				execute := func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					var err error
					switch {
					case r.Method == http.MethodDelete:
						deletes++
						err = store.Delete(r.Context(), point.ID)
					case r.Method == http.MethodPost:
						if order == "late-delete" {
							close(selected)
							<-resume
						}
						_, err = store.Prepare(r.Context(), snapshot, uuid.NewString(), point.ID, "settled")
						if err == nil {
							effects++
						}
					default:
						rows, e := f.service.ListImageRestorePoints(r.Context(), f.project.ID, true)
						err = e
						if err == nil {
							_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": rows})
							return
						}
					}
					if err != nil {
						problem := imageDTO.Neutral(err, "update.persistence-failed", 500)
						w.WriteHeader(problem.Status)
						_ = json.NewEncoder(w).Encode(problem)
						return
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"success": true})
				}
				agent := http.HandlerFunc(execute)
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if lost && r.Method == http.MethodDelete {
						rec := httptest.NewRecorder()
						execute(rec, r)
						conn, _, e := w.(http.Hijacker).Hijack()
						require.NoError(t, e)
						_ = conn.Close()
						return
					}
					execute(w, r)
				}))
				t.Cleanup(server.Close)
				registry := edge.NewTunnelRegistry()
				target := server.URL
				var tunnel *edge.AgentTunnel
				if transport == "edge" {
					conn := &pointOrderConn{handler: agent, lost: &lost}
					tunnel = edge.NewAgentTunnelWithConn("example", conn)
					conn.tunnel = tunnel
					registry.Register("example", tunnel)
					target = "edge://example"
				}
				router := echo.New()
				group := router.Group("/api")
				group.Use(middleware.NewEnvProxyMiddlewareWithParamAndRegistry("0", "id", func(context.Context, string) (string, *string, bool, error) { return target, nil, true, nil }, func(context.Context, *echo.Context) (*authz.PermissionSet, *user.Actor, bool) {
					return authz.SudoPermissionSet(), nil, true
				}, nil, registry, nil, middleware.EnvironmentProxySecurity{PointHolds: func(context.Context, string, []string) (map[string]dto.RestorePointHoldState, error) {
					return map[string]dto.RestorePointHoldState{point.ID: hold}, nil
				}}))
				group.Any("/environments/:id/*", func(c *echo.Context) error { return c.NoContent(418) })
				deletePoint := func() *httptest.ResponseRecorder {
					rec := httptest.NewRecorder()
					router.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/environments/example/updater/restore-points/"+point.ID, http.NoBody))
					return rec
				}
				selectPoint := func() (int, []byte) {
					path := "/api/environments/0/runtime-runs"
					if transport == "edge" {
						status, _, body, e := edge.ProxyRequest(t.Context(), tunnel, http.MethodPost, path, "", nil, []byte(`{}`))
						require.NoError(t, e)
						return status, body
					}
					response, e := http.Post(server.URL+path, "application/json", strings.NewReader(`{}`))
					require.NoError(t, e)
					defer response.Body.Close()
					body, e := io.ReadAll(response.Body)
					require.NoError(t, e)
					return response.StatusCode, body
				}
				if order == "admitted-not-dispatched" {
					committed, release := make(chan struct{}), make(chan struct{})
					go func() { hold = dto.HoldActive; close(committed); <-release }()
					<-committed
					rec := deletePoint()
					close(release)
					require.Equal(t, 409, rec.Code, rec.Body.String())
					require.Zero(t, deletes)
					require.Zero(t, effects)
					return
				}
				if order == "running" {
					status, body := selectPoint()
					require.Equal(t, 200, status, string(body))
					require.Equal(t, 1, effects)
					rec := deletePoint()
					require.Equal(t, 409, rec.Code, rec.Body.String())
					require.Contains(t, rec.Body.String(), "restore.in-use")
					return
				}
				if order == "late-delete" {
					type reply struct {
						status int
						body   []byte
					}
					result := make(chan reply, 1)
					go func() { status, body := selectPoint(); result <- reply{status, body} }()
					<-selected
					rec := deletePoint()
					require.Equal(t, 200, rec.Code, rec.Body.String())
					close(resume)
					response := <-result
					require.Equal(t, 409, response.status, string(response.body))
					require.Contains(t, string(response.body), "restore.unavailable")
					require.Zero(t, effects)
					require.Equal(t, 1, deletes)
					return
				}
				if order == "unknown-delete" {
					lost = true
				}
				rec := deletePoint()
				if lost {
					require.GreaterOrEqual(t, rec.Code, 500)
					list := httptest.NewRecorder()
					router.ServeHTTP(list, httptest.NewRequest(http.MethodGet, "/api/environments/example/updater/restore-points", http.NoBody))
					require.Equal(t, 200, list.Code)
					require.Contains(t, list.Body.String(), `"data":[]`)
				} else {
					require.Equal(t, 200, rec.Code, rec.Body.String())
				}

				status, body := selectPoint()
				require.Equal(t, 409, status, string(body))
				require.Contains(t, string(body), "restore.unavailable")
				require.Zero(t, effects)
				require.Equal(t, 1, deletes)
			})
		}
	}
}
