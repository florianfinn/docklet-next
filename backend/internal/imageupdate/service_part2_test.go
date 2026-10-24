// Source: getarcaneapp/arcane/backend/internal/imageupdate/service_test.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed APNS mobile push; SQLite temp-file persistence fixture.

package imageupdate

import (
	"context"

	"errors"
	"fmt"

	"net/url"
	"strings"

	"testing"
	"time"

	activitytypes "github.com/florianfinn/docklet-next/types/activity"

	"github.com/florianfinn/docklet-next/types/imageupdate"

	"github.com/moby/moby/client"
	"github.com/opencontainers/go-digest"
	"github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/samber/mo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/florianfinn/docklet-next/backend/internal/activity"

	"github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/internal/event"

	"github.com/florianfinn/docklet-next/backend/internal/registry"
)

func TestImageUpdateService_CheckImageUpdate_UsesRegistryFallback(t *testing.T) {
	db := setupImageUpdateTestDB(t)
	localDigest := digest.FromString("localdigest").String()
	remoteDigest := digest.FromString("remotedigest").String()

	server := newImageUpdateFallbackServer(t, "team/app:1.2.3", localDigest, remoteDigest)
	defer server.Close()

	serverURL, err := url.Parse(server.URL)
	require.NoError(t, err)
	imageRef := serverURL.Host + "/team/app:1.2.3"

	registryService := registry.NewContainerRegistryService(db, func(context.Context) (registry.RegistryDaemonClient, error) {
		return &fakeRegistryDaemonClient{
			distributionInspectFn: func(ctx context.Context, imageRef string, options client.DistributionInspectOptions) (client.DistributionInspectResult, error) {
				return client.DistributionInspectResult{}, errors.New("error response from daemon: Not Found")
			},
		}, nil
	}, nil, nil, server.Client())

	dockerService := &docker.DockerClientService{Client: newImageUpdateTestDockerClientInternal(t, server)}
	eventService := event.NewEventService(db, nil, nil)
	svc := NewImageUpdateService(db, nil, registryService, dockerService, eventService, nil, nil)

	result, err := svc.CheckImageUpdate(t.Context(), imageRef)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.HasUpdate)
	assert.Equal(t, "digest", result.UpdateType)
	assert.Equal(t, localDigest, result.CurrentDigest)
	assert.Equal(t, remoteDigest, result.LatestDigest)
	assert.Equal(t, "anonymous", result.AuthMethod)
	assert.Equal(t, serverURL.Host, result.AuthRegistry)

	var saved ImageUpdateRecord
	require.NoError(t, db.WithContext(t.Context()).Where("id = ?", "sha256:local-image-id").First(&saved).Error)
	assert.Equal(t, remoteDigest, mo.PointerToOption(saved.LatestDigest).OrEmpty())
}

func TestImageUpdateService_CheckMultipleImages_UsesRegistryFallback(t *testing.T) {
	db := setupImageUpdateTestDB(t)
	localDigest := digest.FromString("batchlocal").String()
	remoteDigest := digest.FromString("batchremote").String()

	server := newImageUpdateFallbackServer(t, "team/app:1.2.3", localDigest, remoteDigest)
	defer server.Close()

	serverURL, err := url.Parse(server.URL)
	require.NoError(t, err)
	imageRef := serverURL.Host + "/team/app:1.2.3"

	registryService := registry.NewContainerRegistryService(db, func(context.Context) (registry.RegistryDaemonClient, error) {
		return &fakeRegistryDaemonClient{
			distributionInspectFn: func(ctx context.Context, imageRef string, options client.DistributionInspectOptions) (client.DistributionInspectResult, error) {
				return client.DistributionInspectResult{}, errors.New("error response from daemon: <html><body><h1>403 Forbidden</h1> Request forbidden by administrative r" +
					"ules. </body></html>")
			},
		}, nil
	}, nil, nil, server.Client())

	dockerService := &docker.DockerClientService{Client: newImageUpdateTestDockerClientInternal(t, server)}
	eventService := event.NewEventService(db, nil, nil)
	svc := NewImageUpdateService(db, nil, registryService, dockerService, eventService, nil, nil)

	results, err := svc.CheckMultipleImages(t.Context(), []string{imageRef}, nil)
	require.NoError(t, err)
	require.Contains(t, results, imageRef)

	result := results[imageRef]
	require.NotNil(t, result)
	assert.True(t, result.HasUpdate)
	assert.Equal(t, localDigest, result.CurrentDigest)
	assert.Equal(t, remoteDigest, result.LatestDigest)
	assert.Equal(t, "anonymous", result.AuthMethod)
	assert.Equal(t, serverURL.Host, result.AuthRegistry)

	var saved ImageUpdateRecord
	require.NoError(t, db.WithContext(t.Context()).Where("id = ?", "sha256:local-image-id").First(&saved).Error)
	assert.Equal(t, remoteDigest, mo.PointerToOption(saved.LatestDigest).OrEmpty())
}

