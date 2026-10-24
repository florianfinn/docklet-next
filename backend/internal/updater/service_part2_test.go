// Source: getarcaneapp/arcane/backend/internal/updater/service_test.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed APNS mobile push; functional cut: removed GitOps and saved Git repositories; SQLite temp-file persistence fixture.

package updater

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"

	"net/http"
	"net/http/httptest"
	"net/url"

	"strings"

	"testing"
	"time"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/florianfinn/docklet-next/backend/internal/testsqlite"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	projecttypes "github.com/florianfinn/docklet-next/types/project"
	"github.com/florianfinn/docklet-next/types/scheduler"
	arcaneupdater "github.com/florianfinn/docklet-next/types/updater"

	"github.com/moby/moby/api/pkg/authconfig"
	"github.com/moby/moby/api/types/container"
	dockertypesimage "github.com/moby/moby/api/types/image"
	dockerregistry "github.com/moby/moby/api/types/registry"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.getarcane.app/sys/crypto"
	"go.getarcane.app/updater"
	"go.getarcane.app/updater/labels"
	"go.getarcane.app/updater/refs"
	updatertypes "go.getarcane.app/updater/types"
	"gorm.io/gorm"

	"github.com/florianfinn/docklet-next/backend/internal/activity"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/docker"

	"github.com/florianfinn/docklet-next/backend/internal/event"

	"github.com/florianfinn/docklet-next/backend/internal/imageupdate"

	"github.com/florianfinn/docklet-next/backend/internal/project"
	"github.com/florianfinn/docklet-next/backend/internal/registry"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/backend/internal/updater/children/execution"
	activitylib "github.com/florianfinn/docklet-next/backend/pkg/libarcane/activity"
	"github.com/florianfinn/docklet-next/backend/pkg/projects"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/jobcontext"

	francistest "github.com/florianfinn/docklet-next/backend/pkg/utils/francis/testing"
)

func TestUpdaterService_AcceptSingleContainerUpdateFailureFinalizesInternal(t *testing.T) {
	db := setupProjectTestDBInternal(t)
	require.NoError(t, db.AutoMigrate(activity.Models()...))
	activityService := activity.NewActivityService(db, nil)
	svc, err := NewUpdaterService(db, nil, nil, nil, nil, nil, nil, nil, nil, nil, activityService, nil, nil, nil, nil)
	require.NoError(t, err)
	runtime := francistest.New(t)
	require.NoError(t, svc.RegisterActors(runtime))
	// A service whose host has not started rejects durable dispatch.

	accepted, err := svc.AcceptSingleContainerUpdate(t.Context(), "container-1")
	require.Nil(t, accepted)
	require.ErrorContains(t, err, "persist container update")
	var saved activity.Activity
	require.NoError(t, db.First(&saved, "resource_id = ?", "container-1").Error)
	require.Equal(t, activitytypes.StatusFailed, saved.Status)

	unmigrated := setupProjectTestDBInternal(t)
	svc.deps.Activity = activity.NewActivityService(unmigrated, nil)
	accepted, err = svc.AcceptSingleContainerUpdate(t.Context(), "container-2")
	require.Nil(t, accepted)
	require.ErrorContains(t, err, "start container update activity")
}

