// Source: docklet-next/issues/296
// Revision: a910be6a64e97a4ce3a422f12451a5301b115f7a
// Retrieved: 2026-10-29
// License: Apache-2.0
// Deviations: Original notification and Auto Heal foundation.

package notification

import (
	"context"
	"encoding/json/v2"
	"errors"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/internal/notification/notificationoutbox"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	dto "github.com/florianfinn/docklet-next/types/notification"
	"gorm.io/gorm"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"uuid"
)

var eventIDPattern = regexp.MustCompile(`^[A-Za-z0-9:_-]{1,64}$`)

func dispatchInvalid() error { return &dto.Problem{Code: "notification.dispatch-invalid", Status: 400} }
func dispatchStale() error   { return &dto.Problem{Code: "notification.dispatch-stale", Status: 409} }
func validateDispatch(p dto.DispatchRequest) error {
	switch p.Kind {
	case dto.DispatchKindBackupFailed:
		v := p.BackupFailed
		if v == nil || !eventIDPattern.MatchString(v.EventID) || !slices.Contains([]string{"backup", "restore"}, v.Operation) || v.Volume == "" || len(v.Volume) > 128 || len(v.Reason) > 128 || !regexp.MustCompile(`^[a-z][a-z0-9_.-]+$`).MatchString(v.Reason) {
			return dispatchInvalid()
		}
		if v.ActivityID != "" {
			if _, err := uuid.Parse(v.ActivityID); err != nil {
				return dispatchInvalid()
			}
		}
	case dto.DispatchKindAutoHeal:
		v := p.AutoHeal
		if v == nil || !eventIDPattern.MatchString(v.EventID) || !slices.Contains([]string{"restarted", "restart-failed", "limit-reached"}, v.Outcome) || v.Container == "" || len(v.Container) > 128 || len(v.Stack) > 128 || v.Restarts < 0 || v.Restarts > 100 || v.WindowMinutes < 1 || v.WindowMinutes > 1440 {
			return dispatchInvalid()
		}
	case dto.DispatchKindImageUpdate:
		if p.ImageUpdate == nil {
			return dispatchInvalid()
		}
	case dto.DispatchKindBatchImageUpdate:
		if p.BatchImageUpdate == nil {
			return dispatchInvalid()
		}
	case dto.DispatchKindContainerUpdate:
		if p.ContainerUpdate == nil {
			return dispatchInvalid()
		}
	case dto.DispatchKindBatchContainerUpdate:
		if p.BatchContainerUpdate == nil {
			return dispatchInvalid()
		}
	case dto.DispatchKindPruneReport:
		if p.PruneReport == nil {
			return dispatchInvalid()
		}
	default:
		return &dto.Problem{Code: "notification.dispatch-unsupported", Status: 400}
	}
	pointers := reflect.ValueOf(p)
	count := 0
	for i := 0; i < pointers.NumField(); i++ {
		if pointers.Field(i).Kind() == reflect.Pointer && !pointers.Field(i).IsNil() {
			count++
		}
	}
	if count != 1 {
		return dispatchInvalid()
	}

	// Bound agent display facts and lists, including nested legacy report fields.
	b, _ := json.Marshal(p)
	var raw any
	_ = json.Unmarshal(b, &raw)
	var bounded func(any) bool
	bounded = func(v any) bool {
		switch x := v.(type) {
		case string:
			return len(x) <= 128 && !strings.ContainsFunc(x, unicode.IsControl)
		case []any:
			if len(x) > 256 {
				return false
			}
			for _, v := range x {
				if !bounded(v) {
					return false
				}
			}
		case map[string]any:
			if len(x) > 256 {
				return false
			}
			for k, v := range x {
				if len(k) > 128 || !bounded(v) {
					return false
				}
			}
		}
		return true
	}
	if !bounded(raw) {
		return dispatchInvalid()
	}
	return nil
}
func originFor(env *environment.Environment) dto.DispatchOrigin {
	o := dto.DispatchOrigin{EnvironmentID: env.ID}
	if env.ApiKeyID != nil {
		o.APIKeyID = *env.ApiKeyID
	}
	if env.AccessToken != nil {
		o.TokenHash = edge.AgentTokenHash(*env.AccessToken)
	}
	return o
}

// DispatchIngress accepts only the credential/session that actually received this event.
func (s *NotificationService) DispatchIngress(ctx context.Context, o dto.DispatchOrigin, p dto.DispatchRequest) (dto.DispatchResponse, error) {
	if err := validateDispatch(p); err != nil {
		return dto.DispatchResponse{}, err
	}
	if s.dispatchBarrier != nil {
		s.dispatchBarrier()
	}
	if o.SessionID != "" && (s.dispatchSessionCurrent == nil || !s.dispatchSessionCurrent(o)) {
		return dto.DispatchResponse{}, dispatchStale()
	}
	var target NotificationTarget
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var env environment.Environment
		if err := tx.Where("id=?", o.EnvironmentID).First(&env).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return dispatchStale()
			}
			return err
		}
		current := originFor(&env)
		if !env.Enabled || current.APIKeyID != o.APIKeyID || current.TokenHash != o.TokenHash || current.TokenHash == "" {
			return dispatchStale()
		}
		target = NotificationTarget{EnvironmentID: env.ID, EnvironmentName: env.Name}
		var payload database.JSON
		eventID := ""
		switch p.Kind {
		case dto.DispatchKindAutoHeal:
			v := p.AutoHeal
			eventID = v.EventID
			payload = database.JSON{"host": env.Name, "container": v.Container, "stack": v.Stack, "outcome": v.Outcome, "restarts": v.Restarts, "windowMinutes": v.WindowMinutes}
		case dto.DispatchKindBackupFailed:
			v := p.BackupFailed
			eventID = v.EventID
			payload = database.JSON{"host": env.Name, "operation": v.Operation, "volume": v.Volume, "reason": v.Reason, "activityId": v.ActivityID}
		default:
			return nil
		}
		return notificationoutbox.Insert(tx, notificationoutbox.Row{Event: string(p.Kind), EnvironmentID: env.ID, DedupKey: string(p.Kind) + ":" + env.ID + ":" + eventID, Payload: payload})
	})
	if err != nil {
		return dto.DispatchResponse{}, err
	}
	if p.Kind == dto.DispatchKindAutoHeal || p.Kind == dto.DispatchKindBackupFailed {
		return dto.DispatchResponse{Message: "accepted"}, nil
	}
	return s.dispatchForTargetInternal(ctx, target, p)
}
func (s *NotificationService) SetDispatchSessionCurrent(check func(dto.DispatchOrigin) bool) {
	s.dispatchSessionCurrent = check
}

// SendHostEvent sends a live host event once. It never records host-side retry intent.
func (s *NotificationService) SendHostEvent(ctx context.Context, p dto.DispatchRequest) error {
	_, err := s.dispatchNotificationToManagerInternal(ctx, p)
	return err
}
