// Source: docklet-next/issues/439
// Revision: 910032a54830f5ce1d37070964fa2dc7efb8aa2f
// Retrieved: 2026-11-27
// License: Apache-2.0
// Deviations: Original correlated run reservation and settlement, moved from service.go; combined update link (#682).

package upgrade

import (
	"context"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	core "github.com/florianfinn/docklet-next/backend/internal/selfupdate"
	activityDTO "github.com/florianfinn/docklet-next/types/activity"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// fleetLink ties an operation to a combined update: its run joins the parent's
// batch, and only that parent's own activity is exempt from hub admission.
type fleetLink struct {
	parentRunID  string
	registeredAt time.Time
}

func (s *Service) reserveRun(ctx context.Context, store *core.Store, op core.Operation, userID, username string, link *fleetLink) (core.Operation, bool, error) {
	return store.Reserve(ctx, op, func(tx *gorm.DB, current *core.Operation) error {
		except := ""
		if link != nil {
			except = link.parentRunID
		}
		if err := activity.CheckHostUpdateAdmissionExceptTx(tx, current.EnvironmentID, except); err != nil {
			return err
		}
		if current.EnvironmentID != "0" {
			if err := s.boundTx(tx, *current); err != nil {
				return err
			}
			if link != nil {
				if err := registeredTx(tx, current.EnvironmentID, link.registeredAt); err != nil {
					return err
				}
			}
		}
		if s.role() == "agent" {
			return nil
		}
		now := time.Now().UTC()
		kind := activityDTO.TypeHubUpdate
		if current.EnvironmentID != "0" {
			kind = activityDTO.TypeAgentUpdate
		}
		run := activity.Activity{BaseModel: database.BaseModel{ID: uuid.NewString(), CreatedAt: now}, EnvironmentID: current.EnvironmentID, Type: kind, Status: activityDTO.StatusRunning, StartedAt: now, StartedByUserID: new(userID), StartedByUsername: new(username), MessageCode: string(activityDTO.CodeRunStarted), ResourceType: new("system"), ResourceID: new(current.EnvironmentID), Metadata: database.JSON{"opId": current.OpID, "targetVersion": current.TargetVersion, "targetReference": current.TargetRef}}
		if link != nil {
			run.BatchID = new(link.parentRunID)
		}
		if err := tx.Create(&run).Error; err != nil {
			return err
		}
		current.RunID = run.ID
		return nil
	})
}
func (s *Service) settleRun(tx *gorm.DB, op core.Operation) error {
	if op.RunID == "" {
		return nil
	}
	status := activityDTO.StatusFailed
	code := "activity.failed"
	if op.Outcome == "healthy" {
		status = activityDTO.StatusSuccess
		code = "activity.success"
	}
	if op.Code != "" {
		code = op.Code
	}
	now := time.Now().UTC()
	return tx.Model(&activity.Activity{}).Where("id=? AND status IN ?", op.RunID, []activityDTO.Status{activityDTO.StatusRunning, activityDTO.StatusQueued}).Updates(map[string]any{"status": status, "message_code": code, "ended_at": now, "updated_at": now}).Error
}