func TestUpdaterService_SingleContainerActivityResultMessagesInternal(t *testing.T) {
	for _, tt := range []struct {
		name        string
		result      *arcaneupdater.Result
		wantStatus  activitytypes.Status
		wantMessage string
	}{
		{
			name: "updated",
			result: &arcaneupdater.Result{
				Updated: 1,
				Items: []arcaneupdater.ResourceResult{{
					Status:       string(updater.StatusUpdated),
					ResourceName: "web",
				}},
			},
			wantStatus:  activitytypes.StatusSuccess,
			wantMessage: "Container updated",
		},
		{
			name: "restarted",
			result: &arcaneupdater.Result{
				Restarted: 1,
				Items: []arcaneupdater.ResourceResult{{
					Status:       string(updater.StatusRestarted),
					ResourceName: "web",
				}},
			},
			wantStatus:  activitytypes.StatusSuccess,
			wantMessage: "Container updated",
		},
		{
			name: "skipped",
			result: &arcaneupdater.Result{
				Skipped: 1,
				Items: []arcaneupdater.ResourceResult{{
					Status: string(updater.StatusSkipped),
					Error:  "immutable image reference",
				}},
			},
			wantStatus:  activitytypes.StatusSuccess,
			wantMessage: "Container update skipped: immutable image reference",
		},
		{
			name: "already current",
			result: &arcaneupdater.Result{
				Checked: 1,
				Items:   []arcaneupdater.ResourceResult{{Status: string(updater.StatusUpToDate)}},
			},
			wantStatus:  activitytypes.StatusSuccess,
			wantMessage: "Container already current",
		},
		{
			name: "failed",
			result: &arcaneupdater.Result{
				Failed: 1,
				Items: []arcaneupdater.ResourceResult{{
					Status: string(updater.StatusFailed),
					Error:  "pull failed",
				}},
			},
			wantStatus:  activitytypes.StatusFailed,
			wantMessage: "pull failed",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db := setupProjectTestDBInternal(t)
			require.NoError(t, db.AutoMigrate(activity.Models()...))
			activityService := activity.NewActivityService(db, nil)
			svc, err := NewUpdaterService(db, nil, nil, nil, nil, nil, nil, nil, nil, nil, activityService, nil, nil, nil, nil)
			require.NoError(t, err)
			started, err := activityService.StartActivity(
				t.Context(),
				activitylib.StartRequest{
					EnvironmentID: "0",
					Type:          activitytypes.TypeAutoUpdate,
					Metadata:      database.JSON{"containerID": "old-id"},
				},
			)
			require.NoError(t, err)
			svc.finishSingleContainerUpdateInternal(t.Context(), started.ID, tt.result, nil)
			var saved activity.Activity
			require.NoError(t, db.First(&saved, "id = ?", started.ID).Error)
			require.Equal(t, tt.wantStatus, saved.Status)
			require.Equal(t, tt.wantMessage, saved.LatestMessage)
			require.Equal(t, "old-id", saved.Metadata["containerID"])
		})
	}
}

