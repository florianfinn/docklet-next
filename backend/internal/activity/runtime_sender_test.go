// Source: docklet-next/docs/design/runtime-control.md
// Revision: 230a4a3cb92c59cc15e1f6767abd18f017e05ca4b83835524aa8c20e50a89e46
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Bounded remote runtime implementation, active in production since #125.

package activity

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/florianfinn/docklet-next/backend/pkg/remenv"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"github.com/florianfinn/docklet-next/types/contract"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestRuntimeSenderActualHTTPStrictRefusalAndNoRedirect(t *testing.T) {
	for _, mode := range []string{"busy", "cancelling", "generic-conflict", "wrong-status", "extra", "duplicate", "null", "trailing", "oversize", "text", "redirect", "drop"} {
		t.Run(mode, func(t *testing.T) {
			var calls, redirects, cleanups atomic.Int32
			credential := uuid.NewString()
			destination := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { redirects.Add(1) }))
			defer destination.Close()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				require.Equal(t, credential, r.Header.Get("X-Arcane-Agent-Token"))
				require.Empty(t, r.Header.Get("Idempotency-Key"))
				require.Empty(t, r.Header.Get("Authorization"))
				_, _ = io.Copy(io.Discard, r.Body)
				w.Header().Set("Content-Type", "application/json")
				status := 409
				body := `{"code":"run.busy"}`
				switch mode {
				case "cancelling":
					body = `{"code":"run.cancelling"}`
				case "generic-conflict":
					body = `{"code":"CONFLICT","message":"Runtime identity conflict","statusCode":409}`
				case "wrong-status":
					status = 503
				case "extra":
					body = `{"code":"run.busy","activeRunId":"example-hidden"}`
				case "duplicate":
					body = `{"code":"run.busy","code":"run.busy"}`
				case "null":
					body = `null`
				case "trailing":
					body += ` {}`
				case "oversize":
					body += strings.Repeat(" ", activitytypes.RuntimeMaxBodyBytes)
				case "text":
					w.Header().Set("Content-Type", "text/plain")
				case "redirect":
					w.Header().Set("Location", destination.URL)
					status = 307
				case "drop":
					conn, _, err := http.NewResponseController(w).Hijack()
					require.NoError(t, err)
					_ = conn.Close()
					return
				}
				w.WriteHeader(status)
				_, _ = io.WriteString(w, body)
			}))
			defer server.Close()
			sender, err := NewRuntimeSender(RuntimeRemoteTarget{EnvironmentID: "example-remote", Endpoint: server.URL, Token: credential, AgentContractVersion: contract.AgentHubVersion}, func(ctx context.Context, _ string, _ *string, _ string, _ string) (context.Context, context.CancelFunc, error) {
				return ctx, func() { cleanups.Add(1) }, nil
			}, server.Client(), nil)
			require.NoError(t, err)
			id := activitytypes.RuntimeRunIdentity{RuntimeExecutorIdentity: activitytypes.RuntimeExecutorIdentity{ManagerID: "example-manager", RegistrationID: "example-registration", ExecutorGeneration: uuid.NewString()}, RunID: uuid.NewString(), RequestDigest: strings.Repeat("a", 64)}
			_, err = sender.Execute(t.Context(), activitytypes.RuntimeExecutionEnvelope{Identity: id})
			require.Error(t, err)
			var refusal *RuntimeExecutionRefused
			if mode == "busy" || mode == "cancelling" {
				require.ErrorAs(t, err, &refusal)
			} else {
				require.False(t, errors.As(err, &refusal))
				require.ErrorIs(t, err, ErrRuntimeTransport)
			}
			require.EqualValues(t, 1, calls.Load())
			require.Zero(t, redirects.Load())
			require.EqualValues(t, 1, cleanups.Load())
		})
	}
}
func TestRuntimeSenderAccessAndOldContractRefuseBeforeTransport(t *testing.T) {
	for _, isEdge := range []bool{false, true} {
		t.Run(map[bool]string{false: "direct", true: "edge"}[isEdge], func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
			defer server.Close()
			target := RuntimeRemoteTarget{EnvironmentID: "example-remote", Endpoint: server.URL, Token: uuid.NewString(), Edge: isEdge, AgentContractVersion: contract.MinimumAgentVersion - 1}
			access := func(ctx context.Context, _ string, _ *string, _ string, _ string) (context.Context, context.CancelFunc, error) {
				return nil, nil, remenv.ErrAgentUpdateRequired
			}
			_, err := NewRuntimeSender(target, access, server.Client(), nil)
			require.ErrorIs(t, err, remenv.ErrAgentUpdateRequired)
			target.AgentContractVersion = contract.AgentHubVersion
			sender, err := NewRuntimeSender(target, access, server.Client(), nil)
			require.NoError(t, err)
			id := activitytypes.RuntimeRunIdentity{RuntimeExecutorIdentity: activitytypes.RuntimeExecutorIdentity{ManagerID: "example-manager", RegistrationID: "example-registration", ExecutorGeneration: uuid.NewString()}, RunID: uuid.NewString(), RequestDigest: strings.Repeat("a", 64)}
			_, err = sender.Query(t.Context(), id)
			require.ErrorIs(t, err, remenv.ErrAgentUpdateRequired)
			require.Zero(t, calls.Load())
			if isEdge {
				sender.access = func(ctx context.Context, _ string, _ *string, _ string, _ string) (context.Context, context.CancelFunc, error) {
					return ctx, func() {}, nil
				}
				_, err = sender.Query(t.Context(), id)
				require.ErrorIs(t, err, ErrRuntimeTransport)
				require.Zero(t, calls.Load(), "Edge-only has no direct fallback")
			}
		})
	}
}
