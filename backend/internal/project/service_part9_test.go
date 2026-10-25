// Source: getarcaneapp/arcane/backend/internal/project/service_test.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed central variables and .env.global; functional cut: removed GitOps and saved Git repositories; SQLite temp-file persistence fixture.

package project

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"

	"io"
	"io/fs"

	"net/http"

	"os"
	"path/filepath"
	"runtime"
	"strings"

	"testing"
	"time"

	"github.com/compose-spec/compose-go/v2/types"

	imagetypes "github.com/florianfinn/docklet-next/types/image"
	projecttypes "github.com/florianfinn/docklet-next/types/project"
	usertypes "github.com/florianfinn/docklet-next/types/user"

	"github.com/moby/moby/api/types/container"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.getarcane.app/acfs"

	"go.getarcane.app/updater/labels"

	"github.com/florianfinn/docklet-next/backend/internal/config"

	"github.com/florianfinn/docklet-next/backend/internal/event"
	"github.com/florianfinn/docklet-next/backend/internal/image"
	"github.com/florianfinn/docklet-next/backend/internal/imageupdate"

	"github.com/florianfinn/docklet-next/backend/internal/project/children/deployment"
	projectdetails "github.com/florianfinn/docklet-next/backend/internal/project/children/details"

	"github.com/florianfinn/docklet-next/backend/internal/registry"

	"github.com/florianfinn/docklet-next/backend/pkg/libarcane"

	"github.com/florianfinn/docklet-next/backend/pkg/utils/imageref"
)

func TestCountProjectsWithPendingTagUpdatesUsesRuntimeContainers(t *testing.T) {
	db := setupProjectTestDB(t)
	settingsService, err := newSettingsServiceForTest(t, t.Context(), db)
	require.NoError(t, err)
	projectRecords := []Project{
		{Name: "first", Path: "first", ImageRefsJSON: `["example:3.1.0"]`},
		{Name: "second", Path: "second", ImageRefsJSON: `["example:3.1.0"]`},
	}
	require.NoError(t, db.Create(&projectRecords).Error)
	require.NoError(
		t,
		db.Create(
			&imageupdate.ImageUpdateRecord{Platform: new("linux/amd64"), CurrentDigest: new("sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"), LatestDigest: new("sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"), CheckTime: time.Now().UTC(),
				ID: "container::first-container",
				PolicyKey: imageref.UpdatePolicyKey(
					"example:3.1.0",
					map[string]string{
						labels.LabelUpdateStrategy: "tag",
					},
				),
				ContainerID: "first-container",
				ImageID:     "shared",
				HasUpdate:   true,
				UpdateType:  "tag",
			},
		).Error,
	)
	service := &ProjectService{db: db, settingsService: settingsService, imageService: image.NewImageService(db, nil, nil, nil, nil, nil, nil, nil)}
	service.initChildren(nil, nil)
	containers := []container.Summary{
		{
			ID:      "first-container",
			Image:   "example:3.1.0",
			ImageID: "shared",
			Labels: map[string]string{
				labels.LabelUpdateStrategy:   "tag",
				"com.docker.compose.project": "first",
				"com.docker.compose.service": "web",
			},
		},
		{ID: "second-container", Image: "example:3.1.0", ImageID: "shared", Labels: map[string]string{"com.docker.compose.project": "second", "com.docker.compose.service": "web"}},
	}
	count, err := service.CountProjectsWithPendingUpdates(t.Context(), containers)
	require.NoError(t, err)
	require.Equal(t, 1, count)
	containers[0].Labels[libarcane.HiddenResourceLabel] = "true"
	count, err = service.CountProjectsWithPendingUpdates(t.Context(), containers)
	require.NoError(t, err)
	require.Zero(t, count)

	// A digest check stored under another local tag of the running image marks
	// the project once, however many replicas share the image.
	require.NoError(t, db.Create(&Project{Name: "third", Path: "third", ImageRefsJSON: `["app:latest"]`}).Error)
	require.NoError(t, db.Create(&imageupdate.ImageUpdateRecord{Platform: new("linux/amd64"), CurrentDigest: new("sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"), LatestDigest: new("sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"), CheckTime: time.Now().UTC(), ID: "moving-image", Repository: "docker.io/library/app", Tag: "stable", HasUpdate: true, UpdateType: "digest"}).Error)
	containers = append(containers,
		container.Summary{ID: "third-one", Image: "app:latest", ImageID: "moving-image", Labels: map[string]string{"com.docker.compose.project": "third", "com.docker.compose.service": "web"}},
		container.Summary{
			ID:      "third-two",
			Image:   "app:latest",
			ImageID: "moving-image",
			Labels: map[string]string{
				"com.docker.compose.project": "third",
				"com.docker.compose.service": "web",
				labels.LabelUpdater:          "false",
			},
		},
	)
	count, err = service.CountProjectsWithPendingUpdates(t.Context(), containers)
	require.NoError(t, err)
	require.Equal(t, 1, count)
	containers[2].Labels[imageref.UpdateCheckLabel] = "false"
	containers[3].Labels[imageref.UpdateCheckLabel] = "false"
	count, err = service.CountProjectsWithPendingUpdates(t.Context(), containers)
	require.NoError(t, err)
	require.Zero(t, count, "disabling update checks on every replica suppresses the project")
}

