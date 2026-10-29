// Source: docklet-next/issues/296
// Revision: a910be6a64e97a4ce3a422f12451a5301b115f7a
// Retrieved: 2026-10-29
// License: Apache-2.0
// Deviations: Original notification and Auto Heal foundation.

package notification

// Problem carries stable codes and a submitted-field pointer, never provider text.
type Problem struct {
	Code   string `json:"code" enum:"notification.config-invalid,notification.hub-only,notification.dispatch-invalid,notification.dispatch-unsupported,notification.dispatch-stale,notification.test-busy"`
	Field  string `json:"field,omitempty"`
	Status int    `json:"status"`
}

func (p *Problem) Error() string { return p.Code }

// DispatchOrigin identifies the credential and optional Edge session that received an event.
type DispatchOrigin struct{ EnvironmentID, APIKeyID, TokenHash, SessionID string }

func (p *Problem) GetStatus() int { return p.Status }
