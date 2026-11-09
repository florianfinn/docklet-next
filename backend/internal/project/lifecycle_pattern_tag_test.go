// Source: docklet-next/issues/409
// Revision: fddfe5d38eacf1a387b773ce10d41384f8c01459
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original tag-policy selection from the checker fixture through stack preflight and execution.

package project

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	registryService "github.com/florianfinn/docklet-next/backend/internal/registry"
	activityDTO "github.com/florianfinn/docklet-next/types/activity"
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
	"go.getarcane.app/updater/labels"
)

// patternTagCase mirrors the shared fixture that the real checker test in the
// updater package produces (TestPatternTagCheckerMatchesTheSharedFixture).
type patternTagCase struct {
	Name, Pattern, CurrentTag, NextTag string
	Update                             imageDTO.UpdateInfo
}

func patternTagCases(t *testing.T) []patternTagCase {
	t.Helper()
	raw, err := os.ReadFile("../../../web/src/data/http/testdata/pattern-tag-updates.json")
	require.NoError(t, err)
	var cases []patternTagCase
	require.NoError(t, json.Unmarshal(raw, &cases))
	require.NotEmpty(t, cases)
	return cases
}

func newPatternTagStack(t *testing.T, tc patternTagCase) (*lifecycleFixture, string, string, string, func(tag string)) {
	t.Helper()
	f := newLifecycleFixture(t)
	server := httptest.NewServer(registryServer.New())
	t.Cleanup(server.Close)
	host := strings.TrimPrefix(server.URL, "http://")
	write := func(tag string) {
		img, err := mutate.ConfigFile(empty.Image, &v1.ConfigFile{OS: "linux", Architecture: "amd64", Config: v1.Config{Labels: map[string]string{"example.marker": tag}}})
		require.NoError(t, err)
		ref, err := name.NewTag(host+"/app:"+tag, name.Insecure)
		require.NoError(t, err)
		require.NoError(t, remote.Write(ref, img, remote.WithContext(t.Context())))
	}
	write(tc.CurrentTag)
	write(tc.NextTag)
	require.NoError(t, f.db.Create(&registryService.ContainerRegistry{URL: server.URL, Enabled: true, Insecure: true, RegistryType: "generic"}).Error)
	oldRef, newRef := host+"/app:"+tc.CurrentTag, host+"/app:"+tc.NextTag
	source := "services:\n  app:\n    image: " + oldRef + "\n    labels:\n      " + labels.LabelUpdateStrategy + ": tag\n"
	if tc.Pattern != "" {
		source += "      " + labels.LabelUpdateTagPattern + ": '" + tc.Pattern + "'\n"
	}
	require.NoError(t, os.WriteFile(filepath.Join(f.project.Path, "compose.yaml"), []byte(source), 0600))
	f.tags[oldRef] = lifecycleOldID
	app := f.workloads["app"]
	app.Reference = oldRef
	f.workloads["app"] = app
	info := tc.Update
	info.ImageRef, info.CheckTime, info.Platform = oldRef, time.Now(), "linux/amd64"
	info.CurrentDigest, info.LatestDigest = lifecycleOldID, lifecycleNewID
	f.service.SetLifecycleChecker(func(context.Context, string) (*projectDTO.UpdateInfo, error) {
		checked := info
		return &projectDTO.UpdateInfo{ServiceUpdates: map[string]projectDTO.ServiceUpdateInfo{"app": {UpdateInfo: &checked}}}, nil
	})
	return f, oldRef, newRef, source, write
}

// The checker reports the selected version; preflight resolves its literal tag,
// writes it into compose.yaml and the executor creates the container from it.
func TestTagPolicySelectionInstallsTheLiteralTag(t *testing.T) {
	for _, tc := range patternTagCases(t) {
		t.Run(tc.Name, func(t *testing.T) {
			f, oldRef, newRef, source, _ := newPatternTagStack(t, tc)
			current := f.establishBaseline()
			intent, err := f.service.PrepareImageLifecycle(t.Context(), f.project.ID, f.registration, dto.LifecycleRequest{ExpectedRevision: current.SavedRevision, Services: []string{"app"}})
			require.NoError(t, err)
			require.Len(t, intent.Selections, 1)
			selection := intent.Selections[0]
			require.Equal(t, "version", selection.Kind)
			require.Equal(t, newRef, selection.Reference)
			expected := selection.ImageID
			f.onPull = func() {
				f.mu.Lock()
				defer f.mu.Unlock()
				for ref, id := range f.tags {
					if id == lifecycleNewID {
						f.tags[ref] = expected
					}
				}
				f.tags[expected] = expected
			}
			out := f.wait(f.admit(intent))
			require.Equal(t, activityDTO.StatusSuccess, out.Status, out.Error)
			require.Equal(t, newRef, f.workloads["app"].Reference, "the container runs the selected literal tag")
			body, err := os.ReadFile(filepath.Join(f.project.Path, "compose.yaml"))
			require.NoError(t, err)
			require.Equal(t, strings.Replace(source, oldRef, newRef, 1), string(body))
		})
	}
}

// A selection that moved after the check is refused, never installed.
func TestTagPolicySelectionRefusesAMovedTarget(t *testing.T) {
	tc := patternTagCases(t)[0]
	f, _, _, _, write := newPatternTagStack(t, tc)
	current := f.establishBaseline()
	write(strings.Replace(tc.NextTag, "1.1.0", "1.2.0", 1))
	_, err := f.service.PrepareImageLifecycle(t.Context(), f.project.ID, f.registration, dto.LifecycleRequest{ExpectedRevision: current.SavedRevision, Services: []string{"app"}})
	lifecycleCode(t, err, "update.target-changed")
	require.Zero(t, f.pulls)
	require.Zero(t, f.deploys)
}
