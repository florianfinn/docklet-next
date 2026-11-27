// Source: docklet-next/issues/677
// Revision: 99c1b0b8c4dfe56182c72749e092b08202436fa5
// Retrieved: 2026-11-27
// License: Apache-2.0
// Deviations: Original regression proofs.

package tags

import (
	"context"
	"encoding/json"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	docker "github.com/florianfinn/docklet-next/backend/internal/docker"
	registry "github.com/florianfinn/docklet-next/backend/internal/registry"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	"github.com/florianfinn/docklet-next/types/imageupdate"
	"github.com/google/go-containerregistry/pkg/name"
	registryserver "github.com/google/go-containerregistry/pkg/registry"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/require"
	"go.getarcane.app/updater"
	"go.getarcane.app/updater/labels"
	updatertypes "go.getarcane.app/updater/types"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

// Real registry manifests, local platform evidence and both check entry points
// must select the next acceptable tag when the newest build is incompatible.
func TestContainerAndImageChecksFallBackOnMissingPlatform(t *testing.T) {
	server := httptest.NewServer(registryserver.New())
	defer server.Close()
	repository := strings.TrimPrefix(server.URL, "http://") + "/example"
	for tag, arch := range map[string]string{"1.0.0": "amd64", "1.1.0": "amd64", "1.2.0": "arm64"} {
		img, err := mutate.ConfigFile(empty.Image, &v1.ConfigFile{OS: "linux", Architecture: arch})
		require.NoError(t, err)
		ref, err := name.NewTag(repository+":"+tag, name.Insecure)
		require.NoError(t, err)
		require.NoError(t, remote.Write(ref, img))
	}
	current := repository + ":1.0.0"
	daemon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/info"):
			_, _ = w.Write([]byte(`{"OSType":"linux","Architecture":"amd64"}`))
		case strings.Contains(r.URL.Path, "/images/"):
			_ = json.NewEncoder(w).Encode(map[string]any{"Id": "sha256:" + strings.Repeat("a", 64), "Os": "linux", "Architecture": "amd64"})
		case strings.Contains(r.URL.Path, "/containers/"):
			_ = json.NewEncoder(w).Encode(map[string]any{"Id": "app", "Image": "sha256:" + strings.Repeat("a", 64), "Config": map[string]any{"Image": current, "Labels": map[string]string{labels.LabelUpdateStrategy: "tag"}}})
		default:
			w.WriteHeader(404)
		}
	}))
	defer daemon.Close()
	cli, err := client.New(client.WithHost(daemon.URL), client.WithAPIVersion("1.49"))
	require.NoError(t, err)
	defer cli.Close()
	db, err := database.InitializeHub(t.Context(), sqlitestore.URL(filepath.Join(t.TempDir(), "example.db")), database.MigrationOptions{})
	require.NoError(t, err)
	defer db.Close()
	registryService := registry.NewContainerRegistryService(db, nil, nil, nil, server.Client())
	require.NoError(t, db.Create(&registry.ContainerRegistry{URL: server.URL, Enabled: true, Insecure: true, RegistryType: "generic"}).Error)
	dockerService := docker.NewDockerClientService(t.Context(), nil, nil, nil).WithClient(cli)
	adapter := newTagRegistryInternal(registryService, nil, dockerService, nil)
	engine, err := updater.New(updater.Config{DockerClientProvider: adapter, RegistryTagLister: adapter, RegistryDigestResolver: adapter, TagCandidateFilter: adapter})
	require.NoError(t, err)
	defer engine.Close()
	result, err := engine.CheckImageUpdate(t.Context(), updatertypes.CheckRequest{ImageRef: current, Policy: updatertypes.Policy{Strategy: "tag"}})
	require.NoError(t, err)
	require.Equal(t, repository+":1.1.0", result.TargetRef)
	svc := NewService(registryService, dockerService, nil, nil)
	saved := 0
	results, err := svc.Check(t.Context(), []container.Summary{{ID: "app", Image: current, Labels: map[string]string{labels.LabelUpdateStrategy: "tag"}}}, map[string]bool{current: true}, nil, func(context.Context, []container.Summary) error { return nil }, func(_ context.Context, _ container.Summary, result *imageupdate.Response) error {
		saved++
		require.Empty(t, result.Error)
		require.Equal(t, "1.1.0", result.LatestVersion)
		return nil
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, 1, saved)
}
