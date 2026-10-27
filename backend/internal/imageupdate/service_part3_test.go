// Source: getarcaneapp/arcane/backend/internal/imageupdate/service_test.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed APNS mobile push; SQLite temp-file persistence fixture.

package imageupdate

import (
	"context"
	"encoding/json"

	"net/http"
	"net/http/httptest"

	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/testsqlite"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/image"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.getarcane.app/updater/labels"
	"gorm.io/gorm"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/docker"

	"github.com/florianfinn/docklet-next/backend/internal/notification"

	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/backend/pkg/utils"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/imageref"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/notifications"
)

// TestMarkUpdatesAsNotified tests marking images as notified
func TestImageUpdateService_MarkUpdatesAsNotified(t *testing.T) {
	ctx := t.Context()
	db := setupImageUpdateTestDB(t)
	svc := &ImageUpdateService{db: db}

	// Create test records
	imageIDs := []string{"sha256:img1", "sha256:img2", "sha256:img3"}
	for _, id := range imageIDs {
		rec := ImageUpdateRecord{
			ID:               id,
			Repository:       "test/repo",
			Tag:              "latest",
			HasUpdate:        true,
			NotificationSent: false,
		}
		err := db.Create(&rec).Error
		require.NoError(t, err)
	}

	// Mark img1 and img2 as notified
	err := svc.MarkUpdatesAsNotified(ctx, []string{"sha256:img1", "sha256:img2"})
	require.NoError(t, err)

	// Verify img1 and img2 are marked
	var img1 ImageUpdateRecord
	err = db.First(&img1, "id = ?", "sha256:img1").Error
	require.NoError(t, err)
	assert.True(t, img1.NotificationSent)

	var img2 ImageUpdateRecord
	err = db.First(&img2, "id = ?", "sha256:img2").Error
	require.NoError(t, err)
	assert.True(t, img2.NotificationSent)

	// Verify img3 is still false
	var img3 ImageUpdateRecord
	err = db.First(&img3, "id = ?", "sha256:img3").Error
	require.NoError(t, err)
	assert.False(t, img3.NotificationSent)
}

// TestMarkUpdatesAsNotified_EmptyList tests handling of empty ID list
func TestImageUpdateService_MarkUpdatesAsNotified_EmptyList(t *testing.T) {
	ctx := t.Context()
	db := setupImageUpdateTestDB(t)
	svc := &ImageUpdateService{db: db}

	// Should not error on empty list
	err := svc.MarkUpdatesAsNotified(ctx, []string{})
	require.NoError(t, err)

	err = svc.MarkUpdatesAsNotified(ctx, nil)
	require.NoError(t, err)
}

// TestImageUpdateService_SendBatchNotifications_DetachesCanceledContext guards the
// regression where completeImageUpdateActivityInternal cancels the activity-tracked
// ctx before the batch notification step runs, killing the unnotified-updates query
// with "context canceled" so notifications were never dispatched (issue #2920).
// newImageUpdateNotificationDockerServiceInternal stubs the container listing
// the notification flush uses to resolve current update-check eligibility.
func newImageUpdateNotificationDockerServiceInternal(t *testing.T, containers []container.Summary) *docker.DockerClientService {
	t.Helper()
	server := newImageUpdateDiscoveryServerInternal(t, nil, containers)
	t.Cleanup(server.Close)
	return &docker.DockerClientService{Client: newImageUpdateTestDockerClientInternal(t, server)}
}

func newImageUpdateGenericWebhookProviderInternal(t *testing.T, db *database.DB) *atomic.Int32 {
	t.Helper()
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)
	require.NoError(t, db.Create(&notification.NotificationSettings{
		Provider: notifications.NotificationProviderGeneric,
		Enabled:  true,
		Config:   database.JSON{"webhookUrl": server.URL, "method": "POST", "contentType": "application/json"},
	}).Error)
	return &calls
}

