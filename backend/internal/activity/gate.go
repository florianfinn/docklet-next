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
}

type GateConflict struct {
	RunID string `json:"runId"`
	Code  string `json:"code"`
}

func (e *GateConflict) Error() string { return e.Code }

// StartGatedActivity serializes starts for one resource across hub processes.
// The caller owns execution only when Code is run.started.
func (s *ActivityService) StartGatedActivity(ctx context.Context, request GateRequest) (GateAnswer, error) {
	return s.startGatedActivityAtInternal(ctx, request, time.Now().UTC())
}

func (s *ActivityService) startGatedActivityAtInternal(ctx context.Context, request GateRequest, now time.Time) (GateAnswer, error) {
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
	code := "run.started"
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		lockKey := strconv.Itoa(len(request.EnvironmentID)) + ":" + request.EnvironmentID + request.Scope
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?, 0))", lockKey).Error; err != nil {
			return fmt.Errorf("lock run scope: %w", err)
		}
		var active Activity
		err := tx.Where("environment_id = ? AND gate_scope = ? AND status IN ?", request.EnvironmentID, request.Scope, []activitytypes.Status{activitytypes.StatusQueued, activitytypes.StatusRunning}).Order("started_at DESC, id DESC").First(&active).Error
		if err == nil {
			if active.GateKey != request.Operation {
				return &GateConflict{RunID: active.ID, Code: "run.busy"}
			}
			model = active
			code = "run.joined"
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if request.Check && !request.Recheck {
			var fresh Activity
			err = tx.Where("environment_id = ? AND gate_scope = ? AND gate_key = ? AND status = ? AND ended_at > ?", request.EnvironmentID, request.Scope, request.Operation, activitytypes.StatusSuccess, now.Add(-CheckFreshness)).Order("ended_at DESC, id DESC").First(&fresh).Error
			if err == nil {
				model = fresh
				code = "run.fresh"
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
			ResourceType:     new(request.ResourceType),
			ResourceID:       new(request.ResourceID),
			AffectedServices: servicesJSONInternal(request.AffectedServices),
			GateKey:          request.Operation,
			GateScope:        request.Scope,
			StartedAt:        now,
			CreatedAt:        now,
			Metadata:         database.JSON{},
		}
		if err := tx.Create(&model).Error; err != nil {
			return fmt.Errorf("create gated activity: %w", err)
		}
		return nil
	})
	if err != nil {
		return GateAnswer{}, err
	}
	dto := activityToDTOInternal(&model)
	if code == "run.started" {
		s.publishActivityInternal(dto)
	}
	return GateAnswer{Run: dto, Code: code}, nil
}
