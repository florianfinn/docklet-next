// Source: docklet-next/issues/284
// Revision: e5218d798f5302f7fa568a6059a70901bcc83219
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original agent compatibility refusal for central backup routes.

package backuptarget

import (
	"context"
	"errors"

	"github.com/florianfinn/docklet-next/backend/pkg/remenv"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
)

// AgentAccessError carries the host compatibility envelope the environment
// proxy sends for the same verdict, so central backup routes refuse alike.
type AgentAccessError struct {
	Success bool   `json:"success"`
	Code    string `json:"code"`
	status  int
	cause   error
}

func (e *AgentAccessError) Error() string  { return e.Code }
func (e *AgentAccessError) Unwrap() error  { return e.cause }
func (e *AgentAccessError) GetStatus() int { return e.status }

// AgentAccess maps an agent access verdict (outdated agent, refused token or
// upstream 401/403) to its refusal and returns nil for every other error.
func AgentAccess(err error) *AgentAccessError {
	var mapped *AgentAccessError
	if errors.As(err, &mapped) {
		return mapped
	}
	var upstream *remenv.StatusError
	if !errors.Is(err, remenv.ErrAgentUpdateRequired) && !errors.Is(err, remenv.ErrAgentInvalidToken) &&
		(!errors.As(err, &upstream) || !remenv.AgentAuthRejected(upstream.StatusCode)) {
		return nil
	}
	status, code := remenv.AgentAccessFailure(err)
	return &AgentAccessError{Code: code, status: status, cause: err}
}

// RemoteJSON keeps the agent access verdict that the shared proxy
// translation reports as a generic server error.
func RemoteJSON[T any](ctx context.Context, proxy handlerutil.RemoteJSONProxy, environmentID, method, path string, body any) (*T, error) {
	var raw error
	record := handlerutil.RemoteJSONProxy(func(ctx context.Context, id, m, p string, b []byte, out any) error {
		raw = proxy(ctx, id, m, p, b, out)
		return raw
	})
	output, err := record.JSON[T](ctx, environmentID, method, path, body)
	if err != nil && raw != nil {
		if access := AgentAccess(raw); access != nil {
			return nil, access
		}
	}
	return output, err
}