func TestUpdaterService_ApplyPending_ProjectFailureDoesNotBlockOtherProjectsInternal(t *testing.T) {
	ctx := t.Context()

	oldRefFailed := "registry.example.com/team/fail:1.0.0"
	newRefFailed := "registry.example.com/team/fail:1.0.1"
	oldRefUpdated := "registry.example.com/team/success:2.0.0"
	newRefUpdated := "registry.example.com/team/success:2.0.1"
	oldImageIDFailed := "sha256:old-fail"
	newImageIDFailed := "sha256:new-fail"
	oldImageIDUpdated := "sha256:old-success"
	newImageIDUpdated := "sha256:new-success"

	failedLabels := map[string]string{
		"com.docker.compose.project": "proj-fail",
		"com.docker.compose.service": "app",
		labels.LabelUpdateStrategy:   "auto",
	}
	updatedLabels := map[string]string{
		"com.docker.compose.project": "proj-success",
		"com.docker.compose.service": "app",
		labels.LabelUpdateStrategy:   "auto",
	}

	containers := []container.Summary{
		{
			ID:      "container-fail",
			Names:   []string{"/proj-fail-app-1"},
			Image:   oldRefFailed,
			ImageID: oldImageIDFailed,
			Labels:  failedLabels,
			State:   "running",
		},
		{
			ID:      "container-success",
			Names:   []string{"/proj-success-app-1"},
			Image:   oldRefUpdated,
			ImageID: oldImageIDUpdated,
			Labels:  updatedLabels,
			State:   "running",
		},
	}

	verificationByService := map[string][]container.Summary{
		"proj-success/app": {
			{
				ID:      "container-success-new",
				Names:   []string{"/proj-success-app-1"},
				Image:   newRefUpdated,
				ImageID: newImageIDUpdated,
				Labels:  updatedLabels,
				State:   "running",
			},
		},
	}

	inspectByID := map[string]container.InspectResponse{
		"container-success-new": {
			ID:    "container-success-new",
			Image: newImageIDUpdated,
			Config: &container.Config{
				Image:  newRefUpdated,
				Labels: updatedLabels,
			},
			State: &container.State{Running: true},
		},
		"container-fail": {
			ID:    "container-fail",
			Image: oldImageIDFailed,
			Config: &container.Config{
				Image:  oldRefFailed,
				Labels: failedLabels,
			},
		},
		"container-success": {
			ID:    "container-success",
			Image: oldImageIDUpdated,
			Config: &container.Config{
				Image:  oldRefUpdated,
				Labels: updatedLabels,
			},
		},
	}

	imageInspectByRef := map[string]dockertypesimage.InspectResponse{
		newRefFailed: {
			ID:       newImageIDFailed,
			RepoTags: []string{newRefFailed},
		},
		newRefUpdated: {
			ID:       newImageIDUpdated,
			RepoTags: []string{newRefUpdated},
		},
	}

	server := newUpdaterApplyPendingDockerServerInternal(t, containers, verificationByService, inspectByID, imageInspectByRef)
	dockerProvider := fakeDockerClientProviderInternal{client: newTestDockerClientInternal(t, server)}
	puller := &fakeImagePullerInternal{}
	recorder := &fakeRunRecorderInternal{}
	projectUpdater := &fakeProjectUpdaterInternal{
		projects: map[string]updater.ComposeProject{
			"proj-fail":    {ID: "project-fail", Name: "proj-fail"},
			"proj-success": {ID: "project-success", Name: "proj-success"},
		},
		updateErrs: map[string]error{
			"project-fail": errors.New("pull updated service images: unauthorized"),
		},
	}

	svc, svcErr := NewUpdaterService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	require.NoError(t, svcErr)
	engine, engineErr := updater.New(updater.Config{
		DockerClientProvider: dockerProvider,
		ImagePuller:          puller,
		PendingStore: updater.NewMemoryPendingStore(
			updater.ImageUpdateRecord{
				ID:             oldImageIDFailed,
				Repository:     "registry.example.com/team/fail",
				Tag:            "1.0.0",
				HasUpdate:      true,
				UpdateType:     updater.UpdateTypeTag,
				CurrentVersion: "1.0.0",
				LatestVersion:  new("1.0.1"),
			},
			updater.ImageUpdateRecord{
				ID:             oldImageIDUpdated,
				Repository:     "registry.example.com/team/success",
				Tag:            "2.0.0",
				HasUpdate:      true,
				UpdateType:     updater.UpdateTypeTag,
				CurrentVersion: "2.0.0",
				LatestVersion:  new("2.0.1"),
			},
		),
		RunRecorder:    recorder,
		Settings:       fakeSettingsProviderInternal{},
		ProjectUpdater: projectUpdater,
		UsedImageCollector: fakeUsedImageCollectorInternal{images: map[string]struct{}{
			oldRefFailed:  {},
			oldRefUpdated: {},
		}},
		LabelPolicy: updater.DefaultLabelPolicy(),
	})
	require.NoError(t, engineErr)
	t.Cleanup(func() {
		require.NoError(t, engine.Close())
	})
	svc.engine = engine

	result, err := svc.ApplyPending(ctx, arcaneupdater.Options{})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 1, result.Updated, "tag updates report successfully recreated containers")
	assert.Equal(t, 1, result.Failed, "the failed project should be recorded as failed")
	assert.Len(t, result.Items, 2)
	assert.ElementsMatch(t, []string{newRefFailed, newRefUpdated}, puller.pulled)
	assert.ElementsMatch(t, []string{"project-fail:app", "project-success:app"}, projectUpdater.calls)

	assert.Equal(t, updatertypes.ServiceImageChange{ExpectedRef: oldRefUpdated, TargetRef: newRefUpdated}, projectUpdater.imageChanges["project-success"]["app"])
	statusByResource := map[string]string{}
	errorByResource := map[string]string{}
	for _, item := range result.Items {
		statusByResource[item.ResourceID] = item.Status
		errorByResource[item.ResourceID] = item.Error
	}

	assert.Equal(t, string(updater.StatusFailed), statusByResource["container-fail"])
	assert.Contains(t, errorByResource["container-fail"], "pull updated service images: unauthorized")
	assert.Equal(t, string(updater.StatusUpdated), statusByResource["container-success"])

	var recordedStatuses []updater.ResourceStatus
	for _, recorded := range recorder.results {
		recordedStatuses = append(recordedStatuses, recorded.Status)
	}
	assert.Contains(t, recordedStatuses, updater.StatusFailed)
	assert.Contains(t, recordedStatuses, updater.StatusUpdated)
}

