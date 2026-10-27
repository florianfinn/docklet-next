// Source: docklet-next/docs/design/images-updates.md
// Revision: f6033a46c9ef4ad490661addd7dac41ab0ed5a45
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Persisted image audit and canonical removal cleanup regressions.

package image

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/event"
	"github.com/florianfinn/docklet-next/backend/internal/imageupdate"
	"github.com/florianfinn/docklet-next/backend/internal/vulnerability"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	"github.com/florianfinn/docklet-next/types/system"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func testLoadArchive(t *testing.T) []byte {
	t.Helper()
	var body bytes.Buffer
	writer := tar.NewWriter(&body)
	manifest := `[{"Config":"config.json","RepoTags":["registry.example.com/app:1.0.0"],"Layers":[]}]`
	require.NoError(t, writer.WriteHeader(&tar.Header{Name: "manifest.json", Mode: 0600, Size: int64(len(manifest))}))
	_, err := io.WriteString(writer, manifest)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	return body.Bytes()
}
func TestImageMutationAuditSuccessFailureAndPrivacy(t *testing.T) {
	for _, failed := range []bool{false, true} {
		for _, action := range []string{"pull", "tag", "load", "delete", "bulk_prune", "compose-pull"} {
			t.Run(fmt.Sprintf("%s/%v", action, failed), func(t *testing.T) {
				id := "sha256:" + strings.Repeat("a", 64)
				sentinel := "synthetic-private-daemon-diagnostic"
				s, db, _ := safetyFixture(t, func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					p := imageDockerTestPathInternal(r.URL.Path)
					if strings.HasSuffix(p, "/json") && p != "/images/json" && p != "/containers/json" {
						_, _ = fmt.Fprintf(w, `{"Id":%q}`, id)
						return
					}
					if p == "/containers/json" {
						_, _ = fmt.Fprint(w, `[]`)
						return
					}
					if p == "/images/json" {
						_, _ = fmt.Fprintf(w, `[{"Id":%q}]`, id)
						return
					}
					if failed {
						w.WriteHeader(500)
						_, _ = fmt.Fprintf(w, `{"message":%q}`, sentinel)
						return
					}
					if p == "/images/create" {
						_, _ = fmt.Fprint(w, `{"status":"Pull complete"}`)
					} else if p == "/images/load" {
						_, _ = fmt.Fprint(w, `{"stream":"Loaded"}`)
					} else if r.Method == "DELETE" {
						_, _ = fmt.Fprint(w, `[]`)
					} else {
						w.WriteHeader(201)
					}
				})
				actor := user.Actor{ID: "operator", Username: "operator"}
				ctx := context.WithValue(t.Context(), user.CurrentUserContextKey{}, &actor)
				var err error
				switch action {
				case "pull":
					err = s.PullImage(ctx, "registry.example.com/app:1.0.0", io.Discard, actor, nil)
				case "compose-pull":
					err = s.PullComposeImage(ctx, "registry.example.com/app:1.0.0", io.Discard, actor, nil)
				case "tag":
					err = s.TagImage(ctx, id, imageDTO.TagRequest{Repository: "registry.example.com/app", Tag: "1.0.0"}, actor)
				case "load":
					_, err = s.LoadImageFromReader(ctx, bytes.NewReader(testLoadArchive(t)), "image.tar", actor, 1<<20)
				case "delete":
					err = s.RemoveImage(ctx, "registry.example.com/alias:1.0.0", false, actor)
				case "bulk_prune":
					report, e := s.PruneImages(ctx, system.PruneImagesOptions{Mode: system.PruneImageModeAll})
					err = e
					if failed {
						require.Len(t, report.Failed, 1)
					}
				}
				if action != "bulk_prune" {
					if failed {
						require.Error(t, err)
					} else {
						require.NoError(t, err)
					}
				}
				var rows []event.Event
				require.NoError(t, db.Find(&rows).Error)
				require.Len(t, rows, 1, "every action must leave one attributed service event")
				row := rows[0]
				require.Equal(t, actor.ID, *row.UserID)
				require.Equal(t, actor.Username, *row.Username)
				wantAction := action
				if wantAction == "compose-pull" {
					wantAction = "pull"
				}
				require.Equal(t, wantAction, row.Metadata["action"])
				require.NotEmpty(t, row.Metadata["code"])
				if failed {
					require.Equal(t, event.EventSeverityError, row.Severity)
				}
				encoded, e := json.Marshal(rows)
				require.NoError(t, e)
				require.NotContains(t, string(encoded), sentinel)
			})
		}
	}
}
func TestCanonicalRemovalCleansBothRecordStoresAcrossCallers(t *testing.T) {
	for _, action := range []string{"delete", "bulk_prune", "scheduled-prune"} {
		t.Run(action, func(t *testing.T) {
			id := "sha256:" + strings.Repeat("a", 64)
			s, db, _ := safetyFixture(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				p := imageDockerTestPathInternal(r.URL.Path)
				switch {
				case p == "/images/json":
					_, _ = fmt.Fprintf(w, `[{"Id":%q}]`, id)
				case p == "/containers/json":
					_, _ = fmt.Fprint(w, `[]`)
				case r.Method == "DELETE":
					_, _ = fmt.Fprint(w, `[]`)
				default:
					_, _ = fmt.Fprintf(w, `{"Id":%q}`, id)
				}
			})
			s.imageUpdateService = imageupdate.NewImageUpdateService(db, nil, nil, nil, nil, nil, nil)
			s.vulnerabilityService = vulnerability.NewVulnerabilityService(db, &config.Config{}, nil, nil, nil, nil, nil, nil, nil, nil)
			require.NoError(t, db.Create(&imageupdate.ImageUpdateRecord{ID: id, Repository: "registry.example.com/app", Tag: "1.0.0", CheckTime: time.Now()}).Error)
			require.NoError(t, db.Create(&imageupdate.ImageUpdateRecord{ID: "consumer", ImageID: id, ContainerID: "consumer", CheckTime: time.Now()}).Error)
			require.NoError(t, db.Create(&vulnerability.VulnerabilityScanRecord{ID: id, ImageName: "registry.example.com/app:1.0.0", Status: "completed", ScanTime: time.Now()}).Error)
			switch action {
			case "delete":
				require.NoError(t, s.RemoveImage(t.Context(), "registry.example.com/alias:1.0.0", false, user.SystemUser))
			case "bulk_prune":
				_, err := s.PruneImages(t.Context(), system.PruneImagesOptions{Mode: system.PruneImageModeAll})
				require.NoError(t, err)
			case "scheduled-prune":
				c, err := s.dockerService.GetClient(t.Context())
				require.NoError(t, err)
				release, err := s.safety.Acquire(t.Context())
				require.NoError(t, err)
				defer release()
				_, err = s.safety.RemoveLocked(t.Context(), c, id, "bulk_prune")
				require.NoError(t, err)
			}
			for _, table := range []string{"image_updates", "vulnerability_scans"} {
				var count int64
				require.NoError(t, db.Table(table).Count(&count).Error)
				require.Zero(t, count, "cleanup of %s after %s", table, action)
			}
		})
	}
}

func TestLoadStreamFailureIsAuditedAndNeutral(t *testing.T) {
	s, db, _ := safetyFixture(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"errorDetail":{"message":"synthetic-private-load-diagnostic"}}`)
	})
	actor := user.Actor{ID: "operator", Username: "operator"}
	_, err := s.LoadImageFromReader(t.Context(), bytes.NewReader(testLoadArchive(t)), "image.tar", actor, 1<<20)
	require.Error(t, err)
	require.NotContains(t, err.Error(), "synthetic-private-load-diagnostic")
	var rows []event.Event
	require.NoError(t, db.Find(&rows).Error)
	require.Len(t, rows, 1)
	require.Equal(t, event.EventSeverityError, rows[0].Severity)
	require.Equal(t, "image.load-failed", rows[0].Metadata["code"])
	encoded, e := json.Marshal(rows)
	require.NoError(t, e)
	require.NotContains(t, string(encoded), "synthetic-private-load-diagnostic")
}
