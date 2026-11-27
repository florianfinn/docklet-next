// Source: docklet-next/issues/677
// Revision: 99c1b0b8c4dfe56182c72749e092b08202436fa5
// Retrieved: 2026-11-27
// License: Apache-2.0
// Deviations: Original actual project-check platform fallback proof.

package updater

import (
	"context"
	"encoding/json"
	compose "github.com/compose-spec/compose-go/v2/types"
	"github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/internal/imageupdate"
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
	updatertypes "go.getarcane.app/updater/types"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type platformWitness struct {
	t        *testing.T
	expected string
	delegate updatertypes.TagCandidateFilter
}

func (w platformWitness) AcceptTag(ctx context.Context, c updatertypes.TagCandidate) (bool, error) {
	require.Equal(w.t, w.expected, c.Platform)
	return w.delegate.AcceptTag(ctx, c)
}
func TestProjectCheckFallsBackOnNativeAndForeignPlatforms(t *testing.T) {
	for _, arch := range []string{"amd64", "arm64"} {
		t.Run(arch, func(t *testing.T) {
			f := newPolicyFixture(t)
			server := httptest.NewServer(registryserver.New())
			defer server.Close()
			repository := strings.TrimPrefix(server.URL, "http://") + "/example"
			var old v1.Image
			other := "arm64"
			if arch == "arm64" {
				other = "amd64"
			}
			for tag, architecture := range map[string]string{"1.0.0": arch, "1.1.0": arch, "1.2.0": other} {
				img, err := mutate.ConfigFile(empty.Image, &v1.ConfigFile{OS: "linux", Architecture: architecture, Config: v1.Config{Labels: map[string]string{"example.version": tag}}})
				require.NoError(t, err)
				if tag == "1.0.0" {
					old = img
				}
				ref, err := name.NewTag(repository+":"+tag, name.Insecure)
				require.NoError(t, err)
				require.NoError(t, remote.Write(ref, img))
			}
			id, err := old.ConfigName()
			require.NoError(t, err)
			digest, err := old.Digest()
			require.NoError(t, err)
			daemon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case strings.HasSuffix(r.URL.Path, "/info"):
					_, _ = w.Write([]byte(`{"OSType":"linux","Architecture":"amd64"}`))
				case strings.HasSuffix(r.URL.Path, "/containers/json"):
					_, _ = w.Write([]byte(`[]`))
				case strings.Contains(r.URL.Path, "/images/"):
					_ = json.NewEncoder(w).Encode(map[string]any{"Id": id.String(), "Os": "linux", "Architecture": arch, "RepoDigests": []string{repository + "@" + digest.String()}})
				default:
					w.WriteHeader(404)
				}
			}))
			defer daemon.Close()
			cli, err := client.New(client.WithHost(daemon.URL), client.WithAPIVersion("1.49"))
			require.NoError(t, err)
			defer cli.Close()
			registryService := registries.NewContainerRegistryService(f.db, nil, nil, nil, server.Client())
			require.NoError(t, f.db.Create(&registries.ContainerRegistry{URL: server.URL, Enabled: true, Insecure: true, RegistryType: "generic"}).Error)
			dockerService := docker.NewDockerClientService(t.Context(), nil, nil, nil).WithClient(cli)
			observed := imageupdate.NewImageUpdateService(f.db, f.s.deps.Settings, registryService, dockerService, nil, nil, nil)
			service := &UpdaterService{deps: updaterDependenciesInternal{Docker: dockerService, RegistryDigestResolver: registryService, ImageUpdates: observed}}
			config := service.configInternal()
			require.NotNil(t, config.TagCandidateFilter)
			config.TagCandidateFilter = platformWitness{t, "linux/" + arch, config.TagCandidateFilter}
			engine, err := updater.New(config)
			require.NoError(t, err)
			defer engine.Close()
			service.engine = engine
			record := service.checkProjectServiceInternal(context.Background(), "example", compose.ServiceConfig{Name: "app", Image: repository + ":1.0.0", Platform: "linux/" + arch, Labels: map[string]string{labels.LabelUpdateStrategy: "tag"}}, "example")
			require.Nil(t, record.LastError)
			require.True(t, record.HasUpdate)
			require.NotNil(t, record.LatestVersion)
			require.Equal(t, "1.1.0", *record.LatestVersion)
		})
	}
}
