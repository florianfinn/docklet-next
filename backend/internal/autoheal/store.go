// Source: docklet-next/issues/296
// Revision: a910be6a64e97a4ce3a422f12451a5301b115f7a
// Retrieved: 2026-10-29
// License: Apache-2.0
// Deviations: Original notification and Auto Heal foundation.

package autoheal

import (
	"context"
	"errors"
	"fmt"
	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/event"
	"github.com/florianfinn/docklet-next/backend/internal/notification/notificationoutbox"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	activityDTO "github.com/florianfinn/docklet-next/types/activity"
	dto "github.com/florianfinn/docklet-next/types/autoheal"
	notificationDTO "github.com/florianfinn/docklet-next/types/notification"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"log/slog"
	"time"
)

type Store struct {
	Settings *settings.SettingsService
	DB       *database.DB
	Config   *config.Config
	Dispatch func(context.Context, notificationDTO.DispatchRequest) error
	Now      func() time.Time
}
type Baseline struct {
	StartedAt    string
	RestartCount int
	Restarting   bool
}

func (s *Store) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}
func (s *Store) Lookup(ctx context.Context, runID, containerID string) (*Restart, error) {
	var r Restart
	err := s.DB.WithContext(ctx).Where("run_id=? AND container_id=?", runID, containerID).First(&r).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &r, err
}

// Reserve counts every durable attempt. The unique run/container identity closes the scheduler checkpoint gap.
func (s *Store) Reserve(ctx context.Context, runID, id, name, stack string, b Baseline, cfg dto.Settings) (*Restart, bool, error) {
	now := s.now()
	var row *Restart
	restart := false
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing Restart
		err := tx.Where("run_id=? AND container_id=?", runID, id).First(&existing).Error
		if err == nil {
			row = &existing
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		cutoff := now.Add(-time.Duration(cfg.WindowMinutes) * time.Minute)
		var count int64
		if err := tx.Model(&Restart{}).Where("container_id=? AND started_at>=? AND outcome IN ?", id, cutoff, []string{"attempting", "restarted", "restart-failed", "interrupted"}).Count(&count).Error; err != nil {
			return err
		}
		r := Restart{RunID: runID, ContainerID: id, ContainerName: name, Stack: stack, StartedAt: now, BaselineStartedAt: b.StartedAt, BaselineRestartCount: b.RestartCount, Outcome: "attempting"}
		if count >= int64(cfg.MaxRestarts) {
			var markers int64
			if err := tx.Model(&Restart{}).Where("container_id=? AND started_at>=? AND outcome='limit-reached'", id, cutoff).Count(&markers).Error; err != nil {
				return err
			}
			if markers > 0 {
				return nil
			}
			r.Outcome = "limit-reached"
			r.SettledAt = &now
		} else {
			restart = true
		}
		if err := tx.Create(&r).Error; err != nil {
			return err
		}
		row = &r
		return nil
	})
	return row, restart, err
}
func (s *Store) Settle(ctx context.Context, id uint, outcome string) error {
	now := s.now()
	return s.DB.WithContext(context.WithoutCancel(ctx)).Model(&Restart{}).Where("id=? AND outcome='attempting'", id).Updates(map[string]any{"outcome": outcome, "settled_at": now}).Error
}
func (s *Store) Recover(ctx context.Context, inspect func(context.Context, string) (Baseline, error)) error {
	var rows []Restart
	if err := s.DB.WithContext(ctx).Where("outcome='attempting'").Find(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		current, err := inspect(ctx, row.ContainerID)
		outcome := "interrupted"
		if err == nil && !current.Restarting && (current.StartedAt != row.BaselineStartedAt || current.RestartCount != row.BaselineRestartCount) {
			outcome = "restarted"
		}
		if err := s.Settle(ctx, row.ID, outcome); err != nil {
			return err
		}
	}
	return s.Replay(ctx)
}
func (s *Store) Replay(ctx context.Context) error {
	var rows []Restart
	if err := s.DB.WithContext(ctx).Where("outcome<>'attempting' AND projected_at IS NULL").Order("id").Find(&rows).Error; err != nil {
		return err
	}
	for _, r := range rows {
		if err := s.Project(ctx, r.ID, false); err != nil {
			return err
		}
	}
	return s.DB.WithContext(ctx).Where("started_at < ? AND projected_at IS NOT NULL", s.now().Add(-24*time.Hour)).Delete(&Restart{}).Error
}