func TestImageUpdateService_CheckMultipleImagesCompletesActivityWhenRequestContextCanceledInternal(t *testing.T) {
	db := setupImageUpdateTestDB(t)
	require.NoError(t, db.AutoMigrate(activity.Models()...))

	activityService := activity.NewActivityService(db, nil)
	svc := NewImageUpdateService(db, nil, nil, nil, nil, nil, activityService)

	for range 5 {
		require.NoError(t, svc.registryLimiter.Acquire(t.Context(), "docker.io"))
	}
	defer func() {
		for range 5 {
			svc.registryLimiter.Release("docker.io")
		}
	}()

	ctx, cancel := context.WithCancel(t.Context())
	errCh := make(chan error, 1)
	go func() {
		_, err := svc.CheckMultipleImages(ctx, []string{"nginx:latest"}, nil)
		errCh <- err
	}()

	var localActivity activity.Activity
	require.Eventually(t, func() bool {
		return db.Where("type = ?", activitytypes.TypeImageUpdateCheck).First(&localActivity).Error == nil
	}, time.Second, 10*time.Millisecond)

	cancel()

	select {
	case err := <-errCh:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		require.FailNow(t, "timed out waiting for image update check to return")
	}

	require.Eventually(t, func() bool {
		if err := db.First(&localActivity, "id = ?", localActivity.ID).Error; err != nil {
			return false
		}
		return localActivity.Status == activitytypes.StatusFailed
	}, time.Second, 10*time.Millisecond)
	assert.Equal(t, "Image update check complete", localActivity.Step)
	assert.Contains(t, localActivity.LatestMessage, "Image update check failed")
}

func TestImageUpdateService_CheckMultipleImagesTimesOutStalledRegistryCheckInternal(t *testing.T) {
	db := setupImageUpdateTestDB(t)
	require.NoError(t, db.AutoMigrate(activity.Models()...))

	settingsService := newImageUpdateTestSettingsServiceInternal(t, "1", "30")
	activityService := activity.NewActivityService(db, nil)
	dockerServer := newImageUpdateRegistryOnlyServer(t, "team/app:1.2.3", digest.FromString("unused").String())
	defer dockerServer.Close()
	registryService := registry.NewContainerRegistryService(db, func(context.Context) (registry.RegistryDaemonClient, error) {
		return &fakeRegistryDaemonClient{
			distributionInspectFn: func(ctx context.Context, imageRef string, options client.DistributionInspectOptions) (client.DistributionInspectResult, error) {
				<-ctx.Done()
				return client.DistributionInspectResult{}, ctx.Err()
			},
		}, nil
	}, nil, nil)

	parentCtx, cancel := context.WithTimeout(t.Context(), 2500*time.Millisecond)
	defer cancel()

	svc := NewImageUpdateService(
		db,
		settingsService,
		registryService,
		&docker.DockerClientService{Client: newImageUpdateTestDockerClientInternal(
			t,
			dockerServer,
		)},
		nil,
		nil,
		activityService,
	)

	start := time.Now()
	results, err := svc.CheckMultipleImages(parentCtx, []string{"registry.example.com/team/app:1.2.3"}, nil)
	elapsed := time.Since(start)

	require.NoError(t, err)
	require.Less(t, elapsed, 2*time.Second)
	require.Contains(t, results, "registry.example.com/team/app:1.2.3")
	require.NotNil(t, results["registry.example.com/team/app:1.2.3"])
	require.Equal(t, "update.check-unavailable", results["registry.example.com/team/app:1.2.3"].Code)
	require.NotContains(t, results["registry.example.com/team/app:1.2.3"].Error, context.DeadlineExceeded.Error())

	var localActivity activity.Activity
	require.NoError(t, db.Where("type = ?", activitytypes.TypeImageUpdateCheck).First(&localActivity).Error)
	require.Equal(t, activitytypes.StatusFailed, localActivity.Status)
	require.NotNil(t, localActivity.EndedAt)
	require.NotNil(t, localActivity.DurationMs)
	require.Equal(t, "Image update check complete", localActivity.Step)
	require.Contains(t, localActivity.LatestMessage, "0 checked, 1 errors")
}

