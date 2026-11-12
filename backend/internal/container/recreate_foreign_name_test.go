// Source: docklet-next/issues/375
// Revision: 32e24a1b5260c98b6ce490ce2c1f6f50413c1e83
// Retrieved: 2026-11-07
// License: Apache-2.0
// Deviations: Original recovery decision-table cases for a foreign claim on a reserved recreate name.

package container

import (
	"testing"
	"time"

	"github.com/google/uuid"
	dc "github.com/moby/moby/api/types/container"
	"github.com/stretchr/testify/require"
)

const foreignID = "8888888888888888888888888888888888888888888888888888888888888888"

// A container created under the reserved new name between the name check and
// our create: recovery restores the original and never touches the foreign one,
// unless the foreign container also holds the original's name.
func TestRecreateRecoveryForeignClaimOnReservedName(t *testing.T) {
	for _, tc := range []struct {
		name, newName, foreignName, code string
		restored                         bool
	}{
		{name: "new name only compensates", newName: "app-renamed", foreignName: "app-renamed", code: "container.recreate-recovered", restored: true},
		{name: "old name kept as new name stays ambiguous", newName: "app", foreignName: "app", code: "container.recreate-ambiguous"},
		{name: "backup name stays ambiguous", newName: "app-renamed", foreignName: "backup", code: "container.recreate-ambiguous"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, d, _, db := updateFixture(t)
			run := uuid.NewString()
			j := RecreateJournal{RunID: run, OldID: updateOldID, OldName: "app", BackupName: "app-docklet-recreate-" + run, NewName: tc.newName, Phase: "intent-create", WasRunning: true, UpdatedAt: time.Now().UTC().Add(-time.Minute)}
			require.NoError(t, db.Create(&j).Error)
			old := d.containers[updateOldID]
			old.Name = "/" + j.BackupName
			old.State.Running = false
			d.containers[updateOldID] = old
			foreignName := tc.foreignName
			if foreignName == "backup" {
				// The rename never happened and someone else holds the backup
				// name: no safe compensation.
				foreignName = j.BackupName
				old.Name = "/app"
				d.containers[updateOldID] = old
			}
			d.containers[foreignID] = dc.InspectResponse{ID: foreignID, Name: "/" + foreignName, Image: updateOtherImage, State: &dc.State{Running: true}, Config: &dc.Config{Labels: map[string]string{}}}
			c, err := s.dockerService.GetClient(t.Context())
			require.NoError(t, err)

			result, err := s.recoverRecreate(t.Context(), c, &j)
			require.NoError(t, err)
			require.Equal(t, tc.code, result.Code)
			foreign := d.containers[foreignID]
			require.Equal(t, "/"+foreignName, foreign.Name, "the foreign container keeps its name")
			require.True(t, foreign.State.Running, "the foreign container keeps running")
			require.Zero(t, d.counts["remove"], "nothing is removed")
			if tc.restored {
				require.Equal(t, "/app", d.containers[updateOldID].Name)
				require.True(t, d.containers[updateOldID].State.Running)
				require.Equal(t, "rolled-back", j.Phase)
			} else {
				require.Equal(t, "ambiguous", j.Phase)
				require.Zero(t, d.counts["rename"]+d.counts["start"], "an ambiguous journal runs no effect")
			}
		})
	}
}
