// Source: docklet-next/docs/design/images-updates-identity.md
// Revision: fcf8c8d3a079154c299912a7e22e0e903e5e4e38
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Native check producer with deleted historical registry identity.

package imageupdate

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/docker"
	registryService "github.com/florianfinn/docklet-next/backend/internal/registry"
	"github.com/google/go-containerregistry/pkg/name"
	registryServer "github.com/google/go-containerregistry/pkg/registry"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	registryTypes "github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/require"
)

func TestIdentityCheckStores(t *testing.T) {
	for _, store := range []string{"classic", "containerd-index", "containerd-manifest"} {
		for _, change := range []string{"current", "same-config", "rebuild", "version"} {
			t.Run(store+"/"+change, func(t *testing.T) {
				original, e := random.Image(128, 1)
				require.NoError(t, e)
				cfg, e := original.ConfigFile()
				require.NoError(t, e)
				cfg.OS = "linux"
				cfg.Architecture = "amd64"
				cfg.Config.Labels = map[string]string{"org.opencontainers.image.version": "1.0.0"}
				original, e = mutate.ConfigFile(original, cfg)
				require.NoError(t, e)
				original = mutate.MediaType(original, registryTypes.DockerManifestSchema2)
				oldManifest, e := original.Digest()
				require.NoError(t, e)
				oldConfig, e := original.ConfigName()
				require.NoError(t, e)
				latest := original
				if change == "same-config" {
					latest = mutate.MediaType(original, registryTypes.OCIManifestSchema1)
				}
				if change == "rebuild" || change == "version" {
					cfg.Config.Labels["synthetic-generation"] = "next"
					if change == "version" {
						cfg.Config.Labels["org.opencontainers.image.version"] = "1.1.0"
					}
					latest, e = mutate.ConfigFile(original, cfg)
					require.NoError(t, e)
				}
				var historical atomic.Int32
				registryHandler := registryServer.New()
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if strings.Contains(r.URL.Path, strings.Repeat("f", 64)) {
						historical.Add(1)
						w.WriteHeader(410)
						return
					}
					registryHandler.ServeHTTP(w, r)
				}))
				t.Cleanup(server.Close)
				host := strings.TrimPrefix(server.URL, "http://")
				ref := host + "/app:stable"
				tag, e := name.NewTag(ref, name.Insecure)
				require.NoError(t, e)
				require.NoError(t, remote.Write(tag, latest, remote.WithContext(t.Context())))
				localID := oldConfig.String()
				if store == "containerd-index" {
					localID = "sha256:" + strings.Repeat("d", 64)
				} else if store == "containerd-manifest" {
					localID = oldManifest.String()
				}
				daemon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					if strings.HasSuffix(r.URL.Path, "/containers/json") {
						_, _ = w.Write([]byte(`[]`))
						return
					}
					if strings.HasSuffix(r.URL.Path, "/info") {
						_, _ = w.Write([]byte(`{"OSType":"linux","Architecture":"x86_64"}`))
						return
					}
					id := localID
					if store != "classic" && r.URL.Query().Get("platform") != "" {
						id = oldManifest.String()
					}
					row := map[string]any{"Id": id, "Os": "linux", "Architecture": "amd64", "RepoDigests": []string{host + "/app@sha256:" + strings.Repeat("f", 64)}, "Config": map[string]any{"Labels": map[string]string{"org.opencontainers.image.version": "1.0.0"}}}
					if store != "classic" {
						mediaType := "application/vnd.oci.image.index.v1+json"
						if r.URL.Query().Get("platform") != "" {
							mediaType = "application/vnd.oci.image.manifest.v1+json"
						}
						row["Descriptor"] = map[string]any{"digest": id, "mediaType": mediaType}
					}
					_ = json.NewEncoder(w).Encode(row)
				}))
				t.Cleanup(daemon.Close)
				c, e := client.New(client.WithHost(daemon.URL), client.WithHTTPClient(daemon.Client()), client.WithAPIVersion("1.49"))
				require.NoError(t, e)
				t.Cleanup(func() { _ = c.Close() })
				db := setupImageUpdateRegistryTestDBInternal(t)
				require.NoError(t, db.Create(&registryService.ContainerRegistry{URL: server.URL, Enabled: true, Insecure: true, RegistryType: "generic"}).Error)
				svc := NewImageUpdateService(db, nil, registryService.NewContainerRegistryService(db, nil, nil, nil), &docker.DockerClientService{Client: c}, nil, nil, nil)
				got, e := svc.ReadUpdateInfo(t.Context(), ref)
				require.NoError(t, e)
				want := "rebuild"
				if change == "version" {
					want = "version"
				}
				if change == "current" || store == "classic" && change == "same-config" {
					want = "current"
				}
				require.Equal(t, want, got.State)
				require.Zero(t, historical.Load())
				expected := oldConfig.String()
				if store != "classic" {
					expected = oldManifest.String()
				}
				require.Equal(t, expected, got.CurrentDigest)
				var saved ImageUpdateRecord
				require.NoError(t, db.First(&saved, "id = ?", localID).Error)
				require.Equal(t, want, saved.UpdateInfo().State)
				// The legacy version/pull-reference port cannot publish a native config as T.
				stored, found, e := svc.StoredUpdateByImageID(t.Context(), localID)
				require.NoError(t, e)
				require.True(t, found)
				require.False(t, stored.HasUpdate)
				require.Nil(t, stored.LatestDigest)
			})
		}
	}
}
