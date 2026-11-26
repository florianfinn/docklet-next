// Source: docklet-next/issues/634
// Revision: 35f39e8729df29cabccc10a4adc42b4b68220a7c
// Retrieved: 2026-11-26
// License: Apache-2.0
// Deviations: Original platform scope for image-level observations lent to containers and services.

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
// names an index shared by all platforms, so build facts stay only for a
// container proven on the observed platform.
func ScopeSharedObservation(record *ImageUpdateRecord, descriptor *ocispec.Descriptor) *image.UpdateInfo {
	info := record.UpdateInfo()
	observed, err := identity.DecodeObservation(mo.PointerToOption(record.Platform).OrEmpty(), info.CurrentDigest, info.LatestDigest)
	if err != nil || !observed.SharedIndex() {
		return info
	}
	if running, e := identity.DescriptorPlatform(descriptor); e == nil && SamePlatform(running.String(), info.Platform) {
		return info
	}
	return image.Lend(info)
}

// SamePlatform compares two platform strings; unparseable ones never match.
func SamePlatform(a, b string) bool {
	x, err := identity.ParsePlatform(a)
	y, e := identity.ParsePlatform(b)
	return err == nil && e == nil && x.Matches(y)
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
