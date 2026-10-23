// Source: docklet-next/docs/design/storage-workspace.md
// Revision: ce3c4ea4af5b0bc1cdb2bb287d30281bbe20f0a8
// Retrieved: 2026-10-22
// License: Apache-2.0
// Deviations: Original prune event metadata regression for issue #179.
package volume

import (
	"encoding/json"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/event"
	"github.com/florianfinn/docklet-next/backend/internal/testsqlite"
	volumetypes "github.com/florianfinn/docklet-next/types/volume"
	dockervolume "github.com/moby/moby/api/types/volume"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestPruneEventMetadataReflectsRequestedSelection(t *testing.T) {
	for _, transport := range []string{"direct", "edge"} {
		for _, all := range []bool{false, true} {
			t.Run(transport+map[bool]string{false: "/anonymous", true: "/all"}[all], func(t *testing.T) {
				db, err := gorm.Open(testsqlite.Dialector(t, ":memory:"), &gorm.Config{})
				require.NoError(t, err)
				require.NoError(t, db.AutoMigrate(&event.Event{}))
				fixture := &pruneDockerFixture{volumes: []dockervolume.Volume{pruneVolume("selected")}}
				svc := fixture.service(t)
				svc.eventService = event.NewEventService(&database.DB{DB: db}, nil, nil)
				body, err := json.Marshal(volumetypes.PruneRequest{All: all, Candidates: []volumetypes.PruneCandidate{{Name: "selected", CreatedAt: pruneCreated}}})
				require.NoError(t, err)
				call := pruneTransport(t, transport, pruneRouter(t, svc))
				status, response := call("POST", "/api/environments/0/volumes/prune", "", body)
				require.Equal(t, 200, status, string(response))
				var recorded event.Event
				require.NoError(t, db.Where("type = ?", event.EventTypeVolumePrune).First(&recorded).Error)
				require.Equal(t, all, recorded.Metadata["all"])
			})
		}
	}
}
