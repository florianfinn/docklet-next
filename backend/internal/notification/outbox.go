// Source: docklet-next/issues/296
// Revision: a910be6a64e97a4ce3a422f12451a5301b115f7a
// Retrieved: 2026-10-29
// License: Apache-2.0
// Deviations: Original notification and Auto Heal foundation; M19 backend contracts and target identity integration.

package notification

import (
	"context"
	"fmt"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/notification/notificationoutbox"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/notifications"
	"gorm.io/gorm"
	"html"
	"strings"
	"time"
)

// DeliverPending runs only through the coordinator-owned delivery job.
func (s *NotificationService) DeliverPending(ctx context.Context, now time.Time) error {
	if s.config != nil && (s.config.AgentMode || s.config.EdgeAgent) {
		return ErrUnauthorizedNotificationDispatch
	}
	db := s.db.WithContext(ctx)
	if err := db.Where("settled_at < ?", now.Add(-30*24*time.Hour)).Delete(&notificationoutbox.Row{}).Error; err != nil {
		return err
	}
	if err := db.Model(&notificationoutbox.Row{}).Where("status IN ? AND created_at < ?", []string{"pending", "sending"}, now.Add(-24*time.Hour)).Updates(map[string]any{"status": "expired", "settled_at": now, "lease_until": nil}).Error; err != nil {
		return err
	}
	if err := db.Model(&notificationoutbox.Row{}).Where("attempts >= 3 AND (status = 'pending' OR (status = 'sending' AND lease_until <= ?))", now).Updates(map[string]any{"status": "failed", "settled_at": now, "lease_until": nil}).Error; err != nil {
		return err
	}
	var rows []notificationoutbox.Row
	if err := db.Where("attempts < 3 AND (status = 'pending' OR (status = 'sending' AND lease_until <= ?))", now).Order("created_at, id").Find(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		if err := ctx.Err(); err != nil {
			return err
		}
		if s.beforeClaim != nil {
			s.beforeClaim()
		}
		lease := now.Add(5 * time.Minute)
		claimed := db.Model(&notificationoutbox.Row{}).Where("id=? AND attempts=? AND (status='pending' OR (status='sending' AND lease_until <= ?))", row.ID, row.Attempts, now).Updates(map[string]any{"status": "sending", "lease_until": lease, "attempts": gorm.Expr("attempts+1")})
		if claimed.Error != nil {
			return claimed.Error
		}
		if claimed.RowsAffected != 1 {
			continue
		}
		row.Attempts++
		delivered, err := s.deliverOutbox(ctx, row)
		status := "sent"
		var settled any = now
		if delivered == 0 {
			if err == nil {
				status = "unconfigured"
			} else if row.Attempts >= 3 {
				status = "failed"
			} else {
				status = "pending"
				settled = nil
			}
		}
		// A prior claimant cannot settle a reclaimed row.
		settle := s.db.WithContext(context.WithoutCancel(ctx)).Model(&notificationoutbox.Row{}).Where("id=? AND status='sending' AND attempts=? AND lease_until=?", row.ID, row.Attempts, lease).Updates(map[string]any{"status": status, "settled_at": settled, "lease_until": nil})
		if settle.Error != nil {
			return settle.Error
		}
	}
	return nil
}
func (s *NotificationService) deliverOutbox(ctx context.Context, row notificationoutbox.Row) (int, error) {
	target := NotificationTarget{EnvironmentID: row.EnvironmentID}
	target.EnvironmentName, _ = row.Payload["host"].(string)
	if target.EnvironmentName == "" {
		t, err := s.resolveNotificationTargetInternal(ctx, row.EnvironmentID)
		if err != nil {
			return 0, err
		}
		target = t
	}
	appURL := ""
	if s.config != nil {
		appURL = s.config.GetAppURL()
	}
	text := outboxText(row.Event, target.EnvironmentName, row.Payload, appURL)
	content := notifications.Content{Title: row.Event, DefaultTitle: row.Event, Text: map[notifications.MessageFormat]string{}, Vars: notifications.EventVars(target.EnvironmentName, target.EnvironmentID, notifications.NotificationEventType(row.Event))}
	for _, f := range []notifications.MessageFormat{notifications.MessageFormatPlain, notifications.MessageFormatMarkdown, notifications.MessageFormatHTML, notifications.MessageFormatSlack} {
		content.Text[f] = text
	}
	if row.Event == "container_unhealthy" {
		content.Title = strings.SplitN(text, "\n", 2)[0]
		content.DefaultTitle = content.Title
	}
	content.RenderEmail = func() (string, string, error) { return content.Title, "<p>" + html.EscapeString(text) + "</p>", nil }
	return s.notifyEnabledProvidersInternal(ctx, target, notifications.NotificationEventType(row.Event), row.Event, database.JSON{"eventType": row.Event, "notificationId": row.ID}, func(ctx context.Context, p notifications.NotificationProvider, c database.JSON) (bool, error) {
		return notifications.Deliver(ctx, p, c, content)
	})
}

// outboxText is the plain message of one outbox event; load alerts name metric, value and threshold.
func outboxText(eventType, host string, payload database.JSON, appURL string) string {
	if eventType == "container_unhealthy" {
		container, _ := payload["container"].(string)
		text := fmt.Sprintf("Container %s on %s is unhealthy (healthcheck failing for %v min)", container, host, payload["minutes"])
		if payload["reason"] == "restart-loop" {
			text = fmt.Sprintf("Container %s on %s keeps restarting (%v restarts in 10 min)", container, host, payload["restarts"])
		}
		if appURL != "" {
			text += "\n" + appURL
		}
		return text
	}
	text := fmt.Sprintf("%s on %s", eventType, host)
	for _, f := range []string{"stack", "container", "volume", "operation", "outcome", "reason"} {
		if v, ok := payload[f].(string); ok && v != "" {
			text += fmt.Sprintf("\n%s: %s", f, v)
		}
	}
	if metric, ok := payload["metric"].(string); ok && metric != "" {
		number := func(key string) float64 {
			switch v := payload[key].(type) {
			case float64:
				return v
			case int:
				return float64(v)
			}
			return 0
		}
		text += fmt.Sprintf("\nload: %s %.0f%% above %.0f%% for %.0f min", metric, number("percent"), number("threshold"), number("minutes"))
	}
	if appURL != "" {
		text += "\n" + appURL
	}
	return text
}