type serviceTagTransport struct {
	tags  []string
	calls int
}

func (s *serviceTagTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	// The /v2/ version check precedes every listing and is not a tag call.
	if request.URL.Path == "/v2/" {
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: http.NoBody, Request: request}, nil
	}
	s.calls++
	payload, err := json.Marshal(map[string]any{"name": "library/app", "tags": s.tags})
	if err != nil {
		return nil, err
	}
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(string(payload))), Request: request}, nil
}

func TestProjectServiceManualUpdateDiscoversTags(t *testing.T) {
	for _, tt := range []struct {
		name          string
		tags          []string
		wantRef       string
		wantChanged   bool
		skipDiscovery bool
		wantCalls     int
		strategy      string
		omitLabels    bool
	}{
		{name: "new tag", tags: []string{"1.2.0", "1.3.0", "2.0.0"}, wantRef: "app:1.3.0", wantChanged: true, wantCalls: 1},
		{name: "same tag digest fallback", tags: []string{"1.2.0", "2.0.0"}, wantRef: "app:1.2.0", wantCalls: 1},
		{name: "planned dependency does not discover tags", tags: []string{"1.2.0", "1.3.0"}, wantRef: "app:1.2.0", skipDiscovery: true},
		{name: "unlabeled digest default", tags: []string{"1.2.0", "1.3.0", "2.0.0"}, wantRef: "app:1.2.0", omitLabels: true},
		{name: "explicit automatic tag", tags: []string{"1.2.0", "1.3.0", "2.0.0"}, wantRef: "app:1.3.0", wantChanged: true, wantCalls: 1, strategy: "auto"},
		{name: "explicit digest optout", tags: []string{"1.2.0", "1.3.0"}, wantRef: "app:1.2.0", strategy: "digest"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			db := setupProjectTestDB(t)
			directory := t.TempDir()
			require.NoError(t, db.AutoMigrate(&registry.ContainerRegistry{}))
			t.Setenv("PROJECTS_DIRECTORY", directory)
			settingsService, err := newSettingsServiceForTest(t, ctx, db)
			require.NoError(t, err)
			require.NoError(t, settingsService.SetStringSetting(ctx, "projectsDirectory", directory))
			path := createComposeProjectDir(t, directory, "manual-tag")
			source := "services:\n  app:\n    image: app:1.2.0\n    labels:\n      com.getarcaneapp.arcane.updater.strategy: " +
				"tag\n      com.getarcaneapp.arcane.updater.constraint: 1.x\n  selected-digest:\n    image: " +
				"busybox:latest\n  unselected:\n    image: busybox:latest\n"
			if tt.omitLabels {
				source = strings.Replace(source, "    labels:\n      com.getarcaneapp.arcane.updater.strategy: tag\n      com.getarcaneapp.arcane.updater.constraint: 1.x\n", "", 1)
			} else if tt.strategy != "" {
				source = strings.Replace(source, "strategy: tag", "strategy: "+tt.strategy, 1)
				source = strings.Replace(source, "      com.getarcaneapp.arcane.updater.constraint: 1.x\n", "", 1)
			}

			require.NoError(t, os.WriteFile(filepath.Join(path, "compose.yaml"), []byte(source), 0o600))
			proj := &Project{ID: "manual-tag", Name: "manual-tag", Path: path, Status: ProjectStatusRunning}
			require.NoError(t, db.Create(proj).Error)
			deploymentError := errors.New("deployment reached")
			coordinator := &serviceImageCoordinator{err: deploymentError}
			transport := &serviceTagTransport{tags: tt.tags}
			registryService := registry.NewContainerRegistryService(db, nil, nil, nil, &http.Client{Transport: transport})
			service := &ProjectService{db: db, settingsService: settingsService, eventService: event.NewEventService(db, nil, nil), composeCoordinator: coordinator, containerRegistryService: registryService}
			service.initChildren(nil, nil)
			err = service.UpdateProjectServices(ctx, proj.ID, []string{"app", "selected-digest"}, usertypes.SystemUser, !tt.skipDiscovery)
			if !tt.skipDiscovery {
				lifecycleCode(t, err, "update.path-retired")
				require.Empty(t, coordinator.requests)
				require.Zero(t, transport.calls)
				content, readErr := os.ReadFile(filepath.Join(path, "compose.yaml"))
				require.NoError(t, readErr)
				require.Equal(t, source, string(content))
				return
			}
			require.ErrorIs(t, err, deploymentError)
			require.Equal(t, tt.wantCalls, transport.calls)
			require.Len(t, coordinator.requests, 1)
			request := coordinator.requests[0]
			require.ElementsMatch(t, []string{"app", "selected-digest"}, request.Project.ServiceNames())
			require.Equal(t, tt.wantRef, request.Project.Services["app"].Image)
			require.Equal(t, "busybox:latest", request.Project.Services["selected-digest"].Image)
			content, err := os.ReadFile(filepath.Join(path, "compose.yaml"))
			require.NoError(t, err)
			if tt.wantChanged {
				require.Equal(t, strings.Replace(source, "image: app:1.2.0", "image: "+tt.wantRef, 1), string(content))
			} else {
				require.Equal(t, source, string(content))
			}
		})
	}
}

