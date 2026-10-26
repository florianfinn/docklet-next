// Source: docklet-next/docs/design/update-policies.md
// Revision: 839b98c10360da9b7beada6804d70d3da98ac8ed
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Producer and transport regression proofs for #229 point projection.

package middleware

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
	"time"

	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
)

type pointTestConn struct {
	sendErr error
	tunnel  *edge.AgentTunnel
	handler http.Handler
}

func (p *pointTestConn) Send(msg *edge.TunnelMessage) error {
	if p.sendErr != nil {
		return p.sendErr
	}
	if msg.Type != edge.MessageTypeCommandRequest {
		return nil
	}
	if msg.Headers["Accept-Encoding"] != "" || msg.Headers["Origin"] != "" {
		return errors.New("browser header forwarded to point executor")
	}
	if !edge.ValidateEdgeCommand(msg.Command, msg.Method, msg.Path, false) {
		return errors.New("invalid command")
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(msg.Method, msg.Path, bytes.NewReader(msg.Body))
	p.handler.ServeHTTP(rec, req)
	raw, _ := p.tunnel.Pending.Load(msg.ID)
	raw.(*edge.PendingRequest).ResponseCh <- &edge.TunnelMessage{ID: msg.ID, Type: edge.MessageTypeResponse, Status: rec.Code, Body: rec.Body.Bytes(), Headers: map[string]string{"Content-Type": "application/json"}}
	return nil
}
func (*pointTestConn) Receive() (*edge.TunnelMessage, error) { return nil, io.EOF }
func (*pointTestConn) IsExpectedReceiveError(error) bool     { return true }
func (*pointTestConn) Close() error                          { return nil }
func (*pointTestConn) IsClosed() bool                        { return false }
func (*pointTestConn) Transport() string                     { return "test" }

func pointBody(t *testing.T) []byte {
	t.Helper()
	body, err := json.Marshal(map[string]any{"success": true, "data": []dto.RestorePoint{{ID: "point", RegistrationID: "registration", TargetID: "stack", TargetIdentity: "identity", SourceRunID: "run", CreatedAt: time.Unix(100, 0).UTC(), SourceState: "settled", HoldState: dto.HoldNone, Images: []dto.PointImage{}}}})
	require.NoError(t, err)
	return body
}

func pointProxy(t *testing.T, isEdge bool, body *[]byte, state *dto.RestorePointHoldState, dbErr *bool, deletes *int, agentStatus ...int) *echo.Echo {
	t.Helper()
	mw := newTestEnvironmentMiddleware()
	agent := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			*deletes++
			w.WriteHeader(204)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if len(agentStatus) > 0 {
			w.WriteHeader(agentStatus[0])
		}
		_, _ = w.Write(*body)
	})
	server := httptest.NewServer(agent)
	t.Cleanup(server.Close)
	target := server.URL
	if isEdge {
		conn := &pointTestConn{handler: agent}
		tunnel := edge.NewAgentTunnelWithConn("example", conn)
		conn.tunnel = tunnel
		mw.registry.Register("example", tunnel)
		target = "edge://example"
	}
	mw.resolver = func(context.Context, string) (string, *string, bool, error) { return target, nil, true, nil }
	mw.pointHolds = func(context.Context, string, []string) (map[string]dto.RestorePointHoldState, error) {
		if *dbErr {
			return nil, errors.New("synthetic database failure")
		}
		return map[string]dto.RestorePointHoldState{"point": *state}, nil
	}
	router := echo.New()
	group := attachMiddleware(router, mw)
	group.Any("/environments/:id/*", func(c *echo.Context) error { return c.NoContent(418) })
	return router
}
func pointCall(router *echo.Echo, method, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(method, "/api/environments/example"+path, http.NoBody))
	return rec
}