func TestUpdaterService_ApplyPending_RoutesLegacyArcaneServerThroughSelfUpgradeInternal(t *testing.T) {
	ctx := t.Context()

	oldRef := "ghcr.io/getarcaneapp/arcane:1.0.0"
	newRef := "ghcr.io/getarcaneapp/arcane:1.0.1"
	oldImageID := "sha256:old-arcane"
	newImageID := "sha256:new-arcane"
	arcaneLabels := map[string]string{
		"com.docker.compose.project":   "arcane",
		"com.docker.compose.service":   "server",
		labels.LabelArcaneLegacyServer: "true",
		labels.LabelUpdateStrategy:     "auto",
	}

	containers := []container.Summary{
		{
			ID:      "arcane-container",
			Names:   []string{"/arcane"},
			Image:   oldRef,
			ImageID: oldImageID,
			Labels:  arcaneLabels,
			State:   "running",
		},
	}
	verificationByService := map[string][]container.Summary{
		"arcane/server": {
			{
				ID:      "arcane-container-new",
				Names:   []string{"/arcane"},
				Image:   newRef,
				ImageID: newImageID,
				Labels:  arcaneLabels,
				State:   "running",
			},
		},
	}
	inspectByID := map[string]container.InspectResponse{
		"arcane-container": {
			ID:    "arcane-container",
			Name:  "/arcane",
			Image: oldImageID,
			Config: &container.Config{
				Image:  oldRef,
				Labels: arcaneLabels,
			},
		},
	}
	imageInspectByRef := map[string]dockertypesimage.InspectResponse{
		newRef: {
			ID:       newImageID,
			RepoTags: []string{newRef},
		},
	}

	server := newUpdaterApplyPendingDockerServerInternal(t, containers, verificationByService, inspectByID, imageInspectByRef)
	dockerProvider := fakeDockerClientProviderInternal{client: newTestDockerClientInternal(t, server)}
	puller := &fakeImagePullerInternal{}
	projectUpdater := &fakeProjectUpdaterInternal{
		projects: map[string]updater.ComposeProject{
			"arcane": {ID: "project-arcane", Name: "arcane"},
		},
	}
	mockUpgrade := &mockSystemUpgradeServiceInternal{}

	svc, svcErr := NewUpdaterService(nil, nil, nil, nil, nil, nil, nil, nil, nil, mockUpgrade, nil, nil, nil, nil, nil)
	require.NoError(t, svcErr)
	engine, engineErr := updater.New(updater.Config{
		DockerClientProvider: dockerProvider,
		ImagePuller:          puller,
		PendingStore: updater.NewMemoryPendingStore(updater.ImageUpdateRecord{
			ID:             oldImageID,
			Repository:     "ghcr.io/getarcaneapp/arcane",
			Tag:            "1.0.0",
			HasUpdate:      true,
			UpdateType:     updater.UpdateTypeTag,
			CurrentVersion: "1.0.0",
			LatestVersion:  new("1.0.1"),
		}),
		Settings:       fakeSettingsProviderInternal{},
		ProjectUpdater: projectUpdater,
		SelfUpdater:    svc,
		UsedImageCollector: fakeUsedImageCollectorInternal{images: map[string]struct{}{
			oldRef: {},
		}},
		LabelPolicy: updater.DefaultLabelPolicy(),
	})
	require.NoError(t, engineErr)
	t.Cleanup(func() {
		require.NoError(t, engine.Close())
	})
	svc.engine = engine

	frozenPlan, planErr := json.Marshal(struct {
		Targets []arcaneupdater.FrozenUpdateTarget
	}{Targets: []arcaneupdater.FrozenUpdateTarget{{
		ContainerID:     "arcane-container",
		DesiredImageRef: newRef,
		DesiredDigest:   "sha256:desired",
	}}})
	require.NoError(t, planErr)
	ctx = jobcontext.WithExecution(ctx, scheduler.Run{AttemptCount: 1, Outcome: scheduler.Outcome{Targets: []scheduler.TargetOutcome{{ID: "auto-update", RecoveryData: frozenPlan}}}}, nil)
	result, err := svc.ApplyPending(ctx, arcaneupdater.Options{})
	require.ErrorContains(t, err, "self-update was triggered")
	require.NotNil(t, result)
	assert.True(t, mockUpgrade.triggerCalled, "scheduled auto-update should use CLI self-upgrade for legacy Arcane server labels")
	assert.Empty(t, projectUpdater.calls, "legacy Arcane server should not be updated through project services")
}

