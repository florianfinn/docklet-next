// Source: getarcaneapp/arcane/backend/internal/imageupdate/service_test.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed APNS mobile push; SQLite temp-file persistence fixture.

package imageupdate

import (
	"context"
	"encoding/json"
	"errors"

	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/types/imageupdate"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/image"

	"github.com/moby/moby/client"
	"github.com/opencontainers/go-digest"

	"github.com/samber/mo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.getarcane.app/kit/pkg"

	"go.getarcane.app/updater/labels"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/internal/event"

	"github.com/florianfinn/docklet-next/backend/internal/registry"

	"github.com/florianfinn/docklet-next/backend/pkg/utils/imageref"
)

func TestFilterImageSummariesKeepsInstallExcludedContainersMonitoredInternal(t *testing.T) {
	const (
		labelDisabledRef = "local/label-disabled:latest"
		uiExcludedRef    = "local/ui-excluded:latest"
		unmonitoredRef   = "local/unmonitored:latest"
		sharedRef        = "local/shared:latest"
	)

	images := []image.Summary{
		{ID: "sha256:label-disabled", RepoTags: []string{labelDisabledRef}},
		{ID: "sha256:ui-excluded", RepoTags: []string{uiExcludedRef}},
		{ID: "sha256:unmonitored", RepoTags: []string{unmonitoredRef}},
		{ID: "sha256:shared", RepoTags: []string{sharedRef}},
	}
	// Both automatic-installation exclusions keep the image monitored; only the
	// update-check label (any spelling, false-like values only) removes it.
	containers := []container.Summary{
		{ID: "c1", Names: []string{"/label-disabled"}, ImageID: "sha256:label-disabled", Image: labelDisabledRef, Labels: map[string]string{labels.LabelUpdater: "false"}},
		{ID: "c2", Names: []string{"/ui-excluded"}, ImageID: "sha256:ui-excluded", Image: uiExcludedRef},
		{ID: "c3", Names: []string{"/unmonitored"}, ImageID: "sha256:unmonitored", Image: unmonitoredRef, Labels: map[string]string{strings.ToUpper(imageref.UpdateCheckLabel): "no"}},
		{ID: "c4", Names: []string{"/shared-unmonitored"}, ImageID: "sha256:shared", Image: sharedRef, Labels: map[string]string{imageref.UpdateCheckLabel: "off"}},
		{
			ID:      "c5",
			Names:   []string{"/shared-garbage-value"},
			ImageID: "sha256:shared",
			Image:   "docker.io/local/shared:latest",
			Labels: map[string]string{
				imageref.UpdateCheckLabel: "maybe",
				labels.LabelUpdater:       "false",
			},
		},
		{ID: "c6", Names: []string{"/unmonitored-only"}, ImageID: "sha256:pruned", Image: "local/unmonitored-only:1.0", Labels: map[string]string{imageref.UpdateCheckLabel: "0"}},
	}

	got := filterImageSummariesByContainerOptOutInternal(images, containers, 0)

	assert.Equal(t, []string{labelDisabledRef, uiExcludedRef, sharedRef}, got)
}

// testProjectRow is a minimal stand-in for project.Project: the project
// package imports this one, so the in-package test cannot import it back.
type testProjectRow struct {
	database.BaseModel
	Name               string
	Path               string
	BuildImageRefsJSON *string `gorm:"column:build_image_refs_json"`
}

func (testProjectRow) TableName() string { return "projects" }

