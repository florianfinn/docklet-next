// Source: docklet-next/issues/81
// Revision: 6310cbbb69b086b7dd8e68a2d3431e01b89afbb2
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original host registration implementation.

package middleware

import (
	"context"

	"github.com/florianfinn/docklet-next/backend/pkg/projects"
)

type agentCredentialContext struct{}

// WithAgentCredential marks authentication with the configured manager token in agent mode.
func WithAgentCredential(ctx context.Context) context.Context {
	return context.WithValue(ctx, agentCredentialContext{}, true)
}

// WithAgentRequest marks the agent credential and applies the hub's request
// markers, which only the agent-token path may honour.
func WithAgentRequest(ctx context.Context, header func(string) string) context.Context {
	ctx = WithAgentCredential(ctx)
	if header(HeaderPeerNamesHidden) == "1" {
		ctx = projects.WithPeerNamesHidden(ctx)
	}
	return ctx
}

func HasAgentCredential(ctx context.Context) bool {
	authenticated, _ := ctx.Value(agentCredentialContext{}).(bool)
	return authenticated
}