func TestImageUpdateService_CheckMultipleImagesPanicMarksActivityFailedInternal(t *testing.T) {
	db := setupImageUpdateTestDB(t)
	require.NoError(t, db.AutoMigrate(activity.Models()...))

	settingsService := newImageUpdateTestSettingsServiceInternal(t, "30", "30")
	activityService := activity.NewActivityService(db, nil)
	dockerServer := newImageUpdateRegistryOnlyServer(t, "team/app:1.2.3", digest.FromString("unused").String())
	defer dockerServer.Close()
	registryService := registry.NewContainerRegistryService(db, func(context.Context) (registry.RegistryDaemonClient, error) {
		return &fakeRegistryDaemonClient{
			distributionInspectFn: func(ctx context.Context, imageRef string, options client.DistributionInspectOptions) (client.DistributionInspectResult, error) {
				panic("registry check exploded")
			},
		}, nil
	}, nil, nil)

	svc := NewImageUpdateService(
		db,
		settingsService,
		registryService,
		&docker.DockerClientService{Client: newImageUpdateTestDockerClientInternal(
			t,
			dockerServer,
		)},
		nil,
		nil,
		activityService,
	)

	_, err := svc.CheckMultipleImages(t.Context(), []string{"registry.example.com/team/app:1.2.3"}, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "image update check panicked")
	require.Contains(t, err.Error(), "registry check exploded")

	var localActivity activity.Activity
	require.NoError(t, db.Where("type = ?", activitytypes.TypeImageUpdateCheck).First(&localActivity).Error)
	require.Equal(t, activitytypes.StatusFailed, localActivity.Status)
	require.NotNil(t, localActivity.EndedAt)
	require.Contains(t, localActivity.LatestMessage, "Image update check failed")
}

