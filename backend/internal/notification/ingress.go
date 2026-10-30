// Source: docklet-next/issues/296
// Revision: a910be6a64e97a4ce3a422f12451a5301b115f7a
// Retrieved: 2026-10-29
// License: Apache-2.0
// Deviations: Original notification and Auto Heal foundation; legacy reports are clamped, not refused.

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
	"unicode/utf8"
	"uuid"
)

var eventIDPattern = regexp.MustCompile(`^[A-Za-z0-9:_-]{1,64}$`)

func dispatchInvalid() error { return &dto.Problem{Code: "notification.dispatch-invalid", Status: 400} }
func dispatchStale() error   { return &dto.Problem{Code: "notification.dispatch-stale", Status: 409} }
func validateDispatch(p *dto.DispatchRequest) error {
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
	pointers := reflect.ValueOf(*p)
	count := 0
	for i := 0; i < pointers.NumField(); i++ {
		if pointers.Field(i).Kind() == reflect.Pointer && !pointers.Field(i).IsNil() {
			count++
		}
	}
	if count != 1 {
		return dispatchInvalid()
	}

	// Agent display facts stay bounded. Typed payloads are refused when a value is out of
	// bounds; legacy reports are clamped in place so long Docker text or large lists still arrive.
	if p.Kind != dto.DispatchKindBackupFailed && p.Kind != dto.DispatchKindAutoHeal {
		clampLegacyValue(reflect.ValueOf(p).Elem())
	}
	b, err := json.Marshal(*p)
	if err != nil {
		return dispatchInvalid()
	}
	var raw any
	if err = json.Unmarshal(b, &raw); err != nil || !boundedDispatchValue(raw) {
		return dispatchInvalid()
	}
	return nil
}

// boundedDispatchValue checks strings (128 bytes, no control characters), lists and maps (256
// entries) and map keys (128 bytes).
func boundedDispatchValue(v any) bool {
	switch x := v.(type) {
	case string:
		return len(x) <= 128 && !strings.ContainsFunc(x, unicode.IsControl)
	case []any:
		if len(x) > 256 {
			return false
		}
		for _, item := range x {
			if !boundedDispatchValue(item) {
				return false
			}
		}
	case map[string]any:
		if len(x) > 256 {
			return false
		}
		for k, item := range x {
			if len(k) > 128 || !boundedDispatchValue(item) {
				return false
			}
		}
	}
	return true
}

// clampLegacyValue shortens strings and lists of a typed legacy report in place; numbers and
// other values keep their exact wire form. Oversized maps and keys are left for the bound check.
func clampLegacyValue(v reflect.Value) {
	switch v.Kind() {
	case reflect.Pointer:
		if !v.IsNil() {
			clampLegacyValue(v.Elem())
		}
	case reflect.Interface:
		if !v.IsNil() && v.CanSet() {
			c := reflect.New(v.Elem().Type()).Elem()
			c.Set(v.Elem())
			clampLegacyValue(c)
			v.Set(c)
		}
	case reflect.Struct:
		for i := range v.NumField() {
			if f := v.Field(i); f.CanSet() {
				clampLegacyValue(f)
			}
		}
	case reflect.Slice:
		if v.Len() > 256 && v.CanSet() {
			v.Set(v.Slice(0, 256))
		}
		for i := range v.Len() {
			clampLegacyValue(v.Index(i))
		}
	case reflect.Array:
		for i := range v.Len() {
			clampLegacyValue(v.Index(i))
		}
	case reflect.Map:
		for _, k := range v.MapKeys() {
			c := reflect.New(v.Type().Elem()).Elem()
			c.Set(v.MapIndex(k))
			clampLegacyValue(c)
			v.SetMapIndex(k, c)
		}
	case reflect.String:
		if v.CanSet() {
			v.SetString(clampDisplayString(v.String()))
		}
	}
}

// clampDisplayString replaces control characters and cuts to 128 bytes on a rune boundary.
func clampDisplayString(x string) string {
	if len(x) <= 128 && !strings.ContainsFunc(x, unicode.IsControl) {
		return x
	}
	x = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, strings.ToValidUTF8(x, ""))
	for len(x) > 128 {
		_, size := utf8.DecodeLastRuneInString(x)
		x = x[:len(x)-size]
	}
	return x
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
	if err := validateDispatch(&p); err != nil {
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
