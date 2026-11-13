// Source: docklet-next/issues/372
// Revision: e83119924556ce53fea232ceac7a270bd595304b
// Retrieved: 2026-11-13
// License: Apache-2.0
// Deviations: Original frozen copy of the hosts model as shipped in migration 001.

package schema

import (
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/database"
)

// baselineEnvironment is the hosts model exactly as migration 001 created it. Columns
// added by later forward migrations stay on environment.Environment only, so the
// baseline generator keeps reproducing 001 byte for byte.
type baselineEnvironment struct {
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
}

func (baselineEnvironment) TableName() string { return "environments" }