// TestImageUpdateService_SendBatchNotifications_FiltersByUpdateCheckEligibility
// covers issue #3532: containers excluded from automatic installation keep
// receiving notifications, while the update-check label, stale container
// records, and images used only by opted-out containers are held back.
func TestImageUpdateService_SendBatchNotifications_FiltersByUpdateCheckEligibility(t *testing.T) {
	db := setupImageUpdateTestDB(t)
	require.NoError(t, db.AutoMigrate(&notification.NotificationSettings{}))
	calls := newImageUpdateGenericWebhookProviderInternal(t, db)

	containers := []container.Summary{
		{ID: "install-excluded", ImageID: "sha256:install-excluded", Image: "test/install-excluded:latest", Labels: map[string]string{labels.LabelUpdater: "false"}},
		{ID: "unmonitored", ImageID: "sha256:unmonitored", Image: "test/unmonitored:latest", Labels: map[string]string{imageref.UpdateCheckLabel: "false"}},
		{ID: "shared-unmonitored", ImageID: "sha256:shared", Image: "test/shared:latest", Labels: map[string]string{imageref.UpdateCheckLabel: "false"}},
		{ID: "shared-monitored", ImageID: "sha256:shared", Image: "test/shared:latest", Labels: map[string]string{labels.LabelUpdater: "false"}},
		{ID: "ref-only-unmonitored", ImageID: "sha256:rebuilt", Image: "test/ref-only:2.0", Labels: map[string]string{imageref.UpdateCheckLabel: "false"}},
	}
	notif := notification.NewNotificationService(db, nil, nil, nil)
	svc := NewImageUpdateService(db, nil, nil, newImageUpdateNotificationDockerServiceInternal(t, containers), nil, notif, nil)

	records := []ImageUpdateRecord{
		{ID: "sha256:install-excluded", Repository: "docker.io/test/install-excluded", Tag: "latest", HasUpdate: true},
		{ID: "sha256:unmonitored", Repository: "docker.io/test/unmonitored", Tag: "latest", HasUpdate: true},
		{ID: "sha256:shared", Repository: "docker.io/test/shared", Tag: "latest", HasUpdate: true},
		{ID: "sha256:pruned", Repository: "docker.io/test/ref-only", Tag: "2.0", HasUpdate: true},
		{ID: "sha256:unused", Repository: "docker.io/test/unused", Tag: "latest", HasUpdate: true},
		{
			ID:          "container::install-excluded",
			ContainerID: "install-excluded",
			ImageID:     "sha256:install-excluded",
			Repository:  "docker.io/test/install-excluded",
			Tag:         "latest",
			HasUpdate:   true,
			UpdateType:  UpdateTypeTag,
		},
		{
			ID:          "container::unmonitored",
			ContainerID: "unmonitored",
			ImageID:     "sha256:unmonitored",
			Repository:  "docker.io/test/unmonitored",
			Tag:         "latest",
			HasUpdate:   true,
			UpdateType:  UpdateTypeTag,
		},
		{ID: "container::gone", ContainerID: "gone", ImageID: "sha256:gone", Repository: "docker.io/test/gone", Tag: "latest", HasUpdate: true, UpdateType: UpdateTypeTag},
	}
	require.NoError(t, db.Create(&records).Error)

	require.NoError(t, svc.SendBatchUpdateNotifications(t.Context()))

	require.EqualValues(t, 1, calls.Load(), "eligible records are delivered in one batch")
	notified := map[string]bool{}
	var reloaded []ImageUpdateRecord
	require.NoError(t, db.Find(&reloaded).Error)
	for _, record := range reloaded {
		notified[record.ID] = record.NotificationSent
	}
	assert.True(t, notified["sha256:install-excluded"], "updater=false still notifies")
	assert.True(t, notified["sha256:shared"], "one monitored consumer keeps the image notified")
	assert.True(t, notified["sha256:unused"], "unused images stay monitored")
	assert.True(t, notified["container::install-excluded"], "check-only containers notify")
	assert.False(t, notified["sha256:unmonitored"], "image used only by an opted-out container")
	assert.False(t, notified["sha256:pruned"], "reference used only by an opted-out container")
	assert.False(t, notified["container::unmonitored"], "opted-out container record")
	assert.False(t, notified["container::gone"], "stale container record")

	// Re-enabling monitoring lets the held-back records notify on the next flush.
	containers[1].Labels = nil
	svc.dockerService = newImageUpdateNotificationDockerServiceInternal(t, containers)
	require.NoError(t, svc.SendBatchUpdateNotifications(t.Context()))
	require.EqualValues(t, 2, calls.Load())
	var resumed ImageUpdateRecord
	require.NoError(t, db.First(&resumed, "id = ?", "container::unmonitored").Error)
	assert.True(t, resumed.NotificationSent)
}