func TestContainerTagChecksPersistIndependentPoliciesInternal(t *testing.T) {
	db := setupImageUpdateRegistryTestDBInternal(t)
	var tagListings atomic.Int64
	registryServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if registryPingInternal(w, r) {
			return
		}
		if !strings.HasSuffix(r.URL.Path, "/tags/list") {
			http.NotFound(w, r)
			return
		}
		tagListings.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if !assert.NoError(t, json.NewEncoder(w).Encode(map[string]any{"name": "team/app", "tags": []string{"1.0.0", "1.1.0", "2.0.0"}})) {
			return
		}
	}))
	defer registryServer.Close()
	registryURL, err := url.Parse(registryServer.URL)
	require.NoError(t, err)
	imageRef := registryURL.Host + "/team/app:1.0.0"
	imageID := digest.FromString("shared").String()
	values := map[string]map[string]string{
		"one": {labels.LabelUpdateStrategy: "auto"},
		"two": {labels.LabelUpdateStrategy: "auto", labels.LabelUpdateConstraint: "2.x"},
	}
	dockerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/containers/json"):
			if !assert.NoError(
				t,
				json.NewEncoder(w).Encode([]container.Summary{
					{
						ID:      "one",
						Image:   imageRef,
						ImageID: imageID,
						Labels:  values["one"],
					},
					{
						ID:      "two",
						Image:   imageRef,
						ImageID: imageID,
						Labels:  values["two"],
					},
				}),
			) {
				return
			}
		case strings.Contains(r.URL.Path, "/images/"):
			if !assert.NoError(
				t,
				json.NewEncoder(w).Encode(image.InspectResponse{
					ID:          imageID,
					RepoTags:    []string{imageRef},
					RepoDigests: []string{registryURL.Host + "/team/app@" + imageID},
				}),
			) {
				return
			}
		case strings.Contains(r.URL.Path, "/containers/"):
			id := kit.Ternary(strings.Contains(r.URL.Path, "/two/"), "two", "one")
			if !assert.NoError(
				t,
				json.NewEncoder(w).Encode(container.InspectResponse{
					ID:    id,
					Image: imageID,
					Config: &container.Config{
						Image:  imageRef,
						Labels: values[id],
					},
				}),
			) {
				return
			}
		default:
			t.Errorf("unexpected Docker request %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer dockerServer.Close()
	registryService := registry.NewContainerRegistryService(db, func(context.Context) (registry.RegistryDaemonClient, error) {
		return &fakeRegistryDaemonClient{distributionInspectFn: func(context.Context, string, client.DistributionInspectOptions) (client.DistributionInspectResult, error) {
			return client.DistributionInspectResult{}, errors.New("manifest not found")
		}}, nil
	}, nil, nil, registryServer.Client())
	svc := NewImageUpdateService(db, nil, registryService, &docker.DockerClientService{Client: newImageUpdateTestDockerClientInternal(t, dockerServer)}, nil, nil, nil)
	checks, err := svc.checkContainerTagUpdatesInternal(t.Context(), []string{imageRef}, nil)
	require.NoError(t, err)
	require.Len(t, checks, 2)
	require.Equal(t, "1.1.0", checks["one"].LatestVersion)
	require.Equal(t, "2.0.0", checks["two"].LatestVersion)
	require.EqualValues(t, 1, tagListings.Load(), "replicas of one repository share a single tag listing per scan")
	var records []ImageUpdateRecord
	require.NoError(t, db.Order("container_id").Find(&records).Error)
	require.Len(t, records, 2)
	require.Equal(t, "container::one", records[0].ID)
	require.Equal(t, imageID, records[0].ImageID)
	require.Equal(t, imageID, records[1].ImageID)
	require.True(t, records[0].HasUpdate)
	svc.eventService = event.NewEventService(db, nil, nil)
	single, checkErr := svc.CheckImageUpdate(t.Context(), imageRef)
	require.NoError(t, checkErr)
	require.True(t, single.HasUpdate)
	require.Equal(t, UpdateTypeTag, single.UpdateType)
	require.Equal(t, "1.1.0", single.LatestVersion, "the first container candidate promotes the image result")
	require.NotEmpty(t, single.Error, "keep the old-tag digest error visible")
	require.Len(t, single.ContainerUpdates, 2)
	// The image-level digest error is stored independently of container candidates.
	require.NoError(t, db.Where("container_id = ?", "").Delete(&ImageUpdateRecord{}).Error)
	// Changing one policy to an invalid expression clears its actionable state
	// without overwriting the other container's result.
	values["one"][labels.LabelUpdateConstraint] = "invalid"
	checks, err = svc.checkContainerTagUpdatesInternal(t.Context(), []string{imageRef}, nil)
	require.NoError(t, err)
	require.NotEmpty(t, checks["one"].Error)
	require.False(t, checks["one"].HasUpdate)
	records = nil
	require.NoError(t, db.Order("container_id").Find(&records).Error)
	require.False(t, records[0].HasUpdate)
	require.True(t, records[1].HasUpdate)
	limited := &imageupdate.Response{UpdateType: UpdateTypeTag, Error: "registry status: 429", CheckTime: time.Now().UTC()}
	current := container.Summary{ID: "two", Image: imageRef, ImageID: imageID, Labels: values["two"]}
	require.NoError(t, svc.saveContainerTagResultInternal(t.Context(), current, limited))
	var retained ImageUpdateRecord
	require.NoError(t, db.First(&retained, "id = ?", "container::two").Error)
	require.True(t, retained.HasUpdate, "same-policy rate limits retain the last good result")
	require.Equal(t, imageref.UpdatePolicyKey(imageRef, current.Labels), retained.PolicyKey)
	current.Labels = map[string]string{labels.LabelUpdateStrategy: "tag", labels.LabelUpdateConstraint: "1.x"}
	require.NoError(t, svc.saveContainerTagResultInternal(t.Context(), current, limited))
	retained = ImageUpdateRecord{}
	require.NoError(t, db.First(&retained, "id = ?", "container::two").Error)
	require.False(t, retained.HasUpdate, "a changed policy cannot inherit the previous candidate")
	require.Equal(t, imageref.UpdatePolicyKey(imageRef, current.Labels), retained.PolicyKey)
	require.Equal(t, limited.Error, *retained.LastError)
}

