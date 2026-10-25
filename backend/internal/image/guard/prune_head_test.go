// Source: docklet-next/docs/design/images-updates.md
// Revision: f6033a46c9ef4ad490661addd7dac41ab0ed5a45
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Head-only prune and daemon conflict regression coverage.

package guard

import (
	"fmt"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	"github.com/florianfinn/docklet-next/types/system"
	"github.com/stretchr/testify/require"
	"net/http"
	"testing"
)

func TestPruneHeadImagesTimestampAndConflict(t *testing.T) {
	g := testGuard(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case apiPath(r) == "/images/json":
			require.NotContains(t, []string{"true", "1"}, r.URL.Query().Get("all"), "intermediate parent images must not be enumerated")
			_, _ = fmt.Fprintf(w, `[{"Id":%q,"Created":100}]`, imageA)
		case apiPath(r) == "/containers/json":
			_, _ = fmt.Fprint(w, `[]`)
		case r.Method == http.MethodDelete:
			w.WriteHeader(409)
			_, _ = fmt.Fprint(w, `{"message":"synthetic dependent child conflict"}`)
		default:
			_, _ = fmt.Fprintf(w, `{"Id":%q}`, imageA)
		}
	})
	report, err := g.Prune(t.Context(), system.PruneImagesOptions{Mode: system.PruneImageModeOlderThan, Until: "200.5"})
	require.NoError(t, err)
	require.Empty(t, report.Failed)
	require.Equal(t, []imageDTO.PruneItem{{ID: imageA, Code: "image.in-use"}}, report.Skipped)
}
