// Source: docklet-next/issues/342
// Revision: 89f69980c01d08714ce96c49365fb19065e3d6ba
// Retrieved: 2026-11-05
// License: Apache-2.0
// Deviations: Original real local/Direct/Edge previous-image detail projection proofs.

package activity_test

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	containers "github.com/florianfinn/docklet-next/backend/internal/container"
	"github.com/florianfinn/docklet-next/backend/internal/image/guard"
	"github.com/florianfinn/docklet-next/types/base"
	dto "github.com/florianfinn/docklet-next/types/container"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestContainerPreviousImageDetailsLocalDirectEdge(t *testing.T) {
	for _, mode := range []string{"local", "direct", "edge"} {
		t.Run(mode, func(t *testing.T) {
			hub := activationApp(t, false, "")
			executor, env := hub, "0"
			if mode != "local" {
				executor = activationApp(t, true, uuid.NewString())
				env = activationRemote(t, hub, executor, mode)
			}
			daemon, reg := newContainerImageHTTP(t)
			_ = reg
			daemon.images[runtimeContainerID] = daemon.newImage
			daemon.tag = daemon.newImage
			executor.services.http.onCall = daemon.serve
			slot := containers.PreviousImage{ContainerName: "app", ImageID: daemon.oldImage, ImageRef: daemon.ref, RunID: "original-run", CreatedAt: time.Now().UTC(), OwnerContainerID: runtimeContainerID}
			require.NoError(t, executor.services.db.Create(&slot).Error)
			require.NoError(t, executor.services.db.First(&slot, "container_name=?", "app").Error)
			require.NoError(t, executor.services.images.ImageGuard().Protect(t.Context(), guard.Protection{ImageID: slot.ImageID, Reason: "container-previous", OwnerID: "app"}))
			read := func() dto.Details {
				response, err := hub.server.Client().Get(hub.server.URL + "/api/environments/" + env + "/containers/" + runtimeContainerID)
				require.NoError(t, err)
				defer response.Body.Close()
				body, err := io.ReadAll(response.Body)
				require.NoError(t, err)
				require.Equal(t, http.StatusOK, response.StatusCode, string(body))
				var decoded base.ApiResponse[dto.Details]
				require.NoError(t, json.Unmarshal(body, &decoded))
				require.True(t, decoded.Success)
				return decoded.Data
			}
			details := read()
			require.NotNil(t, details.PreviousImage)
			require.Equal(t, slot.ImageRef, details.PreviousImage.ImageRef)
			require.Equal(t, slot.ImageID, details.PreviousImage.ImageID)
			require.True(t, slot.CreatedAt.Equal(details.PreviousImage.CreatedAt))
			require.NoError(t, executor.services.db.Model(&slot).UpdateColumn("owner_container_id", recreateNewID).Error)
			details = read()
			require.Nil(t, details.PreviousImage, "a stale owner must be hidden on this read path")
		})
	}
}