func TestProjectServiceManualUpdateRejectsUnsafeTagPolicies(t *testing.T) {
	for _, tt := range []struct {
		name   string
		change func(*types.ServiceConfig)
	}{
		{name: "disabled", change: func(s *types.ServiceConfig) { s.Labels[labels.LabelUpdater] = "false" }},
		{name: "invalid constraint", change: func(s *types.ServiceConfig) { s.Labels[labels.LabelUpdateConstraint] = "not semver" }},
		{name: "local build", change: func(s *types.ServiceConfig) { s.Build = &types.BuildConfig{Context: "."} }},
		{name: "digest pin", change: func(s *types.ServiceConfig) { s.Image = "app@sha256:" + strings.Repeat("a", 64) }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			serviceConfig := types.ServiceConfig{Image: "app:1.2.0", Labels: types.Labels{labels.LabelUpdateStrategy: "tag"}}
			tt.change(&serviceConfig)
			service := &ProjectService{}
			service.initChildren(nil, nil)
			changes, err := service.updates.ImageChanges(t.Context(), &types.Project{Services: types.Services{"app": serviceConfig}})
			require.Error(t, err)
			require.NotContains(t, err.Error(), "registry service unavailable")
			require.Nil(t, changes)
		})
	}
	local := types.ServiceConfig{Name: "app", Image: "app:1.2.0", Build: &types.BuildConfig{Context: "."}}
	emptyService := &ProjectService{}
	emptyService.initChildren(nil, nil)
	changes, err := emptyService.updates.ImageChanges(t.Context(), &types.Project{Services: types.Services{"app": local}})
	require.NoError(t, err)
	require.Empty(t, changes, "automatic inference must preserve local-build handling")
}

func TestConfiguredProjectTagChecksMatchCurrentServicePolicy(t *testing.T) {
	services := []types.ServiceConfig{
		{Name: "stable", Image: "example:3.1.0", Labels: types.Labels{labels.LabelUpdateStrategy: "tag", labels.LabelUpdateConstraint: "3.x"}},
		{Name: "next", Image: "example:3.1.0", Labels: types.Labels{labels.LabelUpdateStrategy: "tag", labels.LabelUpdateConstraint: "4.x"}},
	}
	byRef := map[string]*imagetypes.UpdateInfo{"example:3.1.0": {HasUpdate: false, UpdateType: "digest", CheckTime: time.Now()}}
	unknown := BuildConfiguredUpdateInfo("project", services, byRef, nil)
	require.Equal(t, "unknown", unknown.Status)
	require.Zero(t, unknown.CheckedImageCount)
	require.Nil(t, unknown.ServiceUpdates["stable"].UpdateInfo)
	require.Nil(t, unknown.ServiceUpdates["next"].UpdateInfo)
	firstTarget, secondTarget := "3.2.0", "4.0.0"
	records := []imageupdate.ImageUpdateRecord{
		{Platform: new("linux/amd64"), CurrentDigest: new("sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"), LatestDigest: new("sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"), CheckTime: time.Now().UTC(), ProjectID: "project", ServiceName: "stable", PolicyKey: imageref.UpdatePolicyKey(services[0].Image, services[0].Labels), HasUpdate: true, UpdateType: "tag", LatestVersion: &firstTarget},
		{Platform: new("linux/amd64"), CurrentDigest: new("sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"), LatestDigest: new("sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"), CheckTime: time.Now().UTC(), ProjectID: "project", ServiceName: "next", PolicyKey: imageref.UpdatePolicyKey(services[1].Image, services[1].Labels), HasUpdate: true, UpdateType: "tag", LatestVersion: &secondTarget},
	}
	checked := BuildConfiguredUpdateInfo("project", services, byRef, records)
	require.Equal(t, "has_update", checked.Status)
	require.Equal(t, firstTarget, checked.ServiceUpdates["stable"].UpdateInfo.LatestVersion)
	require.Equal(t, secondTarget, checked.ServiceUpdates["next"].UpdateInfo.LatestVersion)
	require.Empty(t, checked.UpdateInfoByRef["example:3.1.0"].LatestVersion)
	services[0].Labels[labels.LabelUpdateConstraint] = "3.1.x"
	services[1].Image = "example:4.0.0"
	stale := BuildConfiguredUpdateInfo("project", services, byRef, records)
	require.Equal(t, "unknown", stale.Status)
	require.Nil(t, stale.ServiceUpdates["stable"].UpdateInfo)
	require.Nil(t, stale.ServiceUpdates["next"].UpdateInfo)
}