func TestDirectPointHoldOverlay(t *testing.T) { testPointHoldOverlay(t, false) }
func TestEdgePointHoldOverlay(t *testing.T)   { testPointHoldOverlay(t, true) }
func testPointHoldOverlay(t *testing.T, isEdge bool) {
	body := pointBody(t)
	state := dto.HoldActive
	dbErr := false
	deletes := 0
	router := pointProxy(t, isEdge, &body, &state, &dbErr, &deletes)
	for _, hold := range []dto.RestorePointHoldState{dto.HoldActive, dto.HoldUnknown} {
		state = hold
		for _, path := range []string{"/updater/project/stack/restore-points", "/updater/restore-points"} {
			rec := pointCall(router, http.MethodGet, path)
			require.Equal(t, 200, rec.Code, rec.Body.String())
			require.Contains(t, rec.Body.String(), `"holdState":"`+string(hold)+`"`)
			rec = pointCall(router, http.MethodDelete, path+"/point")
			require.Equal(t, 409, rec.Code, rec.Body.String())
			require.Contains(t, rec.Body.String(), "restore.in-use")
			require.Zero(t, deletes)
		}
	}
	state = dto.HoldNone
	require.Equal(t, 204, pointCall(router, http.MethodDelete, "/updater/restore-points/point").Code)
	require.Equal(t, 1, deletes)
	dbErr = true
	for _, method := range []string{http.MethodGet, http.MethodDelete} {
		path := "/updater/restore-points"
		if method == http.MethodDelete {
			path += "/point"
		}
		rec := pointCall(router, method, path)
		require.Equal(t, 500, rec.Code)
		require.Contains(t, rec.Body.String(), "update.persistence-failed")
	}
	require.Equal(t, 1, deletes)
}

func TestPointOverlayValidation(t *testing.T) {
	for _, isEdge := range []bool{false, true} {
		t.Run(map[bool]string{false: "direct", true: "edge"}[isEdge], func(t *testing.T) {
			good := pointBody(t)
			body := good
			state := dto.HoldNone
			dbErr := false
			deletes := 0
			router := pointProxy(t, isEdge, &body, &state, &dbErr, &deletes)
			bad := [][]byte{append([]byte(`{"success":false,`), good[1:]...), bytes.Replace(good, []byte(`"id":"point"`), []byte{'"', 'i', 'd', '"', ':', '"', 0xff, '"'}, 1), bytes.Replace(good, []byte(`"sourceState"`), []byte(`"code":"undeclared","sourceState"`), 1), bytes.Replace(good, []byte(`"images":[]`), []byte(`"images":[{}]`), 1), bytes.Replace(good, []byte(`"sourceState":"settled"`), []byte(`"sourceState":"undeclared"`), 1), []byte(`{"success":true,"data":null}`), []byte(`{"success":true,"data":{}}`), []byte(`[]`), []byte(`{"success":true,"data":[null]}`), []byte(`{"success":false,"data":[]}`)}
			for _, field := range []string{"protectionReason", "holdState", "failedAttempt"} {
				var envelope map[string]any
				require.NoError(t, json.Unmarshal(good, &envelope))
				point := envelope["data"].([]any)[0].(map[string]any)
				for _, value := range []any{"MISSING", "undeclared", 42} {
					if value == "MISSING" {
						delete(point, field)
					} else {
						point[field] = value
					}
					raw, err := json.Marshal(envelope)
					require.NoError(t, err)
					bad = append(bad, raw)
				}
			}
			var duplicated struct {
				Success bool              `json:"success"`
				Data    []json.RawMessage `json:"data"`
			}
			require.NoError(t, json.Unmarshal(good, &duplicated))
			duplicated.Data = append(duplicated.Data, duplicated.Data[0])
			dupBody, e := json.Marshal(duplicated)
			require.NoError(t, e)
			bad = append(bad, dupBody)
			for _, raw := range bad {
				body = raw
				rec := pointCall(router, http.MethodGet, "/updater/restore-points")
				require.Equal(t, 503, rec.Code, string(raw))
				require.Contains(t, rec.Body.String(), "update.check-unavailable")
			}
			var envelope struct {
				Success bool              `json:"success"`
				Data    []json.RawMessage `json:"data"`
			}
			require.NoError(t, json.Unmarshal(good, &envelope))
			one := envelope.Data[0]
			// Distinct IDs prevent duplicate-identity validation from masking the count bound.
			envelope.Data = nil
			for i := 0; i < dto.MaxRestorePointResponseCount; i++ {
				var row map[string]any
				require.NoError(t, json.Unmarshal(one, &row))
				row["id"] = strings.Repeat("p", i/26+1) + string(rune('a'+i%26))
				raw, err := json.Marshal(row)
				require.NoError(t, err)
				envelope.Data = append(envelope.Data, raw)
			}
			body, _ = json.Marshal(envelope)
			require.Equal(t, 200, pointCall(router, http.MethodGet, "/updater/restore-points").Code)
			envelope.Data = append(envelope.Data, one)
			body, _ = json.Marshal(envelope)
			require.Equal(t, 503, pointCall(router, http.MethodGet, "/updater/restore-points").Code)
			body = append(append([]byte{}, good...), bytes.Repeat([]byte(" "), dto.MaxRestorePointResponseBytes-len(good))...)
			require.Equal(t, 200, pointCall(router, http.MethodGet, "/updater/restore-points").Code)
			body = append(body, ' ')
			require.Equal(t, 503, pointCall(router, http.MethodGet, "/updater/restore-points").Code)
			require.Zero(t, deletes)
		})
	}
}

