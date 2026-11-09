// Source: docklet-next/issues/342
// Revision: 89f69980c01d08714ce96c49365fb19065e3d6ba
// Retrieved: 2026-11-05
// License: Apache-2.0
// Deviations: Original fresh-check and classic/containerd native identity bridge proofs.

package image

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/internal/imageupdate"
	"github.com/florianfinn/docklet-next/backend/internal/registry"
	"github.com/google/go-containerregistry/pkg/name"
	registryHTTP "github.com/google/go-containerregistry/pkg/registry"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	digest "github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/require"
)

func TestContainerImageBridgeFreshAndNativeIdentity(t *testing.T) {
	for _, store := range []string{"classic", "containerd", "pattern", "suffix", "version", "v-version", "label-bump"} {
		t.Run(store, func(t *testing.T) {
			regServer := httptest.NewServer(registryHTTP.New())
			defer regServer.Close()
			repo := strings.TrimPrefix(regServer.URL, "http://") + "/app"
			ref := repo + ":stable"
			if store == "pattern" {
				ref = repo + ":release-1.0.0"
			} else if store == "suffix" {
				ref = repo + ":1.0.00"
			} else if store == "version" {
				ref = repo + ":1.0.0"
			} else if store == "v-version" {
				ref = repo + ":v1.0.0"
			}
			tag, err := name.NewTag(ref, name.Insecure)
			require.NoError(t, err)
			old, err := mutate.ConfigFile(empty.Image, &v1.ConfigFile{OS: "linux", Architecture: "amd64", Config: v1.Config{Labels: map[string]string{"generation": "before"}}})
			require.NoError(t, err)
			nextLabels := map[string]string{"generation": "after"}
			if store == "label-bump" {
				// Same tag, newer version label: a rebuild, not a tag to move to.
				nextLabels["org.opencontainers.image.version"] = "1.2.4"
			}
			next, err := mutate.ConfigFile(empty.Image, &v1.ConfigFile{OS: "linux", Architecture: "amd64", Config: v1.Config{Labels: nextLabels}})
			require.NoError(t, err)
			oldConfig, err := old.ConfigName()
			require.NoError(t, err)
			oldManifest, err := old.Digest()
			require.NoError(t, err)
			newConfig, err := next.ConfigName()
			require.NoError(t, err)
			newManifest, err := next.Digest()
			require.NoError(t, err)
			localID, expected := oldConfig.String(), newConfig.String()
			var desc *ocispec.Descriptor
			if store == "containerd" {
				localID, expected = oldManifest.String(), newManifest.String()
				desc = &ocispec.Descriptor{MediaType: ocispec.MediaTypeImageManifest, Digest: digest.Digest(localID), Platform: &ocispec.Platform{OS: "linux", Architecture: "amd64"}}
			}
			consumerLabels := map[string]string{}
			if store == "version" || store == "v-version" {
				consumerLabels = map[string]string{"com.getarcaneapp.arcane.updater.strategy": "tag"}
			}
			localLabels := map[string]any{}
			if store == "label-bump" {
				localLabels["org.opencontainers.image.version"] = "1.2.3"
			}
			if store == "pattern" {
				consumerLabels = map[string]string{"com.getarcaneapp.arcane.updater.strategy": "tag", "com.getarcaneapp.arcane.updater.tag-pattern": "release-(?P<version>.*)", "com.getarcaneapp.arcane.updater.constraint": "~1.0"}
			}
			if store == "suffix" {
				// The literal suffix also ends the normalised version.
				consumerLabels = map[string]string{"com.getarcaneapp.arcane.updater.strategy": "tag", "com.getarcaneapp.arcane.updater.tag-pattern": `(?P<version>\d+\.\d+\.\d+)0`, "com.getarcaneapp.arcane.updater.constraint": "^1.0.0"}
			}
			inspected := container.InspectResponse{ID: "consumer", Image: localID, Config: &container.Config{Image: ref, Labels: consumerLabels}, ImageManifestDescriptor: desc}
			daemon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case strings.HasSuffix(r.URL.Path, "/info"):
					_, _ = w.Write([]byte(`{"OSType":"linux","Architecture":"amd64"}`))
				case strings.HasSuffix(r.URL.Path, "/containers/json"):
					_ = json.NewEncoder(w).Encode([]map[string]any{{"Id": "consumer", "Image": ref, "ImageID": localID, "Labels": consumerLabels}})
				case strings.Contains(r.URL.Path, "/containers/"):
					_ = json.NewEncoder(w).Encode(inspected)
				default:
					_ = json.NewEncoder(w).Encode(map[string]any{"Id": localID, "RepoDigests": []string{repo + "@" + oldManifest.String()}, "Os": "linux", "Architecture": "amd64", "Descriptor": desc, "Config": map[string]any{"Labels": localLabels}})
				}
			}))
			defer daemon.Close()
			c, err := client.New(client.WithHost(daemon.URL), client.WithAPIVersion("1.49"))
			require.NoError(t, err)
			defer c.Close()
			s, db, _ := safetyFixture(t, func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) })
			d := docker.NewDockerClientService(t.Context(), nil, nil, nil).WithClient(c)
			require.NoError(t, db.Create(&registry.ContainerRegistry{URL: regServer.URL, Enabled: true, Insecure: true, RegistryType: "generic"}).Error)
			reg := registry.NewContainerRegistryService(db, nil, nil, nil, regServer.Client())
			s.dockerService = d
			s.registryService = reg
			s.imageUpdateService = imageupdate.NewImageUpdateService(db, nil, reg, d, nil, nil, nil)
			require.NoError(t, remote.Write(tag, old, remote.WithContext(t.Context())))
			current, err := s.CheckContainerImage(t.Context(), inspected)
			require.NoError(t, err)
			require.False(t, current.HasUpdate)
			targetRef := ref
			if store == "version" {
				targetRef = repo + ":1.1.0"
				tag, err = name.NewTag(targetRef, name.Insecure)
				require.NoError(t, err)
			}
			if store == "v-version" {
				targetRef = repo + ":v1.1.0"
				tag, err = name.NewTag(targetRef, name.Insecure)
				require.NoError(t, err)
			}
			if store == "pattern" {
				targetRef = repo + ":release-1.0.1"
				tag, err = name.NewTag(targetRef, name.Insecure)
				require.NoError(t, err)
			}
			if store == "suffix" {
				targetRef = repo + ":1.1.00"
				tag, err = name.NewTag(targetRef, name.Insecure)
				require.NoError(t, err)
			}
			require.NoError(t, remote.Write(tag, next, remote.WithContext(t.Context())))
			fresh, err := s.CheckContainerImage(t.Context(), inspected)
			require.NoError(t, err)
			require.True(t, fresh.HasUpdate, "check must not reuse the earlier current observation")
			if store == "pattern" {
				require.Equal(t, "release-1.0.1", fresh.LatestVersion)
			} else if store == "suffix" {
				require.Equal(t, "version", fresh.State)
				require.Equal(t, "1.1.00", fresh.LatestVersion)
			} else if store == "version" {
				require.Equal(t, "1.1.0", fresh.LatestVersion)
			} else if store == "v-version" {
				require.Equal(t, "version", fresh.State)
				require.Equal(t, "v1.1.0", fresh.LatestVersion)
			} else if store == "label-bump" {
				require.Equal(t, "1.2.4", fresh.LatestVersion)
				require.Equal(t, "rebuild", fresh.State, "a same-reference candidate never moves the tag")
			}
			id, platform, err := s.ExpectedContainerImage(t.Context(), inspected, targetRef)
			require.NoError(t, err)
			require.Equal(t, expected, id)
			require.Equal(t, "linux/amd64", platform)
		})
	}
}