func TestStoppedProjectTagPolicyNeverInheritsSharedDigestCheck(t *testing.T) {
	db := setupProjectTestDB(t)
	projectsDir := t.TempDir()
	t.Setenv("PROJECTS_DIRECTORY", projectsDir)
	settingsService, err := newSettingsServiceForTest(t, t.Context(), db)
	require.NoError(t, err)
	require.NoError(t, settingsService.SetStringSetting(t.Context(), "projectsDirectory", projectsDir))
	projectPath := createComposeProjectDir(t, projectsDir, "tag-stopped")
	require.NoError(
		t,
		os.WriteFile(
			filepath.Join(
				projectPath,
				"compose.yaml",
			),
			[]byte(
				"x-arcane:\n  hidden: true\nservices:\n  app:\n    image: nginx:3.1.0\n    labels:\n      "+
					"com.getarcaneapp.arcane.updater.strategy: tag\n      com.getarcaneapp.arcane.updater.constraint: 3.x\n",
			),
			0o644,
		),
	)
	projectRecord := &Project{ID: "tag-stopped", Name: "tag-stopped", DirName: new("tag-stopped"), Path: projectPath, Status: ProjectStatusStopped, ImageRefsJSON: `["nginx:3.1.0"]`}
	require.NoError(t, db.Create(projectRecord).Error)
	require.NoError(t, db.Create(&imageupdate.ImageUpdateRecord{ID: "shared-digest", Repository: "docker.io/library/nginx", Tag: "3.1.0", UpdateType: "digest", CheckTime: time.Now()}).Error)
	imageService := image.NewImageService(db, nil, nil, nil, nil, nil, nil, nil)
	service := NewProjectService(db, settingsService, nil, imageService, nil, nil, nil, config.Load(), nil, nil)
	detail, err := service.GetProjectDetails(t.Context(), projectRecord.ID, projecttypes.AllDetails())
	require.NoError(t, err)
	require.Equal(t, "unknown", detail.UpdateInfo.Status)
	require.Nil(t, detail.UpdateInfo.ServiceUpdates["app"].UpdateInfo)
	list := []projecttypes.Details{{ID: projectRecord.ID}}
	service.enrichProjectsWithUpdateInfo(t.Context(), []Project{*projectRecord}, list, true, nil)
	require.Equal(t, "unknown", list[0].UpdateInfo.Status)
	require.Nil(t, list[0].UpdateInfo.ServiceUpdates["app"].UpdateInfo)
	target := "3.2.0"
	policy := map[string]string{labels.LabelUpdateStrategy: "tag", labels.LabelUpdateConstraint: "3.x"}
	require.NoError(
		t,
		db.Create(
			&imageupdate.ImageUpdateRecord{Platform: new("linux/amd64"), CurrentDigest: new("sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"), LatestDigest: new("sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"),
				ID:          "project::tag-stopped::app",
				ProjectID:   projectRecord.ID,
				ServiceName: "app",
				PolicyKey: imageref.UpdatePolicyKey(
					"nginx:3.1.0",
					policy,
				),
				HasUpdate:     true,
				UpdateType:    "tag",
				LatestVersion: &target,
				CheckTime:     time.Now(),
			},
		).Error,
	)
	detail, err = service.GetProjectDetails(t.Context(), projectRecord.ID, projecttypes.AllDetails())
	require.NoError(t, err)
	require.Equal(t, "has_update", detail.UpdateInfo.Status)
	require.Equal(t, target, detail.UpdateInfo.ServiceUpdates["app"].UpdateInfo.LatestVersion)
	service.enrichProjectsWithUpdateInfo(t.Context(), []Project{*projectRecord}, list, true, nil)
	require.Equal(t, "has_update", list[0].UpdateInfo.Status)
	count, err := service.CountProjectsWithPendingUpdates(t.Context(), []container.Summary{})
	require.NoError(t, err)
	require.Zero(t, count, "hidden configured services must not contribute to the dashboard badge")
}