func TestImageUpdateService_SendBatchNotifications_UnresolvedEligibilityLeavesUnnotified(t *testing.T) {
	db := setupImageUpdateTestDB(t)
	require.NoError(t, db.AutoMigrate(&notification.NotificationSettings{}))
	calls := newImageUpdateGenericWebhookProviderInternal(t, db)

	notif := notification.NewNotificationService(db, nil, nil, nil)
	svc := NewImageUpdateService(db, nil, nil, nil, nil, notif, nil)
	require.NoError(t, db.Create(&ImageUpdateRecord{ID: "sha256:img", Repository: "test/repo", Tag: "latest", HasUpdate: true}).Error)

	require.ErrorContains(t, svc.SendBatchUpdateNotifications(t.Context()), "resolve update-check eligibility: docker service unavailable")

	require.Zero(t, calls.Load(), "nothing is sent when eligibility cannot be resolved")
	var reloaded ImageUpdateRecord
	require.NoError(t, db.First(&reloaded, "id = ?", "sha256:img").Error)
	assert.False(t, reloaded.NotificationSent)
}

func TestImageUpdateService_SendBatchNotifications_DetachesCanceledContext(t *testing.T) {
	db := setupImageUpdateTestDB(t)
	require.NoError(t, db.AutoMigrate(&notification.NotificationSettings{}))

	// A provider that actually delivers (returns 200), so the record is only marked
	// notified when the send genuinely reaches it.
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	require.NoError(t, db.Create(&notification.NotificationSettings{
		Provider: notifications.NotificationProviderGeneric,
		Enabled:  true,
		Config: database.JSON{
			"webhookUrl":  server.URL,
			"method":      "POST",
			"contentType": "application/json",
		},
	}).Error)

	notif := notification.NewNotificationService(db, nil, nil, nil)
	svc := NewImageUpdateService(db, nil, nil, newImageUpdateNotificationDockerServiceInternal(t, nil), nil, notif, nil)

	rec := ImageUpdateRecord{
		ID:               "sha256:img1",
		Repository:       "test/repo",
		Tag:              "latest",
		HasUpdate:        true,
		NotificationSent: false,
	}
	require.NoError(t, db.Create(&rec).Error)

	// Simulate the post-activity-completion state: the tracked ctx is already
	// canceled. Derive it from a lifecycle-marked parent to mirror production,
	// where every request/scheduler ctx inherits the marker via the server
	// BaseContext — the detach must work even then.
	ctx, cancel := context.WithCancel(utils.WithAppLifecycleContext(t.Context()))
	cancel()

	require.NoError(t, svc.SendBatchUpdateNotifications(ctx))

	// The provider being reached, and the record being marked notified, both prove
	// GetUnnotifiedUpdates + the send + MarkUpdatesAsNotified ran despite the canceled
	// parent ctx (issue #2920).
	require.EqualValues(t, 1, calls.Load())
	var reloaded ImageUpdateRecord
	require.NoError(t, db.First(&reloaded, "id = ?", "sha256:img1").Error)
	assert.True(t, reloaded.NotificationSent)
}

