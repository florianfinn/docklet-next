// Source: docklet-next/docs/design/images-updates.md
// Revision: 474f31a45f4909c153dee30a37306fcd838678da
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Real prune skip counters and pre-daemon delete audit regressions.

package image

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/internal/event"
	"github.com/florianfinn/docklet-next/backend/internal/image/guard"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	"github.com/florianfinn/docklet-next/types/system"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/stretchr/testify/require"
)

func TestPruneSkipsDoNotInflateErrorCounters(t *testing.T) {
	for _, scenario := range []string{"in-use", "restore-point", "active-update", "system", "not-found", "changed", "daemon-conflict"} {
		t.Run(scenario, func(t *testing.T) {
			id := "sha256:" + strings.Repeat("a", 64)
			var inspections atomic.Int32
			s, db, _ := safetyFixture(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch p := imageDockerTestPathInternal(r.URL.Path); {
				case p == "/images/json":
					_, _ = fmt.Fprintf(w, `[{"Id":%q}]`, id)
				case p == "/containers/json":
					if scenario == "in-use" {
						_, _ = fmt.Fprintf(w, `[{"Id":"consumer","ImageID":%q}]`, id)
					} else {
						_, _ = fmt.Fprint(w, `[]`)
					}
				case r.Method == "DELETE":
					require.Equal(t, "daemon-conflict", scenario)
					w.WriteHeader(409)
					_, _ = fmt.Fprint(w, `{"message":"synthetic in-use diagnostic"}`)
				case strings.HasSuffix(p, "/json"):
					if scenario == "not-found" {
						w.WriteHeader(404)
						_, _ = fmt.Fprint(w, `{"message":"not found"}`)
					} else if inspections.Add(1) > 1 && scenario == "changed" {
						_, _ = fmt.Fprintf(w, `{"Id":%q}`, "sha256:"+strings.Repeat("b", 64))
					} else {
						_, _ = fmt.Fprintf(w, `{"Id":%q}`, id)
					}
				}
			})
			if scenario == "restore-point" || scenario == "active-update" || scenario == "system" {
				require.NoError(t, db.Create(&guard.Protection{ImageID: id, Reason: scenario, OwnerID: "owner"}).Error)
			}
			before, err := s.eventService.GetEventSeverityCounts(t.Context())
			require.NoError(t, err)
			report, err := s.PruneImages(t.Context(), system.PruneImagesOptions{Mode: system.PruneImageModeAll})
			require.NoError(t, err)
			require.Len(t, report.Skipped, 1)
			require.Empty(t, report.Failed)
			require.Empty(t, report.ImagesDeleted)
			after, err := s.eventService.GetEventSeverityCounts(t.Context())
			require.NoError(t, err)
			require.Equal(t, before.Error, after.Error, "expected skips cannot change dashboard errors")
		})
	}
}

func TestPruneLeaseFailureRemainsAnErrorEvent(t *testing.T) {
	s, _, _ := safetyFixture(t, func(http.ResponseWriter, *http.Request) { t.Error("busy prune must not call Docker") })
	release, err := s.ImageGuard().Acquire(t.Context())
	require.NoError(t, err)
	defer release()
	_, err = s.PruneImages(t.Context(), system.PruneImagesOptions{Mode: system.PruneImageModeAll})
	require.Equal(t, "image.busy", imageDTO.Neutral(err, "unexpected", 500).Problem.Code)
	counts, err := s.eventService.GetEventSeverityCounts(t.Context())
	require.NoError(t, err)
	require.EqualValues(t, 1, counts.Error)
}

func TestDeleteLeaseBusyHasAttributedAuditBeforeDaemon(t *testing.T) {
	var daemonCalls atomic.Int32
	s, db, _ := safetyFixture(t, func(http.ResponseWriter, *http.Request) { daemonCalls.Add(1) })
	release, err := s.ImageGuard().Acquire(t.Context())
	require.NoError(t, err)
	defer release()
	actor := user.Actor{ID: "operator", Username: "operator"}
	err = s.RemoveImage(context.Background(), "registry.example.com/app:1.0.0", false, actor)
	require.Equal(t, "image.busy", imageDTO.Neutral(err, "unexpected", 500).Problem.Code)
	require.Zero(t, daemonCalls.Load())
	var rows []event.Event
	require.NoError(t, db.Find(&rows).Error)
	require.Len(t, rows, 1)
	require.Equal(t, event.EventSeverityError, rows[0].Severity)
	require.Equal(t, actor.ID, *rows[0].UserID)
	require.Equal(t, actor.Username, *rows[0].Username)
	require.Equal(t, "delete", rows[0].Metadata["action"])
	require.Equal(t, "image.busy", rows[0].Metadata["code"])
}

func TestDeleteClientUnavailableHasAttributedAuditBeforeDaemon(t *testing.T) {
	var daemonCalls atomic.Int32
	s, db, _ := safetyFixture(t, func(http.ResponseWriter, *http.Request) { daemonCalls.Add(1) })
	s.dockerService = docker.NewDockerClientService(t.Context(), db, &config.Config{DockerHost: "invalid-docker-host"}, nil)
	actor := user.Actor{ID: "operator", Username: "operator"}
	err := s.RemoveImage(t.Context(), "registry.example.com/app:1.0.0", false, actor)
	require.Equal(t, "image.remove-failed", imageDTO.Neutral(err, "unexpected", 500).Problem.Code)
	require.Zero(t, daemonCalls.Load())
	var rows []event.Event
	require.NoError(t, db.Find(&rows).Error)
	require.Len(t, rows, 1)
	require.Equal(t, event.EventSeverityError, rows[0].Severity)
	require.Equal(t, actor.ID, *rows[0].UserID)
	require.Equal(t, actor.Username, *rows[0].Username)
	require.Equal(t, "delete", rows[0].Metadata["action"])
	require.Equal(t, "image.remove-failed", rows[0].Metadata["code"])
	release, err := s.ImageGuard().Acquire(t.Context())
	require.NoError(t, err, "client failure must release the image lease")
	release()
}