// Project commits the activity, event and hub notice with the projection marker.
// Agent sends happen only after the live projection commits and never on replay.
func (s *Store) Project(ctx context.Context, id uint, live bool) error {
	now := s.now()
	window := 30
	if s.Settings != nil {
		window = s.Settings.GetIntSetting(ctx, "autoHealRestartWindow", 30)
	}
	if window < 5 || window > 1440 {
		window = 30
	}
	var dispatch *notificationDTO.DispatchRequest
	err := s.DB.WithContext(context.WithoutCancel(ctx)).Transaction(func(tx *gorm.DB) error {
		var r Restart
		if err := tx.First(&r, id).Error; err != nil {
			return err
		}
		if r.ProjectedAt != nil || r.Outcome == "attempting" {
			return nil
		}
		claimed := tx.Model(&Restart{}).Where("id=? AND projected_at IS NULL", id).Update("projected_at", now)
		if claimed.Error != nil {
			return claimed.Error
		}
		if claimed.RowsAffected != 1 {
			return nil
		}
		// The attempt start keeps the identity unique when an agent store is recreated and row IDs restart.
		identity := fmt.Sprintf("autoheal:%d:%d", id, r.StartedAt.UnixNano())
		code := "autoheal." + r.Outcome
		status, severity := activityDTO.StatusFailed, event.EventSeverityError
		if r.Outcome == "restarted" {
			status = activityDTO.StatusSuccess
			severity = event.EventSeveritySuccess
		}
		if r.Outcome == "interrupted" {
			severity = event.EventSeverityWarning
		}
		resourceType, envID := "container", "0"
		model := activity.Activity{BaseModel: database.BaseModel{ID: uuid.NewSHA1(uuid.NameSpaceOID, []byte(identity)).String(), CreatedAt: now}, EnvironmentID: envID, Type: activityDTO.Type("container_auto_heal"), Status: status, ResourceType: &resourceType, ResourceID: &r.ContainerID, ResourceName: &r.ContainerName, StartedAt: r.StartedAt, EndedAt: r.SettledAt, MessageCode: code, LatestMessage: code, Metadata: database.JSON{"autoHealRestartId": id, "stack": r.Stack, "severity": string(severity)}}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&model).Error; err != nil {
			return err
		}
		e := event.Event{BaseModel: database.BaseModel{ID: uuid.NewSHA1(uuid.NameSpaceOID, []byte("event:"+identity)).String(), CreatedAt: now}, DeduplicationKey: &identity, Type: event.EventTypeContainerRestart, Severity: severity, Title: code, Description: code, MessageCode: code, EnvironmentID: &envID, ResourceType: &resourceType, ResourceID: &r.ContainerID, ResourceName: &r.ContainerName, Timestamp: now, Metadata: database.JSON{"autoHealRestartId": id, "action": "auto-heal", "outcome": r.Outcome}}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&e).Error; err != nil {
			return err
		}
		if !live || r.Outcome == "interrupted" {
			return nil
		}
		// Limits are display facts only; the durable attempts remain the authority.
		var count int64
		if err := tx.Model(&Restart{}).Where("container_id=? AND started_at>=? AND outcome IN ?", r.ContainerID, now.Add(-time.Duration(window)*time.Minute), []string{"attempting", "restarted", "restart-failed", "interrupted"}).Count(&count).Error; err != nil {
			return err
		}
		if count > 100 {
			count = 100
		}
		p := notificationDTO.DispatchAutoHeal{EventID: identity, Outcome: r.Outcome, Container: r.ContainerName, Stack: r.Stack, Restarts: int(count), WindowMinutes: window}
		if s.Config != nil && (s.Config.AgentMode || s.Config.EdgeAgent) {
			dispatch = &notificationDTO.DispatchRequest{Kind: notificationDTO.DispatchKindAutoHeal, AutoHeal: &p}
			return nil
		}
		return notificationoutbox.Insert(tx, notificationoutbox.Row{Event: "auto_heal", EnvironmentID: "0", DedupKey: "autoheal:0:" + fmt.Sprint(id), Payload: database.JSON{"container": r.ContainerName, "stack": r.Stack, "outcome": r.Outcome, "restarts": int(count), "windowMinutes": window}, CreatedAt: now})
	})
	if err != nil {
		return err
	}
	if dispatch != nil && s.Dispatch != nil {
		if err := s.Dispatch(ctx, *dispatch); err != nil {
			slog.WarnContext(ctx, "Auto Heal notification could not be dispatched", "code", "notification.dispatch-failed")
		}
		return nil
	}
	return nil
}