// The engine picks the reference Arcane's updates are applied to, so its choice
// is worth pinning here as well as in the library.
func TestPullableImageRefInternal(t *testing.T) {
	tests := []struct {
		name         string
		summaryImage string
		inspectImage string
		repoTags     []string
		expectedRef  string
	}{
		{
			name:         "inspect config image wins",
			summaryImage: "nginx:latest",
			inspectImage: "registry.example.com/nginx:stable",
			expectedRef:  "registry.example.com/nginx:stable",
		},
		{
			name:         "summary image used when inspect is image ID",
			summaryImage: "redis:7",
			inspectImage: "sha256:abcdef",
			expectedRef:  "redis:7",
		},
		{
			name:         "repo tag fallback skips none tag",
			summaryImage: "sha256:abcdef",
			inspectImage: "sha256:abcdef",
			repoTags:     []string{"<none>:<none>", "postgres:16"},
			expectedRef:  "postgres:16",
		},
		{
			name:         "no pullable ref",
			summaryImage: "sha256:abcdef",
			inspectImage: "sha256:abcdef",
			repoTags:     []string{"<none>:<none>"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expectedRef, refs.PullableImageRef(tt.summaryImage, tt.inspectImage, tt.repoTags))
		})
	}
}

// createTestPullRegistryInternal inserts an enabled generic registry with an encrypted token.
func createTestPullRegistryInternal(t *testing.T, db *database.DB, localUrl, username, token string) {
	t.Helper()

	encryptedToken, err := crypto.Encrypt(token)
	require.NoError(t, err)

	reg := &registry.ContainerRegistry{
		URL:          localUrl,
		Username:     username,
		Token:        encryptedToken,
		Enabled:      true,
		RegistryType: registry.RegistryTypeGeneric,
	}
	require.NoError(t, db.WithContext(t.Context()).Create(reg).Error)
}

// decodeRegistryAuthInternal decodes a base64 X-Registry-Auth header.
func decodeRegistryAuthInternal(t *testing.T, encoded string) dockerregistry.AuthConfig {
	t.Helper()

	cfg, err := authconfig.Decode(encoded)
	require.NoError(t, err)
	return *cfg
}

// newTestDockerClientInternal builds a Docker client pointed at a fake Docker HTTP server.
func newTestDockerClientInternal(t *testing.T, server *httptest.Server) *client.Client {
	t.Helper()

	httpClient := server.Client()
	cli, err := client.New(
		client.WithHost(server.URL),
		client.WithAPIVersion("1.41"),
		client.WithHTTPClient(httpClient),
	)
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = cli.Close()
	})

	return cli
}

