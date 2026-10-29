// Source: docklet-next/issues/292
// Revision: a910be6a64e97a4ce3a422f12451a5301b115f7a
// Retrieved: 2026-10-29
// License: Apache-2.0
// Deviations: Original agent access envelope shared by proxied domain routes and the environment middleware.

package handlerutil

import (
	"context"
	"errors"

	"github.com/florianfinn/docklet-next/backend/pkg/remenv"
)

// AgentAccessError carries the host access envelope the environment
// middleware sends for the same verdict, so proxied domain routes refuse alike.
type AgentAccessError struct {
	Success bool   `json:"success"`
	Code    string `json:"code"`
	status  int
	cause   error
}

func (e *AgentAccessError) Error() string  { return e.Code }
func (e *AgentAccessError) Unwrap() error  { return e.cause }
func (e *AgentAccessError) GetStatus() int { return e.status }

// AgentVerdict maps an agent access verdict (outdated agent, refused token or
// upstream 401/403) to its refusal and returns nil for every other error.
func AgentVerdict(err error) *AgentAccessError {
	var mapped *AgentAccessError
	if errors.As(err, &mapped) {
		return mapped
	}
	var upstream *remenv.StatusError
	if !errors.Is(err, remenv.ErrAgentUpdateRequired) && !errors.Is(err, remenv.ErrAgentInvalidToken) &&
		(!errors.As(err, &upstream) || !remenv.AgentAuthRejected(upstream.StatusCode)) {
		return nil
	}
	return newAgentAccessErrorInternal(err)
}

// AgentAccess extends AgentVerdict with an unreachable or stalled host, so proxied routes
// answer like the middleware; domain statuses and decode failures stay distinct.
func AgentAccess(err error) *AgentAccessError {
	if verdict := AgentVerdict(err); verdict != nil {
		return verdict
	}
	var transport *remenv.TransportError
	if errors.Is(err, remenv.ErrEnvironmentUnavailable) || errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &transport) && transport.Unavailable()) {
		return newAgentAccessErrorInternal(err)
	}
	return nil
}

func newAgentAccessErrorInternal(err error) *AgentAccessError {
	status, code := remenv.AgentAccessFailure(err)
	return &AgentAccessError{Code: code, status: status, cause: err}
}
