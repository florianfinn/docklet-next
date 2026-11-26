// Source: docklet-next/issues/634
// Revision: 35f39e8729df29cabccc10a4adc42b4b68220a7c
// Retrieved: 2026-11-26
// License: Apache-2.0
// Deviations: Original platform scope for image-level observations lent to containers.

package imageupdate

import (
	"gorm.io/gorm"

	"github.com/florianfinn/docklet-next/backend/internal/imageupdate/children/identity"
	"github.com/florianfinn/docklet-next/types/image"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/samber/mo"
)

// ScopeSharedObservation lends an image-level observation to one container of
// that image. A classic image ID names one platform's build. A containerd ID
// names an index shared by all platforms: a container proven on the observed
// platform keeps everything, one proven on another platform gets an unknown
// observation, and an unproven one keeps the state without build facts.
func ScopeSharedObservation(record *ImageUpdateRecord, descriptor *ocispec.Descriptor) *image.UpdateInfo {
	info := record.UpdateInfo()
	observed, err := identity.DecodeObservation(mo.PointerToOption(record.Platform).OrEmpty(), info.CurrentDigest, info.LatestDigest)
	if err != nil || !observed.SharedIndex() {
		return info
	}
	if running, e := identity.DescriptorPlatform(descriptor); e == nil {
		want, _ := identity.ParsePlatform(info.Platform)
		if running.Matches(want) {
			return info
		}
		return foreignPlatform(info)
	}
	// Without the platform field the web shows neither a date nor a short ID.
	info.Platform, info.CurrentCreated, info.LatestCreated = "", nil, nil
	return info
}

// ScopeServiceObservation lends an image-level observation, made at the
// daemon's default platform, to a Compose service. A service pinned to another
// platform gets an unknown observation instead of that platform's result.
func ScopeServiceObservation(info *image.UpdateInfo, platform string) *image.UpdateInfo {
	if info == nil || platform == "" {
		return info
	}
	pinned, err := identity.ParsePlatform(platform)
	observed, e := identity.ParsePlatform(info.Platform)
	if err == nil && e == nil && pinned.Matches(observed) {
		return info
	}
	return foreignPlatform(info)
}

func foreignPlatform(info *image.UpdateInfo) *image.UpdateInfo {
	unknown := &image.UpdateInfo{ImageRef: info.ImageRef, CheckTime: info.CheckTime, Code: "update.check-unavailable"}
	image.ProjectUpdate(unknown)
	return unknown
}

// ImageRecords loads image-level observations by local image ID.
func ImageRecords(db *gorm.DB, ids []string) (map[string]*ImageUpdateRecord, error) {
	out := make(map[string]*ImageUpdateRecord, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	var rows []ImageUpdateRecord
	if err := db.Where("container_id = ? AND project_id = ? AND id IN ?", "", "", ids).Find(&rows).Error; err != nil {
		return nil, err
	}
	for i := range rows {
		out[rows[i].ID] = &rows[i]
	}
	return out, nil
}