// newImagePullServerWithObserverInternal is NewImagePullServer with a pull callback.
func newImagePullServerWithObserverInternal(t *testing.T, inspectByRef map[string]dockertypesimage.InspectResponse, onPull func(fullRef, authHeader string)) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/images/create"):
			fullRef := strings.TrimSpace(r.URL.Query().Get("fromImage"))
			tag := strings.TrimSpace(r.URL.Query().Get("tag"))
			if fullRef != "" && tag != "" {
				lastSlash := strings.LastIndex(fullRef, "/")
				lastColon := strings.LastIndex(fullRef, ":")
				if lastColon <= lastSlash {
					fullRef += ":" + tag
				}
			}
			if onPull != nil {
				onPull(fullRef, strings.TrimSpace(r.Header.Get("X-Registry-Auth")))
			}

			w.Header().Set("Content-Type", "application/json")
			_ = json.MarshalWrite(w, map[string]string{"status": "Pulled", "id": fullRef})
			return
		case strings.Contains(r.URL.Path, "/images/") && strings.HasSuffix(r.URL.Path, "/json"):
			path := r.URL.Path
			imagePathIndex := strings.Index(path, "/images/")
			if !assert.NotEqual(t, -1, imagePathIndex) {
				return
			}
			encodedRef := strings.TrimSuffix(path[imagePathIndex+len("/images/"):], "/json")
			imageRef, err := url.PathUnescape(encodedRef)
			if !assert.NoError(t, err) {
				return
			}

			inspect, ok := inspectByRef[imageRef]
			if !ok {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			if !assert.NoError(t, json.MarshalWrite(w, inspect)) {
				return
			}
			return
		default:
			http.NotFound(w, r)
		}
	}))

	t.Cleanup(server.Close)

	return server
}