func TestConfiguredProjectUsesScheduledRuntimeChecks(t *testing.T) {
	for _, tt := range []struct {
		name                                           string
		sourceRef, sourceConstraint, runtimeConstraint string
		wantUpdate                                     bool
	}{
		{name: "scheduled automatic check", sourceRef: "example:3.1.0", wantUpdate: true},
		{name: "source image changed", sourceRef: "example:3.0.0"},
		{name: "source policy changed", sourceRef: "example:3.1.0", sourceConstraint: "3.1.x"},
		{name: "runtime policy changed", sourceRef: "example:3.1.0", runtimeConstraint: "3.1.x"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			db := setupProjectTestDB(t)
			directory := t.TempDir()
			t.Setenv("PROJECTS_DIRECTORY", directory)
			settingsService, err := newSettingsServiceForTest(t, ctx, db)
			require.NoError(t, err)
			require.NoError(t, settingsService.SetStringSetting(ctx, "projectsDirectory", directory))
			path := createComposeProjectDir(t, directory, "scheduled-project")
			sourceLabels := types.Labels{labels.LabelUpdateStrategy: "auto"}
			source := "services:\n  web:\n    image: " + tt.sourceRef + "\n    labels:\n      com.getarcaneapp.arcane.updater.strategy: auto\n"
			if tt.sourceConstraint != "" {
				sourceLabels[labels.LabelUpdateConstraint] = tt.sourceConstraint
				source += "      com.getarcaneapp.arcane.updater.constraint: " + tt.sourceConstraint + "\n"
			}
			require.NoError(t, os.WriteFile(filepath.Join(path, "compose.yaml"), []byte(source), 0o600))
			proj := Project{ID: "scheduled-project", Name: "scheduled-project", Path: path}
			require.NoError(t, db.Create(&proj).Error)
			target := "3.2.0"
			require.NoError(
				t,
				db.Create(
					&imageupdate.ImageUpdateRecord{Platform: new("linux/amd64"), CurrentDigest: new("sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"), LatestDigest: new("sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"),
						ID:          "container::scheduled",
						ContainerID: "scheduled",
						ImageID:     "shared",
						PolicyKey: imageref.UpdatePolicyKey(
							"example:3.1.0",
							map[string]string{
								labels.LabelUpdateStrategy: "auto",
							},
						),
						Repository:    "docker.io/library/example",
						Tag:           "3.1.0",
						LatestVersion: &target,
						HasUpdate:     true,
						UpdateType:    "tag",
						CheckTime:     time.Now(),
					},
				).Error,
			)
			runtimeLabels := map[string]string{"com.docker.compose.project": "scheduled-project", "com.docker.compose.service": "web", labels.LabelUpdateStrategy: "auto"}
			runtimeLabels[labels.LabelUpdateConstraint] = cmp.Or(tt.runtimeConstraint, runtimeLabels[labels.LabelUpdateConstraint])
			runtimeServices := []projecttypes.RuntimeService{{Name: "web", ContainerID: "scheduled", Image: "example:3.1.0", ContainerLabels: runtimeLabels}}
			service := &ProjectService{db: db, settingsService: settingsService, imageService: image.NewImageService(db, nil, nil, nil, nil, nil, nil, nil)}
			service.initChildren(nil, nil)
			detail := projecttypes.Details{ID: proj.ID, Services: []types.ServiceConfig{{Name: "web", Image: tt.sourceRef, Labels: sourceLabels}}, RuntimeServices: runtimeServices}
			service.enrichProjectUpdateInfo(ctx, &detail)
			require.Equal(t, tt.wantUpdate, detail.UpdateInfo.HasUpdate)
			list := []projecttypes.Details{{ID: proj.ID, RuntimeServices: runtimeServices}}
			service.enrichProjectsWithUpdateInfo(ctx, []Project{proj}, list, true, nil)
			require.Equal(t, tt.wantUpdate, list[0].UpdateInfo.HasUpdate)
			for _, info := range []*projecttypes.UpdateInfo{detail.UpdateInfo, list[0].UpdateInfo} {
				if tt.wantUpdate {
					require.Equal(t, target, info.ServiceUpdates["web"].UpdateInfo.LatestVersion)
				} else {
					require.Nil(t, info.ServiceUpdates["web"].UpdateInfo)
					require.Equal(t, "unknown", info.Status)
				}
			}
		})
	}
}

func TestConfiguredProjectAggregatesReplicaAndPreviewChecks(t *testing.T) {
	configs := []types.ServiceConfig{{Name: "web", Image: "example:3.1.0"}}
	runtimeServices := []projecttypes.RuntimeService{
		{Name: "web", ContainerID: "one", Image: "example:3.1.0"},
		{Name: "web", ContainerID: "two", Image: "docker.io/library/example:3.1.0"},
		{Name: "other", ContainerID: "unrelated", Image: "example:3.1.0"},
	}
	scoped := map[string]*imagetypes.UpdateInfo{
		"one":       {HasUpdate: false, LatestVersion: "3.1.0", UpdateType: "tag"},
		"two":       {HasUpdate: true, LatestVersion: "3.2.0", UpdateType: "tag"},
		"unrelated": {HasUpdate: true, LatestVersion: "4.0.0", UpdateType: "tag"},
	}
	runtimeUpdates := projectdetails.ConfiguredRuntimeServiceUpdateInfo(configs, runtimeServices, scoped)
	require.Len(t, runtimeUpdates, 1)
	require.True(t, runtimeUpdates["web"].HasUpdate)
	require.Empty(t, runtimeUpdates["web"].LatestVersion, "conflicting replica targets must not choose an arbitrary version")
	previewTarget := "3.3.0"
	records := []imageupdate.ImageUpdateRecord{
		{Platform: new("linux/amd64"), CurrentDigest: new("sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"), LatestDigest: new("sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"), CheckTime: time.Now().UTC(),
			ProjectID:   "project",
			ServiceName: "web",
			PolicyKey: imageref.UpdatePolicyKey(
				"example:3.1.0",
				nil,
			),
			HasUpdate:     true,
			LatestVersion: &previewTarget,
			UpdateType:    "tag",
		},
	}
	summary := BuildConfiguredUpdateInfo("project", configs, nil, records, runtimeUpdates)
	require.True(t, summary.HasUpdate)
	require.Empty(t, summary.ServiceUpdates["web"].UpdateInfo.LatestVersion)
	require.Empty(t, summary.UpdateInfoByRef["example:3.1.0"].LatestVersion)
	require.Equal(t, "3.2.0", scoped["two"].LatestVersion, "aggregation must not modify shared checks")

	// A configured check reporting no update does not hide a pending replica.
	records[0].HasUpdate = false
	records[0].LatestDigest = records[0].CurrentDigest
	records[0].LatestVersion = nil
	summary = BuildConfiguredUpdateInfo("project", configs, nil, records, runtimeUpdates)
	require.True(t, summary.HasUpdate)
	require.True(t, summary.ServiceUpdates["web"].UpdateInfo.HasUpdate)
	require.Equal(t, "3.2.0", scoped["two"].LatestVersion)

	// A stale preview still advertising an update is superseded by a runtime
	// check at least as recent that found nothing; a newer preview still wins.
	previewTime := time.Now().Add(-time.Hour)
	records[0].HasUpdate = true
	records[0].LatestDigest = new("sha256:" + strings.Repeat("b", 64))
	records[0].LatestVersion = &previewTarget
	records[0].CheckTime = previewTime
	current := map[string]*imagetypes.UpdateInfo{"web": {HasUpdate: false, UpdateType: "digest", CheckTime: previewTime.Add(time.Minute)}}
	byRef := map[string]*imagetypes.UpdateInfo{"example:3.1.0": current["web"]}
	summary = BuildConfiguredUpdateInfo("project", configs, byRef, records, current)
	require.False(t, summary.HasUpdate)
	require.False(t, summary.ServiceUpdates["web"].UpdateInfo.HasUpdate)
	require.Equal(t, "up_to_date", summary.Status)

	current["web"].CheckTime = previewTime.Add(-time.Minute)
	summary = BuildConfiguredUpdateInfo("project", configs, byRef, records, current)
	require.True(t, summary.HasUpdate)
	require.True(t, summary.ServiceUpdates["web"].UpdateInfo.HasUpdate)
}