// TestContainerTagChecksSeparateMonitoringFromInstallationInternal covers
// issue #3532: a container excluded from automatic installation still gets a
// tag check, while the update-check label skips the check and leaves the
// container's stored result untouched for the installer.
func TestContainerTagChecksSeparateMonitoringFromInstallationInternal(t *testing.T) {
	db := setupImageUpdateRegistryTestDBInternal(t)
	registryServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if registryPingInternal(w, r) {
			return
		}
		if !strings.HasSuffix(r.URL.Path, "/tags/list") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if !assert.NoError(t, json.NewEncoder(w).Encode(map[string]any{"name": "team/app", "tags": []string{"1.0.0", "1.1.0"}})) {
			return
		}
	}))
	defer registryServer.Close()
	registryURL, err := url.Parse(registryServer.URL)
	require.NoError(t, err)
	imageRef := registryURL.Host + "/team/app:1.0.0"
	imageID := digest.FromString("shared").String()
	values := map[string]map[string]string{
		"install-excluded": {labels.LabelUpdateStrategy: "auto", labels.LabelUpdater: "false"},
		"unmonitored":      {labels.LabelUpdateStrategy: "auto", imageref.UpdateCheckLabel: "false"},
	}
	var inspected []string
	dockerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/containers/json"):
			if !assert.NoError(t, json.NewEncoder(w).Encode([]container.Summary{
				{ID: "install-excluded", Names: []string{"/install-excluded"}, Image: imageRef, ImageID: imageID, Labels: values["install-excluded"]},
				{ID: "unmonitored", Names: []string{"/unmonitored"}, Image: imageRef, ImageID: imageID, Labels: values["unmonitored"]},
			})) {
				return
			}
		case strings.Contains(r.URL.Path, "/containers/"):
			id := kit.Ternary(strings.Contains(r.URL.Path, "/unmonitored/"), "unmonitored", "install-excluded")
			inspected = append(inspected, id)
			if !assert.NoError(
				t,
				json.NewEncoder(w).Encode(container.InspectResponse{
					ID:    id,
					Name:  "/" + id,
					Image: imageID,
					Config: &container.Config{
						Image:  imageRef,
						Labels: values[id],
					},
				}),
			) {
				return
			}
		default:
			t.Errorf("unexpected Docker request %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer dockerServer.Close()
	registryService := registry.NewContainerRegistryService(db, func(context.Context) (registry.RegistryDaemonClient, error) {
		return &fakeRegistryDaemonClient{}, nil
	}, nil, nil, registryServer.Client())
	settingsService := newImageUpdateTestSettingsServiceInternal(t, "5", "5")
	// The UI exclusion governs installation only; it must not reach the checker.
	require.NoError(t, settingsService.SetStringSetting(t.Context(), "autoUpdateExcludedContainers", "install-excluded"))
	svc := NewImageUpdateService(db, settingsService, registryService, &docker.DockerClientService{Client: newImageUpdateTestDockerClientInternal(t, dockerServer)}, nil, nil, nil)

	pending := "1.1.0"
	stale := ImageUpdateRecord{
		ID:          "container::unmonitored",
		ContainerID: "unmonitored",
		ImageID:     imageID,
		PolicyKey: imageref.UpdatePolicyKey(
			imageRef,
			values["unmonitored"],
		),
		Repository:    registryURL.Host + "/team/app",
		Tag:           "1.0.0",
		HasUpdate:     true,
		UpdateType:    UpdateTypeTag,
		LatestVersion: &pending,
		CheckTime:     time.Now().UTC(),
	}
	require.NoError(t, db.Create(&stale).Error)

	checks, err := svc.checkContainerTagUpdatesInternal(t.Context(), []string{imageRef}, nil)
	require.NoError(t, err)
	require.Len(t, checks, 1)
	require.True(t, checks["install-excluded"].HasUpdate, "updater=false and the UI exclusion do not block checks")
	require.Equal(t, "1.1.0", checks["install-excluded"].LatestVersion)
	require.Empty(t, checks["install-excluded"].Error)
	require.Equal(t, []string{"install-excluded"}, inspected, "opted-out containers are never inspected")

	var retained ImageUpdateRecord
	require.NoError(t, db.First(&retained, "id = ?", "container::unmonitored").Error)
	require.True(t, retained.HasUpdate, "monitoring opt-out keeps pending updates for the installer")
	require.Equal(t, stale.PolicyKey, retained.PolicyKey)
}

