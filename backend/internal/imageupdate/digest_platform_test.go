// Source: docklet-next/issues/634
// Revision: 35f39e8729df29cabccc10a4adc42b4b68220a7c
// Retrieved: 2026-11-26
// License: Apache-2.0
// Deviations: Original regression for issue #634: digest consumers on another platform.

package imageupdate

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/docker"
	registries "github.com/florianfinn/docklet-next/backend/internal/registry"
	"github.com/google/go-containerregistry/pkg/name"
	registryserver "github.com/google/go-containerregistry/pkg/registry"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/require"
)

// A digest consumer running arm64 must not inherit the host's amd64 same-tag
// result: neither its state, its candidate digest nor its build facts.
func TestDigestConsumerOnOtherPlatformIsUnknown(t *testing.T) {
	server := httptest.NewServer(registryserver.New())
	t.Cleanup(server.Close)
	repo := strings.TrimPrefix(server.URL, "http://") + "/app"
	ref := repo + ":latest"
	next, err := mutate.ConfigFile(empty.Image, &v1.ConfigFile{OS: "linux", Architecture: "amd64"})
	require.NoError(t, err)
	tag, err := name.NewTag(ref, name.Insecure)
	require.NoError(t, err)
	require.NoError(t, remote.Write(tag, next, remote.WithContext(t.Context())))
	index := "sha256:" + strings.Repeat("d", 64)
	amd := "sha256:" + strings.Repeat("a", 64)
	arm := "sha256:" + strings.Repeat("e", 64)
	labels := map[string]string{"com.getarcaneapp.arcane.updater.strategy": "digest"}
	manifest := "application/vnd.oci.image.manifest.v1+json"
	daemon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/info"):
			_ = json.NewEncoder(w).Encode(map[string]string{"OSType": "linux", "Architecture": "x86_64"})
		case strings.HasSuffix(r.URL.Path, "/containers/json"):
			_ = json.NewEncoder(w).Encode([]any{map[string]any{"Id": "consumer", "Image": ref, "ImageID": index, "Labels": labels}})
		case strings.Contains(r.URL.Path, "/containers/consumer/json"):
			_ = json.NewEncoder(w).Encode(map[string]any{"Id": "consumer", "Image": index, "Config": map[string]any{"Image": ref, "Labels": labels}, "ImageManifestDescriptor": map[string]any{"digest": arm, "mediaType": manifest, "platform": map[string]string{"os": "linux", "architecture": "arm64"}}})
		default:
			row := map[string]any{"Id": index, "Os": "linux", "Architecture": "amd64", "Created": "2026-11-12T08:00:00Z", "RepoDigests": []string{repo + "@" + amd}, "Descriptor": map[string]any{"digest": index, "mediaType": "application/vnd.oci.image.index.v1+json"}}
			if p := r.URL.Query().Get("platform"); p != "" {
				id := amd
				if strings.Contains(p, "arm64") {
					id, row["Architecture"] = arm, "arm64"
				}
				row["Id"], row["Descriptor"] = id, map[string]any{"digest": id, "mediaType": manifest}
			}
			_ = json.NewEncoder(w).Encode(row)
		}
	}))
	t.Cleanup(daemon.Close)
	cli, err := client.New(client.WithHost(daemon.URL), client.WithHTTPClient(daemon.Client()), client.WithAPIVersion("1.49"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = cli.Close() })
	db := setupImageUpdateRegistryTestDBInternal(t)
	require.NoError(t, db.Create(&registries.ContainerRegistry{URL: server.URL, Enabled: true, Insecure: true, RegistryType: "generic"}).Error)
	svc := NewImageUpdateService(db, nil, registries.NewContainerRegistryService(db, nil, nil, nil), &docker.DockerClientService{Client: cli}, nil, nil, nil)

	result, err := svc.ReadUpdateInfo(t.Context(), ref)
	require.NoError(t, err)
	require.NotNil(t, result.ImageUpdate)
	require.Equal(t, "rebuild", result.ImageUpdate.State, "precondition: the host platform sees a rebuild")
	require.Equal(t, "linux/amd64", result.ImageUpdate.Platform)
	got := result.ContainerUpdates["consumer"]
	require.NotNil(t, got)
	require.Equal(t, "unknown", got.State)
	require.Equal(t, "unknown", got.AuthState, "the contract enum has no empty auth state")
	require.False(t, got.HasUpdate)
	require.Empty(t, got.CurrentDigest, "no foreign digest becomes a short build ID or update target")
	require.Empty(t, got.LatestDigest)
	require.Nil(t, got.CurrentCreated)
	require.Nil(t, got.LatestCreated)
}
