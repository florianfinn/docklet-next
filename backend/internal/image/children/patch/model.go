// Source: getarcaneapp/arcane/backend/internal/image/children/patch/model.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace

package patch

import (
	"github.com/florianfinn/docklet-next/types/imagepatch"

	"github.com/florianfinn/docklet-next/backend/internal/database"
)

// ImagePatchRecord stores one Copacetic image patch run.
type ImagePatchRecord struct {
	database.BaseModel

	EnvironmentID   string  `json:"environmentId" gorm:"column:environment_id;index:idx_image_patches_env_image"`
	OriginalImageID string  `json:"originalImageId" gorm:"column:original_image_id;index:idx_image_patches_env_image"`
	OriginalRef     string  `json:"originalRef" gorm:"column:original_ref"`
	OriginalDigest  string  `json:"originalDigest,omitempty" gorm:"column:original_digest"`
	PatchedRef      string  `json:"patchedRef" gorm:"column:patched_ref"`
	Mode            string  `json:"mode" gorm:"column:mode"`
	Status          string  `json:"status" gorm:"column:status" sortable:"true"`
	PackagesUpdated *int    `json:"packagesUpdated,omitempty" gorm:"column:packages_updated"`
	Error           *string `json:"error,omitempty" gorm:"column:error"`
	ActivityID      *string `json:"activityId,omitempty" gorm:"column:activity_id"`
	DurationMs      *int64  `json:"durationMs,omitempty" gorm:"column:duration_ms"`
}

func (r *ImagePatchRecord) TableName() string {
	return "image_patches"
}

func (r *ImagePatchRecord) ToDto() imagepatch.PatchRecord {
	return imagepatch.PatchRecord{
		ID:              r.ID,
		EnvironmentID:   r.EnvironmentID,
		OriginalImageID: r.OriginalImageID,
		OriginalRef:     r.OriginalRef,
		OriginalDigest:  r.OriginalDigest,
		PatchedRef:      r.PatchedRef,
		Mode:            imagepatch.PatchMode(r.Mode),
		Status:          imagepatch.PatchStatus(r.Status),
		PackagesUpdated: r.PackagesUpdated,
		Error:           r.Error,
		ActivityID:      r.ActivityID,
		DurationMs:      r.DurationMs,
		CreatedAt:       r.CreatedAt,
		UpdatedAt:       r.UpdatedAt,
	}
}
