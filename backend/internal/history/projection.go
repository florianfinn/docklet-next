// Source: docklet-next/issues/338
// Revision: 14e11ba9c022d57c29bd36c8ed24780763327d0c
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original atomic hub condition events and sustained-load notices.

package history

import (
	"fmt"
	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/internal/event"
	"github.com/florianfinn/docklet-next/backend/internal/notification/notificationoutbox"
	activityDTO "github.com/florianfinn/docklet-next/types/activity"
	dto "github.com/florianfinn/docklet-next/types/history"
	"gorm.io/gorm"
	"time"
	"uuid"
)

func (s *Service) projectTransitions(tx *gorm.DB, host string, now time.Time, current []dto.Condition, previous dto.Conditions) error {
	before := map[string]dto.Condition{}
	for _, c := range previous.Items {
		if c.Kind == "container" {
			before[c.ContainerID] = c
		}
	}
	for _, c := range current {
		if c.Kind != "container" {
			continue
		}
		p, exists := before[c.ContainerID]
		if exists && p.Condition == c.Condition && p.Reason == c.Reason {
			continue
		}
		code := ""
		switch {
		case c.Reason == "restart-loop":
			code = dto.EventRestartLoop
		case c.Condition == dto.Down:
			code = dto.EventDown
		case c.Condition == dto.Unhealthy:
			code = dto.EventUnhealthy
		case exists && c.Condition == dto.Running && (p.Condition == dto.Down || p.Condition == dto.Unhealthy):
			code = dto.EventRunningAgain
		}
		if code == "" {
			continue
		}
		severity := event.EventSeverityWarning
		if c.Condition == dto.Down {
			severity = event.EventSeverityError
		}
		if c.Condition == dto.Running {
			severity = event.EventSeveritySuccess
		}
		kind := "container"
		metadata := dto.ConditionEventMetadata{HostID: host, ContainerName: c.Name, ExitCode: c.ExitCode, Restarts: c.Restarts10m}
		row := event.Event{BaseModel: database.BaseModel{ID: uuid.New().String(), CreatedAt: now}, Type: event.EventType(code), Severity: severity, Title: code, Description: code, MessageCode: code, EnvironmentID: new(host), ResourceType: &kind, ResourceID: new(c.ContainerID), ResourceName: new(c.Name), Timestamp: now, Metadata: database.JSON(metadata.Fields())}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
	}
	return nil
}
func (s *Service) projectLoad(tx *gorm.DB, host environment.Environment, now time.Time, observations []observation, next *hostState) error {
	enabled, cpuThreshold, memoryThreshold, minutes := true, 90., 90., 10
	if s.settings != nil {
		cfg := s.settings.GetSettingsConfig()
		enabled = cfg.LoadAlertsEnabled.IsTrue()
		cpuThreshold = float64(cfg.LoadAlertCpuPercent.AsInt())
		memoryThreshold = float64(cfg.LoadAlertMemoryPercent.AsInt())
		minutes = cfg.LoadAlertMinutes.AsInt()
	}
	if !enabled {
		next.episodes = map[string]episode{}
		return nil
	}
	alive := map[string]bool{}
	for _, o := range observations {
		if o.kind != "host" && (o.kind != "container" || (o.state != dto.Running && o.state != dto.Unhealthy)) {
			continue
		}
		var memory *float64
		if o.memory != nil && o.memoryLimit > 0 {
			memory = new(100 * *o.memory / o.memoryLimit)
		}
		for _, metric := range []struct {
			name      string
			value     *float64
			threshold float64
		}{{"cpu", o.cpu, cpuThreshold}, {"memory", memory, memoryThreshold}} {
			key := o.kind + ":" + o.key + ":" + o.identity + ":" + metric.name
			alive[key] = true
			var alert bool
			next.episodes[key], alert = evaluateLoad(next.episodes[key], now.Unix(), metric.value, metric.threshold, minutes)
			if !alert {
				continue
			}
			resource := o.kind
			metadata := dto.LoadAlertMetadata{Metric: metric.name, Percent: *metric.value, Threshold: metric.threshold, Minutes: minutes, Host: host.Name, Container: o.key}
			data := database.JSON(metadata.Fields())
			row := activity.Activity{BaseModel: database.BaseModel{ID: uuid.New().String(), CreatedAt: now}, EnvironmentID: host.ID, Type: activityDTO.TypeLoadSustained, Status: activityDTO.StatusSuccess, ResourceType: &resource, ResourceID: new(o.key), ResourceName: new(o.key), StartedAt: now, EndedAt: new(now), MessageCode: "load.sustained", LatestMessage: "load.sustained", Metadata: data}
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
			if err := notificationoutbox.Insert(tx, notificationoutbox.Row{Event: "sustained_load", EnvironmentID: host.ID, DedupKey: fmt.Sprintf("load:%s:%s:%d", host.ID, key, now.Unix()), Payload: data, CreatedAt: now}); err != nil {
				return err
			}
		}
	}
	for key := range next.episodes {
		if !alive[key] {
			delete(next.episodes, key)
		}
	}
	return nil
}