func TestContainerTagChecksUseRegistryTagTimeoutInternal(t *testing.T) {
	for _, tt := range []struct {
		name            string
		tagTimeout      string
		respond         func(w http.ResponseWriter, r *http.Request)
		wantErrContains string
	}{
		{name: "listing slower than registry timeout succeeds within tag budget", tagTimeout: "30", respond: func(w http.ResponseWriter, _ *http.Request) {
			time.Sleep(1300 * time.Millisecond)
			w.Header().Set("Content-Type", "application/json")
			if !assert.NoError(t, json.NewEncoder(w).Encode(map[string]any{"name": "team/app", "tags": []string{"1.0.0", "1.1.0"}})) {
				return
			}
		}},
		{name: "stalled listing fails at tag budget", tagTimeout: "1", respond: func(_ http.ResponseWriter, r *http.Request) {
			<-r.Context().Done()
		}, wantErrContains: context.DeadlineExceeded.Error()},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db := setupImageUpdateRegistryTestDBInternal(t)
			t.Setenv("REGISTRY_TAG_TIMEOUT", tt.tagTimeout)
			settingsService := newImageUpdateTestSettingsServiceInternal(t, "1", "30")
			registryServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if registryPingInternal(w, r) {
					return
				}
				if !strings.HasSuffix(r.URL.Path, "/tags/list") {
					http.NotFound(w, r)
					return
				}
				tt.respond(w, r)
			}))
			defer registryServer.Close()
			registryURL, err := url.Parse(registryServer.URL)
			require.NoError(t, err)
			imageRef := registryURL.Host + "/team/app:1.0.0"
			imageID := digest.FromString("tag-timeout").String()
			dockerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case strings.HasSuffix(r.URL.Path, "/containers/json"):
					if !assert.NoError(t, json.NewEncoder(w).Encode([]container.Summary{{ID: "one", Image: imageRef, ImageID: imageID, Labels: autoLabels}})) {
						return
					}
				case strings.Contains(r.URL.Path, "/images/"):
					if !assert.NoError(t, json.NewEncoder(w).Encode(image.InspectResponse{ID: imageID, RepoTags: []string{imageRef}})) {
						return
					}
				case strings.Contains(r.URL.Path, "/containers/"):
					if !assert.NoError(
						t,
						json.NewEncoder(w).Encode(container.InspectResponse{
							ID:    "one",
							Image: imageID,
							Config: &container.Config{
								Image:  imageRef,
								Labels: autoLabels,
							},
						}),
					) {
						return
					}
				default:
					http.NotFound(w, r)
				}
			}))
			defer dockerServer.Close()
			registryService := registry.NewContainerRegistryService(db, func(context.Context) (registry.RegistryDaemonClient, error) {
				return &fakeRegistryDaemonClient{distributionInspectFn: func(context.Context, string, client.DistributionInspectOptions) (client.DistributionInspectResult, error) {
					return client.DistributionInspectResult{}, errors.New("manifest not found")
				}}, nil
			}, nil, settingsService, registryServer.Client())
			svc := NewImageUpdateService(db, settingsService, registryService, &docker.DockerClientService{Client: newImageUpdateTestDockerClientInternal(t, dockerServer)}, nil, nil, nil)

			start := time.Now()
			checks, err := svc.checkContainerTagUpdatesInternal(t.Context(), []string{imageRef}, nil)
			elapsed := time.Since(start)
			require.NoError(t, err)
			require.Contains(t, checks, "one")
			require.Less(t, elapsed, 3*time.Second)
			if tt.wantErrContains != "" {
				require.Equal(t, "update.check-unavailable", checks["one"].Code)
				require.NotContains(t, checks["one"].Error, tt.wantErrContains)
				return
			}
			require.Empty(t, checks["one"].Error)
			require.True(t, checks["one"].HasUpdate)
			require.Equal(t, "1.1.0", checks["one"].LatestVersion)
		})
	}
}

