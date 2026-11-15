// Source: docklet-next/issues/409
// Revision: 45ee2635d68e71671b571cbd8970425ddd194b59
// Retrieved: 2026-11-12
// License: Apache-2.0
// Deviations: Original stale previous-image slot cleanup coverage at prune.

package guard

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/types/system"
	"github.com/stretchr/testify/require"
)

// #409.1: a slot whose owner container is gone no longer pins its image; a slot
// whose owner still exists, even under a recreate's backup name, keeps it.
func TestPruneReleasesStalePreviousSlots(t *testing.T) {
	owner := strings.Repeat("1", 64)
	for _, present := range []bool{false, true} {
		t.Run(fmt.Sprint("owner present ", present), func(t *testing.T) {
			deletes := 0
			g := testGuard(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case r.Method == http.MethodDelete:
					deletes++
					_, _ = fmt.Fprintf(w, `[{"Deleted":%q}]`, imageA)
				case apiPath(r) == "/images/json":
					_, _ = fmt.Fprintf(w, `[{"Id":%q,"Created":100}]`, imageA)
				case apiPath(r) == "/containers/json" && present:
					_, _ = fmt.Fprintf(w, `[{"Id":%q,"Names":["/app-docklet-recreate-run"],"ImageID":"sha256:other"}]`, owner)
				case apiPath(r) == "/containers/json":
					_, _ = fmt.Fprint(w, `[]`)
				default:
					_, _ = fmt.Fprintf(w, `{"Id":%q}`, imageA)
				}
			})
			require.NoError(t, g.DB.Exec("INSERT INTO container_previous_images (container_name, image_id, image_ref, owner_container_id, run_id, created_at) VALUES (?,?,?,?,?,?)", "app", imageA, "registry.example.com/app:stable", owner, "run", time.Now().UTC()).Error)
			require.NoError(t, g.Protect(t.Context(), Protection{ImageID: imageA, Reason: "container-previous", OwnerID: "app"}))
			report, err := g.Prune(t.Context(), system.PruneImagesOptions{Mode: system.PruneImageModeAll})
			require.NoError(t, err)
			var slots int64
			require.NoError(t, g.DB.Table("container_previous_images").Count(&slots).Error)
			reasons, err := g.Reasons(t.Context(), imageA)
			require.NoError(t, err)
			if present {
				require.EqualValues(t, 1, slots)
				require.Len(t, reasons, 1)
				require.Zero(t, deletes)
				require.Len(t, report.Skipped, 1)
				return
			}
			require.Zero(t, slots)
			require.Empty(t, reasons)
			require.Equal(t, 1, deletes)
			require.Equal(t, []string{imageA}, report.ImagesDeleted)
		})
	}
}
