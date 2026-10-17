// Source: docklet-next/issues/81
// Revision: 6310cbbb69b086b7dd8e68a2d3431e01b89afbb2
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original host registration implementation.

package remenv

import (
	"context"
	"errors"
	"net/http"
)

var (
	ErrAgentUpdateRequired = errors.New("agent update required")
	ErrAgentInvalidToken   = errors.New("invalid environment access token")
)

func AgentAuthRejected(status int) bool {
	return status == http.StatusUnauthorized
}

// AgentAccessFailure keeps host connectivity failures distinct from caller authentication.
func AgentAccessFailure(err error) (int, string) {
	if errors.Is(err, ErrAgentUpdateRequired) {
		return http.StatusConflict, "agent.update-required"
	}
	var status *StatusError
	if errors.Is(err, ErrAgentInvalidToken) || (errors.As(err, &status) && (AgentAuthRejected(status.StatusCode) || status.StatusCode == http.StatusForbidden)) {
		return http.StatusBadGateway, "agent.invalid-token"
	}
	return http.StatusBadGateway, "agent.unavailable"
}

// AgentAccessCheck binds a forwarded operation to the current host credential.
type AgentAccessCheck func(context.Context, string, *string, string, string) (context.Context, context.CancelFunc, error)
