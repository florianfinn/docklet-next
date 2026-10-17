// Source: docklet-next/issues/81
// Revision: 6310cbbb69b086b7dd8e68a2d3431e01b89afbb2
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original host registration implementation.

package middleware

import "context"

type agentCredentialContext struct{}

// WithAgentCredential marks authentication with the configured manager token in agent mode.
func WithAgentCredential(ctx context.Context) context.Context {
	return context.WithValue(ctx, agentCredentialContext{}, true)
}

func HasAgentCredential(ctx context.Context) bool {
	authenticated, _ := ctx.Value(agentCredentialContext{}).(bool)
	return authenticated
}