func TestPrepareProjectBindDirectories(t *testing.T) {
	t.Parallel()

	newProject := func(volumes ...types.ServiceVolumeConfig) *types.Project {
		return &types.Project{Services: types.Services{
			"app": {Name: "app", Volumes: volumes},
		}}
	}
	bind := func(source string) types.ServiceVolumeConfig {
		return types.ServiceVolumeConfig{Type: types.VolumeTypeBind, Source: source, Target: "/t", Bind: &types.ServiceVolumeBind{CreateHostPath: true}}
	}

	t.Run("creates missing nested directories only", func(t *testing.T) {
		t.Parallel()
		projectPath := t.TempDir()
		outside := t.TempDir()
		existingFile := filepath.Join(projectPath, "Caddyfile")
		require.NoError(t, os.WriteFile(existingFile, []byte("x"), 0o600))
		existingDir := filepath.Join(projectPath, "data")
		require.NoError(t, os.Mkdir(existingDir, 0o700))

		noCreate := bind(filepath.Join(projectPath, "manual"))
		noCreate.Bind.CreateHostPath = false
		project := newProject(
			bind(filepath.Join(projectPath, "caddy", "conf")),
			types.ServiceVolumeConfig{Type: types.VolumeTypeBind, Source: filepath.Join(projectPath, "plain"), Target: "/p"},
			bind(existingFile),
			bind(existingDir),
			bind(projectPath),
			bind(filepath.Join(outside, "elsewhere")),
			noCreate,
			types.ServiceVolumeConfig{Type: types.VolumeTypeVolume, Source: "named", Target: "/v"},
		)

		require.NoError(t, deployment.PrepareProjectBindDirectories(projectPath)(t.Context(), project))

		assert.DirExists(t, filepath.Join(projectPath, "caddy", "conf"))
		assert.DirExists(t, filepath.Join(projectPath, "plain"), "long syntax without bind block is auto-created like the daemon would")
		assert.FileExists(t, existingFile)
		info, err := os.Stat(existingDir)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o700), info.Mode().Perm(), "existing directory permissions preserved")
		assert.NoDirExists(t, filepath.Join(outside, "elsewhere"))
		assert.NoDirExists(t, filepath.Join(projectPath, "manual"), "create_host_path: false is respected")
		assert.NoDirExists(t, filepath.Join(projectPath, "named"))
	})

	t.Run("skips sources reached through symlinks escaping the project", func(t *testing.T) {
		t.Parallel()
		parent := t.TempDir()
		projectPath := filepath.Join(parent, "project")
		require.NoError(t, os.Mkdir(projectPath, 0o755))
		outside := filepath.Join(parent, "appdata")
		existingExternal := filepath.Join(outside, "jellyfin", "data")
		require.NoError(t, os.MkdirAll(existingExternal, 0o700))
		require.NoError(t, os.Symlink(outside, filepath.Join(projectPath, "abs")))
		require.NoError(t, os.Symlink(filepath.Join("..", "appdata"), filepath.Join(projectPath, "rel")))

		project := newProject(
			bind(filepath.Join(projectPath, "abs", "jellyfin", "data")),
			bind(filepath.Join(projectPath, "abs", "missing")),
			bind(filepath.Join(projectPath, "rel", "jellyfin", "data")),
			bind(filepath.Join(projectPath, "rel", "other")),
			bind(filepath.Join(projectPath, "local", "conf")),
		)

		require.NoError(t, deployment.PrepareProjectBindDirectories(projectPath)(t.Context(), project))

		info, err := os.Stat(existingExternal)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o700), info.Mode().Perm(), "external directory permissions preserved")
		assert.NoDirExists(t, filepath.Join(outside, "missing"))
		assert.NoDirExists(t, filepath.Join(outside, "other"))
		assert.DirExists(t, filepath.Join(projectPath, "local", "conf"))
	})

	t.Run("reports symlink loops and canceled contexts", func(t *testing.T) {
		t.Parallel()
		projectPath := t.TempDir()
		require.NoError(t, os.Symlink("loop", filepath.Join(projectPath, "loop")))

		err := deployment.PrepareProjectBindDirectories(projectPath)(t.Context(), newProject(bind(filepath.Join(projectPath, "loop", "conf"))))
		require.ErrorIs(t, err, acfs.ErrSymlinkLoop)
		assert.Contains(t, err.Error(), "service app")

		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		err = deployment.PrepareProjectBindDirectories(projectPath)(ctx, newProject(bind(filepath.Join(projectPath, "conf"))))
		require.ErrorIs(t, err, context.Canceled)
		assert.NoDirExists(t, filepath.Join(projectPath, "conf"))
	})
}