func TestPointProxyRevokedGrants(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodDelete} {
		grants := []string{"projects:read", "image-updates:read"}
		path := "/updater/project/stack/restore-points"
		if method == http.MethodDelete {
			grants = []string{"projects:deploy", "projects:update", "images:pull", "image-updates:check", "activities:read"}
			path += "/point"
		}
		for _, missing := range grants {
			mw := newTestEnvironmentMiddleware()
			ps := authz.NewPermissionSet()
			for _, grant := range grants {
				if grant != missing {
					ps.AddEnv("example", grant)
				} else {
					ps.AddEnv("foreign", grant)
				}
			}
			mw.authValidator = func(context.Context, *echo.Context) (*authz.PermissionSet, *user.Actor, bool) { return ps, nil, true }
			router := echo.New()
			group := attachMiddleware(router, mw)
			group.Any("/environments/:id/*", func(c *echo.Context) error { t.Error("reached route"); return c.NoContent(418) })
			rec := pointCall(router, method, path)
			require.Equal(t, 403, rec.Code, missing)
		}
	}
}

func TestPointEvidenceMissingPort(t *testing.T) {
	mw := &EnvironmentMiddleware{}
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(httptest.NewRequest(http.MethodDelete, "/", http.NoBody), rec)
	handled, err := mw.checkPointDelete(c, "example", "point")
	require.NoError(t, err)
	require.True(t, handled)
	require.Equal(t, 500, rec.Code)
	rec = httptest.NewRecorder()
	c = echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/", http.NoBody), rec)
	require.NoError(t, mw.writePointList(c, "example", 200, http.Header{}, bytes.NewReader(pointBody(t))))
	require.Equal(t, 500, rec.Code)
}

func TestEdgePointListRequestHeaders(t *testing.T) {
	body := pointBody(t)
	state := dto.HoldNone
	dbErr := false
	deletes := 0
	router := pointProxy(t, true, &body, &state, &dbErr, &deletes)
	req := httptest.NewRequest(http.MethodGet, "/api/environments/example/updater/restore-points", http.NoBody)
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("Origin", "https://example.com")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, 200, rec.Code, rec.Body.String())
}