// TestImageUpdateService_SendBatchNotifications_NoEligibleProviders_LeavesUnnotified
// is the regression guard for issue #3079: a background check that finds an update
// while no provider has the image-update event enabled must NOT mark the record
// notified — otherwise, once the user later configures a provider, the still-pending
// update is permanently suppressed (it only re-surfaces on a future digest change).
func TestImageUpdateService_SendBatchNotifications_NoEligibleProviders_LeavesUnnotified(t *testing.T) {
	db := setupImageUpdateTestDB(t)
	require.NoError(t, db.AutoMigrate(&notification.NotificationSettings{}))

	notif := notification.NewNotificationService(db, nil, nil, nil)
	svc := NewImageUpdateService(db, nil, nil, newImageUpdateNotificationDockerServiceInternal(t, nil), nil, notif, nil)

	rec := ImageUpdateRecord{
		ID:               "sha256:img-no-provider",
		Repository:       "test/repo",
		Tag:              "latest",
		HasUpdate:        true,
		NotificationSent: false,
	}
	require.NoError(t, db.Create(&rec).Error)

	require.NoError(t, svc.SendBatchUpdateNotifications(t.Context()))

	var reloaded ImageUpdateRecord
	require.NoError(t, db.First(&reloaded, "id = ?", "sha256:img-no-provider").Error)
	assert.False(t, reloaded.NotificationSent)

	unnotified, err := svc.GetUnnotifiedUpdates(t.Context())
	require.NoError(t, err)
	require.Contains(t, unnotified, "sha256:img-no-provider")
}

// TestImageUpdateService_SendBatchNotifications_PartialFailureStillMarksNotified
// guards the duplicate-notification bug: when one provider delivers and another
// fails, the record must still be marked notified — otherwise the healthy provider
// re-sends the same update on every poll until the broken provider is fixed.
func TestImageUpdateService_SendBatchNotifications_PartialFailureStillMarksNotified(t *testing.T) {
	db := setupImageUpdateTestDB(t)
	require.NoError(t, db.AutoMigrate(&notification.NotificationSettings{}))

	healthy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer healthy.Close()
	broken := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer broken.Close()

	for _, serverURL := range []string{healthy.URL, broken.URL} {
		require.NoError(t, db.Create(&notification.NotificationSettings{
			Provider: notifications.NotificationProviderGeneric,
			Enabled:  true,
			Config: database.JSON{
				"webhookUrl":  serverURL,
				"method":      "POST",
				"contentType": "application/json",
			},
		}).Error)
	}

	notif := notification.NewNotificationService(db, nil, nil, nil)
	svc := NewImageUpdateService(db, nil, nil, newImageUpdateNotificationDockerServiceInternal(t, nil), nil, notif, nil)

	rec := ImageUpdateRecord{
		ID:               "sha256:img-partial",
		Repository:       "test/repo",
		Tag:              "latest",
		HasUpdate:        true,
		NotificationSent: false,
	}
	require.NoError(t, db.Create(&rec).Error)

	require.NoError(t, svc.SendBatchUpdateNotifications(t.Context()))

	var reloaded ImageUpdateRecord
	require.NoError(t, db.First(&reloaded, "id = ?", "sha256:img-partial").Error)
	assert.True(t, reloaded.NotificationSent)
}

