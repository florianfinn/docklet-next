// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Successful host health and authenticated Edge reconnect recovery boundary.

package environment

import (
	"context"
	"log/slog"
)

// SetRuntimeRecovery installs the hub's query-only recovery boundary at construction.
func (s *EnvironmentService) SetRuntimeRecovery(recover func(context.Context, string) error) {
	s.runtimeRecovery = recover
}

// RecoverRuntime runs after a successful health check or authenticated Edge reconnect.
func (s *EnvironmentService) RecoverRuntime(ctx context.Context, id string) {
	if id != "0" && s.runtimeRecovery != nil {
		if err := s.runtimeRecovery(ctx, id); err != nil {
			slog.WarnContext(ctx, "Runtime recovery scheduling failed", "environmentId", id, "code", "runtime.recovery-failed")
		}
	}
}

// CheckReconnectedAgent admits recovery only after the new Edge session proves
// compatibility. The caller can then deliver the hub's configuration metadata.
func (s *EnvironmentService) CheckReconnectedAgent(ctx context.Context, id string) error {
	if _, err := s.CheckAgentCompatibility(ctx, id); err != nil {
		return err
	}
	s.RecoverRuntime(ctx, id)
	return nil
}
