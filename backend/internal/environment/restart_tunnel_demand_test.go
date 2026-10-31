// Source: docklet-next/issues/318
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original restart tunnel-demand renewal test after a hub restart.

package environment

import (
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/stretchr/testify/require"
)

// After a hub restart the process-local tunnel demand is empty; observing a
// recovered Edge restart renews it so a poll-mode agent reopens its tunnel.
func TestRestartObservationRenewsEdgeTunnelDemandAfterReopen(t *testing.T) {
	f := newRestartFixture(t)
	id := "example-edge-restart-demand"
	require.NoError(t, f.c.service.db.Create(&Environment{ID: id, Name: "Example edge host", ApiUrl: "edge://example", IsEdge: true, Enabled: true, AccessToken: new("example")}).Error)
	w := &restartWork{row: AgentRestartRequest{RunID: "example-run", EnvironmentID: id, RegistrationID: id, OldInstance: "example-old", State: "awaiting", CreatedAt: f.now, UpdatedAt: f.now, SentAt: &f.now, Deadline: f.now.Add(2 * time.Minute)}}
	require.NoError(t, f.c.service.db.Create(&w.row).Error)
	f.reopen(t, w)
	require.Equal(t, edge.TunnelStatusIdle, edge.GetDemandRegistry().DesiredStatus(id, false, time.Now()), "a fresh hub process has no demand")
	var env Environment
	require.NoError(t, f.c.service.db.First(&env, "id=?", id).Error)
	_, _ = f.c.observe(t.Context(), w.row, env)
	require.Equal(t, edge.TunnelStatusRequired, edge.GetDemandRegistry().DesiredStatus(id, false, time.Now()))
	require.Equal(t, edge.TunnelStatusIdle, edge.GetDemandRegistry().DesiredStatus(id, false, w.row.Deadline.Add(time.Second)), "demand ends with the stored deadline")
}
