// Source: getarcaneapp/arcane/backend/internal/environment/model.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed Swarm; M14 container and host operation contracts; M16 bind mounts and backup completion; M25 status check revision for the host verdict rule; #573 network route and tailnet observation.

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
	// NetworkRoute is lan, domain or tailnet; NULL keeps legacy rows unclassified (migration 007).
	NetworkRoute *string `json:"networkRoute" gorm:"column:network_route"`
	// Tailnet observation is diagnostic only; it never authenticates (token and mTLS do).
	TailnetNodeID     *string    `json:"-" gorm:"column:tailnet_node_id"`
	TailnetTagged     *bool      `json:"-" gorm:"column:tailnet_tagged"`
	TailnetObservedAt *time.Time `json:"-" gorm:"column:tailnet_observed_at"`
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
