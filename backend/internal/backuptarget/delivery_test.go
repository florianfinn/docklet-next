// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original full-set delivery, lost-ack and receipt integrity proofs.

package backuptarget

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	dto "github.com/florianfinn/docklet-next/types/backup"
	"github.com/florianfinn/docklet-next/types/base"
	"github.com/stretchr/testify/require"
)

func TestDeliveryLostAckReplayAndReceiptIntegrity(t *testing.T) {
	hub, agent := testStore(t), testStore(t)
	target, err := NewService(Dependencies{Store: hub}).Create(t.Context(), dto.TargetInput{TargetLocation: dto.TargetLocation{Kind: "smb", Server: "nas.example.com", Share: "archive"}, Name: "Archive"})
	require.NoError(t, err)
	lost := true
	corrupt := false
	calls := 0
	proxy := func(ctx context.Context, host, method, path string, payload []byte, output any) error {
		calls++
		require.Equal(t, "host-a", host)
		require.Equal(t, http.MethodPut, method)
		require.Equal(t, "/api/environments/0/volumes/backups/targets", path)
		var set dto.TargetSet
		require.NoError(t, json.Unmarshal(payload, &set))
		receipt, err := agent.Apply(ctx, set)
		require.NoError(t, err)
		if lost {
			lost = false
			return errors.New("lost acknowledgement")
		}
		if corrupt {
			receipt.ContentHash = "wrong"
		}
		*output.(*base.ApiResponse[dto.TargetSetReceipt]) = base.ApiResponse[dto.TargetSetReceipt]{Success: true, Data: receipt}
		return nil
	}
	_, err = SyncHost(t.Context(), hub.DB, "host-a", proxy)
	code(t, err, "backup.target.sync-failed")
	receipt, err := SyncHost(t.Context(), hub.DB, "host-a", proxy)
	require.NoError(t, err)
	held, err := agent.Resolve(t.Context(), target.ID, "host-a", true)
	require.NoError(t, err)
	require.Equal(t, target.ConfigVersion, held.ConfigVersion)
	require.Equal(t, 2, calls)
	recorded, err := readSetting(hub.DB.DB, "backupTargetDelivery:host-a")
	require.NoError(t, err)
	var saved dto.TargetSetReceipt
	require.NoError(t, json.Unmarshal([]byte(recorded), &saved))
	require.Equal(t, receipt, saved)
	require.NotContains(t, recorded, held.RecoveryKey)
	corrupt = true
	_, err = SyncHost(t.Context(), hub.DB, "host-a", proxy)
	code(t, err, "backup.target.stale-set")
	after, err := readSetting(hub.DB.DB, "backupTargetDelivery:host-a")
	require.NoError(t, err)
	require.Equal(t, recorded, after, "an unverified receipt must not replace delivery evidence")
}