func TestImageUpdateService_GetAllImageRefsUsesDockerAPITimeoutInternal(t *testing.T) {
	db := setupImageUpdateTestDB(t)
	settingsService := newImageUpdateTestSettingsServiceInternal(t, "30", "1")
	server := newBlockedDockerAPIServerInternal(t, "/images/json")
	svc := NewImageUpdateService(db, settingsService, nil, &docker.DockerClientService{Client: newImageUpdateTestDockerClientInternal(t, server)}, nil, nil, nil)

	parentCtx, cancel := context.WithTimeout(t.Context(), 2500*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := svc.getAllImageRefsInternal(parentCtx, 0)
	elapsed := time.Since(start)

	require.Error(t, err)
	require.Less(t, elapsed, 2*time.Second)
	require.Contains(t, err.Error(), context.DeadlineExceeded.Error())
}

func TestImageUpdateService_InspectLocalImageSnapshotUsesDockerAPITimeoutInternal(t *testing.T) {
	db := setupImageUpdateTestDB(t)
	settingsService := newImageUpdateTestSettingsServiceInternal(t, "30", "1")
	server := newBlockedDockerAPIServerInternal(t, "/images/")
	svc := NewImageUpdateService(db, settingsService, nil, &docker.DockerClientService{Client: newImageUpdateTestDockerClientInternal(t, server)}, nil, nil, nil)

	parentCtx, cancel := context.WithTimeout(t.Context(), 2500*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := svc.inspectLocalImageSnapshotInternal(parentCtx, "registry.example.com/team/app:1.2.3", nil)
	elapsed := time.Since(start)

	require.Error(t, err)
	require.Less(t, elapsed, 2*time.Second)
	require.Contains(t, err.Error(), context.DeadlineExceeded.Error())
}

func TestImageUpdateService_CheckMultipleImages_UsesDockerHubCredentialsOnFirstAttempt(t *testing.T) {
	db := setupImageUpdateRegistryTestDBInternal(t)
	require.NoError(t, db.AutoMigrate(&ImageUpdateRecord{}, &event.Event{}, &testProjectRow{}))
	createImageUpdateTestPullRegistryInternal(t, db, "https://index.docker.io/v1/", "docker-user", "docker-token")

	localDigest := digest.FromString("batchlocal-rate-limit").String()
	remoteDigest := digest.FromString("batchremote-rate-limit").String()

	server := newImageUpdateFallbackServer(t, "library/registry:3", localDigest, remoteDigest)
	defer server.Close()

	var calls int
	registryService := registry.NewContainerRegistryService(db, func(context.Context) (registry.RegistryDaemonClient, error) {
		return &fakeRegistryDaemonClient{
			distributionInspectFn: func(ctx context.Context, imageRef string, options client.DistributionInspectOptions) (client.DistributionInspectResult, error) {
				calls++
				assert.Equal(t, "docker.io/library/registry:3", imageRef)

				authCfg := decodeImageUpdateRegistryAuthInternal(t, options.EncodedRegistryAuth)
				assert.Equal(t, "docker-user", authCfg.Username)
				assert.Equal(t, "docker-token", authCfg.Password)
				assert.Equal(t, "https://index.docker.io/v1/", authCfg.ServerAddress)

				return client.DistributionInspectResult{
					Descriptor: v1.Descriptor{
						Digest: digest.Digest(remoteDigest),
					},
				}, nil
			},
		}, nil
	}, nil, nil)

	dockerService := &docker.DockerClientService{Client: newImageUpdateTestDockerClientInternal(t, server)}
	eventService := event.NewEventService(db, nil, nil)
	svc := NewImageUpdateService(db, nil, registryService, dockerService, eventService, nil, nil)

	results, err := svc.CheckMultipleImages(t.Context(), []string{"docker.io/library/registry:3"}, nil)
	require.NoError(t, err)
	require.Contains(t, results, "docker.io/library/registry:3")

	result := results["docker.io/library/registry:3"]
	require.NotNil(t, result)
	assert.True(t, result.HasUpdate)
	assert.Equal(t, 1, calls)
	assert.Equal(t, localDigest, result.CurrentDigest)
	assert.Equal(t, remoteDigest, result.LatestDigest)
	assert.Equal(t, "credential", result.AuthMethod)
	assert.Equal(t, "docker-user", result.AuthUsername)
	assert.Equal(t, "docker.io", result.AuthRegistry)
	assert.True(t, result.UsedCredential)
}

func TestImageUpdateService_CheckMultipleImages_ReportsNotPulledWhenLocalImageMissing(t *testing.T) {
	db := setupImageUpdateTestDB(t)
	remoteDigest := digest.FromString("registry-only-remote").String()

	server := newImageUpdateRegistryOnlyServer(t, "library/nginx:alpine", remoteDigest)
	defer server.Close()

	serverURL, err := url.Parse(server.URL)
	require.NoError(t, err)
	imageRef := serverURL.Host + "/library/nginx:alpine"

	registryService := registry.NewContainerRegistryService(db, func(context.Context) (registry.RegistryDaemonClient, error) {
		return &fakeRegistryDaemonClient{
			distributionInspectFn: func(ctx context.Context, imageRef string, options client.DistributionInspectOptions) (client.DistributionInspectResult, error) {
				return client.DistributionInspectResult{}, errors.New("error response from daemon: Not Found")
			},
		}, nil
	}, nil, nil, server.Client())

	dockerService := &docker.DockerClientService{Client: newImageUpdateTestDockerClientInternal(t, server)}
	eventService := event.NewEventService(db, nil, nil)
	svc := NewImageUpdateService(db, nil, registryService, dockerService, eventService, nil, nil)

	results, err := svc.CheckMultipleImages(t.Context(), []string{imageRef}, nil)
	require.NoError(t, err)
	require.Contains(t, results, imageRef)
	require.NotNil(t, results[imageRef])
	assert.Empty(t, results[imageRef].Error)
	assert.False(t, results[imageRef].HasUpdate)
	assert.Equal(t, UpdateTypeNotPulled, results[imageRef].UpdateType)
	assert.Equal(t, remoteDigest, results[imageRef].LatestDigest)

	var saved ImageUpdateRecord
	repository := serverURL.Host + "/library/nginx"
	require.NoError(t, db.WithContext(t.Context()).Where("id = ?", fmt.Sprintf("ref::%s@alpine", strings.ToLower(strings.TrimSpace(repository)))).First(&saved).Error)
	assert.Equal(t, repository, saved.Repository)
	assert.Equal(t, "alpine", saved.Tag)
	assert.False(t, saved.HasUpdate)
	assert.Equal(t, UpdateTypeNotPulled, saved.UpdateType)
	assert.Nil(t, saved.LastError)
}

func TestImageUpdateService_SaveUpdateResultWithSnapshotInternal_PersistsRegistryOnlySuccessWithSyntheticID(t *testing.T) {
	db := setupImageUpdateTestDB(t)
	remoteDigest := digest.FromString("registry-only-success-remote").String()

	server := newImageUpdateRegistryOnlyServer(t, "library/nginx:alpine", remoteDigest)
	defer server.Close()

	serverURL, err := url.Parse(server.URL)
	require.NoError(t, err)
	imageRef := serverURL.Host + "/library/nginx:alpine"

	svc := NewImageUpdateService(db, nil, nil, &docker.DockerClientService{Client: newImageUpdateTestDockerClientInternal(t, server)}, nil, nil, nil)
	result := &imageupdate.Response{
		HasUpdate:      true,
		UpdateType:     "digest",
		CurrentVersion: "alpine",
		LatestVersion:  "alpine",
		CurrentDigest:  digest.FromString("registry-only-success-local").String(),
		LatestDigest:   remoteDigest,
		CheckTime:      time.Now(),
		ResponseTimeMs: 25,
	}

	require.NoError(t, svc.saveUpdateResultWithSnapshotInternal(t.Context(), imageRef, result, nil))

	var saved ImageUpdateRecord
	repository := serverURL.Host + "/library/nginx"
	require.NoError(t, db.WithContext(t.Context()).Where("id = ?", fmt.Sprintf("ref::%s@alpine", strings.ToLower(strings.TrimSpace(repository)))).First(&saved).Error)
	assert.Equal(t, repository, saved.Repository)
	assert.Equal(t, "alpine", saved.Tag)
	assert.True(t, saved.HasUpdate)
	assert.Equal(t, remoteDigest, mo.PointerToOption(saved.LatestDigest).OrEmpty())
	assert.Nil(t, saved.LastError)
}

func TestImageUpdateService_MarkImageRefUpToDateAfterPull_ClearsMatchingRecordsAndPersistsCurrentImage(t *testing.T) {
	db := setupImageUpdateTestDB(t)
	localDigest := digest.FromString("mark-up-to-date-local").String()
	remoteDigest := digest.FromString("mark-up-to-date-remote").String()

	server := newImageUpdateFallbackServer(t, "team/app:1.2.3", localDigest, remoteDigest)
	defer server.Close()

	serverURL, err := url.Parse(server.URL)
	require.NoError(t, err)
	imageRef := serverURL.Host + "/team/app:1.2.3"
	repository := serverURL.Host + "/team/app"

	now := time.Now().UTC().Add(-time.Hour)

	// Real sha256 records for OTHER containers running the old image — must not be cleared.
	require.NoError(t, db.Create(&ImageUpdateRecord{
		ID:             "sha256:old-full",
		Repository:     repository,
		Tag:            "1.2.3",
		HasUpdate:      true,
		UpdateType:     UpdateTypeDigest,
		CurrentVersion: "1.2.3",
		CheckTime:      now,
	}).Error)
	require.NoError(t, db.Create(&ImageUpdateRecord{
		ID:             "sha256:old-short",
		Repository:     "team/app",
		Tag:            "1.2.3",
		HasUpdate:      true,
		UpdateType:     UpdateTypeDigest,
		CurrentVersion: "1.2.3",
		CheckTime:      now.Add(time.Minute),
	}).Error)

	// Synthetic ref:: record — must be cleared when new image is pulled.
	syntheticID := "ref::" + repository + "@1.2.3"
	require.NoError(t, db.Create(&ImageUpdateRecord{
		ID:             syntheticID,
		Repository:     repository,
		Tag:            "1.2.3",
		HasUpdate:      true,
		UpdateType:     UpdateTypeDigest,
		CurrentVersion: "1.2.3",
		CheckTime:      now,
	}).Error)

	// Project preview rows for the same source reference: a digest preview is
	// satisfied by the pull, a pending tag move is not.
	oldDigest := "sha256:old-project"
	require.NoError(t, db.Create(&ImageUpdateRecord{
		ID:             "project::proj::web",
		ProjectID:      "proj",
		ServiceName:    "web",
		Repository:     repository,
		Tag:            "1.2.3",
		HasUpdate:      true,
		UpdateType:     UpdateTypeDigest,
		CurrentVersion: "1.2.3",
		LatestVersion:  new("1.2.3"),
		CurrentDigest:  &oldDigest,
		LatestDigest:   &localDigest,
		CheckTime:      now,
	}).Error)
	require.NoError(t, db.Create(&ImageUpdateRecord{
		ID:             "project::proj::api",
		ProjectID:      "proj",
		ServiceName:    "api",
		Repository:     repository,
		Tag:            "1.2.3",
		HasUpdate:      true,
		UpdateType:     "tag",
		CurrentVersion: "1.2.3",
		LatestVersion:  new("1.3.0"),
		CheckTime:      now,
	}).Error)
	newerDigest := digest.FromString("mark-up-to-date-newer").String()
	require.NoError(t, db.Create(&ImageUpdateRecord{
		ID:             "project::proj::worker",
		ProjectID:      "proj",
		ServiceName:    "worker",
		Repository:     repository,
		Tag:            "1.2.3",
		HasUpdate:      true,
		UpdateType:     UpdateTypeDigest,
		CurrentVersion: "1.2.3",
		CurrentDigest:  &oldDigest,
		LatestDigest:   &newerDigest,
		CheckTime:      now,
	}).Error)
	require.NoError(t, db.Create(&ImageUpdateRecord{
		ID:          "project::proj::broken",
		ProjectID:   "proj",
		ServiceName: "broken",
		Repository:  repository,
		Tag:         "1.2.3",
		LastError:   new("registry unreachable"),
		CheckTime:   now,
	}).Error)

	svc := NewImageUpdateService(db, nil, nil, &docker.DockerClientService{Client: newImageUpdateTestDockerClientInternal(t, server)}, nil, nil, nil)

	require.NoError(t, svc.MarkImageRefUpToDateAfterPull(t.Context(), imageRef))

	var projectDigestRecord ImageUpdateRecord
	require.NoError(t, db.WithContext(t.Context()).Where("id = ?", "project::proj::web").First(&projectDigestRecord).Error)
	assert.False(t, projectDigestRecord.HasUpdate, "project digest preview must be refreshed after pull")
	assert.Equal(t, localDigest, mo.PointerToOption(projectDigestRecord.CurrentDigest).OrEmpty())
	assert.Equal(t, localDigest, mo.PointerToOption(projectDigestRecord.LatestDigest).OrEmpty())
	assert.True(t, projectDigestRecord.CheckTime.After(now))

	var projectTagRecord ImageUpdateRecord
	require.NoError(t, db.WithContext(t.Context()).Where("id = ?", "project::proj::api").First(&projectTagRecord).Error)
	assert.True(t, projectTagRecord.HasUpdate, "project tag preview is not satisfied by pulling the same tag")
	assert.Equal(t, "1.3.0", mo.PointerToOption(projectTagRecord.LatestVersion).OrEmpty())

	var projectNewerRecord ImageUpdateRecord
	require.NoError(t, db.WithContext(t.Context()).Where("id = ?", "project::proj::worker").First(&projectNewerRecord).Error)
	assert.True(t, projectNewerRecord.HasUpdate, "a preview targeting a digest other than the pulled one is kept")
	assert.Equal(t, newerDigest, mo.PointerToOption(projectNewerRecord.LatestDigest).OrEmpty())

	var projectFailedRecord ImageUpdateRecord
	require.NoError(t, db.WithContext(t.Context()).Where("id = ?", "project::proj::broken").First(&projectFailedRecord).Error)
	assert.Equal(t, "registry unreachable", mo.PointerToOption(projectFailedRecord.LastError).OrEmpty(), "a failed preview check is not cleared by a pull")

	// Sha256 records for old images that other containers are still running must stay HasUpdate=true.
	var fullRecord ImageUpdateRecord
	require.NoError(t, db.WithContext(t.Context()).Where("id = ?", "sha256:old-full").First(&fullRecord).Error)
	assert.True(t, fullRecord.HasUpdate, "sha256 record for old image still in use must not be cleared")

	var shortRecord ImageUpdateRecord
	require.NoError(t, db.WithContext(t.Context()).Where("id = ?", "sha256:old-short").First(&shortRecord).Error)
	assert.True(t, shortRecord.HasUpdate, "sha256 record for old image still in use must not be cleared")

	// Synthetic ref:: record must be cleared since a fresh image was pulled.
	var synthRecord ImageUpdateRecord
	require.NoError(t, db.WithContext(t.Context()).Where("id = ?", syntheticID).First(&synthRecord).Error)
	assert.False(t, synthRecord.HasUpdate, "synthetic ref:: record must be cleared after pull")

	// The newly pulled image record must be saved as up-to-date.
	var currentRecord ImageUpdateRecord
	require.NoError(t, db.WithContext(t.Context()).Where("id = ?", "sha256:local-image-id").First(&currentRecord).Error)
	assert.False(t, currentRecord.HasUpdate)
	assert.Equal(t, repository, currentRecord.Repository)
	assert.Equal(t, "1.2.3", currentRecord.Tag)
	assert.Equal(t, localDigest, mo.PointerToOption(currentRecord.CurrentDigest).OrEmpty())
	assert.Equal(t, localDigest, mo.PointerToOption(currentRecord.LatestDigest).OrEmpty())
	assert.Equal(t, "1.2.3", currentRecord.CurrentVersion)
	require.NotNil(t, currentRecord.LatestVersion)
	assert.Equal(t, "1.2.3", *currentRecord.LatestVersion)
}

// TestNotificationSentLogic tests the notification_sent flag behavior
func TestImageUpdateService_NotificationSentLogic(t *testing.T) {
	db := setupImageUpdateTestDB(t)

	imageID := "sha256:test123"
	repo := "docker.io/library/nginx"
	tag := "latest"

	t.Run("new record starts with notification_sent=false", func(t *testing.T) {
		result := &imageupdate.Response{
			HasUpdate:      true,
			UpdateType:     "digest",
			CurrentVersion: "1.0",
			LatestVersion:  "2.0",
			CurrentDigest:  "sha256:old",
			LatestDigest:   "sha256:new",
			CheckTime:      time.Now(),
			ResponseTimeMs: 100,
		}

		updateRecord := buildImageUpdateRecord(imageID, repo, tag, result)

		// New record should have NotificationSent = false
		assert.False(t, updateRecord.NotificationSent)

		err := db.Create(updateRecord).Error
		require.NoError(t, err)

		// Verify it was saved correctly
		var saved ImageUpdateRecord
		err = db.First(&saved, "id = ?", imageID).Error
		require.NoError(t, err)
		assert.False(t, saved.NotificationSent)
	})
}

// TestNotificationSentReset tests that notification_sent resets when update state changes
func TestImageUpdateService_NotificationSentReset(t *testing.T) {
	db := setupImageUpdateTestDB(t)

	imageID := "sha256:test456"
	repo := "docker.io/library/redis"
	tag := "alpine"

	tests := []struct {
		name             string
		existingRecord   *ImageUpdateRecord
		newResult        *imageupdate.Response
		expectNotifReset bool
		reason           string
	}{
		{
			name: "digest changed - should reset",
			existingRecord: &ImageUpdateRecord{
				ID:               imageID,
				Repository:       repo,
				Tag:              tag,
				HasUpdate:        true,
				UpdateType:       "digest",
				CurrentVersion:   "7.0",
				LatestDigest:     new("sha256:old"),
				NotificationSent: true,
			},
			newResult: &imageupdate.Response{
				HasUpdate:      true,
				UpdateType:     "digest",
				CurrentVersion: "7.0",
				LatestDigest:   "sha256:new",
				CheckTime:      time.Now(),
				ResponseTimeMs: 50,
			},
			expectNotifReset: true,
			reason:           "digest changed from old to new",
		},
		{
			name: "version changed - should reset",
			existingRecord: &ImageUpdateRecord{
				ID:               imageID,
				Repository:       repo,
				Tag:              tag,
				HasUpdate:        true,
				UpdateType:       "tag",
				CurrentVersion:   "7.0",
				LatestVersion:    new("7.0.1"),
				NotificationSent: true,
			},
			newResult: &imageupdate.Response{
				HasUpdate:      true,
				UpdateType:     "tag",
				CurrentVersion: "7.0",
				LatestVersion:  "7.0.2",
				CheckTime:      time.Now(),
				ResponseTimeMs: 50,
			},
			expectNotifReset: true,
			reason:           "version changed from 7.0.1 to 7.0.2",
		},
		{
			name: "update state changed - should reset",
			existingRecord: &ImageUpdateRecord{
				ID:               imageID,
				Repository:       repo,
				Tag:              tag,
				HasUpdate:        false,
				UpdateType:       "digest",
				CurrentVersion:   "7.0",
				NotificationSent: true,
			},
			newResult: &imageupdate.Response{
				HasUpdate:      true,
				UpdateType:     "digest",
				CurrentVersion: "7.0",
				CheckTime:      time.Now(),
				ResponseTimeMs: 50,
			},
			expectNotifReset: true,
			reason:           "HasUpdate changed from false to true",
		},
		{
			name: "nothing changed - should keep flag",
			existingRecord: &ImageUpdateRecord{
				ID:               imageID,
				Repository:       repo,
				Tag:              tag,
				HasUpdate:        true,
				UpdateType:       "digest",
				CurrentVersion:   "7.0",
				LatestDigest:     new("sha256:same"),
				LatestVersion:    new("7.0.1"),
				NotificationSent: true,
			},
			newResult: &imageupdate.Response{
				HasUpdate:      true,
				UpdateType:     "digest",
				CurrentVersion: "7.0",
				LatestDigest:   "sha256:same",
				LatestVersion:  "7.0.1",
				CheckTime:      time.Now(),
				ResponseTimeMs: 50,
			},
			expectNotifReset: false,
			reason:           "nothing changed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clean up any existing record
			db.Exec("DELETE FROM image_updates WHERE id = ?", imageID)

			// Insert existing record
			err := db.Create(tt.existingRecord).Error
			require.NoError(t, err)

			// Verify it was marked as notified
			var check ImageUpdateRecord
			err = db.First(&check, "id = ?", imageID).Error
			require.NoError(t, err)
			assert.True(t, check.NotificationSent, "existing record should be marked as notified")

			// Simulate comparison logic from saveUpdateResultByIDInternal
			updateRecord := buildImageUpdateRecord(imageID, repo, tag, tt.newResult)

			var existingRecord ImageUpdateRecord
			err = db.Where("id = ?", imageID).First(&existingRecord).Error
			require.NoError(t, err)

			// This is the logic we're testing - comparing string values not pointers
			stateChanged := existingRecord.HasUpdate != updateRecord.HasUpdate
			digestChanged := mo.PointerToOption(existingRecord.LatestDigest).OrEmpty() != mo.PointerToOption(updateRecord.LatestDigest).OrEmpty()
			versionChanged := mo.PointerToOption(existingRecord.LatestVersion).OrEmpty() != mo.PointerToOption(updateRecord.LatestVersion).OrEmpty()

			updateRecord.NotificationSent = !stateChanged && (!updateRecord.HasUpdate || (!digestChanged && !versionChanged)) && existingRecord.NotificationSent

			// Save the updated record
			err = db.Save(updateRecord).Error
			require.NoError(t, err)

			// Verify the result
			var updated ImageUpdateRecord
			err = db.First(&updated, "id = ?", imageID).Error
			require.NoError(t, err)

			if tt.expectNotifReset {
				assert.False(t, updated.NotificationSent, "notification_sent should be reset because: %s", tt.reason)
			} else {
				assert.True(t, updated.NotificationSent, "notification_sent should be preserved because: %s", tt.reason)
			}
		})
	}
}

func TestImageUpdateService_RateLimitErrorPreservesPreviousResult(t *testing.T) {
	db := setupImageUpdateTestDB(t)

	imageID := "sha256:ratelimit1"
	repo := "docker.io/library/nginx"
	tag := "latest"
	checkTime := time.Now().Add(-1 * time.Hour).UTC().Truncate(time.Second)

	tests := []struct {
		name            string
		resultError     string
		expectPreserved bool
	}{
		{
			name:            "rate limit error preserves previous good record",
			resultError:     "registry manifest inspect failed for docker.io/library/nginx:latest: manifest request failed with status: 429",
			expectPreserved: true,
		},
		{
			name:            "non-rate-limit error overwrites previous good record",
			resultError:     "registry manifest inspect failed for docker.io/library/nginx:latest: manifest unknown",
			expectPreserved: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db.Exec("DELETE FROM image_updates WHERE id = ?", imageID)

			existing := &ImageUpdateRecord{
				ID:             imageID,
				Repository:     repo,
				Tag:            tag,
				HasUpdate:      false,
				CurrentVersion: tag,
				CurrentDigest:  new("sha256:current"),
				LatestDigest:   new("sha256:current"),
				CheckTime:      checkTime,
			}
			require.NoError(t, db.Create(existing).Error)

			result := &imageupdate.Response{
				Error:     tt.resultError,
				CheckTime: time.Now(),
			}
			require.NoError(t, savePreparedUpdateResultWithTxInternal(db.DB, imageID, repo, tag, result))

			var saved ImageUpdateRecord
			require.NoError(t, db.First(&saved, "id = ?", imageID).Error)

			if tt.expectPreserved {
				assert.Nil(t, saved.LastError, "previous good record should be preserved")
				assert.Equal(t, "sha256:current", mo.PointerToOption(saved.LatestDigest).OrEmpty())
				assert.Equal(t, checkTime, saved.CheckTime.UTC())
			} else {
				assert.Equal(t, tt.resultError, mo.PointerToOption(saved.LastError).OrEmpty())
				assert.Nil(t, saved.LatestDigest)
			}
		})
	}
}

