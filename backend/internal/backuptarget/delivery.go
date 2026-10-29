// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original metadata-only full-set delivery and authority receipt verification.

package backuptarget

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	dto "github.com/florianfinn/docklet-next/types/backup"
	"github.com/florianfinn/docklet-next/types/base"
)

type JSONProxy func(context.Context, string, string, string, []byte, any) error

// SyncHost uses the existing authenticated AgentAccess transport. It never
// creates a Docker volume, tests a mount or includes secrets in its receipt.
func SyncHost(ctx context.Context, db *database.DB, hostID string, proxy JSONProxy) (dto.TargetSetReceipt, error) {
	store := &Store{DB: db}
	set, err := store.SetForHost(ctx, hostID)
	if err != nil {
		return dto.TargetSetReceipt{}, err
	}
	payload, err := json.Marshal(set)
	if err != nil {
		return dto.TargetSetReceipt{}, err
	}
	var response base.ApiResponse[dto.TargetSetReceipt]
	if err = proxy(ctx, hostID, http.MethodPut, "/api/environments/0/volumes/backups/targets", payload, &response); err != nil {
		if access := AgentAccess(err); access != nil {
			return dto.TargetSetReceipt{}, access
		}
		return dto.TargetSetReceipt{}, dto.NewProblem(502, "backup.target.sync-failed", err)
	}
	receipt := response.Data
	if !response.Success || receipt.Authority != set.Authority || receipt.SetVersion != set.SetVersion || receipt.ContentHash != set.ContentHash {
		return receipt, dto.NewProblem(409, "backup.target.stale-set", nil)
	}
	encoded, err := json.Marshal(receipt)
	if err != nil {
		return receipt, err
	}
	if err = writeSetting(db.WithContext(ctx), "backupTargetDelivery:"+hostID, string(encoded)); err != nil {
		return receipt, err
	}
	if len(receipt.Conflicts) > 0 {
		return receipt, dto.NewProblem(409, "backup.target.conflict", nil)
	}
	return receipt, nil
}
