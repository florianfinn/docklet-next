// Source: docklet-next/issues/343
// Revision: 89f69980c01d08714ce96c49365fb19065e3d6ba
// Retrieved: 2026-11-05
// License: Apache-2.0
// Deviations: Original native-registry selected-service candidate and rewrite proof.

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

func TestServiceScopeNativeCandidateAndLiteralRewrite(t *testing.T) {
	f := newLifecycleFixture(t)
	server := httptest.NewServer(registryServer.New())
	t.Cleanup(server.Close)
	host := strings.TrimPrefix(server.URL, "http://")
	oldRef, newRef := host+"/app:1.0.0", host+"/app:1.1.0"
	image, err := mutate.ConfigFile(empty.Image, &v1.ConfigFile{OS: "linux", Architecture: "amd64"})
	require.NoError(t, err)
	tag, err := name.NewTag(newRef, name.Insecure)
	require.NoError(t, err)
	require.NoError(t, remote.Write(tag, image, remote.WithContext(t.Context())))
	require.NoError(t, f.db.Create(&registryService.ContainerRegistry{URL: server.URL, Enabled: true, Insecure: true, RegistryType: "generic"}).Error)
	source := "services:\n  app:\n    image: " + oldRef + "\n  side:\n    image: " + oldRef + " # separate literal, stays unchanged\n"
	require.NoError(t, os.WriteFile(filepath.Join(f.project.Path, "compose.yaml"), []byte(source), 0600))
	f.tags[oldRef] = lifecycleOldID
	app := f.workloads["app"]
	app.Reference = oldRef
	f.workloads["app"] = app
	side := app
	side.ID = strings.Repeat("1", 64)
	side.Service = "side"
	f.workloads["side"] = side
	current := f.establishBaseline()
	f.service.SetLifecycleChecker(func(context.Context, string) (*projectDTO.UpdateInfo, error) {
		return &projectDTO.UpdateInfo{ServiceUpdates: map[string]projectDTO.ServiceUpdateInfo{
			"app":  {UpdateInfo: &imageDTO.UpdateInfo{HasUpdate: true, CurrentVersion: "1.0.0", LatestVersion: "1.1.0"}},
			"side": {UpdateInfo: &imageDTO.UpdateInfo{HasUpdate: true, CurrentVersion: "1.0.0", LatestVersion: "9.9.9"}},
		}}, nil
	})
	intent, err := f.service.PrepareImageLifecycle(t.Context(), f.project.ID, f.registration, dto.LifecycleRequest{ExpectedRevision: current.SavedRevision, Services: []string{"app"}})
	require.NoError(t, err)
	require.Len(t, intent.Selections, 1)
	require.Equal(t, "app", intent.Selections[0].Service)
	require.Equal(t, newRef, intent.Selections[0].Reference)
	expected, err := image.ConfigName()
	require.NoError(t, err)
	require.Equal(t, expected.String(), intent.Selections[0].ImageID)
	desired, err := lifecycleVersionFiles(current, intent.Selections)
	require.NoError(t, err)
	for _, file := range desired.Files {
		if file.Path == "compose.yaml" {
			require.Equal(t, strings.Replace(source, oldRef, newRef, 1), string(file.Content))
		}
	}
	require.Equal(t, oldRef, desired.Model.Services["side"].Image)
	assertNoServiceAdmission(t, f)
}
