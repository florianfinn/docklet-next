// Source: florianfinn/docklet-next/backend/internal/activity
// Revision: 65c9e0dceb6f994ac884904e2a6b7348f75580fa
// Retrieved: 2026-10-14
// License: Apache-2.0
// Deviations: New durable activity admission gate.

package activity

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/dbutil"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	usertypes "github.com/florianfinn/docklet-next/types/user"
	"gorm.io/gorm"
)

const CheckFreshness = 30 * time.Second
const checkFreshnessOperator = ">"

type GateRequest struct {
	EnvironmentID    string
	Operation        string
	Scope            string
	Type             activitytypes.Type
	ResourceType     string
	ResourceID       string
	StartedBy        *usertypes.Actor
	AffectedServices []string
	Check            bool
	Recheck          bool
}

type GateAnswer struct {
	Run  activitytypes.Activity `json:"run"`
	Code string                 `json:"code"`
	// WorkContext belongs only to run.started and identifies its local worker.
	WorkContext context.Context `json:"-"`
}

type GateConflict struct {
	RunID string `json:"runId"`
	Code  string `json:"code"`
}

func (e *GateConflict) Error() string { return e.Code }

// StartGatedActivity serializes starts for one resource in one immediate transaction.
// Only run.started owns execution.
func (s *ActivityService) StartGatedActivity(ctx context.Context, request GateRequest) (GateAnswer, error) {
	return s.startGatedActivityAtInternal(ctx, request)
}

func (s *ActivityService) startGatedActivityAtInternal(ctx context.Context, request GateRequest) (GateAnswer, error) {
	if err := s.checkInitInternal(); err != nil {
		return GateAnswer{}, err
	}
	request.EnvironmentID = strings.TrimSpace(request.EnvironmentID)
	if request.EnvironmentID == "" {
		request.EnvironmentID = "0"
	}
	request.Operation = strings.TrimSpace(request.Operation)
	request.Scope = strings.TrimSpace(request.Scope)
	if request.Operation == "" || request.Scope == "" || request.Type == "" {
		return GateAnswer{}, errors.New("operation, scope and type are required")
	}

	var model Activity
	code := string(activitytypes.CodeRunStarted)
	s.runningMu.Lock()
	defer s.runningMu.Unlock()
	admit := func(tx *gorm.DB) error {
		now := s.nowInternal().UTC().Truncate(time.Microsecond)
		var active Activity
		err := tx.Where("environment_id = ? AND gate_scope = ? AND status IN ?", request.EnvironmentID, request.Scope, []activitytypes.Status{activitytypes.StatusQueued, activitytypes.StatusRunning}).Order("started_at DESC, id DESC").First(&active).Error
		if err == nil {
			if s.cancelling[active.ID] && active.GateKey == request.Operation {
				return &GateConflict{RunID: active.ID, Code: string(activitytypes.CodeRunCancelling)}
			}
			if active.GateKey != request.Operation {
				return &GateConflict{RunID: active.ID, Code: string(activitytypes.CodeRunBusy)}
			}
			model = active
			code = string(activitytypes.CodeRunJoined)
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if request.Check && !request.Recheck {
			var fresh Activity
			err = tx.Where("environment_id = ? AND gate_scope = ? AND gate_key = ? AND status = ? AND ended_at "+checkFreshnessOperator+" ?", request.EnvironmentID, request.Scope, request.Operation, activitytypes.StatusSuccess, now.Add(-CheckFreshness)).Order("ended_at DESC, id DESC").First(&fresh).Error
			if err == nil {
				model = fresh
				code = string(activitytypes.CodeRunFresh)
				return nil
			}
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
		}
		model = Activity{
			EnvironmentID:    request.EnvironmentID,
			Type:             request.Type,
			Status:           activitytypes.StatusRunning,
			MessageCode:      string(activitytypes.CodeRunStarted),
			ResourceType:     new(request.ResourceType),
			ResourceID:       new(request.ResourceID),
			AffectedServices: servicesJSONInternal(request.AffectedServices),
			GateKey:          request.Operation,
			GateScope:        request.Scope,
			StartedAt:        now,
			BaseModel:        database.BaseModel{CreatedAt: now, UpdatedAt: new(now)},
			Metadata:         database.JSON{},
		}
		if request.StartedBy != nil {
			if id := strings.TrimSpace(request.StartedBy.ID); id != "" {
				model.StartedByUserID = &id
			}
			if username := strings.TrimSpace(request.StartedBy.Username); username != "" {
				model.StartedByUsername = &username
			}
			if request.StartedBy.DisplayName != nil {
				if name := strings.TrimSpace(*request.StartedBy.DisplayName); name != "" {
					model.StartedByDisplayName = &name
				}
			}
		}
		if err := tx.Create(&model).Error; err != nil {
			return fmt.Errorf("create gated activity: %w", err)
		}

		return nil
	}
	var err error
	for attempt := 0; attempt < 2; attempt++ {
		err = s.db.WithContext(ctx).Transaction(admit)
		if err == nil || !dbutil.IsUniqueConstraint(err) {
			break
		}
	}
	if err != nil {
		return GateAnswer{}, err
	}
	dto := activityToDTOInternal(&model)
	var workCtx context.Context
	if code == string(activitytypes.CodeRunStarted) {
		var cancel context.CancelCauseFunc
		workCtx, cancel = context.WithCancelCause(context.WithValue(context.WithoutCancel(ctx), gateRunContextKey{}, model.ID))
		s.running[model.ID] = cancel
		s.publishActivityInternal(dto)
	}
	return GateAnswer{Run: dto, Code: code, WorkContext: workCtx}, nil
}