// setupProjectTestDBInternal builds an in-memory DB migrated for project-related tests.
func setupProjectTestDBInternal(t *testing.T) *database.DB {
	t.Helper()
	db, err := gorm.Open(testsqlite.Dialector(t, ":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&project.Project{}, &settings.SettingVariable{}, &imageupdate.ImageUpdateRecord{}, &event.Event{}))
	return &database.DB{DB: db}
}

func TestUpdaterService_ScopedPendingRecordsIsolationInternal(t *testing.T) {
	db := setupProjectTestDBInternal(t)
	imageRef := "registry.example.com/team/app:1.2.3"
	stored := []imageupdate.ImageUpdateRecord{
		{ID: "shared-image", Repository: "registry.example.com/team/app", Tag: "1.2.3", HasUpdate: true, UpdateType: imageupdate.UpdateTypeDigest},
		{
			ID:            "container::tag-a",
			ContainerID:   "tag-a",
			ImageID:       "shared-image",
			Repository:    "registry.example.com/team/app",
			Tag:           "1.2.3",
			HasUpdate:     true,
			UpdateType:    imageupdate.UpdateTypeTag,
			LatestVersion: new("1.3.0"),
		},
		{
			ID:            "container::tag-b",
			ContainerID:   "tag-b",
			ImageID:       "shared-image",
			Repository:    "registry.example.com/team/app",
			Tag:           "1.2.3",
			HasUpdate:     true,
			UpdateType:    imageupdate.UpdateTypeTag,
			LatestVersion: new("1.2.4"),
		},
	}
	require.NoError(t, db.Create(&stored).Error)
	containers := []container.Summary{
		{ID: "tag-a", Image: imageRef, ImageID: "shared-image", Labels: map[string]string{labels.LabelUpdateStrategy: "tag"}, State: container.StateRunning},
		{ID: "tag-b", Image: imageRef, ImageID: "shared-image", Labels: map[string]string{labels.LabelUpdateStrategy: "tag"}, State: container.StateRunning},
		{ID: "digest-a", Labels: map[string]string{labels.LabelUpdateStrategy: "digest"}, Image: imageRef, ImageID: "shared-image", State: container.StateRunning},
		{ID: "digest-b", Labels: map[string]string{labels.LabelUpdateStrategy: "digest"}, Image: imageRef, ImageID: "shared-image", State: container.StateRunning},
	}
	server := newUpdaterApplyPendingDockerServerInternal(t, containers, nil, nil, nil)
	dockerSvc := docker.NewDockerClientService(t.Context(), nil, nil, nil).WithClient(newTestDockerClientInternal(t, server))
	svc, err := NewUpdaterService(db, nil, dockerSvc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	require.NoError(t, err)
	records, err := svc.PendingImageUpdates(t.Context())
	require.NoError(t, err)
	require.Len(t, records, 4)
	byContainer := map[string]updater.ImageUpdateRecord{}
	for _, record := range records {
		byContainer[record.ContainerID] = record
	}
	assert.Equal(t, "1.3.0", *byContainer["tag-a"].LatestVersion)
	assert.Equal(t, "1.2.4", *byContainer["tag-b"].LatestVersion)
	assert.Equal(t, updater.UpdateTypeDigest, byContainer["digest-a"].UpdateType)
	assert.Equal(t, updater.UpdateTypeDigest, byContainer["digest-b"].UpdateType)
	assert.Equal(t, "shared-image", byContainer["digest-b"].ID)

	require.NoError(t, svc.ClearImageUpdateRecord(t.Context(), byContainer["tag-a"]))
	var pending []imageupdate.ImageUpdateRecord
	require.NoError(t, db.Where("has_update = ?", true).Find(&pending).Error)
	var pendingIDs []string
	for _, record := range pending {
		pendingIDs = append(pendingIDs, record.ID)
	}
	assert.ElementsMatch(t, []string{"shared-image", "container::tag-b"}, pendingIDs)
}

func TestUpdaterService_SharedDigestRecordClearingInternal(t *testing.T) {
	for _, tt := range []struct {
		name           string
		remainingImage string
		wantPending    bool
	}{
		{name: "another container still needs image", remainingImage: "old-image", wantPending: true},
		{name: "all digest containers updated", remainingImage: "new-image", wantPending: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db := setupProjectTestDBInternal(t)
			imageRef := "registry.example.com/team/app:1.2.3"
			record := imageupdate.ImageUpdateRecord{ID: "shared-image", Repository: "registry.example.com/team/app", Tag: "1.2.3", HasUpdate: true, UpdateType: imageupdate.UpdateTypeDigest}
			require.NoError(t, db.Create(&record).Error)
			containers := []container.Summary{
				{ID: "updated", Labels: map[string]string{labels.LabelUpdateStrategy: "digest"}, Image: imageRef, ImageID: "new-image", State: container.StateRunning},
				{ID: "remaining", Labels: map[string]string{labels.LabelUpdateStrategy: "digest"}, Image: imageRef, ImageID: tt.remainingImage, State: container.StateRunning},
				{ID: "separate-tag-policy", Image: imageRef, ImageID: "old-image", State: container.StateRunning, Labels: map[string]string{labels.LabelUpdateStrategy: "tag"}},
			}
			server := newUpdaterApplyPendingDockerServerInternal(t, containers, nil, nil, map[string]dockertypesimage.InspectResponse{imageRef: {ID: "new-image"}})
			dockerSvc := docker.NewDockerClientService(t.Context(), nil, nil, nil).WithClient(newTestDockerClientInternal(t, server))
			svc, err := NewUpdaterService(db, nil, dockerSvc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
			require.NoError(t, err)
			converted := imageUpdateRecordToModuleInternal(record)
			converted.ContainerID = "updated"
			require.NoError(t, svc.ClearImageUpdateRecord(t.Context(), converted))
			var actual imageupdate.ImageUpdateRecord
			require.NoError(t, db.First(&actual, "id = ?", record.ID).Error)
			assert.Equal(t, tt.wantPending, actual.HasUpdate)
		})
	}
}

type projectCheckRegistryInternal struct{}

func (projectCheckRegistryInternal) ListTags(context.Context, string) ([]string, error) {
	return []string{"3.20.0", "3.20.1", "3.20.2"}, nil
}

func (projectCheckRegistryInternal) ImageDigest(context.Context, string) (string, error) {
	return "sha256:" + strings.Repeat("a", 64), nil
}

func TestUpdaterProjectChecksWithoutContainersInternal(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !assert.Contains(t, r.URL.Path, "/images/", "project checks must not need containers") {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if !assert.NoError(
			t,
			json.MarshalWrite(
				w,
				dockertypesimage.InspectResponse{
					ID: "sha256:" + strings.Repeat(
						"a",
						64,
					),
					RepoDigests: []string{"alpine@sha256:" + strings.Repeat(
						"a",
						64,
					)},
				},
			),
		) {
			return
		}
	}))
	defer server.Close()
	puller := &fakeImagePullerInternal{}
	engine, err := updater.New(updater.Config{
		DockerClientProvider: fakeDockerClientProviderInternal{client: newTestDockerClientInternal(
			t,
			server,
		)},
		RegistryTagLister:      projectCheckRegistryInternal{},
		RegistryDigestResolver: projectCheckRegistryInternal{},
		ImagePuller:            puller,
	})
	require.NoError(t, err)
	service := &UpdaterService{engine: engine}
	for _, metadata := range []bool{false, true} {
		for _, tt := range []struct {
			name, constraint, target string
			available                bool
		}{
			{"first", "=3.20.1", "3.20.1", true},
			{"second", "=3.20.2", "3.20.2", true},
			{"control", "=3.20.0", "3.20.0", false},
		} {
			localConfig := types.ServiceConfig{Name: tt.name, Image: "alpine:3.20.0", Labels: map[string]string{labels.LabelUpdateConstraint: tt.constraint}}
			if metadata {
				model, loadErr := projects.LoadComposeProjectFromContent(
					t.Context(),
					projecttypes.ComposeContentOptions{
						ProjectName: "metadata",
						WorkingDir:  t.TempDir(),
						ComposeContent: fmt.Sprintf(
							"x-arcane:\n  updater:\n    strategy: auto\nservices:\n  %s:\n    image: alpine:3.20.0\n    x-arcane:\n     "+
								" updater:\n        constraint: %q\n",
							tt.name,
							tt.constraint,
						),
					},
				)
				require.NoError(t, loadErr)
				localConfig = model.Services[tt.name]
			}
			record := service.checkProjectServiceInternal(t.Context(), "project", localConfig)
			require.Nil(t, record.LastError)
			require.Equal(t, tt.available, record.HasUpdate)
			require.Equal(t, tt.target, *record.LatestVersion)
			require.Equal(t, "project", record.ProjectID)
			require.Empty(t, record.ContainerID)
		}
	}
	require.Empty(t, puller.pulled)
}

func TestSingleUpdatePersistsSuccessfulActivity(t *testing.T) {
	db := setupProjectTestDBInternal(t)
	require.NoError(t, db.AutoMigrate(append(activity.Models(), &AutoUpdateRecord{})...))
	pool, err := db.DB.DB()
	require.NoError(t, err)
	pool.SetMaxOpenConns(1)
	immutableRef := "registry.example.com/app@sha256:" + strings.Repeat("a", 64)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/containers/json"):
			if !assert.NoError(t, json.MarshalWrite(w, []container.Summary{{ID: "app", Names: []string{"/app"}, Image: immutableRef, State: "running"}})) {
				return
			}
		case strings.HasSuffix(r.URL.Path, "/containers/app/json"):
			if !assert.NoError(
				t,
				json.MarshalWrite(
					w,
					container.InspectResponse{
						ID:     "app",
						Name:   "/app",
						Image:  "sha256:baseline",
						Config: &container.Config{Image: immutableRef},
						State: &container.State{
							StartedAt: "baseline",
							Running:   true,
						},
					},
				),
			) {
				return
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	activityService := activity.NewActivityService(db, nil)
	svc, err := NewUpdaterService(
		db,
		nil,
		(&docker.DockerClientService{}).WithClient(newTestDockerClientInternal(
			t,
			server,
		)),
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		activityService,
		nil,
		nil,
		nil,
		nil,
	)
	require.NoError(t, err)
	runtime := francistest.New(t)
	require.NoError(t, svc.RegisterActors(runtime))
	francistest.Start(t, runtime)
	require.NoError(t, svc.Start(t.Context()))
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(t.Context()), time.Second)
		defer cancel()
		require.NoError(t, svc.Stop(ctx))
	})
	accepted, err := svc.AcceptSingleContainerUpdate(t.Context(), "app")
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		var state arcaneupdater.SingleUpdateState
		if runtime.Service().GetState(t.Context(), execution.StateType, accepted.ID, &state) != nil || state.Status != "completed" {
			return false
		}
		detail, getActivityDetailErr := activityService.GetActivityDetail(t.Context(), "0", accepted.ID, 1)
		return getActivityDetailErr == nil && detail.Activity.Status == activitytypes.StatusSuccess
	}, 3*time.Second, 10*time.Millisecond)
	detail, err := activityService.GetActivityDetail(t.Context(), "0", accepted.ID, 1)
	require.NoError(t, err)
	require.Equal(t, activitytypes.StatusSuccess, detail.Activity.Status)
}
