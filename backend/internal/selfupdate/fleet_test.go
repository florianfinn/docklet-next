// Source: docklet-next/issues/682
// Revision: 99c1b0b8c4dfe56182c72749e092b08202436fa5
// Retrieved: 2026-11-27
// License: Apache-2.0
// Deviations: Original tests for the combined update plan store.

package selfupdate

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestDispatchMarkerIsAcquiredOnceFromReserved(t *testing.T) {
	store := openStore(t, filepath.Join(t.TempDir(), "state.db"))
	defer closeStore(t, store)
	op := operationFixture()
	_, _, err := store.Reserve(t.Context(), op, nil)
	require.NoError(t, err)
	acquired, err := store.AcquireDispatch(t.Context(), op.OpID, nil)
	require.NoError(t, err)
	require.True(t, acquired)
	for range 2 {
		acquired, err = store.AcquireDispatch(t.Context(), op.OpID, nil)
		require.NoError(t, err)
		require.False(t, acquired, "a second caller acquired the dispatch marker")
	}
	current, err := store.Read(t.Context())
	require.NoError(t, err)
	require.Equal(t, "helper-started", current.Phase)
}

func TestFleetPlanDecodesOnlyItsOwnSchema(t *testing.T) {
	plan := FleetPlan{SchemaVersion: FleetSchemaVersion, FleetID: "fleet", RunID: "run", Phase: "hub", TargetVersion: "1.0.1", Hub: FleetTarget{EnvironmentID: "0", OpID: FleetOpID("fleet", "0")}, Agents: []FleetTarget{{EnvironmentID: "a", OpID: FleetOpID("fleet", "a")}}}
	store := openStore(t, filepath.Join(t.TempDir(), "plan.db"))
	defer closeStore(t, store)
	fleet := FleetStore{DB: store.DB.DB}
	require.NoError(t, fleet.Admit(t.Context(), plan, nil))
	read, _, err := fleet.Read(t.Context())
	require.NoError(t, err)
	require.Equal(t, plan.Agents[0].OpID, read.Agents[0].OpID)
	require.True(t, ValidOpID(read.Hub.OpID))
	// A tampered operation id or another version is unreadable, never replanned.
	changed, err := fleet.Change(t.Context(), "fleet", func(_ *gorm.DB, p *FleetPlan) error { p.Agents[0].OpID = "other"; return nil })
	require.NoError(t, err)
	require.True(t, changed)
	_, _, err = fleet.Read(t.Context())
	require.ErrorIs(t, err, ErrFleetUnreadable)
	for _, raw := range []string{`{"schemaVersion":2,"fleetId":"f","runId":"r","phase":"hub","targetVersion":"1","hub":{"opId":"x"}}`, `{"schemaVersion":1}`, `[`} {
		_, err := DecodeFleet(raw)
		require.ErrorIs(t, err, ErrFleetUnreadable, raw)
	}
}
