// Source: getarcaneapp/arcane/backend/internal/registry/model.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace

package registry

import (
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/database"
)

type ContainerRegistry struct {
	database.BaseModel

	URL                 string               `json:"url" sortable:"true"`
	Username            string               `json:"-" sortable:"true"`
	Token               string               `json:"-"`
	Description         *string              `json:"description,omitempty" sortable:"true"`
	Insecure            bool                 `json:"insecure" sortable:"true"`
	Enabled             bool                 `json:"enabled" sortable:"true"`
	RegistryType        string               `json:"registryType" sortable:"true"`
	RepositoryNames     database.StringSlice `json:"repositoryNames" gorm:"column:repository_names;type:text;not null;default:'[]'"`
	AWSAccessKeyID      string               `json:"-"`
	AWSSecretAccessKey  string               `json:"-"`
	AWSRegion           string               `json:"awsRegion"`
	ECRToken            string               `json:"-"`
	ECRTokenGeneratedAt *time.Time           `json:"-"`
	CreatedAt           time.Time            `json:"createdAt" sortable:"true"`
	UpdatedAt           time.Time            `json:"updatedAt" sortable:"true"`
}

func (ContainerRegistry) TableName() string {
	return "container_registries"
}
