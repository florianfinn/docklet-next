// Source: docklet-next/issues/81
// Revision: 579ca3a12d93da7c7a6e68686d20425a642423fc
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original stale credential observer regression tests.

package environment

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/types/version"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type staleAgentObserver struct{}

func TestAgentStaleCredentialObserverDoesNotRevokeCurrentSession(t *testing.T) {
	for _, mode := range []string{"direct", "edge"} {
		for _, change := range []string{"token", "endpoint", "key-id"} {
			t.Run(mode+"/"+change, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
				defer cancel()
				f := newAgentFixture(t, mode, version.Info{})
				_, finish, err := f.service.AgentAccess(ctx, f.id, &f.token, http.MethodGet, "/api/health")
				require.NoError(t, err)
				defer finish()
				observed, release := make(chan struct{}), make(chan struct{})
				var blocked atomic.Bool
				require.NoError(t, f.service.db.Callback().Query().After("gorm:query").Register("test:stale-credential", func(db *gorm.DB) {
					if db.Statement.Context.Value(staleAgentObserver{}) == true && blocked.CompareAndSwap(false, true) {
						close(observed)
						select {
						case <-release:
						case <-ctx.Done():
						}
					}
				}))
				done := make(chan error, 1)
				joined := make(chan struct{})
				go func() {
					defer close(joined)
					_, cleanup, err := f.service.AgentAccess(context.WithValue(ctx, staleAgentObserver{}, true), f.id, &f.token, http.MethodGet, "/api/health")
					if cleanup != nil {
						cleanup()
					}
					done <- err
				}()
				defer func() { cancel(); <-joined }()
				select {
				case <-observed:
				case <-ctx.Done():
					t.Fatal("older request did not read its credential snapshot")
				}
				updates := map[string]any{}
				currentToken := f.token
				switch change {
				case "token":
					currentToken = "test-only"
					updates["access_token"] = currentToken
				case "endpoint":
					updates["api_url"] = "http://replacement.example.com"
					if mode == "edge" {
						updates["api_url"] = "edge://replacement"
					}
					updates["access_token"] = currentToken
				case "key-id":
					updates["api_key_id"] = "test-only"
				}
				_, err = f.service.UpdateEnvironment(ctx, f.id, updates, nil, nil)
				require.NoError(t, err)
				streamCtx, stopStream, err := f.service.AgentAccess(ctx, f.id, &currentToken, http.MethodGet, "/api/health")
				require.NoError(t, err)
				defer stopStream()
				f.service.agentRuntime.mu.Lock()
				current := f.service.agentRuntime.sessions[f.id]
				f.service.agentRuntime.mu.Unlock()
				close(release)
				select {
				case err := <-done:
					require.NoError(t, current.ctx.Err(), "an older credential observer must not revoke the newer session")
					require.ErrorIs(t, err, ErrInvalidEnvironmentAccessToken)
				case <-ctx.Done():
					t.Fatal("older credential observer remained blocked")
				}
				require.NoError(t, streamCtx.Err(), "the current stream must remain open")
				f.service.agentRuntime.mu.Lock()
				selected := f.service.agentRuntime.sessions[f.id]
				f.service.agentRuntime.mu.Unlock()
				require.Same(t, current, selected)
				require.Empty(t, f.calls, "credential session selection must not forward requests")
			})
		}
	}
}
