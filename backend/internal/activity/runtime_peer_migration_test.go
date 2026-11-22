// Source: docklet-next/docs/design/runtime-control.md
// Revision: d4229b124f079376c959142b63235f6a739be2a9711e0228ec5a63ab552f8cf0
// Retrieved: 2026-11-20
// License: Apache-2.0
// Deviations: Original peer identity confirmation contract for issue 531; #573 undoes migration 7 first.

package activity

import (
	"context"
	"strings"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	"github.com/stretchr/testify/require"
)

func TestS4MigrationPreservesLegacyReceiptsAndReopenedConfirmation(t *testing.T) {
	f := b1Fixture(t)
	c := f.command()
	result, err := f.service.Admit(t.Context(), c)
	require.NoError(t, err)
	var original RuntimeReceipt
	require.NoError(t, f.store.svc.db.First(&original, "run_id = ?", result.ActivityID).Error)
	// Reconstruct the shipped v5 schema with a real retained receipt, then run the
	// same forward migrator used at startup. No earlier migration is edited.
	// Later migrations are undone first so the version history stays contiguous.
	for _, sql := range []string{
		"ALTER TABLE environments DROP COLUMN network_route", "ALTER TABLE environments DROP COLUMN tailnet_node_id",
		"ALTER TABLE environments DROP COLUMN tailnet_tagged", "ALTER TABLE environments DROP COLUMN tailnet_observed_at",
		"DELETE FROM goose_db_version WHERE version_id = 7",
		"ALTER TABLE runtime_receipts DROP COLUMN acknowledged_uncertainty", "ALTER TABLE runtime_receipts DROP COLUMN peer_overrides", "DELETE FROM goose_db_version WHERE version_id = 6"} {
		require.NoError(t, f.store.svc.db.Exec(sql).Error)
	}
	require.NoError(t, f.store.svc.db.Migrate(t.Context()))
	var migrated RuntimeReceipt
	require.NoError(t, f.store.svc.db.First(&migrated, "run_id = ?", result.ActivityID).Error)
	require.Empty(t, migrated.AcknowledgedUncertainty, "legacy receipts acquire the empty acknowledgement default")
	require.JSONEq(t, "{}", migrated.PeerOverrides, "legacy receipts acquire empty actual-use evidence")
	require.Equal(t, original.RequestDigest, migrated.RequestDigest)
	require.Equal(t, original.GateKey, migrated.GateKey)
	digest := strings.Repeat("a", 64)
	require.NoError(t, f.store.svc.db.Model(&RuntimeReceipt{}).Where("run_id = ?", result.ActivityID).Update("acknowledged_uncertainty", digest).Error)
	require.NoError(t, f.store.svc.db.Close())
	reopened, err := database.InitializeHub(context.Background(), sqlitestore.URL(f.store.path), database.MigrationOptions{})
	require.NoError(t, err)
	defer reopened.Close()
	var restored RuntimeReceipt
	require.NoError(t, reopened.First(&restored, "run_id = ?", result.ActivityID).Error)
	require.Equal(t, digest, restored.AcknowledgedUncertainty, "confirmation survives a database connection restart")
	require.JSONEq(t, "{}", restored.PeerOverrides)
}
