// Source: docklet-next/issues/409
// Revision: b11b3e70e671dd3c862b966b1d279251842c7319
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original pattern-tag policy selection through the projection and stack preflight.

package project

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
	"go.getarcane.app/updater"
	"go.getarcane.app/updater/labels"
	"go.getarcane.app/updater/pkg/utils/tagpolicy"
)

// The check result mirrors the project checker: the selector's version capture
// as CurrentVersion and the selected literal tag as LatestVersion, projected by
// the shared update projection before the real preflight consumes it.
func TestTagPolicySelectionInstallsTheLiteralTag(t *testing.T) {
	for _, tc := range []struct{ name, current, next, pattern string }{
		{"release pattern", "release-1.0.0", "release-1.1.0", `release-(?P<version>\d+\.\d+\.\d+)`},
		{"v control", "v1.0.0", "v1.1.0", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newLifecycleFixture(t)
			server := httptest.NewServer(registryServer.New())
			t.Cleanup(server.Close)
			host := strings.TrimPrefix(server.URL, "http://")
			oldRef, newRef := host+"/app:"+tc.current, host+"/app:"+tc.next
			image, err := mutate.ConfigFile(empty.Image, &v1.ConfigFile{OS: "linux", Architecture: "amd64"})
			require.NoError(t, err)
			tag, err := name.NewTag(newRef, name.Insecure)
			require.NoError(t, err)
			require.NoError(t, remote.Write(tag, image, remote.WithContext(t.Context())))
			require.NoError(t, f.db.Create(&registryService.ContainerRegistry{URL: server.URL, Enabled: true, Insecure: true, RegistryType: "generic"}).Error)

			serviceLabels := map[string]string{labels.LabelUpdateStrategy: "tag"}
			source := "services:\n  app:\n    image: " + oldRef + "\n    labels:\n      " + labels.LabelUpdateStrategy + ": tag\n"
			if tc.pattern != "" {
				serviceLabels[labels.LabelUpdateTagPattern] = tc.pattern
				source += "      " + labels.LabelUpdateTagPattern + ": '" + tc.pattern + "'\n"
			}
			require.NoError(t, os.WriteFile(filepath.Join(f.project.Path, "compose.yaml"), []byte(source), 0600))
			f.tags[oldRef] = lifecycleOldID
			app := f.workloads["app"]
			app.Reference = oldRef
			f.workloads["app"] = app
			current := f.establishBaseline()

			policy, err := tagpolicy.Resolve(oldRef, updater.DefaultLabelPolicy().TagPolicy(serviceLabels))
			require.NoError(t, err)
			selected, err := tagpolicy.Select(tc.current, []string{tc.current, tc.next, "unrelated-9.9.9"}, policy)
			require.NoError(t, err)
			require.Equal(t, tc.next, selected)
			capture, err := tagpolicy.Version(tc.current, policy)
			require.NoError(t, err)
			info := &imageDTO.UpdateInfo{ImageRef: oldRef, CheckTime: time.Now(), Platform: "linux/amd64", CurrentDigest: lifecycleOldID, LatestDigest: lifecycleNewID, CurrentVersion: capture, LatestVersion: selected, UpdateType: "tag", HasUpdate: true}
			imageDTO.ProjectUpdate(info)
			f.service.SetLifecycleChecker(func(context.Context, string) (*projectDTO.UpdateInfo, error) {
				return &projectDTO.UpdateInfo{ServiceUpdates: map[string]projectDTO.ServiceUpdateInfo{"app": {UpdateInfo: info}}}, nil
			})

			intent, err := f.service.PrepareImageLifecycle(t.Context(), f.project.ID, f.registration, dto.LifecycleRequest{ExpectedRevision: current.SavedRevision, Services: []string{"app"}})
			require.NoError(t, err)
			require.Len(t, intent.Selections, 1)
			require.Equal(t, "version", intent.Selections[0].Kind)
			require.Equal(t, newRef, intent.Selections[0].Reference)
			desired, err := lifecycleVersionFiles(current, intent.Selections)
			require.NoError(t, err)
			require.Equal(t, newRef, desired.Model.Services["app"].Image)
			for _, file := range desired.Files {
				if file.Path == "compose.yaml" {
					require.Equal(t, strings.Replace(source, oldRef, newRef, 1), string(file.Content))
				}
			}
		})
	}
}
