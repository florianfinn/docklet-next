// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original target-check collection through the authenticated activity producer.

package backup

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/backuptarget"
	activitydto "github.com/florianfinn/docklet-next/types/activity"
	dto "github.com/florianfinn/docklet-next/types/backup"
	"github.com/florianfinn/docklet-next/types/base"
	"gorm.io/gorm/clause"
)

func (s *Service) recordRemoteCheck(ctx context.Context, id, host string, version int64, activityID string) error {
	configVersion := int64(0)
	if id != "local" {
		var row backuptarget.Target
		if err := s.deps.DB.WithContext(ctx).First(&row, "id = ?", id).Error; err != nil {
			return err
		}
		configVersion = row.ConfigVersion
	}
	return s.deps.DB.WithContext(ctx).Clauses(clause.OnConflict{UpdateAll: true}).Create(&backuptarget.Check{TargetID: id, HostID: host, SetVersion: version, ConfigVersion: configVersion, ActivityID: activityID, At: time.Now().UTC()}).Error
}

// Collection is a bounded read during target-list requests, so it creates no
// untracked worker that could write through a hub database replacement.
func (s *Service) refreshTargetChecks(ctx context.Context, proxy backuptarget.JSONProxy) error {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	var rows []backuptarget.Check
	if err := s.deps.DB.WithContext(ctx).Where("host_id <> ? AND ok = ? AND (code IS NULL OR code = '')", "0", false).Find(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		var response base.ApiResponse[activitydto.Detail]
		if err := proxy(ctx, row.HostID, http.MethodGet, "/api/environments/0/activities/"+url.PathEscape(row.ActivityID)+"?limit=1", nil, &response); err != nil {
			continue
		}
		activity := response.Data.Activity
		if !response.Success || activity.ID != row.ActivityID || (activity.Status != "success" && activity.Status != "failed" && activity.Status != "cancelled") {
			continue
		}
		if activity.Metadata["backupTargetId"] != row.TargetID {
			continue
		}
		encoded, err := json.Marshal(activity.Metadata["backupTargetCheck"])
		if err != nil {
			return err
		}
		var check dto.TargetCheck
		if err = json.Unmarshal(encoded, &check); err != nil {
			continue
		}
		if check.HostID != row.HostID || check.ActivityID != row.ActivityID || check.ConfigVersion != row.ConfigVersion || check.SetVersion != row.SetVersion {
			continue
		}
		// Only safe structured fields leave the owning host. Unknown diagnostics
		// are refused instead of entering the hub's saved result.
		if check.Code != "" && !dto.IsProblemCode(check.Code) {
			check.Code = "volume.backup.repository-check-failed"
		}
		if err = s.deps.DB.WithContext(ctx).Model(&backuptarget.Check{}).Where("target_id = ? AND host_id = ? AND activity_id = ?", row.TargetID, row.HostID, row.ActivityID).Updates(map[string]any{"ok": check.OK, "code": check.Code, "at": check.At, "snapshot_count": check.SnapshotCount}).Error; err != nil {
			return err
		}
	}
	return nil
}
