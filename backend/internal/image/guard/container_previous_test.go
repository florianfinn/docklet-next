// Source: docklet-next/issues/342
// Revision: 89f69980c01d08714ce96c49365fb19065e3d6ba
// Retrieved: 2026-11-05
// License: Apache-2.0
// Deviations: Original previous-container image delete/prune protection and public reason proofs.

package guard

import (
	"fmt"
	"net/http"
	"testing"

	dto "github.com/florianfinn/docklet-next/types/image"
	"github.com/florianfinn/docklet-next/types/system"
	"github.com/stretchr/testify/require"
)

func TestContainerPreviousProtectionDeleteAndPrune(t *testing.T) {
	deletes := 0
	g := testGuard(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodDelete:
			deletes++
			_, _ = fmt.Fprint(w, `[]`)
		case apiPath(r) == "/images/json":
			_, _ = fmt.Fprintf(w, `[{"Id":%q,"Created":100}]`, imageA)
		case apiPath(r) == "/containers/json":
			_, _ = fmt.Fprint(w, `[]`)
		default:
			_, _ = fmt.Fprintf(w, `{"Id":%q}`, imageA)
		}
	})
	require.NoError(t, g.Protect(t.Context(), Protection{ImageID: imageA, Reason: "container-previous", OwnerID: "app"}))
	reasons, err := g.Reasons(t.Context(), imageA)
	require.NoError(t, err)
	require.Equal(t, []dto.Protection{{Reason: "restore-point", OwnerID: "app"}}, reasons)
	var pin Protection
	require.NoError(t, g.DB.First(&pin).Error)
	require.Equal(t, "container-previous", pin.Reason)
	release, err := g.Acquire(t.Context())
	require.NoError(t, err)
	c, err := g.Docker(t.Context())
	require.NoError(t, err)
	_, err = g.RemoveLocked(t.Context(), c, imageA)
	require.Equal(t, "image.restore-point-protected", dto.Neutral(err, "unexpected", 500).Problem.Code)
	release()
	report, err := g.Prune(t.Context(), system.PruneImagesOptions{Mode: system.PruneImageModeAll})
	require.NoError(t, err)
	require.Equal(t, []dto.PruneItem{{ID: imageA, Code: "image.restore-point-protected"}}, report.Skipped)
	require.Empty(t, report.ImagesDeleted)
	require.Empty(t, report.Failed)
	require.Zero(t, deletes)
}
