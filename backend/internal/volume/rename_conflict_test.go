// Source: docklet-next/issues/217
// Revision: 3cba55fd10c311e4a5b3361afabb8c93fca706a0
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Original name clash regression through real service, HTTP and Edge.

package volume

import (
	"encoding/json"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/event"
	"github.com/florianfinn/docklet-next/backend/internal/testsqlite"
	volumetypes "github.com/florianfinn/docklet-next/types/volume"
	"github.com/moby/moby/api/types/volume"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestRenameExistingVolumeThroughDirectAndEdge(t *testing.T) {
	for _, mode := range []string{"direct", "edge"} {
		t.Run(mode, func(t *testing.T) {
			f := &pruneDockerFixture{volumes: []volume.Volume{pruneVolume("source"), pruneVolume("occupied")}}
			svc := f.service(t)
			db, err := gorm.Open(testsqlite.Dialector(t, ":memory:"), &gorm.Config{})
			require.NoError(t, err)
			require.NoError(t, db.AutoMigrate(&event.Event{}))
			svc.eventService = event.NewEventService(&database.DB{DB: db}, nil, nil)
			call := pruneTransport(t, mode, pruneRouter(t, svc))
			status, body := call("POST", "/api/environments/0/volumes/source/rename", "", []byte(`{"name":"occupied"}`))
			require.Equal(t, 409, status, string(body))
			var problem volumetypes.ExistsError
			require.NoError(t, json.Unmarshal(body, &problem))
			require.Equal(t, "volume.exists", problem.Code)
			require.Equal(t, 409, problem.Status)
			require.Equal(t, "A volume with this name already exists", problem.Detail)
			require.Empty(t, f.removed)
			require.Len(t, f.volumes, 2)
		})
	}
}
