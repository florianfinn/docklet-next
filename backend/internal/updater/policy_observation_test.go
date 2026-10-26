// Source: docklet-next/docs/design/update-policies.md
// Revision: cdfe93c3dd2557cb21586ae542ebc05cae9bc0af
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Real registry manifests and controlled Docker reads feed the actual configured-service producer.

package updater

import (
	"context"
	"encoding/json"
	compose "github.com/compose-spec/compose-go/v2/types"
	"github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/internal/imageupdate"
	"github.com/florianfinn/docklet-next/backend/internal/kv"
	registries "github.com/florianfinn/docklet-next/backend/internal/registry"
	"github.com/google/go-containerregistry/pkg/name"
	registryserver "github.com/google/go-containerregistry/pkg/registry"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/require"
	"go.getarcane.app/updater"
	"go.getarcane.app/updater/labels"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPolicyConfiguredObservationHasComparableCandidateWithoutEffects(t *testing.T) {
	f := newPolicyFixture(t)
	registry := httptest.NewServer(registryserver.New(registryserver.Logger(log.New(io.Discard, "", 0))))
	t.Cleanup(registry.Close)
	repository := strings.TrimPrefix(registry.URL, "http://") + "/example/app"
	makeImage := func(marker string) v1.Image {
		img, err := mutate.ConfigFile(empty.Image, &v1.ConfigFile{OS: "linux", Architecture: "amd64", Config: v1.Config{Labels: map[string]string{"org.opencontainers.image.version": "1.0.0", "example.marker": marker}}})
		require.NoError(t, err)
		return img
	}
	old, next := makeImage("old"), makeImage("next")
	oldDigest, err := old.Digest()
	require.NoError(t, err)
	nextDigest, err := next.Digest()
	require.NoError(t, err)
	id, err := old.ConfigName()
	require.NoError(t, err)
	for tag, img := range map[string]v1.Image{"retained": old, "1.0.0": next} {
		ref, err := name.NewTag(repository+":"+tag, name.Insecure)
		require.NoError(t, err)
		require.NoError(t, remote.Write(ref, img, remote.WithContext(t.Context())))
	}
	effects := 0
	daemon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodGet {
			effects++
			w.WriteHeader(500)
			return
		}
		if strings.Contains(r.URL.Path, "/distribution/") {
			_ = json.NewEncoder(w).Encode(map[string]any{"Descriptor": map[string]any{"digest": nextDigest.String()}})
			return
		}
		if strings.HasSuffix(r.URL.Path, "/containers/json") {
			_, _ = w.Write([]byte("[]"))
			return
		}
		if strings.Contains(r.URL.Path, "/images/") && strings.HasSuffix(r.URL.Path, "/json") {
			_ = json.NewEncoder(w).Encode(map[string]any{"Id": id.String(), "Os": "linux", "Architecture": "amd64", "RepoDigests": []string{repository + "@" + oldDigest.String()}, "Config": map[string]any{"Labels": map[string]string{"org.opencontainers.image.version": "1.0.0"}}})
			return
		}
		w.WriteHeader(404)
	}))
	t.Cleanup(daemon.Close)
	cli, err := client.New(client.WithHost(daemon.URL), client.WithAPIVersion("1.48"), client.WithHTTPClient(daemon.Client()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = cli.Close() })
	dockerSvc := &docker.DockerClientService{Client: cli}
	registrySvc := registries.NewContainerRegistryService(f.db, func(context.Context) (registries.RegistryDaemonClient, error) { return cli, nil }, kv.NewKVService(f.db), f.s.deps.Settings, registry.Client())
	observed := imageupdate.NewImageUpdateService(f.db, f.s.deps.Settings, registrySvc, dockerSvc, nil, nil, nil)
	engine, err := updater.New(updater.Config{DockerClientProvider: fakeDockerClientProviderInternal{client: cli}, RegistryDigestResolver: registrySvc})
	require.NoError(t, err)
	service := &UpdaterService{engine: engine, deps: updaterDependenciesInternal{ImageUpdates: observed, RegistryDigestResolver: registrySvc}}
	record := service.checkProjectServiceInternal(t.Context(), "example-stack", compose.ServiceConfig{Name: "app", Image: repository + ":1.0.0", Labels: map[string]string{labels.LabelUpdateStrategy: "digest"}})
	if record.LastError != nil {
		t.Fatalf("configured observation failed: %s", *record.LastError)
	}
	require.NotNil(t, record.Platform)
	require.Equal(t, "linux/amd64", *record.Platform)
	require.NotNil(t, record.CurrentDigest)
	require.Equal(t, oldDigest.String(), *record.CurrentDigest)
	require.NotNil(t, record.LatestDigest)
	require.Equal(t, nextDigest.String(), *record.LatestDigest)
	require.True(t, record.HasUpdate)
	require.Equal(t, "rebuild", record.UpdateInfo().State)
	require.Zero(t, effects)
}
