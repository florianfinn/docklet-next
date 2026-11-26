// Source: docklet-next/docs/design/image-prune-progress.md
// Revision: b637b8c19497e52b7a62987a497ca4ee05b1b5af
// Retrieved: 2026-11-27
// License: Apache-2.0
// Deviations: Per-image prune progress regression coverage for issue #661.

package guard

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	imageDTO "github.com/florianfinn/docklet-next/types/image"
	"github.com/florianfinn/docklet-next/types/system"
	"github.com/stretchr/testify/require"
)

type recordingObserver struct {
	planned []int
	counts  [][3]int
	cancel  context.CancelFunc
}

func (o *recordingObserver) Begin(ctx context.Context, planned int) context.Context {
	o.planned = append(o.planned, planned)
	return ctx
}

func (o *recordingObserver) Image(_ context.Context, r *imageDTO.PruneReport) {
	o.counts = append(o.counts, [3]int{len(r.ImagesDeleted), len(r.Skipped), len(r.Failed)})
	if o.cancel != nil {
		o.cancel()
	}
}

func pruneDaemon(t *testing.T, deletes *atomic.Int32, list string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		p := apiPath(r)
		switch {
		case p == "/images/json":
			_, _ = fmt.Fprint(w, list)
		case p == "/containers/json":
			_, _ = fmt.Fprint(w, `[]`)
		case r.Method == http.MethodDelete:
			deletes.Add(1)
			if strings.HasSuffix(p, imageC) {
				http.Error(w, "synthetic raw failure", 500)
				return
			}
			_, _ = fmt.Fprint(w, `[{"Deleted":"x"}]`)
		case strings.HasPrefix(p, "/images/") && strings.HasSuffix(p, "/json"):
			_, _ = fmt.Fprintf(w, `{"Id":%q}`, strings.TrimSuffix(strings.TrimPrefix(p, "/images/"), "/json"))
		default:
			t.Errorf("unexpected %s", p)
		}
	}
}

func TestPruneObserverSeesPlanAndEveryImage(t *testing.T) {
	var deletes atomic.Int32
	g := testGuard(t, pruneDaemon(t, &deletes, fmt.Sprintf(`[{"Id":%q},{"Id":%q},{"Id":%q},{"Id":%q}]`, imageA, imageB, imageC, imageB)))
	require.NoError(t, g.Protect(t.Context(), Protection{ImageID: imageA, Reason: "restore-point", OwnerID: "point"}))
	o := &recordingObserver{}
	report, err := g.Prune(WithPruneObserver(t.Context(), o), system.PruneImagesOptions{Mode: system.PruneImageModeAll})
	require.NoError(t, err)
	// Duplicate list entries are planned once; every processed image reports the counts so far.
	require.Equal(t, []int{3}, o.planned)
	require.Equal(t, [][3]int{{0, 1, 0}, {1, 1, 0}, {1, 1, 1}}, o.counts)
	require.Equal(t, [3]int{len(report.ImagesDeleted), len(report.Skipped), len(report.Failed)}, o.counts[len(o.counts)-1])
}

func TestPruneObserverNotStartedWithoutCandidates(t *testing.T) {
	var deletes atomic.Int32
	g := testGuard(t, pruneDaemon(t, &deletes, `[]`))
	o := &recordingObserver{}
	report, err := g.Prune(WithPruneObserver(t.Context(), o), system.PruneImagesOptions{Mode: system.PruneImageModeDangling})
	require.NoError(t, err)
	require.Empty(t, report.ImagesDeleted)
	require.Empty(t, o.planned)
}

func TestPruneStopsBeforeNextImageWhenCancelled(t *testing.T) {
	var deletes atomic.Int32
	g := testGuard(t, pruneDaemon(t, &deletes, fmt.Sprintf(`[{"Id":%q},{"Id":%q}]`, imageA, imageB)))
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	o := &recordingObserver{cancel: cancel}
	report, err := g.Prune(WithPruneObserver(ctx, o), system.PruneImagesOptions{Mode: system.PruneImageModeAll})
	require.ErrorIs(t, err, context.Canceled)
	require.EqualValues(t, 1, deletes.Load(), "no removal after cancellation")
	require.Equal(t, []string{imageA}, report.ImagesDeleted)
	require.Empty(t, report.Failed, "interrupted images are not counted as failed")
}
