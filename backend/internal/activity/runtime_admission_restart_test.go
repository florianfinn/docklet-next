// Source: docklet-next/issues/318
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-30
// License: Apache-2.0
// Deviations: Original transactional runtime restart exclusion tests.

package activity

import (
	"context"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/environment"
	dto "github.com/florianfinn/docklet-next/types/environment"
	"github.com/stretchr/testify/require"
)

func TestRuntimeRestartGuardBeforeAndAfterTargetRead(t *testing.T) {
	for _, phase := range []string{"before-read", "during-read"} {
		t.Run(phase, func(t *testing.T) {
			f := b1Fixture(t)
			c := f.command()
			reserve := func() {
				now := time.Now()
				require.NoError(t, f.store.svc.db.Create(&environment.AgentRestartRequest{RunID: "example-restart", EnvironmentID: c.Store.EnvironmentID, RegistrationID: c.Store.RegistrationID, State: "reserved", CreatedAt: now, UpdatedAt: now, Deadline: now.Add(3 * time.Minute)}).Error)
			}
			if phase == "before-read" {
				reserve()
			} else {
				f.resolver.resolve = func(_ context.Context, target RuntimeTarget) (RuntimeResolvedTarget, error) {
					reserve()
					return RuntimeResolvedTarget{Target: target}, nil
				}
			}
			result, err := f.service.Admit(t.Context(), c)
			b1Zero(t, result)
			var p *dto.HostProblem
			require.ErrorAs(t, err, &p)
			require.Equal(t, "host.restarting", p.Code)
			require.Equal(t, 409, p.Status)
			if phase == "before-read" {
				require.Zero(t, f.resolver.calls.Load())
			}
			before := runtimeBoundarySnapshot(t, f.store)
			require.Empty(t, before.Receipts)
			require.Empty(t, before.Bindings)
			require.NoError(t, f.store.svc.db.Model(&environment.AgentRestartRequest{}).Where("run_id=?", "example-restart").Updates(map[string]any{"state": "connected", "settled_at": time.Now()}).Error)
			f.resolver.resolve = nil
			_, err = f.service.Admit(t.Context(), c)
			require.NoError(t, err)
		})
	}
}

// A known key keeps its durable answer during a restart; a new key is refused.
func TestRuntimeRestartGuardKeepsKnownKeyAnswer(t *testing.T) {
	f := b1Fixture(t)
	c := f.command()
	first, err := f.service.Admit(t.Context(), c)
	require.NoError(t, err)
	require.NoError(t, f.store.svc.db.Model(&RuntimeReceipt{}).Where("run_id=?", first.ActivityID).Updates(map[string]any{"executor_state": "success", "executor_settled_at": time.Now(), "settled_at": time.Now()}).Error)
	now := time.Now()
	require.NoError(t, f.store.svc.db.Create(&environment.AgentRestartRequest{RunID: "example-restart", EnvironmentID: c.Store.EnvironmentID, RegistrationID: c.Store.RegistrationID, State: "reserved", CreatedAt: now, UpdatedAt: now, Deadline: now.Add(3 * time.Minute)}).Error)
	replay, err := f.service.Admit(t.Context(), c)
	require.NoError(t, err, "a retry of a known key returns its stored result")
	require.Equal(t, first.ActivityID, replay.ActivityID)
	fresh := f.command()
	_, err = f.service.Admit(t.Context(), fresh)
	var p *dto.HostProblem
	require.ErrorAs(t, err, &p)
	require.Equal(t, "host.restarting", p.Code)
}