// TestGetUnnotifiedUpdates tests retrieving updates that haven't been notified
func TestImageUpdateService_GetUnnotifiedUpdates(t *testing.T) {
	ctx := t.Context()
	db := setupImageUpdateTestDB(t)
	svc := &ImageUpdateService{db: db}

	// Create test records
	records := []ImageUpdateRecord{
		{
			ID:               "sha256:img1",
			Repository:       "nginx",
			Tag:              "latest",
			HasUpdate:        true,
			NotificationSent: false,
		},
		{
			ID:               "sha256:img2",
			Repository:       "redis",
			Tag:              "alpine",
			HasUpdate:        true,
			NotificationSent: true, // Already notified
		},
		{
			ID:               "sha256:img3",
			Repository:       "postgres",
			Tag:              "14",
			HasUpdate:        false, // No update
			NotificationSent: false,
		},
		{
			ID:               "sha256:img4",
			Repository:       "traefik",
			Tag:              "latest",
			HasUpdate:        true,
			NotificationSent: false,
		},
	}

	for _, rec := range records {
		err := db.Create(&rec).Error
		require.NoError(t, err)
	}

	// Get unnotified updates
	unnotified, err := svc.GetUnnotifiedUpdates(ctx)
	require.NoError(t, err)

	// Should only return img1 and img4 (has_update=true AND notification_sent=false)
	assert.Len(t, unnotified, 2, "should return 2 unnotified updates")
	assert.Contains(t, unnotified, "sha256:img1")
	assert.Contains(t, unnotified, "sha256:img4")
	assert.NotContains(t, unnotified, "sha256:img2", "img2 already notified")
	assert.NotContains(t, unnotified, "sha256:img3", "img3 has no update")
}
