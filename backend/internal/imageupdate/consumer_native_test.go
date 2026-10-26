// Source: docklet-next/docs/design/images-updates-identity.md
// Revision: fcf8c8d3a079154c299912a7e22e0e903e5e4e38
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Review correction with real producer and behavioral guard proofs.

package imageupdate

import (
	"encoding/json"
	"github.com/florianfinn/docklet-next/backend/internal/docker"
	registries "github.com/florianfinn/docklet-next/backend/internal/registry"
	"github.com/google/go-containerregistry/pkg/name"
	registryserver "github.com/google/go-containerregistry/pkg/registry"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	registryTypes "github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestConsumerVersionNativeCandidateStores(t *testing.T) {
	for _, store := range []string{"classic", "containerd"} {
		t.Run(store, func(t *testing.T) {
			server := httptest.NewServer(registryserver.New())
			t.Cleanup(server.Close)
			repo := strings.TrimPrefix(server.URL, "http://") + "/app"
			ref := repo + ":1.0.0"
			original, e := mutate.ConfigFile(empty.Image, &v1.ConfigFile{OS: "linux", Architecture: "amd64", Config: v1.Config{Labels: map[string]string{"org.opencontainers.image.version": "1.0.0"}}})
			require.NoError(t, e)
			original = mutate.MediaType(original, registryTypes.DockerManifestSchema2)
			latest := mutate.MediaType(original, registryTypes.OCIManifestSchema1)
			config, e := original.ConfigName()
			require.NoError(t, e)
			manifest, e := original.Digest()
			require.NoError(t, e)
			for tag, img := range map[string]v1.Image{"1.0.0": original, "1.0.1": latest} {
				r, e := name.NewTag(repo+":"+tag, name.Insecure)
				require.NoError(t, e)
				require.NoError(t, remote.Write(r, img, remote.WithContext(t.Context())))
			}
			local := config.String()
			if store == "containerd" {
				local = "sha256:" + strings.Repeat("d", 64)
			}
			labels := map[string]string{"com.getarcaneapp.arcane.updater.strategy": "tag", "com.getarcaneapp.arcane.updater.constraint": "~1.0"}
			daemon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if strings.HasSuffix(r.URL.Path, "/info") {
					_ = json.NewEncoder(w).Encode(map[string]string{"OSType": "linux", "Architecture": "x86_64"})
					return
				}
				if strings.HasSuffix(r.URL.Path, "/containers/json") {
					_ = json.NewEncoder(w).Encode([]any{map[string]any{"Id": "consumer", "Image": ref, "ImageID": local, "Labels": labels}})
					return
				}
				if strings.Contains(r.URL.Path, "/containers/consumer/json") {
					row := map[string]any{"Image": local, "Config": map[string]any{"Image": ref, "Labels": labels}}
					if store == "containerd" {
						row["ImageManifestDescriptor"] = map[string]any{"digest": manifest.String(), "mediaType": "application/vnd.oci.image.manifest.v1+json", "platform": map[string]string{"os": "linux", "architecture": "amd64"}}
					}
					_ = json.NewEncoder(w).Encode(row)
					return
				}
				id := local
				media := "application/vnd.oci.image.index.v1+json"
				if r.URL.Query().Get("platform") != "" {
					id = manifest.String()
					media = "application/vnd.oci.image.manifest.v1+json"
				}
				row := map[string]any{"Id": id, "Os": "linux", "Architecture": "amd64", "RepoDigests": []string{repo + "@" + manifest.String()}, "Config": map[string]any{"Labels": map[string]string{"org.opencontainers.image.version": "1.0.0"}}}
				if store == "containerd" {
					row["Descriptor"] = map[string]any{"digest": id, "mediaType": media}
				}
				_ = json.NewEncoder(w).Encode(row)
			}))
			t.Cleanup(daemon.Close)
			cli, e := client.New(client.WithHost(daemon.URL), client.WithHTTPClient(daemon.Client()), client.WithAPIVersion("1.49"))
			require.NoError(t, e)
			t.Cleanup(func() { _ = cli.Close() })
			db := setupImageUpdateRegistryTestDBInternal(t)
			require.NoError(t, db.Create(&registries.ContainerRegistry{URL: server.URL, Enabled: true, Insecure: true, RegistryType: "generic"}).Error)
			svc := NewImageUpdateService(db, nil, registries.NewContainerRegistryService(db, nil, nil, nil), &docker.DockerClientService{Client: cli}, nil, nil, nil)
			result, e := svc.ReadUpdateInfo(t.Context(), ref)
			require.NoError(t, e)
			require.Contains(t, result.ContainerUpdates, "consumer")
			want := "current"
			if store == "containerd" {
				want = "version"
			}
			require.Equal(t, want, result.ContainerUpdates["consumer"].State)
			require.Equal(t, store == "containerd", result.ContainerUpdates["consumer"].HasUpdate)
		})
	}
}