func TestImageUpdateService_GetUpdateSummaryForImageIDs_FiltersToLiveImages(t *testing.T) {
	ctx := t.Context()
	db := setupImageUpdateTestDB(t)
	svc := &ImageUpdateService{db: db}
	now := time.Now()

	records := []ImageUpdateRecord{
		{
			ID:             "sha256:live-1",
			Repository:     "docker.io/library/nginx",
			Tag:            "latest",
			HasUpdate:      true,
			UpdateType:     "digest",
			CurrentVersion: "latest",
			CheckTime:      now,
		},
		{
			ID:             "sha256:live-2",
			Repository:     "docker.io/library/redis",
			Tag:            "latest",
			HasUpdate:      false,
			UpdateType:     "digest",
			CurrentVersion: "latest",
			LastError:      new("rate limited"),
			CheckTime:      now,
		},
		{
			ID:             "sha256:stale-1",
			Repository:     "docker.io/library/postgres",
			Tag:            "latest",
			HasUpdate:      true,
			UpdateType:     "digest",
			CurrentVersion: "latest",
			LastError:      new("stale failure"),
			CheckTime:      now,
		},
	}
	for i := range records {
		err := db.Create(&records[i]).Error
		require.NoError(t, err)
	}

	summary, err := svc.getUpdateSummaryForImageIDsInternal(ctx, []string{"sha256:live-1", "sha256:live-2"})
	require.NoError(t, err)

	assert.Equal(t, 2, summary.TotalImages)
	assert.Equal(t, 1, summary.ImagesWithUpdates)
	assert.Equal(t, 1, summary.DigestUpdates)
	assert.Equal(t, 1, summary.ErrorsCount)
}

func TestImageUpdateService_GetUpdateSummaryForImageIDs_EmptyLiveSet(t *testing.T) {
	ctx := t.Context()
	db := setupImageUpdateTestDB(t)
	svc := &ImageUpdateService{db: db}

	summary, err := svc.getUpdateSummaryForImageIDsInternal(ctx, nil)
	require.NoError(t, err)

	assert.Equal(t, 0, summary.TotalImages)
	assert.Equal(t, 0, summary.ImagesWithUpdates)
	assert.Equal(t, 0, summary.DigestUpdates)
	assert.Equal(t, 0, summary.ErrorsCount)
}

func TestImageUpdateService_ParseAndGroupImages_DedupesNormalizedRefs(t *testing.T) {
	svc := &ImageUpdateService{}

	refs := []string{
		"nginx:latest",
		"docker.io/library/nginx:latest",
		"redis:7",
		"docker.io/library/redis:7",
	}

	regRepos, initialResults, grouped := svc.parseAndGroupImagesInternal(refs)
	require.Empty(t, initialResults)
	require.Len(t, grouped, 2)
	require.Contains(t, regRepos, "docker.io")
	require.Len(t, regRepos["docker.io"], 2)

	firstRefSet := map[string]struct{}{}
	for _, imageRef := range grouped[0].refs {
		firstRefSet[imageRef] = struct{}{}
	}
	secondRefSet := map[string]struct{}{}
	for _, imageRef := range grouped[1].refs {
		secondRefSet[imageRef] = struct{}{}
	}

	// Each normalized image should only be checked once, while retaining all aliases.
	assert.True(t, (containsAll(firstRefSet, "nginx:latest", "docker.io/library/nginx:latest") &&
		containsAll(secondRefSet, "redis:7", "docker.io/library/redis:7")) ||
		(containsAll(secondRefSet, "nginx:latest", "docker.io/library/nginx:latest") &&
			containsAll(firstRefSet, "redis:7", "docker.io/library/redis:7")))
}

