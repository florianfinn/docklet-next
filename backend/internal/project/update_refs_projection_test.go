// Source: docklet-next/issues/248
// Revision: 5a4ecc67a7712349cb869574429d09f09a96e895
// Retrieved: 2026-11-05
// License: Apache-2.0
// Deviations: Original proofs that the configured and degraded projections join equivalent spellings.

package project

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/compose-spec/compose-go/v2/types"
	image "github.com/florianfinn/docklet-next/backend/internal/image"
	"github.com/florianfinn/docklet-next/backend/internal/imageupdate"
	projectdetails "github.com/florianfinn/docklet-next/backend/internal/project/children/details"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/imageref"
	imagetypes "github.com/florianfinn/docklet-next/types/image"
	projecttypes "github.com/florianfinn/docklet-next/types/project"
	"github.com/stretchr/testify/require"
	"go.getarcane.app/updater/labels"
)

// A digest observation stored under the fully qualified spelling reaches a
// service that names the familiar spelling.
func TestConfiguredUpdateInfoJoinsEquivalentSpelling(t *testing.T) {
	byRef := map[string]*imagetypes.UpdateInfo{"docker.io/library/nginx:latest": {HasUpdate: true, State: "rebuild"}}
	info := BuildConfiguredUpdateInfo("project", []types.ServiceConfig{{Name: "web", Image: "nginx"}}, byRef, nil)
	require.NotNil(t, info.ServiceUpdates["web"].UpdateInfo)
	require.True(t, info.ServiceUpdates["web"].UpdateInfo.HasUpdate)
}

// Without a parseable Compose file the stack summary reads the persisted refs; a
// container observation under an equivalent runtime spelling must still reach it.
func TestDegradedEnrichmentJoinsEquivalentSpelling(t *testing.T) {
	for _, pair := range [][2]string{{"nginx", "docker.io/library/nginx:latest"}, {"docker.io/library/nginx:latest", "nginx"}} {
		stored, runtime := pair[0], pair[1]
		t.Run(stored, func(t *testing.T) {
			db := setupProjectTestDB(t)
			dir := t.TempDir()
			t.Setenv("PROJECTS_DIRECTORY", dir)
			settings, err := newSettingsServiceForTest(t, t.Context(), db)
			require.NoError(t, err)
			require.NoError(t, settings.SetStringSetting(t.Context(), "projectsDirectory", dir))
			path := createComposeProjectDir(t, dir, "broken")
			require.NoError(t, os.WriteFile(filepath.Join(path, "compose.yaml"), []byte("services: [invalid"), 0o600))
			p := Project{ID: "broken", Name: "broken", Path: path, ImageRefsJSON: `["` + stored + `"]`}
			require.NoError(t, db.Create(&p).Error)
			lab := map[string]string{labels.LabelUpdateStrategy: "tag"}
			require.NoError(t, db.Create(&imageupdate.ImageUpdateRecord{ID: "container::web", ContainerID: "web", PolicyKey: imageref.UpdatePolicyKey(runtime, lab), Repository: "docker.io/library/nginx", Tag: "latest", CurrentVersion: "1.0.0", LatestVersion: new("2.0.0"), Platform: new("native-v1:classic:linux/amd64"), CurrentDigest: new("sha256:" + strings.Repeat("a", 64)), LatestDigest: new("sha256:" + strings.Repeat("b", 64)), HasUpdate: true, CheckTime: time.Now().UTC(), UpdateType: "tag"}).Error)
			s := &ProjectService{db: db, settingsService: settings, imageService: image.NewImageService(db, nil, nil, nil, nil, nil, nil, nil)}
			s.initChildren(nil, nil)
			details := []projecttypes.Details{{ID: p.ID, RuntimeServices: []projecttypes.RuntimeService{{ContainerID: "web", Image: runtime, ContainerLabels: lab}}}}
			scoped := s.details.ContainerUpdateInfo(t.Context(), details)
			require.True(t, scoped["web"] != nil && scoped["web"].HasUpdate)
			s.enrichProjectsWithUpdateInfo(t.Context(), []Project{p}, details, true, nil)
			require.NotNil(t, details[0].UpdateInfo)
			require.True(t, details[0].UpdateInfo.HasUpdate)
		})
	}
}

// A scoped result replaces a stale shared result stored under another spelling.
func TestScopedResultReplacesEquivalentStaleSharedResult(t *testing.T) {
	for _, pair := range [][2]string{{"nginx", "docker.io/library/nginx:latest"}, {"docker.io/library/nginx:latest", "nginx"}} {
		stored, runtime := pair[0], pair[1]
		base := map[string]*imagetypes.UpdateInfo{stored: {HasUpdate: false}}
		merged := projectdetails.MergeProjectContainerUpdateInfo(base, []projecttypes.RuntimeService{{ContainerID: "web", Image: runtime}}, map[string]*imagetypes.UpdateInfo{"web": {HasUpdate: true}})
		require.True(t, buildUpdateInfoSummary([]string{stored}, merged).HasUpdate, stored)
		require.False(t, base[stored].HasUpdate, "the shared input stays unchanged")
	}
}
