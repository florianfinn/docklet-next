// Source: docklet-next/issues/349
// Revision: 4aa230d8e013057514050fef6048f1559d6c5a34
// Retrieved: 2026-11-06
// License: Apache-2.0
// Deviations: Original unreachable-agent admission refusal test.

package activity

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"syscall"
	"testing"

	"github.com/florianfinn/docklet-next/backend/pkg/remenv"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// A stopped Direct agent or a disconnected Edge agent refuses a new command with agent.unavailable before admission,
// so a bulk summary can name the reason; nothing is admitted.
func TestM19UnreachableAgentRefusesAdmissionWithItsCode(t *testing.T) {
	for name, cause := range map[string]error{
		"direct-dial-refused": &remenv.TransportError{Err: &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED}},
		"edge-disconnected":   remenv.ErrEnvironmentUnavailable,
	} {
		t.Run(name, func(t *testing.T) {
			f := b1Fixture(t)
			c := f.command()
			mux, path := activationLegacyMux(t, f, c, activationLegacyPermission(c))
			f.service.activity.runtimeHTTP.ports.Store = func(context.Context, string) (RuntimeStoreBinding, error) {
				return RuntimeStoreBinding{}, cause
			}
			f.service.activity.runtimeHTTP.ports.PrepareTarget = func(context.Context, string, RuntimeTarget) (RuntimeResolvedTarget, error) {
				t.Fatal("target prepared for an unreachable agent")
				return RuntimeResolvedTarget{}, nil
			}
			req := httptest.NewRequest(http.MethodPost, path, nil)
			req.Header.Set("Idempotency-Key", uuid.NewString())
			req.Header.Set("Prefer", "respond-async")
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, req)
			require.Equal(t, http.StatusServiceUnavailable, response.Code, response.Body.String())
			require.Contains(t, response.Body.String(), `"code":"agent.unavailable"`)
			require.Empty(t, runtimeBoundarySnapshot(t, f.store).Receipts)
		})
	}
}