func newImageUpdateTestSettingsServiceInternal(t *testing.T, registryTimeout, dockerAPITimeout string) *settings.SettingsService {
	t.Helper()
	t.Setenv("REGISTRY_TIMEOUT", registryTimeout)
	t.Setenv("DOCKER_API_TIMEOUT", dockerAPITimeout)
	ctx := t.Context()
	db, err := gorm.Open(testsqlite.Dialector(t, ":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&settings.SettingVariable{}))
	dbWrap := &database.DB{DB: db}
	service, err := settings.NewSettingsService(ctx, dbWrap)
	if err == nil {
		t.Cleanup(func() { require.NoError(t, service.Stop(context.WithoutCancel(t.Context()))) })
	}
	require.NoError(t, err)
	return service
}

func newBlockedDockerAPIServerInternal(t *testing.T, pathContains string) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/containers/json") {
			w.Header().Set("Content-Type", "application/json")
			if !assert.NoError(t, json.NewEncoder(w).Encode([]container.Summary{})) {
				return
			}
			return
		}

		if strings.Contains(r.URL.Path, pathContains) {
			<-r.Context().Done()
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(server.Close)
	return server
}

func containsAll(set map[string]struct{}, refs ...string) bool {
	for _, imageRef := range refs {
		if _, ok := set[imageRef]; !ok {
			return false
		}
	}
	return true
}

func newImageUpdateDiscoveryServerInternal(
	t *testing.T,
	images []image.Summary,
	containers []container.Summary,
) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/images/json"):
			w.Header().Set("Content-Type", "application/json")
			if !assert.NoError(t, json.NewEncoder(w).Encode(images)) {
				return
			}
		case strings.HasSuffix(r.URL.Path, "/containers/json"):
			w.Header().Set("Content-Type", "application/json")
			if !assert.NoError(t, json.NewEncoder(w).Encode(containers)) {
				return
			}
		default:
			http.NotFound(w, r)
		}
	}))
}

func TestImageUpdateService_GetAllImageRefsHonorsExclusiveContainerOptOutInternal(t *testing.T) {
	const (
		disabledRef = "local/disabled:latest"
		enabledRef  = "local/enabled:latest"
		sharedRef   = "local/shared:latest"
		unusedRef   = "local/unused:latest"
	)

	const (
		runningOnlyRef  = "ghcr.io/example/running-only:1.2"
		stoppedOnlyRef  = "example/stopped-only:2.0"
		disabledOnlyRef = "example/disabled-only:3.0"
		pinnedRef       = "example/pinned@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
		enabledAlias    = "local/enabled"
		idOnlyImage     = "sha256:00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"
	)

	images := []image.Summary{
		{ID: "sha256:disabled", RepoTags: []string{disabledRef}},
		{ID: "sha256:enabled", RepoTags: []string{enabledRef}},
		{ID: "sha256:shared", RepoTags: []string{sharedRef}},
		{ID: "sha256:unused", RepoTags: []string{unusedRef}},
		{ID: "sha256:untagged", RepoTags: []string{"<none>:<none>"}},
	}
	containers := []container.Summary{
		{
			ID:     "disabled-container",
			Image:  disabledRef,
			Labels: map[string]string{imageref.UpdateCheckLabel: "false"},
		},
		{
			ID:    "enabled-container",
			Image: enabledRef,
		},
		{
			ID:     "shared-disabled-container",
			Image:  sharedRef,
			Labels: map[string]string{imageref.UpdateCheckLabel: "off"},
		},
		{
			ID:    "shared-enabled-container",
			Image: sharedRef,
		},
		{ID: "running-only", ImageID: "sha256:untagged", Image: runningOnlyRef, State: container.StateRunning},
		{ID: "stopped-only", ImageID: "sha256:pruned", Image: stoppedOnlyRef, State: container.StateExited},
		{ID: "disabled-only", ImageID: "sha256:pruned-disabled", Image: disabledOnlyRef, Labels: map[string]string{imageref.UpdateCheckLabel: "false"}},
		{ID: "enabled-alias", ImageID: "sha256:enabled", Image: enabledAlias},
		{ID: "pinned-container", ImageID: "sha256:pinned", Image: pinnedRef},
		{ID: "id-only", ImageID: idOnlyImage, Image: idOnlyImage},
	}

	server := newImageUpdateDiscoveryServerInternal(t, images, containers)
	t.Cleanup(server.Close)

	svc := NewImageUpdateService(
		nil,
		nil,
		nil,
		&docker.DockerClientService{Client: newImageUpdateTestDockerClientInternal(t, server)},
		nil,
		nil,
		nil,
	)

	got, err := svc.getAllImageRefsInternal(t.Context(), 0)

	require.NoError(t, err)
	assert.Equal(t, []string{enabledRef, sharedRef, unusedRef, runningOnlyRef, stoppedOnlyRef, pinnedRef}, got)
}

