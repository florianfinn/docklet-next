// Source: docklet-next/issues/81
// Revision: 6310cbbb69b086b7dd8e68a2d3431e01b89afbb2
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original host registration implementation.

package edge

import (
	"context"
	"errors"
)

type sessionKeyContext struct{}

func WithSessionKey(ctx context.Context, keyID string) context.Context {
	return context.WithValue(ctx, sessionKeyContext{}, keyID)
}

// RequireSessionKey prevents sending a replacement token into an older tunnel.
func RequireSessionKey(ctx context.Context, tunnel *AgentTunnel) error {
	expected, checked := ctx.Value(sessionKeyContext{}).(string)
	if checked && (tunnel == nil || tunnel.APIKeyID != expected) {
		if tunnel != nil {
			_ = tunnel.CloseWithReason("agent credential revoked")
		}
		return errors.New("agent tunnel credential is no longer current")
	}
	return nil
}

func (s *TunnelServer) SetSessionKeyResolver(resolve func(context.Context, string, string) (string, error)) {
	s.sessionKeyResolver = resolve
}

func (s *TunnelServer) bindSessionKey(ctx context.Context, tunnel *AgentTunnel, token string) error {
	if s.sessionKeyResolver == nil {
		return nil
	}
	keyID, err := s.sessionKeyResolver(ctx, tunnel.EnvironmentID, token)
	if err != nil {
		_ = tunnel.CloseWithReason("agent credential revoked")
		return err
	}
	tunnel.APIKeyID = keyID
	return nil
}

// ForgetPollState removes both observed liveness and pending tunnel demand.
func ForgetPollState(envID string) {
	polls := GetPollRuntimeRegistry()
	polls.mu.Lock()
	delete(polls.states, envID)
	polls.mu.Unlock()
	demands := GetDemandRegistry()
	demands.mu.Lock()
	delete(demands.demands, envID)
	demands.mu.Unlock()
}