func TestContainerTagChecksPersistResultsFinishedBeforeScanDeadlineInternal(t *testing.T) {
	db := setupImageUpdateRegistryTestDBInternal(t)
	t.Setenv("REGISTRY_TAG_TIMEOUT", "30")
	settingsService := newImageUpdateTestSettingsServiceInternal(t, "30", "30")
	registryServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case registryPingInternal(w, r):
		case strings.HasSuffix(r.URL.Path, "/team/fast/tags/list"):
			w.Header().Set("Content-Type", "application/json")
			if !assert.NoError(t, json.NewEncoder(w).Encode(map[string]any{"name": "team/fast", "tags": []string{"1.0.0", "1.1.0"}})) {
				return
			}
		case strings.HasSuffix(r.URL.Path, "/team/slow/tags/list"):
			// Stall until the scan deadline cancels the request.
			<-r.Context().Done()
		default:
			http.NotFound(w, r)
		}
	}))
	defer registryServer.Close()
	registryURL, err := url.Parse(registryServer.URL)
	require.NoError(t, err)
	imageRefs := map[string]string{"fast": registryURL.Host + "/team/fast:1.0.0", "slow": registryURL.Host + "/team/slow:1.0.0"}
	imageIDs := map[string]string{"fast": digest.FromString("fast").String(), "slow": digest.FromString("slow").String()}
	nameFor := func(path string) string {
		return kit.Ternary(strings.Contains(path, "slow"), "slow", "fast")
	}
	dockerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		name := nameFor(r.URL.Path)
		switch {
		case strings.HasSuffix(r.URL.Path, "/containers/json"):
			if !assert.NoError(t, json.NewEncoder(w).Encode([]container.Summary{
				{ID: "fast", Image: imageRefs["fast"], ImageID: imageIDs["fast"], Labels: autoLabels},
				{ID: "slow", Image: imageRefs["slow"], ImageID: imageIDs["slow"], Labels: autoLabels},
			})) {
				return
			}
		case strings.Contains(r.URL.Path, "/images/"):
			if !assert.NoError(t, json.NewEncoder(w).Encode(image.InspectResponse{ID: imageIDs[name], RepoTags: []string{imageRefs[name]}})) {
				return
			}
		case strings.Contains(r.URL.Path, "/containers/"):
			if !assert.NoError(
				t,
				json.NewEncoder(w).Encode(container.InspectResponse{
					ID:    name,
					Image: imageIDs[name],
					Config: &container.Config{
						Image:  imageRefs[name],
						Labels: autoLabels,
					},
				}),
			) {
				return
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer dockerServer.Close()
	registryService := registry.NewContainerRegistryService(db, func(context.Context) (registry.RegistryDaemonClient, error) {
		return &fakeRegistryDaemonClient{distributionInspectFn: func(context.Context, string, client.DistributionInspectOptions) (client.DistributionInspectResult, error) {
			return client.DistributionInspectResult{}, errors.New("manifest not found")
		}}, nil
	}, nil, settingsService, registryServer.Client())
	svc := NewImageUpdateService(db, settingsService, registryService, &docker.DockerClientService{Client: newImageUpdateTestDockerClientInternal(t, dockerServer)}, nil, nil, nil)

	scanCtx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	_, err = svc.checkContainerTagUpdatesInternal(scanCtx, []string{imageRefs["fast"], imageRefs["slow"]}, nil)
	require.ErrorContains(t, err, context.DeadlineExceeded.Error())
	var records []ImageUpdateRecord
	require.NoError(t, db.Find(&records).Error)
	require.Len(t, records, 1, "only the check that finished before the scan deadline is stored")
	require.Equal(t, "container::fast", records[0].ID)
	require.True(t, records[0].HasUpdate)
	require.Equal(t, "1.1.0", mo.PointerToOption(records[0].LatestVersion).OrEmpty())
	require.Empty(t, mo.PointerToOption(records[0].LastError).OrEmpty(), "a stored result must not be overwritten by the cancellation")
}
