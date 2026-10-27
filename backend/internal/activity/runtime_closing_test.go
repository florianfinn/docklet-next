// Source: docklet-next/docs/design/images-updates.md
// Revision: beec37b5b4339a07a946bb56d19fb8062ba8bc66
// Retrieved: 2026-10-27
// License: Apache-2.0
// Deviations: Actual Direct/Edge response boundary and descriptor regression proofs for #229.

package activity

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/florianfinn/docklet-next/backend/pkg/remenv"
	a "github.com/florianfinn/docklet-next/types/activity"
	"github.com/florianfinn/docklet-next/types/contract"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type closingConn struct {
	tunnel  *edge.AgentTunnel
	handler http.Handler
}

func (p *closingConn) Send(m *edge.TunnelMessage) error {
	if m.Type != edge.MessageTypeCommandRequest {
		return nil
	}
	if !edge.ValidateEdgeCommand(m.Command, m.Method, m.Path, false) {
		return ErrRuntimeTransport
	}
	rec := httptest.NewRecorder()
	p.handler.ServeHTTP(rec, httptest.NewRequest(m.Method, m.Path, http.NoBody))
	raw, _ := p.tunnel.Pending.Load(m.ID)
	raw.(*edge.PendingRequest).ResponseCh <- &edge.TunnelMessage{ID: m.ID, Type: edge.MessageTypeResponse, Status: rec.Code, Body: rec.Body.Bytes(), Headers: map[string]string{"Content-Type": "application/json"}}
	return nil
}
func (*closingConn) Receive() (*edge.TunnelMessage, error) { return nil, io.EOF }
func (*closingConn) IsExpectedReceiveError(error) bool     { return true }
func (*closingConn) Close() error                          { return nil }
func (*closingConn) IsClosed() bool                        { return false }
func (*closingConn) Transport() string                     { return "test" }
func closingSender(t *testing.T, isEdge bool, handler http.Handler) *RuntimeSender {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	registry := edge.NewTunnelRegistry()
	if isEdge {
		conn := &closingConn{handler: handler}
		conn.tunnel = edge.NewAgentTunnelWithConn("example", conn)
		registry.Register("example", conn.tunnel)
	}
	sender, err := NewRuntimeSender(RuntimeRemoteTarget{EnvironmentID: "example", Endpoint: server.URL, Token: uuid.NewString(), Edge: isEdge, AgentContractVersion: contract.AgentHubVersion}, func(ctx context.Context, _ string, _ *string, _ string, _ string) (context.Context, context.CancelFunc, error) {
		return ctx, func() {}, nil
	}, server.Client(), registry)
	require.NoError(t, err)
	return sender
}

func TestClosingLifecycleBoundary(t *testing.T) {
	for _, isEdge := range []bool{false, true} {
		for _, tc := range []struct {
			name, code, baseline, want string
			status, wantStatus         int
		}{
			{"undeclared", "undeclared", "", "runtime.outcome-unknown", 409, 503},
			{"unauthorized", "restore.unavailable", "", "runtime.outcome-unknown", 401, 503},
			{"forbidden", "restore.unavailable", "", "runtime.outcome-unknown", 403, 503},
			{"declared", "restore.unavailable", "", "restore.unavailable", 409, 409},
			{"success-status", "restore.unavailable", "", "runtime.outcome-unknown", 200, 503},
			{"zero-status", "restore.unavailable", "", "runtime.outcome-unknown", 0, 503},
			{"oversize-status", "restore.unavailable", "", "runtime.outcome-unknown", 1000, 503},
			{"baseline-code", "restore.unavailable", "undeclared", "runtime.outcome-unknown", 409, 503},
		} {
			t.Run(fmt.Sprintf("edge=%t/%s", isEdge, tc.name), func(t *testing.T) {
				id := a.RuntimeRunIdentity{RuntimeExecutorIdentity: a.RuntimeExecutorIdentity{ManagerID: "example", RegistrationID: "example", ExecutorGeneration: uuid.NewString()}, RunID: uuid.NewString(), RequestDigest: strings.Repeat("a", 64)}
				outcome := remoteAnswer(id, time.Now().UTC(), a.StatusSuccess)
				outcome.Detail.Activity.Status = a.StatusFailed
				outcome.Detail.Activity.MessageCode = string(a.CodeRuntimeFailed)
				rawResult := dto.LifecycleResult{RunID: id.RunID, Code: tc.code, BaselineCode: tc.baseline, HTTPStatus: tc.status}
				outcome.Detail.Activity.Metadata = map[string]any{"imageLifecycle": rawResult}
				outcome.Detail.Activity.ImageLifecycle = &rawResult
				sender := closingSender(t, isEdge, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					body, err := json.Marshal(outcome, json.FormatNilSliceAsNull(true))
					require.NoError(t, err)
					_, _ = w.Write(body)
				}))
				got, err := sender.Query(t.Context(), id)
				require.NoError(t, err)
				require.NotNil(t, got.Detail.Activity.ImageLifecycle)
				require.Equal(t, tc.want, got.Detail.Activity.ImageLifecycle.Code)
				status, code := runtimeWaitFailure(got.Detail.Activity, nil)
				require.Equal(t, tc.wantStatus, status)
				require.Equal(t, tc.want, string(code))
				// The interactive guard also protects retained snapshots from older hubs.
				got.Detail.Activity.ImageLifecycle = &dto.LifecycleResult{Code: tc.code, BaselineCode: tc.baseline, HTTPStatus: tc.status}
				status, code = runtimeWaitFailure(got.Detail.Activity, nil)
				require.Equal(t, tc.wantStatus, status)
				require.Equal(t, tc.want, string(code))
			})
		}
	}
}