func TestImageUpdateService_GetAllImageRefsAppliesLimitAfterOptOutFilteringInternal(t *testing.T) {
	const (
		disabledRef      = "local/disabled:latest"
		enabledRef       = "local/enabled:latest"
		unusedRef        = "local/unused:latest"
		containerOnlyRef = "local/container-only:1.0"
	)

	images := []image.Summary{
		{ID: "sha256:disabled", RepoTags: []string{disabledRef}},
		{ID: "sha256:enabled", RepoTags: []string{enabledRef}},
		{ID: "sha256:unused", RepoTags: []string{unusedRef}},
	}
	containers := []container.Summary{
		{
			ID:     "disabled-container",
			Image:  disabledRef,
			Labels: map[string]string{imageref.UpdateCheckLabel: "0"},
		},
		{
			ID:    "enabled-container",
			Image: enabledRef,
		},
		{ID: "container-only", ImageID: "sha256:container-only", Image: containerOnlyRef},
	}

	server := newImageUpdateDiscoveryServerInternal(t, images, containers)
	t.Cleanup(server.Close)

	svc := NewImageUpdateService(
		nil,
		nil,
		nil,
		&docker.DockerClientService{Client: newImageUpdateTestDockerClientInternal(t, server)},
		nil,
		nil,
		nil,
	)

	got, err := svc.getAllImageRefsInternal(t.Context(), 2)
	require.NoError(t, err)
	assert.Equal(t, []string{enabledRef, unusedRef}, got)

	got, err = svc.getAllImageRefsInternal(t.Context(), 3)
	require.NoError(t, err)
	assert.Equal(t, []string{enabledRef, unusedRef, containerOnlyRef}, got)
}

func TestImageUpdateService_GetAllImageRefsExcludesAliasesOfOptedOutImageInternal(t *testing.T) {
	const (
		imageID    = "sha256:local-build"
		primaryRef = "local/caddy:latest"
		aliasRef   = "local/caddy:dev"
		enabledRef = "local/enabled:latest"
	)

	images := []image.Summary{
		{
			ID:       imageID,
			RepoTags: []string{primaryRef, aliasRef},
		},
		{
			ID:       "sha256:enabled",
			RepoTags: []string{enabledRef},
		},
	}
	containers := []container.Summary{
		{
			ID:      "disabled-container",
			ImageID: imageID,
			Image:   primaryRef,
			Labels:  map[string]string{imageref.UpdateCheckLabel: "false"},
		},
		{
			ID:      "enabled-container",
			ImageID: "sha256:enabled",
			Image:   enabledRef,
		},
	}

	server := newImageUpdateDiscoveryServerInternal(t, images, containers)
	t.Cleanup(server.Close)

	svc := NewImageUpdateService(
		nil,
		nil,
		nil,
		&docker.DockerClientService{Client: newImageUpdateTestDockerClientInternal(t, server)},
		nil,
		nil,
		nil,
	)

	got, err := svc.getAllImageRefsInternal(t.Context(), 0)

	require.NoError(t, err)
	assert.NotContains(t, got, primaryRef)
	assert.NotContains(t, got, aliasRef)
	assert.Contains(t, got, enabledRef)
}

func TestImageUpdateService_GetAllImageRefsKeepsImageSharedByEligibleContainerInternal(t *testing.T) {
	const (
		imageID  = "sha256:shared-image"
		imageRef = "local/shared:latest"
		aliasRef = "local/shared:dev"
	)

	images := []image.Summary{
		{
			ID:       imageID,
			RepoTags: []string{imageRef, aliasRef},
		},
	}
	containers := []container.Summary{
		{
			ID:      "disabled-container",
			ImageID: imageID,
			Image:   imageRef,
			Labels:  map[string]string{imageref.UpdateCheckLabel: "false"},
		},
		{
			ID:      "enabled-container",
			ImageID: imageID,
			Image:   imageRef,
		},
	}

	server := newImageUpdateDiscoveryServerInternal(t, images, containers)
	t.Cleanup(server.Close)

	svc := NewImageUpdateService(
		nil,
		nil,
		nil,
		&docker.DockerClientService{Client: newImageUpdateTestDockerClientInternal(t, server)},
		nil,
		nil,
		nil,
	)

	got, err := svc.getAllImageRefsInternal(t.Context(), 0)

	require.NoError(t, err)
	assert.Contains(t, got, imageRef)
	assert.Contains(t, got, aliasRef)
}

