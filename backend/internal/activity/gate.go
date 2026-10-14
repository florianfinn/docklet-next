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
	"strconv"
	"strings"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const CheckFreshness = 30 * time.Second

type GateRequest struct {
	EnvironmentID    string
	Operation        string
	Scope            string
	Type             activitytypes.Type
	ResourceType     string
	ResourceID       string
	AffectedServices []string
	Check            bool
	Recheck          bool
}

type GateAnswer struct {
	Run  activitytypes.Activity `json:"run"`
	Code string                 `json:"code"`
	// WorkContext belongs only to run.started and carries its fenced owner.
	WorkContext context.Context `json:"-"`
}

type GateConflict struct {
	RunID string `json:"runId"`
	Code  string `json:"code"`
}

func (e *GateConflict) Error() string { return e.Code }

// StartGatedActivity serializes starts for one resource in PostgreSQL.
// Only run.started owns execution; its heartbeat lets every process distinguish
// live gated work from a process that stopped before completion.
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
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		lockKey := strconv.Itoa(len(request.EnvironmentID)) + ":" + request.EnvironmentID + request.Scope
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?, 0))", lockKey).Error; err != nil {
			return fmt.Errorf("lock run scope: %w", err)
		}
		var active Activity
		err := tx.Where("environment_id = ? AND gate_scope = ? AND status IN ?", request.EnvironmentID, request.Scope, []activitytypes.Status{activitytypes.StatusQueued, activitytypes.StatusRunning}).Order("started_at DESC, id DESC").First(&active).Error
		if err == nil {
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
			err = tx.Where("environment_id = ? AND gate_scope = ? AND gate_key = ? AND status = ? AND ended_at > NOW() - (? * INTERVAL '1 second')", request.EnvironmentID, request.Scope, request.Operation, activitytypes.StatusSuccess, int(CheckFreshness/time.Second)).Order("ended_at DESC, id DESC").First(&fresh).Error
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
			GateOwner:        uuid.NewString(),
			StartedAt:        s.nowInternal(),
			Metadata:         database.JSON{},
		}
		if err := tx.Create(&model).Error; err != nil {
			return fmt.Errorf("create gated activity: %w", err)
		}
		if err := tx.Model(&Activity{}).Where("id = ?", model.ID).Updates(map[string]any{
			"started_at": gorm.Expr("NOW()"), "created_at": gorm.Expr("NOW()"), "updated_at": gorm.Expr("NOW()"), "gate_lease_at": gorm.Expr("NOW()"),
		}).Error; err != nil {
			return fmt.Errorf("set gated activity database time: %w", err)
		}
		if err := tx.First(&model, "id = ?", model.ID).Error; err != nil {
			return fmt.Errorf("load gated activity database time: %w", err)
		}
		return nil
	})
	if err != nil {
		return GateAnswer{}, err
	}
	dto := activityToDTOInternal(&model)
	var workCtx context.Context
	if code == string(activitytypes.CodeRunStarted) {
		workCtx = s.Track(context.WithValue(context.WithoutCancel(ctx), gateOwnerContextKey{}, model.GateOwner), model.ID)
		s.startGateHeartbeatInternal(workCtx, model.ID, model.GateOwner)
		s.publishActivityInternal(dto)
	}
	return GateAnswer{Run: dto, Code: code, WorkContext: workCtx}, nil
}