func TestPrepareProjectBindDirectories_PermissionFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires Unix permission semantics")
	}
	if os.Geteuid() == 0 {
		t.Skip("permission bits are ignored when running as root")
	}

	for _, tc := range []struct {
		name       string
		dirMode    os.FileMode
		fileMode   os.FileMode
		createFile bool
		wantError  bool
	}{
		{name: "missing directory in unwritable parent", dirMode: 0o500, wantError: true},
		{name: "missing directory in non-searchable parent", dirMode: 0o600},
		{name: "existing file in non-searchable parent", dirMode: 0o600, fileMode: 0o640, createFile: true},
		{name: "unreadable file in searchable parent", dirMode: 0o700, fileMode: 0o000, createFile: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			projectPath := t.TempDir()
			locked := filepath.Join(projectPath, "locked")
			require.NoError(t, os.Mkdir(locked, 0o700))
			t.Cleanup(func() { assert.NoError(t, os.Chmod(locked, 0o700)) })
			source := filepath.Join(locked, "conf")
			var originalInfo os.FileInfo
			if tc.createFile {
				require.NoError(t, os.WriteFile(source, []byte("config"), tc.fileMode))
				var err error
				originalInfo, err = os.Stat(source)
				require.NoError(t, err)
			}
			require.NoError(t, os.Chmod(locked, tc.dirMode))
			if tc.createFile {
				_, err := os.ReadFile(source)
				require.ErrorIs(t, err, fs.ErrPermission)
			}
			_, statErr := os.Lstat(source)
			switch {
			case tc.dirMode&0o100 == 0:
				require.ErrorIs(t, statErr, fs.ErrPermission)
			case tc.createFile:
				require.NoError(t, statErr)
			default:
				require.ErrorIs(t, statErr, fs.ErrNotExist)
			}

			project := &types.Project{Services: types.Services{
				"app": {Name: "app", Volumes: []types.ServiceVolumeConfig{{
					Type: types.VolumeTypeBind, Source: source, Target: "/etc/caddy",
				}}},
			}}

			err := deployment.PrepareProjectBindDirectories(projectPath)(t.Context(), project)
			if tc.wantError {
				require.ErrorIs(t, err, fs.ErrPermission)
				assert.Contains(t, err.Error(), source)
				assert.Contains(t, err.Error(), "service app")
			} else {
				require.NoError(t, err)
			}
			dirInfo, err := os.Stat(locked)
			require.NoError(t, err)
			assert.Equal(t, tc.dirMode, dirInfo.Mode().Perm())
			require.NoError(t, os.Chmod(locked, 0o700))
			if !tc.createFile {
				_, lstatErr := os.Lstat(source)
				require.ErrorIs(t, lstatErr, fs.ErrNotExist)
				return
			}
			info, err := os.Stat(source)
			require.NoError(t, err)
			assert.True(t, os.SameFile(originalInfo, info))
			assert.Equal(t, originalInfo.Mode(), info.Mode())
			assert.Equal(t, originalInfo.ModTime(), info.ModTime())
			require.NoError(t, os.Chmod(source, 0o600))
			content, err := os.ReadFile(source)
			require.NoError(t, err)
			assert.Equal(t, "config", string(content))
		})
	}
}

func newEnvDirectoryProject(t *testing.T, id string) (*ProjectService, *Project, string, context.Context) {
	t.Helper()

	db := setupProjectTestDB(t)
	ctx := t.Context()

	projectsDir := t.TempDir()
	t.Setenv("PROJECTS_DIRECTORY", projectsDir)

	settingsService, err := newSettingsServiceForTest(t, ctx, db)
	require.NoError(t, err)
	require.NoError(t, settingsService.SetStringSetting(ctx, "projectsDirectory", projectsDir))

	svc := NewProjectService(db, settingsService, event.NewEventService(db, nil, nil), nil, nil, nil, nil, config.Load(), nil, nil)

	projectPath := createComposeProjectDir(t, projectsDir, id)
	envDir := filepath.Join(projectPath, ".env")
	require.NoError(t, os.Mkdir(envDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(envDir, "keep"), []byte("x"), 0o644))

	project := &Project{
		ID:      "proj-" + id,
		Name:    id,
		DirName: new(id),
		Path:    projectPath,
		Status:  ProjectStatusStopped,
	}
	require.NoError(t, db.Create(project).Error)

	return svc, project, envDir, ctx
}