func TestPointOverlayErrorStatus(t *testing.T) {
	for _, isEdge := range []bool{false, true} {
		for _, status := range []int{404, 500} {
			body := []byte(`{"success":false,"code":"undeclared"}`)
			state := dto.HoldNone
			dbErr := false
			deletes := 0
			router := pointProxy(t, isEdge, &body, &state, &dbErr, &deletes, status)
			rec := pointCall(router, http.MethodGet, "/updater/restore-points")
			require.Equal(t, 503, rec.Code)
			require.Contains(t, rec.Body.String(), "update.check-unavailable")
			require.NotContains(t, rec.Body.String(), "undeclared")
			require.Zero(t, deletes)
		}
	}
}

func TestPointOverlayKeepsExecutorHold(t *testing.T) {
	for _, isEdge := range []bool{false, true} {
		t.Run(map[bool]string{false: "direct", true: "edge"}[isEdge], func(t *testing.T) {
			for _, agentHold := range []dto.RestorePointHoldState{dto.HoldActive, dto.HoldUnknown} {
				body := bytes.Replace(pointBody(t), []byte(`"holdState":"none"`), []byte(`"holdState":"`+string(agentHold)+`"`), 1)
				hubHold := dto.HoldNone
				dbErr := false
				deletes := 0
				router := pointProxy(t, isEdge, &body, &hubHold, &dbErr, &deletes)
				for _, path := range []string{"/updater/project/stack/restore-points", "/updater/restore-points"} {
					rec := pointCall(router, http.MethodGet, path)
					require.Equal(t, 200, rec.Code)
					require.Contains(t, rec.Body.String(), `"holdState":"`+string(agentHold)+`"`)
				}
			}
		})
	}
}
func TestPointOverlayTypedAgentErrors(t *testing.T) {
	for _, isEdge := range []bool{false, true} {
		t.Run(map[bool]string{false: "direct", true: "edge"}[isEdge], func(t *testing.T) {
			for _, problem := range []*imageDTO.OperationError{imageDTO.Failure("update.persistence-failed", 500), imageDTO.Failure("restore.unavailable", 409), imageDTO.Failure("update.invalid-target", 400), imageDTO.Failure("update.permission-denied", 403)} {
				body, err := json.Marshal(problem)
				require.NoError(t, err)
				hold := dto.HoldNone
				dbErr := false
				deletes := 0
				rec := pointCall(pointProxy(t, isEdge, &body, &hold, &dbErr, &deletes, problem.Status), http.MethodGet, "/updater/restore-points")
				require.Equal(t, problem.Status, rec.Code)
				var got imageDTO.OperationError
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
				require.Equal(t, problem.Problem, got.Problem)
			}
		})
	}
}

type pointErrorTransport struct{ err error }

func (p pointErrorTransport) RoundTrip(*http.Request) (*http.Response, error) { return nil, p.err }
func TestPointOverlayTransportFailures(t *testing.T) {
	for _, isEdge := range []bool{false, true} {
		t.Run(map[bool]string{false: "direct", true: "edge"}[isEdge], func(t *testing.T) {
			for _, failure := range []error{errors.New("synthetic connection failure"), context.DeadlineExceeded} {
				mw := newTestEnvironmentMiddleware()
				target := "https://example.com"
				mw.httpClient = &http.Client{Transport: pointErrorTransport{failure}}
				if isEdge {
					target = "edge://example"
					mw.registry.Register("example", edge.NewAgentTunnelWithConn("example", &pointTestConn{sendErr: failure}))
				}
				mw.resolver = func(context.Context, string) (string, *string, bool, error) { return target, nil, true, nil }
				router := echo.New()
				group := attachMiddleware(router, mw)
				group.Any("/environments/:id/*", func(c *echo.Context) error { return c.NoContent(418) })
				rec := pointCall(router, http.MethodGet, "/updater/restore-points")
				require.Equal(t, 502, rec.Code)
				require.Contains(t, rec.Body.String(), `"code":"agent.unavailable"`)
			}
		})
	}
}
