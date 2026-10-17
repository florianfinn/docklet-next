// Source: docklet-next/issues/82
// Revision: 579ca3a12d93da7c7a6e68686d20425a642423fc
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original cached inventory regression tests.

package environment

import (
	"github.com/florianfinn/docklet-next/backend/pkg/hoststate"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestDeleteEnvironmentForgetsOnlyItsSnapshots(t *testing.T) {
	db := setupEnvironmentServiceTestDB(t)
	svc := NewEnvironmentService(db, nil, nil, nil, nil, nil)
	createTestEnvironmentWithState(t, db, "host-a", "edge://agent", string(EnvironmentStatusPending), true, nil)
	a := hoststate.Key{HostID: "host-a", List: "/containers?limit=100"}
	other := hoststate.Key{HostID: "host-b", List: a.List}
	svc.snapshots.Put(a, []byte(`{"success":true,"data":[]}`))
	svc.snapshots.Put(other, []byte(`{"success":true,"data":[]}`))
	require.NoError(t, svc.DeleteEnvironment(t.Context(), "host-a", nil, nil))
	_, retained := svc.snapshots.Get(a)
	require.False(t, retained, "removed host must release its retained inventory")
	_, retained = svc.snapshots.Get(other)
	require.True(t, retained, "removal must preserve other hosts")
}
