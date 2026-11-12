// Source: docklet-next/issues/296
// Revision: a910be6a64e97a4ce3a422f12451a5301b115f7a
// Retrieved: 2026-10-29
// License: Apache-2.0
// Deviations: Original notification and Auto Heal foundation; shared application-heartbeat observation clock.

package notification

import (
	"context"
	"errors"
	"fmt"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/internal/notification/notificationoutbox"
	envDTO "github.com/florianfinn/docklet-next/types/environment"
	"gorm.io/gorm"
	"time"
)

func (s *NotificationService) ResetAvailability(ctx context.Context, now time.Time) error {
	return s.db.WithContext(ctx).Model(&notificationoutbox.Availability{}).Where("notified_offline_at IS NULL").Update("offline_since", now).Error
}
func (s *NotificationService) ObserveAvailability(ctx context.Context, now time.Time) error {
	var rows []environment.Environment
	if err := s.db.WithContext(ctx).Where("id <> '0'").Find(&rows).Error; err != nil {
		return err
	}
	ids := []string{}
	for _, row := range rows {
		ids = append(ids, row.ID)
		p := envDTO.Environment{ID: row.ID, Name: row.Name, Enabled: row.Enabled, Status: row.Status, IsEdge: row.IsEdge}
		if s.environmentSvc != nil {
			s.environmentSvc.ApplyHostRuntimeStateAt(ctx, &p, now)
		}
		excluded := !row.Enabled || (row.LastSeen == nil && p.LastPollAt == nil && (p.Connected == nil || !*p.Connected))
		if err := s.observeHost(ctx, now, row.ID, row.Name, p.Status, excluded); err != nil {
			return err
		}
	}
	query := s.db.WithContext(ctx).Where("1=1")
	if len(ids) > 0 {
		query = query.Where("environment_id NOT IN ?", ids)
	}
	return query.Delete(&notificationoutbox.Availability{}).Error
}
func (s *NotificationService) observeHost(ctx context.Context, now time.Time, id, name, status string, excluded bool) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if excluded {
			return tx.Where("environment_id=?", id).Delete(&notificationoutbox.Availability{}).Error
		}
		switch status {
		case "pending", "update-required", "token-rejected":
			return nil
		case "online", "standby", "offline", "error":
		default:
			return nil
		}
		var state notificationoutbox.Availability
		err := tx.Where("environment_id=?", id).First(&state).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		down := status == "offline" || status == "error"
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if !down {
				return nil
			}
			return tx.Create(&notificationoutbox.Availability{EnvironmentID: id, OfflineSince: now}).Error
		}
		if down {
			if state.NotifiedOfflineAt != nil || now.Sub(state.OfflineSince) < 5*time.Minute {
				return nil
			}
			if err := notificationoutbox.Insert(tx, notificationoutbox.Row{Event: "host_offline", EnvironmentID: id, DedupKey: fmt.Sprintf("host:%s:%d", id, state.OfflineSince.Unix()), Payload: database.JSON{"host": name, "outcome": "offline"}, CreatedAt: now}); err != nil {
				return err
			}
			return tx.Model(&state).Update("notified_offline_at", now).Error
		}
		if state.NotifiedOfflineAt != nil {
			if err := notificationoutbox.Insert(tx, notificationoutbox.Row{Event: "host_offline", EnvironmentID: id, DedupKey: fmt.Sprintf("host:%s:%d:online", id, state.OfflineSince.Unix()), Payload: database.JSON{"host": name, "outcome": "online"}, CreatedAt: now}); err != nil {
				return err
			}
		}
		return tx.Delete(&state).Error
	})
}
