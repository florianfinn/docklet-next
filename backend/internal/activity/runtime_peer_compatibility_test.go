// Source: docklet-next/docs/design/runtime-control.md
// Revision: d4229b124f079376c959142b63235f6a739be2a9711e0228ec5a63ab552f8cf0
// Retrieved: 2026-11-20
// License: Apache-2.0
// Deviations: Original peer identity confirmation contract for issue 531.

package activity_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/florianfinn/docklet-next/backend/pkg/remenv"
	a "github.com/florianfinn/docklet-next/types/activity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type s4CapturedWriter struct {
	*httptest.ResponseRecorder
	underlying http.ResponseWriter
}

func (w *s4CapturedWriter) Unwrap() http.ResponseWriter { return w.underlying }

func TestS4LegacyAgentAndHubContractsDirectEdge(t *testing.T) {
	for _, isEdge := range []bool{false, true} {
		t.Run(map[bool]string{false: "direct", true: "edge"}[isEdge], func(t *testing.T) {
			f := newRemoteIntegration(t, isEdge)
			f.agent.project("example-target", "target", "name: target_stack\n"+identitySDKServices)
			req := a.RuntimeTargetRequest{Registration: a.RuntimeExecutorIdentity{ManagerID: f.store.ManagerID, RegistrationID: f.store.RegistrationID, ExecutorGeneration: f.store.ExecutorGeneration}, Target: activity.RuntimeTarget{Kind: "stack", ID: "example-target"}}
			access := func(ctx context.Context, id string, _ *string, _, _ string) (context.Context, context.CancelFunc, error) {
				ctx = edge.WithSessionCredential(ctx, "example-key", f.token, nil)
				if isEdge {
					tunnel, ok := f.tunnels.Get(id).Get()
					require.True(t, ok)
					ctx = edge.WithSessionTunnel(ctx, tunnel)
				}
				bound, cancel := context.WithCancel(ctx)
				return bound, cancel, nil
			}
			old, err := activity.NewRuntimeSender(activity.RuntimeRemoteTarget{EnvironmentID: f.store.EnvironmentID, Endpoint: f.endpoint, Token: f.token, Edge: isEdge, AgentContractVersion: 19}, access, nil, f.tunnels)
			require.NoError(t, err)
			previous := f.receiverHandler
			var bodies []string
			f.handlerMu.Lock()
			f.receiverHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				bodies = append(bodies, string(body))
				require.NotContains(t, string(body), "acknowledgedUncertainty", "old agents must never receive the new member")
				r.Body = io.NopCloser(strings.NewReader(string(body)))
				previous.ServeHTTP(w, r)
			})
			f.handlerMu.Unlock()
			_, err = old.Resolve(t.Context(), req)
			require.NoError(t, err, "ordinary old-agent metadata remains valid")
			command := f.command(activity.RuntimeStartContainer, activity.RuntimeTarget{Kind: "container", ID: runtimeContainerID})
			admitted, err := f.hub.Admit(t.Context(), command)
			require.NoError(t, err)
			var row activity.RuntimeReceipt
			require.NoError(t, f.db.First(&row, "run_id = ?", admitted.ActivityID).Error)
			envelope := a.RuntimeExecutionEnvelope{Identity: a.RuntimeRunIdentity{RuntimeExecutorIdentity: req.Registration, RunID: row.RunID, RequestDigest: row.RequestDigest}, Operation: command.Operation, Target: a.RuntimeTarget{Kind: *row.TargetKind, ID: *row.TargetID, Name: row.TargetName, ParentProjectID: row.ParentProjectID}, Options: command.Options, Attribution: a.RuntimeAttribution{ID: command.Actor.ID, Username: command.Actor.Username}}
			_, err = old.Execute(t.Context(), envelope)
			require.NoError(t, err, "empty-acknowledgement actions execute through the old-agent wire shape")
			result, err := f.agent.waitLocal(row.RunID)
			require.NoError(t, err)
			require.Equal(t, a.StatusSuccess, result.Status)
			require.NotEmpty(t, identitySDKEffects(f.agent.http.snapshot()))
			effectCount := len(f.agent.http.snapshot())
			before := len(bodies)
			req.AcknowledgedUncertainty = strings.Repeat("a", 64)
			_, err = old.Resolve(t.Context(), req)
			require.ErrorIs(t, err, remenv.ErrAgentUpdateRequired)
			require.Len(t, bodies, before, "confirmed old-agent requests must fail before dispatch")
			_, err = old.Execute(t.Context(), a.RuntimeExecutionEnvelope{Options: a.RuntimeOptions{AcknowledgedUncertainty: req.AcknowledgedUncertainty}})
			require.ErrorIs(t, err, remenv.ErrAgentUpdateRequired)
			require.Len(t, bodies, before)
			f.agent.project("example-peer", "peer", "name: ${PEER_NAME:?required}\n"+identitySDKServices)
			req.AcknowledgedUncertainty = ""
			_, oldRefusal := old.Resolve(t.Context(), req)
			legacyBlocker := s4RuntimeProblem(t, oldRefusal)
			require.False(t, legacyBlocker.Confirmable, "a host below v20 must not expose confirmation even when a newer receiver responds")
			require.Empty(t, legacyBlocker.PeerID)
			require.Nil(t, legacyBlocker.Uncertainty)
			for _, header := range []string{"", "invalid", "19"} {
				f.handlerMu.Lock()
				f.receiverHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					r.Header.Set(a.RuntimeHubContractHeader, header)
					rec := httptest.NewRecorder()
					previous.ServeHTTP(&s4CapturedWriter{ResponseRecorder: rec, underlying: w}, r)
					var body struct {
						Error struct {
							Blocker map[string]any `json:"blocker"`
						} `json:"error"`
					}
					assert.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
					assert.Len(t, body.Error.Blocker, 3, "unannounced hubs get only the legacy three-field blocker: header=%q status=%d body=%s", header, rec.Code, rec.Body.String())
					assert.Equal(t, "compose.identity-unreadable", body.Error.Blocker["reason"])
					for key, values := range rec.Header() {
						w.Header()[key] = values
					}
					w.WriteHeader(rec.Code)
					_, _ = w.Write(rec.Body.Bytes())
				})
				f.handlerMu.Unlock()
				req.AcknowledgedUncertainty = ""
				_, err = f.sender.Resolve(t.Context(), req)
				b := s4RuntimeProblem(t, err)
				require.False(t, b.Confirmable)
				require.Nil(t, b.Uncertainty)
				require.Empty(t, b.PeerID)
			}
			f.handlerMu.Lock()
			f.receiverHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				r.Header.Del(a.RuntimeHubContractHeader)
				previous.ServeHTTP(w, r)
			})
			f.handlerMu.Unlock()
			req.AcknowledgedUncertainty = strings.Repeat("a", 64)
			_, err = f.sender.Resolve(t.Context(), req)
			require.Error(t, err, "missing hub announcement cannot authorize a confirmation")
			require.Empty(t, identitySDKEffects(f.agent.http.snapshot()[effectCount:]))
			require.EqualValues(t, 1, f.executeCalls.Load())
		})
	}
}