func assertEnvDirectoryIntact(t *testing.T, envDir string) {
	t.Helper()
	info, err := os.Stat(envDir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
	assert.FileExists(t, filepath.Join(envDir, "keep"))
}

func TestProjectService_EnvDirectory_SyncAndDetails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires Unix permission semantics")
	}
	svc, project, envDir, ctx := newEnvDirectoryProject(t, "env-dir-scan")

	require.NoError(t, svc.SyncProjectsFromFileSystem(ctx))
	all, err := svc.ListAllProjects(ctx)
	require.NoError(t, err)
	require.Len(t, all, 1)
	assert.Equal(t, project.Path, all[0].Path)

	details, err := svc.GetProjectDetails(ctx, project.ID, projecttypes.AllDetails())
	require.NoError(t, err)
	assert.Empty(t, details.EnvContent)
	assert.Contains(t, details.ComposeContent, "nginx:alpine")

	assertEnvDirectoryIntact(t, envDir)
}

func TestProjectService_EnvDirectory_UpdateCompose(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires Unix permission semantics")
	}
	svc, project, envDir, ctx := newEnvDirectoryProject(t, "env-dir-update")

	_, err := svc.UpdateProject(ctx, project.ID, nil, new("services:\n  app:\n    image: nginx:1.27-alpine\n"), nil, nil, usertypes.Actor{ID: "u1", Username: "tester"})
	require.NoError(t, err)

	composeBytes, err := os.ReadFile(filepath.Join(project.Path, "compose.yaml"))
	require.NoError(t, err)
	assert.Contains(t, string(composeBytes), "nginx:1.27-alpine")
	assertEnvDirectoryIntact(t, envDir)
}

func TestProjectService_EnvDirectory_EnvSaveFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires Unix permission semantics")
	}
	svc, project, envDir, ctx := newEnvDirectoryProject(t, "env-dir-save")

	_, err := svc.UpdateProject(ctx, project.ID, nil, nil, new("FOO=bar\n"), nil, usertypes.Actor{ID: "u1", Username: "tester"})
	require.ErrorContains(t, err, ".env is a directory")
	assertEnvDirectoryIntact(t, envDir)
}

func TestComposeFile(t *testing.T) {
	t.Parallel()

	const resolved = "/projects/app/compose.yaml"
	transient := errors.New("transient")

	type call struct {
		wantPath  string
		wantErr   error
		wantCalls int
	}

	tests := []struct {
		name      string
		env       *projectMetadataEnv
		projectID string
		// results is returned by resolve in order; the last entry repeats.
		results []error
		calls   []call
	}{
		{
			name:      "caches success",
			env:       &projectMetadataEnv{},
			projectID: "p1",
			results:   []error{nil},
			calls: []call{
				{wantPath: resolved, wantCalls: 1},
				{wantPath: resolved, wantCalls: 1},
			},
		},
		{
			name:      "retries after failure then caches success",
			env:       &projectMetadataEnv{},
			projectID: "p1",
			results:   []error{transient, nil},
			calls: []call{
				{wantErr: transient, wantCalls: 1},
				{wantPath: resolved, wantCalls: 2},
				{wantPath: resolved, wantCalls: 2},
			},
		},
		{
			name:      "repeated failures are never memoized",
			env:       &projectMetadataEnv{},
			projectID: "p1",
			results:   []error{transient},
			calls: []call{
				{wantErr: transient, wantCalls: 1},
				{wantErr: transient, wantCalls: 2},
			},
		},
		{
			name:      "nil env resolves every call",
			env:       nil,
			projectID: "p1",
			results:   []error{nil},
			calls: []call{
				{wantPath: resolved, wantCalls: 1},
				{wantPath: resolved, wantCalls: 2},
			},
		},
		{
			name:      "empty project ID resolves every call",
			env:       &projectMetadataEnv{},
			projectID: "",
			results:   []error{nil},
			calls: []call{
				{wantPath: resolved, wantCalls: 1},
				{wantPath: resolved, wantCalls: 2},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			calls := 0
			resolve := func() (string, error) {
				calls++
				err := tt.results[min(calls, len(tt.results))-1]
				if err != nil {
					return "", err
				}
				return resolved, nil
			}

			for i, want := range tt.calls {
				path, err := tt.env.composeFile(tt.projectID, resolve)
				if !errors.Is(err, want.wantErr) {
					t.Fatalf("call %d: err = %v, want %v", i+1, err, want.wantErr)
				}
				if path != want.wantPath {
					t.Fatalf("call %d: path = %q, want %q", i+1, path, want.wantPath)
				}
				if calls != want.wantCalls {
					t.Fatalf("call %d: resolve invoked %d times, want %d", i+1, calls, want.wantCalls)
				}
			}
		})
	}
}
