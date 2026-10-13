// Source: getarcaneapp/arcane/backend/internal/project/model.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed GitOps and saved Git repositories.

package project

import (
	"time"

	projecttypes "github.com/florianfinn/docklet-next/types/project"

	"github.com/florianfinn/docklet-next/backend/internal/database"
)

type ProjectStatus string

const (
	ProjectStatusRunning          ProjectStatus = projecttypes.StatusRunning
	ProjectStatusStopped          ProjectStatus = projecttypes.StatusStopped
	ProjectStatusPartiallyRunning ProjectStatus = projecttypes.StatusPartiallyRunning
	ProjectStatusUnknown          ProjectStatus = projecttypes.StatusUnknown
	ProjectStatusDeploying        ProjectStatus = projecttypes.StatusDeploying
	ProjectStatusStopping         ProjectStatus = projecttypes.StatusStopping
	ProjectStatusRestarting       ProjectStatus = projecttypes.StatusRestarting
)

type Project struct {
	database.BaseModel

	Name               string        `json:"name" sortable:"true" gorm:"index:idx_projects_name"`
	DirName            *string       `json:"dir_name"`
	Path               string        `json:"path" sortable:"true" gorm:"uniqueIndex"`
	Status             ProjectStatus `json:"status" sortable:"true"`
	StatusReason       *string       `json:"status_reason"`
	ServiceCount       int           `json:"service_count" sortable:"true"`
	RunningCount       int           `json:"running_count" sortable:"true"`
	ComposeProjectName *string       `json:"compose_project_name,omitempty" gorm:"column:compose_project_name"`
	ImageRefsJSON      string        `json:"image_refs_json,omitempty" gorm:"column:image_refs_json"`
	BuildImageRefsJSON *string       `json:"-" gorm:"column:build_image_refs_json"`
	IsArchived         bool          `json:"is_archived" gorm:"column:is_archived;default:false;index"`
	ArchivedAt         *time.Time    `json:"archived_at,omitempty" gorm:"column:archived_at"`
}

func (Project) TableName() string {
	return "projects"
}

// ProjectTag stores one normalized tag association and its management source.
type ProjectTag struct {
	ProjectID string `json:"projectId" gorm:"column:project_id;primaryKey"`
	Name      string `json:"name" gorm:"column:name;primaryKey"`
	Source    string `json:"source" gorm:"column:source;primaryKey"`
	Color     string `json:"color" gorm:"column:color;not null;default:gray"`
}

// TableName returns the database table used for project tag associations.
func (ProjectTag) TableName() string {
	return "project_tags"
}
