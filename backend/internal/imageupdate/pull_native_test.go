// Source: docklet-next/docs/design/images-updates-identity.md
// Revision: fcf8c8d3a079154c299912a7e22e0e903e5e4e38
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Native pull acknowledgement, consumer and freshness boundaries.

package imageupdate

import (
	"encoding/json"
	"github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestIdentityPullAcknowledgementStores(t *testing.T) {
	for _, store := range []string{"classic", "containerd"} {
		for _, scenario := range []string{"matching", "newer-preview", "old-container", "wrong-native"} {
			t.Run(store+"/"+scenario, func(t *testing.T) {
				db := setupImageUpdateRegistryTestDBInternal(t)
				old := "sha256:" + strings.Repeat("a", 64)
				next := "sha256:" + strings.Repeat("b", 64)
				later := "sha256:" + strings.Repeat("c", 64)
				record := ImageUpdateRecord{ID: "project::example", ProjectID: "project", Repository: "registry.example.com/app", Tag: "stable", CheckTime: time.Now().UTC(), Platform: new("native-v1:" + store + ":linux/amd64"), CurrentDigest: &old, LatestDigest: &next, HasUpdate: true, UpdateType: UpdateTypeDigest}
				if scenario == "wrong-native" {
					record.LatestDigest = &later
				}
				if scenario == "old-container" {
					record.ContainerID = "old"
					record.ImageID = old
				}
				require.NoError(t, db.Create(&record).Error)
				changed := false
				daemon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					if scenario == "newer-preview" && !changed {
						changed = true
						require.NoError(t, db.Model(&ImageUpdateRecord{}).Where("id = ?", record.ID).Update("latest_digest", later).Error)
					}
					row := map[string]any{"Id": next, "Os": "linux", "Architecture": "amd64"}
					if store == "containerd" {
						row["Descriptor"] = map[string]any{"digest": next, "mediaType": "application/vnd.oci.image.manifest.v1+json"}
					}
					_ = json.NewEncoder(w).Encode(row)
				}))
				t.Cleanup(daemon.Close)
				c, e := client.New(client.WithHost(daemon.URL), client.WithHTTPClient(daemon.Client()), client.WithAPIVersion("1.49"))
				require.NoError(t, e)
				t.Cleanup(func() { _ = c.Close() })
				svc := &ImageUpdateService{db: db, dockerService: &docker.DockerClientService{Client: c}}
				require.NoError(t, svc.MarkImageRefUpToDateAfterPull(t.Context(), "registry.example.com/app:stable"))
				var got ImageUpdateRecord
				require.NoError(t, db.First(&got, "id = ?", record.ID).Error)
				if scenario == "matching" {
					require.False(t, got.HasUpdate)
					require.Equal(t, next, *got.CurrentDigest)
				} else {
					require.True(t, got.HasUpdate)
					require.Equal(t, old, *got.CurrentDigest)
				}
				if scenario == "newer-preview" {
					require.Equal(t, later, *got.LatestDigest)
				}
			})
		}
	}
}
