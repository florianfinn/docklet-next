// Source: docklet-next/docs/design/image-prune-progress.md
// Revision: b637b8c19497e52b7a62987a497ca4ee05b1b5af
// Retrieved: 2026-11-27
// License: Apache-2.0
// Deviations: Image prune activity outcome regression coverage for issue #661.

package image

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/pkg/pagination"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/stretchr/testify/require"
)

const (
	pruneImageA = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	pruneImageB = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

func pruneHandlerFixture(t *testing.T, list string, failing string, onDelete func()) (*ImageHandler, *activity.ActivityService) {
	s, _, a := safetyFixture(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		p := imageDockerTestPathInternal(r.URL.Path)
		switch {
		case p == "/images/json":
			_, _ = fmt.Fprint(w, list)
		case p == "/containers/json":
			_, _ = fmt.Fprint(w, `[]`)
		case r.Method == http.MethodDelete:
			if onDelete != nil {
				onDelete()
			}
			if failing != "" && strings.HasSuffix(p, failing) {
				http.Error(w, "synthetic raw failure", 500)
				return
			}
			_, _ = fmt.Fprint(w, `[{"Deleted":"x"}]`)
		case strings.HasPrefix(p, "/images/") && strings.HasSuffix(p, "/json"):
			_, _ = fmt.Fprintf(w, `{"Id":%q}`, strings.TrimSuffix(strings.TrimPrefix(p, "/images/"), "/json"))
		default:
			w.WriteHeader(404)
			_, _ = fmt.Fprint(w, `{"message":"missing"}`)
		}
	})
	return &ImageHandler{imageService: s, activityService: a}, a
}

func prunedActivities(t *testing.T, a *activity.ActivityService) []activitytypes.Activity {
	items, _, err := a.ListActivitiesPaginated(t.Context(), "0", pagination.QueryParams{})
	require.NoError(t, err)
	out := []activitytypes.Activity{}
	for _, item := range items {
		if item.Type == activitytypes.TypeImagePrune {
			out = append(out, item)
		}
	}
	return out
}

func pruneInput() *PruneImagesInput {
	mode := "all"
	input := &PruneImagesInput{EnvironmentID: "0"}
	input.Body = &struct {
		Mode     *string             `json:"mode,omitempty"`
		Until    *string             `json:"until,omitempty"`
		Dangling *bool               `json:"dangling,omitempty"`
		Filters  map[string][]string `json:"filters,omitempty"`
	}{Mode: &mode}
	return input
}

func userContext(t *testing.T) context.Context {
	actor := user.SystemUser
	return context.WithValue(t.Context(), user.CurrentUserContextKey{}, &actor)
}

func TestImagePruneActivityOutcomes(t *testing.T) {
	two := fmt.Sprintf(`[{"Id":%q},{"Id":%q}]`, pruneImageA, pruneImageB)
	for _, tc := range []struct {
		name, list, failing string
		status              activitytypes.Status
		bulk                *activitytypes.BulkProgress
	}{
		{"all removed", two, "", activitytypes.StatusSuccess, &activitytypes.BulkProgress{Unit: "target", Planned: 2, PlannedKnown: true, Completed: 2, Succeeded: 2}},
		{"one failed", two, pruneImageB, activitytypes.StatusFailed, &activitytypes.BulkProgress{Unit: "target", Planned: 2, PlannedKnown: true, Completed: 2, Succeeded: 1, Failed: 1}},
		{"nothing selected", `[]`, "", "", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, a := pruneHandlerFixture(t, tc.list, tc.failing, nil)
			out, err := h.PruneImages(userContext(t), pruneInput())
			require.NoError(t, err)
			runs := prunedActivities(t, a)
			if tc.bulk == nil {
				require.Empty(t, runs, "an empty cleanup records no activity")
				return
			}
			require.Len(t, runs, 1)
			require.Equal(t, tc.status, runs[0].Status)
			require.Equal(t, tc.bulk, runs[0].Bulk)
			require.Equal(t, tc.bulk.Succeeded, len(out.Body.Data.ImagesDeleted))
			require.Equal(t, tc.bulk.Failed, len(out.Body.Data.Failed))
		})
	}
}

func TestImagePruneActivityKeepsCountsWhenCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(userContext(t))
	defer cancel()
	deletes := 0
	// The first image is removed; cancellation arrives while the second removal is in flight.
	h, a := pruneHandlerFixture(t, fmt.Sprintf(`[{"Id":%q},{"Id":%q}]`, pruneImageA, pruneImageB), "", func() {
		if deletes++; deletes == 2 {
			cancel()
			// Hold the response so the client observes the cancellation, not a late success.
			time.Sleep(100 * time.Millisecond)
		}
	})
	_, err := h.PruneImages(ctx, pruneInput())
	require.Error(t, err)
	runs := prunedActivities(t, a)
	require.Len(t, runs, 1)
	require.NotEqual(t, activitytypes.StatusRunning, runs[0].Status, "an interrupted cleanup is settled")
	require.NotEqual(t, activitytypes.StatusSuccess, runs[0].Status)
	require.Equal(t, 1, runs[0].Bulk.Completed)
	require.Equal(t, 0, runs[0].Bulk.Failed, "the interrupted image is not counted as failed")
}

func TestImagePruneActivityInterruptedByRestartIsSettled(t *testing.T) {
	_, db, a := safetyFixture(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(404) })
	actor := user.SystemUser
	progress := &pruneActivity{service: a, environmentID: "0", user: &actor}
	progress.Begin(t.Context(), 3)
	progress.Image(t.Context(), &imageDTO.PruneReport{ImagesDeleted: []string{pruneImageA}})
	// A new process has no tracked work; startup repair settles the orphaned cleanup with its counts.
	restarted := activity.NewActivityService(db, nil)
	_, err := restarted.RepairStartup(t.Context())
	require.NoError(t, err)
	runs := prunedActivities(t, restarted)
	require.Len(t, runs, 1)
	require.Equal(t, activitytypes.StatusFailed, runs[0].Status)
	require.Equal(t, 1, runs[0].Bulk.Succeeded)
}

func TestImagePruneActivitySettledOnPanic(t *testing.T) {
	_, _, a := safetyFixture(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(404) })
	actor := user.SystemUser
	progress := &pruneActivity{service: a, environmentID: "0", user: &actor}
	require.Panics(t, func() {
		_, _ = progress.run(t.Context(), func(ctx context.Context) (*imageDTO.PruneReport, error) {
			progress.Begin(ctx, 2)
			panic("synthetic prune panic")
		})
	})
	runs := prunedActivities(t, a)
	require.Len(t, runs, 1)
	require.Equal(t, activitytypes.StatusFailed, runs[0].Status)
}
