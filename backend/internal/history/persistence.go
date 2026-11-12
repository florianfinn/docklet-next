// Source: docklet-next/issues/352
// Revision: b5068aff3da7eac11f208d1f4d8c9068d4a8c073
// Retrieved: 2026-11-06
// License: Apache-2.0
// Deviations: Original transactional condition persistence and delayed unhealthy notices.

package history

import (
	"encoding/json"
	"fmt"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/internal/notification/notificationoutbox"
	dto "github.com/florianfinn/docklet-next/types/history"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"maps"
	"slices"
	"time"
)

type conditionRow struct {
	HostID              string `gorm:"primaryKey;not null"`
	Kind                string `gorm:"primaryKey;not null"`
	EntityID            string `gorm:"primaryKey;not null"`
	Revision            string `gorm:"not null"`
	Condition           string `gorm:"not null"`
	Restarts            string `gorm:"not null;default:'[]'"`
	UnhealthySince      *int64
	UnhealthyNotifiedAt *int64
}

func (conditionRow) TableName() string { return "history_conditions" }

func seedConditions(tx *gorm.DB, host, revision string, old *hostState) error {
	rows := []conditionRow{}
	if err := tx.Where("host_id=? AND revision=?", host, revision).Find(&rows).Error; err != nil {
		return err
	}
	// Seeding works on a private map; the cached state is published only after commit.
	old.restarts = maps.Clone(old.restarts)
	if old.restarts == nil {
		old.restarts = map[string][]restartPoint{}
	}
	old.conditions, old.stacks = map[string]dto.Condition{}, map[string]dto.Condition{}
	for _, row := range rows {
		var c dto.Condition
		if json.Unmarshal([]byte(row.Condition), &c) != nil {
			continue
		}
		if row.Kind == "container" {
			old.conditions[row.EntityID] = c
			var points []restartPoint
			if json.Unmarshal([]byte(row.Restarts), &points) == nil {
				old.restarts[row.EntityID] = points
			}
		} else {
			old.stacks[row.EntityID] = c
		}
	}
	return nil
}

// Conditions, restart evidence, quiet timestamps and notices commit with the minute and events.
func persistConditions(tx *gorm.DB, host environment.Environment, now time.Time, sample dto.Sample, next hostState) error {
	rows := []conditionRow{}
	if err := tx.Where("host_id=?", host.ID).Find(&rows).Error; err != nil {
		return err
	}
	before := map[string]conditionRow{}
	for _, row := range rows {
		before[row.Kind+":"+row.EntityID] = row
	}
	alive := map[string]bool{}
	for _, id := range sample.Uninspected {
		alive["container:"+id] = true
	}
	items := map[string]dto.Condition{}
	for id, c := range next.conditions {
		if !slices.Contains(sample.Uninspected, id) {
			items["container:"+id] = c
		}
	}
	for id, c := range next.stacks {
		items["stack:"+id] = c
	}
	for key, c := range items {
		alive[key] = true
		prior := before[key]
		id := c.ContainerID
		if c.Kind == "stack" {
			id = c.ProjectID
		}
		data, err := json.Marshal(c)
		if err != nil {
			return err
		}
		points := next.restarts[id]
		if len(points) < 2 || points[len(points)-1].Count == points[0].Count {
			points = []restartPoint{}
		}
		restartData, err := json.Marshal(points)
		if err != nil {
			return err
		}
		row := conditionRow{HostID: host.ID, Kind: c.Kind, EntityID: id, Revision: next.revision, Condition: string(data), Restarts: string(restartData), UnhealthyNotifiedAt: prior.UnhealthyNotifiedAt}
		if c.Kind == "container" && c.Condition == dto.Unhealthy {
			since := now.Unix()
			if prior.Revision == next.revision && prior.UnhealthySince != nil {
				since = *prior.UnhealthySince
			}
			row.UnhealthySince = &since
			// One notice per unhealthy period; a notice of an earlier period starts the 60-minute quiet time.
			earlier := prior.UnhealthyNotifiedAt == nil || (*prior.UnhealthyNotifiedAt < since && now.Unix()-*prior.UnhealthyNotifiedAt >= int64(time.Hour/time.Second))
			if now.Unix()-since >= int64(10*time.Minute/time.Second) && earlier {
				payload := database.JSON{"host": host.Name, "container": c.Name, "stack": c.ProjectID, "reason": c.Reason, "minutes": (now.Unix() - since) / 60, "restarts": c.Restarts10m}
				if err := notificationoutbox.Insert(tx, notificationoutbox.Row{Event: "container_unhealthy", EnvironmentID: host.ID, DedupKey: fmt.Sprintf("unhealthy:%s:%s:%d", host.ID, id, since), Payload: payload, CreatedAt: now}); err != nil {
					return err
				}
				notified := now.Unix()
				row.UnhealthyNotifiedAt = &notified
			}
		}
		if prior.Revision == row.Revision && sameCondition(prior.Condition, row.Condition) && prior.Restarts == row.Restarts && sameStamp(prior.UnhealthySince, row.UnhealthySince) && sameStamp(prior.UnhealthyNotifiedAt, row.UnhealthyNotifiedAt) {
			continue
		}
		if err := tx.Clauses(clause.OnConflict{UpdateAll: true}).Create(&row).Error; err != nil {
			return err
		}
	}
	for key, row := range before {
		if !alive[key] {
			if err := tx.Delete(&row).Error; err != nil {
				return err
			}
		}
	}
	return nil
}
func sameStamp(a, b *int64) bool { return a == nil && b == nil || a != nil && b != nil && *a == *b }

func sameCondition(a, b string) bool {
	var first, second dto.Condition
	if json.Unmarshal([]byte(a), &first) != nil || json.Unmarshal([]byte(b), &second) != nil {
		return false
	}
	first.SampledAt, second.SampledAt = time.Time{}, time.Time{}
	return first == second
}
