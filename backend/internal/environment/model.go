// Source: getarcaneapp/arcane/backend/internal/environment/model.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed Swarm; M14 container and host operation contracts; M16 bind mounts and backup completion; M25 status check revision for the host verdict rule.

package environment

import (
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/database"
)

type Environment struct {
	database.BaseModel

	Name               string     `json:"name" sortable:"true" unorm:"nfc" trim:"true"`
	ApiUrl             string     `json:"apiUrl" gorm:"column:api_url" sortable:"true"`
	Status             string     `json:"status" sortable:"true"`
	Enabled            bool       `json:"enabled" sortable:"true"`
	IsEdge             bool       `json:"isEdge" gorm:"column:is_edge;default:false"`
	LastSeen           *time.Time `json:"lastSeen" gorm:"column:last_seen"`
	LastEdgeTransport  *string    `json:"lastEdgeTransport" gorm:"column:last_edge_transport"`
	AccessToken        *string    `json:"-" gorm:"column:access_token"`
	ApiKeyID           *string    `json:"-" gorm:"column:api_key_id"`
	CredentialRevision int64      `json:"registrationRevision" gorm:"column:credential_revision;not null;default:0"`
	Color              *string    `json:"color"`
	// StatusCheckRevision rises with every compatibility result; migration 004 lets a
	// verdict leave update-required only with a higher credential or check revision.
	StatusCheckRevision int64 `json:"-" gorm:"column:status_check_revision;not null;default:0"`
}

func (Environment) TableName() string { return "environments" }

type EnvironmentStatus string

const (
	EnvironmentStatusOnline         EnvironmentStatus = "online"
	EnvironmentStatusStandby        EnvironmentStatus = "standby"
	EnvironmentStatusOffline        EnvironmentStatus = "offline"
	EnvironmentStatusError          EnvironmentStatus = "error"
	EnvironmentStatusPending        EnvironmentStatus = "pending"
	EnvironmentStatusUpdateRequired EnvironmentStatus = "update-required"
	EnvironmentStatusTokenRejected  EnvironmentStatus = "token-rejected"
)