func TestClosingPreflightBoundary(t *testing.T) {
	for _, isEdge := range []bool{false, true} {
		for _, status := range []int{401, 403, 409} {
			for _, code := range []string{"undeclared", "job.skipped", "restore.unavailable"} {
				t.Run(fmt.Sprintf("edge=%t/%d/%s", isEdge, status, code), func(t *testing.T) {
					sender := closingSender(t, isEdge, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(status)
						problem := imageDTO.Failure(code, status)
						problem.Problem.Message = "synthetic-private"
						body, err := json.Marshal(problem)
						require.NoError(t, err)
						_, _ = w.Write(body)
					}))
					var result any
					err := sender.metadata(t.Context(), "/api/environments/0/runtime-target", map[string]string{}, &result)
					if status == 401 || status == 403 {
						require.ErrorIs(t, err, remenv.ErrAgentInvalidToken)
					} else if code == "restore.unavailable" {
						var p *imageDTO.OperationError
						require.ErrorAs(t, err, &p)
						require.Equal(t, status, p.Status)
						require.Equal(t, code, p.Problem.Code)
						require.NotContains(t, p.Error(), "synthetic-private")
					} else {
						require.ErrorIs(t, err, ErrRuntimeTransport)
					}
				})
			}
		}
	}
}

func TestClosingPointDescriptors(t *testing.T) {
	for _, env := range []string{"0", "direct", "edge"} {
		for _, admin := range []bool{false, true} {
			for _, orphan := range []bool{false, true} {
				ps := authz.EnvironmentPermissionSet(env)
				if admin {
					ps = authz.SudoPermissionSet()
				}
				ctx := context.WithValue(t.Context(), middleware.ContextKeyUserPermissions, ps)
				reads := 0
				out, err := getCapabilitiesWithPoints(ctx, &GetCapabilitiesInput{Kind: "stack", ID: env + "~stack"}, func(_ context.Context, gotEnv, target, point string) error {
					reads++
					require.Equal(t, env, gotEnv)
					require.Equal(t, "stack", target)
					if orphan {
						return imageDTO.Failure("update.invalid-target", 400)
					}
					return nil
				})
				require.NoError(t, err)
				for _, d := range out.Body.Data {
					if d.OperationID == "listRestorePoints" || d.OperationID == "deleteRestorePoint" {
						require.Equal(t, admin || !orphan, d.Available)
					}
				}
				if admin {
					require.Zero(t, reads)
				} else {
					require.Equal(t, 1, reads)
				}
			}
		}
	}
}

func TestClosingLegacyActivityLifecycle(t *testing.T) {
	for _, code := range []string{"undeclared", "restore.unavailable"} {
		item := a.Activity{ID: "example", Metadata: map[string]any{"imageLifecycle": dto.LifecycleResult{RunID: "example", Code: code, HTTPStatus: 409}}}
		applyActivitySourceInternal(&item, "direct-example", "Example")
		want := code
		if code == "undeclared" {
			want = "runtime.outcome-unknown"
		}
		require.NotNil(t, item.ImageLifecycle)
		require.Equal(t, want, item.ImageLifecycle.Code)
		encoded, err := json.Marshal(item.Metadata)
		require.NoError(t, err)
		if code == "undeclared" {
			require.NotContains(t, string(encoded), "undeclared")
		}
	}
}

func TestClosingRetainedLifecycleBoundary(t *testing.T) {
	p := &remoteTransportProbe{}
	f := remoteFixture(t, p, RuntimeReconcileSchedule{})
	p.execute = func(_ context.Context, e a.RuntimeExecutionEnvelope) (a.RuntimeExecutionOutcome, error) {
		out := remoteAnswer(e.Identity, f.store.now, a.StatusSuccess)
		out.Detail.Activity.Status = a.StatusFailed
		out.Detail.Activity.MessageCode = string(a.CodeRuntimeFailed)
		out.Detail.Activity.Metadata = map[string]any{"imageLifecycle": dto.LifecycleResult{RunID: e.Identity.RunID, Code: "undeclared", HTTPStatus: 409}}
		return out, nil
	}
	f.start(t)
	snapshot, err := f.control.Snapshot(t.Context(), f.ref)
	require.NoError(t, err)
	require.NotNil(t, snapshot.ImageLifecycle)
	require.Equal(t, "runtime.outcome-unknown", snapshot.ImageLifecycle.Code)
	require.Equal(t, 503, snapshot.ImageLifecycle.HTTPStatus)
	var row Activity
	require.NoError(t, f.store.svc.db.First(&row, "id = ?", f.ref.RunID).Error)
	raw, err := json.Marshal(row.Metadata)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "undeclared")
}
