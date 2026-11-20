// Source: docklet-next/docs/design/runtime-control.md
// Revision: d4229b124f079376c959142b63235f6a739be2a9711e0228ec5a63ab552f8cf0
// Retrieved: 2026-11-20
// License: Apache-2.0
// Deviations: Original peer identity confirmation contract for issue 531.

package activity

import (
	"context"
	"encoding/json"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type runtimePeerSinkKey struct{}

// Called in the admission transaction after the activity exists, or just before
// effects. Only a fresh resolution can supply actual-use evidence.
func runtimePeerEvidenceTx(tx *gorm.DB, now time.Time, runID, digest string, used *imageDTO.RuntimePeerOverrides) error {
	evidence := "{}"
	if used != nil {
		if imageDTO.SafeRuntimePeerOverrides(used) == nil || digest == "" {
			return errRuntimeInvalidInternal
		}
		encoded, err := json.Marshal(used)
		if err != nil {
			return err
		}
		evidence = string(encoded)
	}
	var row RuntimeReceipt
	if err := tx.First(&row, "run_id = ?", runID).Error; err != nil {
		return err
	}
	if used == nil && row.PeerOverrides != "" {
		evidence = row.PeerOverrides
	}
	if err := tx.Model(&RuntimeReceipt{}).Where("run_id = ?", runID).UpdateColumns(map[string]any{"acknowledged_uncertainty": digest, "peer_overrides": evidence}).Error; err != nil {
		return err
	}
	if used == nil || row.PeerOverrides == evidence {
		return nil
	}
	// Display data excludes IDs; receipt storage retains the bounded identity proof.
	peers := make([]map[string]any, 0, min(used.Count, 16))
	for _, peer := range used.Peers[:min(len(used.Peers), 16)] {
		peers = append(peers, map[string]any{"name": peer.Name, "reason": peer.Reason, "file": peer.File})
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&ActivityMessage{BaseModel: database.BaseModel{ID: uuid.NewSHA1(uuid.NameSpaceOID, []byte(runID+":runtime.peer-acknowledged:"+evidence)).String(), CreatedAt: now, UpdatedAt: new(now)}, ActivityID: runID, Level: activitytypes.MessageLevelInfo, Code: "runtime.peer-acknowledged", Message: "runtime.peer-acknowledged", Payload: database.JSON{"count": used.Count, "peers": peers}}).Error
}

func (s *RuntimeService) recordRuntimePeerEvidence(ctx context.Context, runID, digest string, used *imageDTO.RuntimePeerOverrides) error {
	return s.transactionInternal(ctx, func(tx *gorm.DB, now time.Time) error { return runtimePeerEvidenceTx(tx, now, runID, digest, used) })
}
