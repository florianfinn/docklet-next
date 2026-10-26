// Source: docklet-next/docs/design/images-updates-identity.md
// Revision: fcf8c8d3a079154c299912a7e22e0e903e5e4e38
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Expected local ID prediction and no-op refusal before admission.

package project

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	registryService "github.com/florianfinn/docklet-next/backend/internal/registry"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	projectDTO "github.com/florianfinn/docklet-next/types/project"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"github.com/google/go-containerregistry/pkg/name"
	registryServer "github.com/google/go-containerregistry/pkg/registry"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/stretchr/testify/require"
)

func TestIdentityPreflightStores(t *testing.T) {
	for _, store := range []string{"classic", "containerd", "classic-noop"} {
		t.Run(store, func(t *testing.T) {
			f := newLifecycleFixture(t)
			f.containerd = store == "containerd"
			server := httptest.NewServer(registryServer.New())
			t.Cleanup(server.Close)
			host := strings.TrimPrefix(server.URL, "http://")
			ref := host + "/app:1.0.0"
			img, e := mutate.ConfigFile(empty.Image, &v1.ConfigFile{OS: "linux", Architecture: "amd64"})
			require.NoError(t, e)
			manifest, e := img.Digest()
			require.NoError(t, e)
			config, e := img.ConfigName()
			require.NoError(t, e)
			tag, e := name.NewTag(ref, name.Insecure)
			require.NoError(t, e)
			require.NoError(t, remote.Write(tag, img, remote.WithContext(t.Context())))
			require.NoError(t, f.db.Create(&registryService.ContainerRegistry{URL: server.URL, Enabled: true, Insecure: true, RegistryType: "generic"}).Error)
			require.NoError(t, os.WriteFile(filepath.Join(f.project.Path, "compose.yaml"), []byte(strings.ReplaceAll(lifecycleOriginal, "registry.example.com", host)), 0600))
			id := lifecycleOldID
			if store == "classic-noop" {
				id = config.String()
			}
			f.tags[ref] = id
			f.tags[id] = id
			w := f.workloads["app"]
			w.Reference = ref
			w.ImageID = id
			f.workloads["app"] = w
			f.establishBaseline()
			f.service.SetLifecycleChecker(func(context.Context, string) (*projectDTO.UpdateInfo, error) {
				return &projectDTO.UpdateInfo{ServiceUpdates: map[string]projectDTO.ServiceUpdateInfo{"app": {ImageRef: ref, UpdateInfo: &imageDTO.UpdateInfo{HasUpdate: true, State: "rebuild", CurrentVersion: "1.0.0", LatestVersion: "1.0.0"}}}}, nil
			})
			_, current, e := f.service.lifecycleCurrent(t.Context(), f.project.ID, f.registration)
			require.NoError(t, e)
			intent, e := f.service.PrepareImageLifecycle(t.Context(), f.project.ID, f.registration, dto.LifecycleRequest{ExpectedRevision: current.SavedRevision})
			if store == "classic-noop" {
				require.Error(t, e)
				require.Nil(t, intent)
			} else {
				require.NoError(t, e)
				require.Len(t, intent.Selections, 1)
				expected := config.String()
				if f.containerd {
					expected = manifest.String()
				}
				require.Equal(t, expected, intent.Selections[0].ImageID)
				require.Equal(t, manifest.String(), intent.Selections[0].Digest)
			}
			require.Zero(t, f.pulls)
			require.Zero(t, f.deploys)
			require.Empty(t, lifecyclePoints(t, f))
			var receipts int64
			require.NoError(t, f.db.Table("runtime_receipts").Count(&receipts).Error)
			require.Zero(t, receipts)
		})
	}
}