func TestImageUpdateService_GetAllImageRefsFallsBackToRefWhenImageIDsDifferInternal(t *testing.T) {
	const imageRef = "local/caddy:latest"

	images := []image.Summary{
		{
			ID:       "sha256:image-list-id",
			RepoTags: []string{imageRef},
		},
	}
	containers := []container.Summary{
		{
			ID:      "disabled-container",
			ImageID: "sha256:container-list-id",
			Image:   imageRef,
			Labels:  map[string]string{imageref.UpdateCheckLabel: "false"},
		},
	}

	server := newImageUpdateDiscoveryServerInternal(t, images, containers)
	t.Cleanup(server.Close)

	svc := NewImageUpdateService(
		nil,
		nil,
		nil,
		&docker.DockerClientService{Client: newImageUpdateTestDockerClientInternal(t, server)},
		nil,
		nil,
		nil,
	)

	got, err := svc.getAllImageRefsInternal(t.Context(), 0)

	require.NoError(t, err)
	assert.NotContains(t, got, imageRef)
}

func TestImageUpdateService_GetAllImageRefsMergesIDAndReferenceEligibilityInternal(t *testing.T) {
	const (
		imageRef = "local/shared:latest"
		imageID  = "sha256:image-summary-id"
	)

	images := []image.Summary{
		{
			ID:       imageID,
			RepoTags: []string{imageRef},
		},
	}
	containers := []container.Summary{
		{
			ID:      "disabled-container",
			ImageID: imageID,
			Image:   imageRef,
			Labels:  map[string]string{imageref.UpdateCheckLabel: "false"},
		},
		{
			ID:      "eligible-container",
			ImageID: "sha256:different-container-id",
			Image:   imageRef,
		},
	}

	server := newImageUpdateDiscoveryServerInternal(t, images, containers)
	t.Cleanup(server.Close)

	svc := NewImageUpdateService(
		nil,
		nil,
		nil,
		&docker.DockerClientService{Client: newImageUpdateTestDockerClientInternal(t, server)},
		nil,
		nil,
		nil,
	)

	got, err := svc.getAllImageRefsInternal(t.Context(), 0)

	require.NoError(t, err)
	assert.Contains(t, got, imageRef)
}

func TestImageUpdateService_GetAllImageRefsFallsBackWhenContainerDiscoveryFailsInternal(t *testing.T) {
	const (
		firstRef  = "local/first:latest"
		secondRef = "local/second:latest"
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/images/json"):
			w.Header().Set("Content-Type", "application/json")
			if !assert.NoError(t, json.NewEncoder(w).Encode([]image.Summary{
				{ID: "sha256:first", RepoTags: []string{firstRef}},
				{ID: "sha256:second", RepoTags: []string{secondRef}},
			})) {
				return
			}
		case strings.HasSuffix(r.URL.Path, "/containers/json"):
			http.Error(w, "container listing denied", http.StatusForbidden)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	svc := NewImageUpdateService(
		nil,
		nil,
		nil,
		&docker.DockerClientService{Client: newImageUpdateTestDockerClientInternal(t, server)},
		nil,
		nil,
		nil,
	)

	got, err := svc.getAllImageRefsInternal(t.Context(), 0)

	require.NoError(t, err)
	assert.Equal(t, []string{firstRef, secondRef}, got)
}
