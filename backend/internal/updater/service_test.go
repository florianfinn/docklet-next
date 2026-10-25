// Source: getarcaneapp/arcane/backend/internal/updater/service_test.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed APNS mobile push; functional cut: removed GitOps and saved Git repositories; SQLite temp-file persistence fixture.

package updater

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"errors"

	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	activitytypes "github.com/florianfinn/docklet-next/types/activity"

	"github.com/florianfinn/docklet-next/types/scheduler"
	arcaneupdater "github.com/florianfinn/docklet-next/types/updater"
	usertypes "github.com/florianfinn/docklet-next/types/user"

	"github.com/moby/moby/api/types/container"
	dockertypesimage "github.com/moby/moby/api/types/image"

	"github.com/moby/moby/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.getarcane.app/kit/pkg"
	"go.getarcane.app/sys/crypto"
	"go.getarcane.app/updater"
	"go.getarcane.app/updater/labels"

	updatertypes "go.getarcane.app/updater/types"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/internal/event"
	"github.com/florianfinn/docklet-next/backend/internal/image"
	"github.com/florianfinn/docklet-next/backend/internal/imageupdate"
	"github.com/florianfinn/docklet-next/backend/internal/kv"
	"github.com/florianfinn/docklet-next/backend/internal/notification"
	"github.com/florianfinn/docklet-next/backend/internal/project"
	"github.com/florianfinn/docklet-next/backend/internal/registry"

	activitylib "github.com/florianfinn/docklet-next/backend/pkg/libarcane/activity"

	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/jobcontext"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/runs"
	"github.com/florianfinn/docklet-next/backend/pkg/utils"
	francistest "github.com/florianfinn/docklet-next/backend/pkg/utils/francis/testing"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/notifications"
)

type fakeDockerClientProviderInternal struct {
	client any
}

func (f fakeDockerClientProviderInternal) DockerClient(context.Context) (*client.Client, error) {
	cli, _ := f.client.(*client.Client)
	if cli == nil {
		return nil, errors.New("docker client unavailable")
	}
	return cli, nil
}

type fakeImagePullerInternal struct {
	pulled []string
	fail   map[string]error
}

func (f *fakeImagePullerInternal) PullImage(_ context.Context, imageRef string, _ io.Writer) error {
	f.pulled = append(f.pulled, imageRef)
	if f.fail == nil {
		return nil
	}
	return f.fail[imageRef]
}

type fakeRunRecorderInternal struct {
	results []updater.ResourceResult
}

func (f *fakeRunRecorderInternal) RecordUpdateRun(_ context.Context, result updater.ResourceResult) error {
	f.results = append(f.results, result)
	return nil
}

type fakeSettingsProviderInternal struct{}

func (fakeSettingsProviderInternal) ExcludedContainers(context.Context) ([]string, error) {
	return nil, nil
}

type fakeUsedImageCollectorInternal struct {
	images map[string]struct{}
}

func (f fakeUsedImageCollectorInternal) UsedImages(context.Context) (map[string]struct{}, error) {
	return f.images, nil
}

type fakeProjectUpdaterInternal struct {
	projects     map[string]updater.ComposeProject
	updateErrs   map[string]error
	calls        []string
	imageChanges map[string]map[string]updatertypes.ServiceImageChange
}

func (f *fakeProjectUpdaterInternal) ProjectByComposeName(_ context.Context, composeName string) (updater.ComposeProject, error) {
	if localProject, ok := f.projects[composeName]; ok {
		return localProject, nil
	}
	return updater.ComposeProject{}, errors.New("project not found")
}

func (f *fakeProjectUpdaterInternal) UpdateServices(_ context.Context, projectID string, services []string) error {
	f.calls = append(f.calls, projectID+":"+strings.Join(services, ","))
	if f.updateErrs == nil {
		return nil
	}
	return f.updateErrs[projectID]
}

func (f *fakeProjectUpdaterInternal) UpdateServiceImages(_ context.Context, projectID string, changes map[string]updatertypes.ServiceImageChange) error {
	if f.imageChanges == nil {
		f.imageChanges = map[string]map[string]updatertypes.ServiceImageChange{}
	}
	f.imageChanges[projectID] = changes
	services := slices.Sorted(maps.Keys(changes))
	f.calls = append(f.calls, projectID+":"+strings.Join(services, ","))
	return f.updateErrs[projectID]
}

func newUpdaterApplyPendingDockerServerInternal(
	t *testing.T,
	containers []container.Summary,
	verificationByService map[string][]container.Summary,
	inspectByID map[string]container.InspectResponse,
	imageInspectByRef map[string]dockertypesimage.InspectResponse,
) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/_ping"):
			_, _ = io.WriteString(w, "OK")
		case strings.HasSuffix(r.URL.Path, "/version"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.MarshalWrite(w, map[string]string{
				"ApiVersion":    "1.41",
				"MinAPIVersion": "1.24",
				"Version":       "24.0.0",
			})
		case strings.HasSuffix(r.URL.Path, "/images/json"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.MarshalWrite(w, []dockertypesimage.Summary{})
		case strings.Contains(r.URL.Path, "/images/") && strings.HasSuffix(r.URL.Path, "/json"):
			encodedRef := strings.TrimSuffix(r.URL.Path[strings.LastIndex(r.URL.Path, "/images/")+len("/images/"):], "/json")
			imageRef, err := url.PathUnescape(encodedRef)
			if !assert.NoError(t, err) {
				return
			}
			inspect, ok := imageInspectByRef[imageRef]
			if !ok {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.MarshalWrite(w, inspect)
		case strings.HasSuffix(r.URL.Path, "/containers/json"):
			response := containers
			if filters := strings.TrimSpace(r.URL.Query().Get("filters")); filters != "" {
				var raw map[string]map[string]bool
				if !assert.NoError(t, json.Unmarshal([]byte(filters), &raw)) {
					return
				}
				projectName := ""
				serviceName := ""
				for value := range raw["label"] {
					switch {
					case strings.HasPrefix(value, "com.docker.compose.project="):
						projectName = strings.TrimPrefix(value, "com.docker.compose.project=")
					case strings.HasPrefix(value, "com.docker.compose.service="):
						serviceName = strings.TrimPrefix(value, "com.docker.compose.service=")
					}
				}
				if projectName != "" && serviceName != "" {
					if matched, ok := verificationByService[projectName+"/"+serviceName]; ok {
						response = matched
					} else {
						response = nil
					}
				}
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.MarshalWrite(w, response)
		case strings.Contains(r.URL.Path, "/containers/") && strings.HasSuffix(r.URL.Path, "/json"):
			containerID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path[strings.LastIndex(r.URL.Path, "/containers/"):], "/containers/"), "/json")
			inspect, ok := inspectByID[containerID]
			if !ok {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.MarshalWrite(w, inspect)
		default:
			http.NotFound(w, r)
		}
	}))

	t.Cleanup(server.Close)
	return server
}

type mockSystemUpgradeServiceInternal struct {
	triggerCalled  bool
	triggerError   error
	capturedUser   *usertypes.Actor
	capturedTarget *updater.SelfUpdateTarget
}

func (m *mockSystemUpgradeServiceInternal) TriggerUpgradeViaCLI(_ context.Context, user usertypes.Actor, target updater.SelfUpdateTarget) (string, error) {
	m.triggerCalled = true
	m.capturedUser = &user
	m.capturedTarget = &target
	return "upgrader-container-id", m.triggerError
}

func TestUpdaterService_ApplyPendingNoRecordsInternal(t *testing.T) {
	ctx := t.Context()
	db := setupProjectTestDBInternal(t)
	svc, svcErr := NewUpdaterService(db, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	require.NoError(t, svcErr)

	var checkpoints []scheduler.TargetOutcome
	ctx = jobcontext.WithExecution(ctx, scheduler.Run{AttemptCount: 1}, func(target scheduler.TargetOutcome) error { checkpoints = append(checkpoints, target); return nil })
	result, err := svc.ApplyPending(ctx, arcaneupdater.Options{DryRun: true})
	require.Len(t, checkpoints, 2)
	assert.Equal(t, scheduler.Running, checkpoints[0].Status)
	assert.Equal(t, scheduler.Succeeded, checkpoints[1].Status)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Zero(t, result.Checked)
	assert.Zero(t, result.Updated)
	assert.Zero(t, result.Skipped)
	assert.Zero(t, result.Failed)
	assert.Empty(t, result.Items)
	ctx = jobcontext.WithExecution(t.Context(), scheduler.Run{AttemptCount: 1}, func(scheduler.TargetOutcome) error { return errors.New("checkpoint unavailable") })
	_, err = svc.ApplyPending(ctx, arcaneupdater.Options{DryRun: true})
	require.ErrorContains(t, err, "checkpoint unavailable")
	var outcomeErr *scheduler.OutcomeError
	require.ErrorAs(t, err, &outcomeErr)
	assert.Equal(t, scheduler.NeedsAttention, outcomeErr.Outcome.Status)
	checkpoints = nil
	ctx = jobcontext.WithExecution(
		t.Context(),
		scheduler.Run{AttemptCount: 1},
		func(target scheduler.TargetOutcome) error {
			checkpoints = append(
				checkpoints,
				target,
			)
			return nil
		},
	)
	svc.engine = nil
	require.Panics(t, func() { _, _ = svc.ApplyPending(ctx, arcaneupdater.Options{}) })
	require.NotEmpty(t, checkpoints)
	assert.Equal(t, scheduler.NeedsAttention, checkpoints[len(checkpoints)-1].Status)
}

// Type and ResourceIds are Arcane-side scoping the engine never acted on;
// ApplyPending handles a scoped request itself before reaching the engine.
// A pending run, forced or scoped, never lifts settings exclusions: that
// override belongs to UpdateSingleContainer alone.
func TestUpdaterService_ModuleOptionsFromUpdaterOptionsInternal(t *testing.T) {
	got := moduleOptionsFromUpdaterOptionsInternal(arcaneupdater.Options{
		Type:        string(updater.ResourceTypeContainer),
		ResourceIds: []string{"container-1", "container-2"},
		ForceUpdate: true,
		DryRun:      true,
	})

	assert.Equal(t, updater.Options{Force: true, DryRun: true}, got)
	assert.False(t, got.IgnoreSettingsExclusions)
}

func TestUpdaterService_ResultFromModulePreservesRestartedInternal(t *testing.T) {
	got := resultFromModuleInternal(&updater.Result{
		Success:   true,
		Checked:   2,
		Updated:   1,
		Restarted: 1,
		Items: []updater.ResourceResult{
			{
				ResourceID:    "web-id",
				ResourceName:  "web",
				ResourceType:  updater.ResourceTypeContainer,
				Status:        updater.StatusRestarted,
				UpdateApplied: true,
			},
		},
	})

	require.NotNil(t, got)
	assert.Equal(t, 1, got.Restarted)
	require.Len(t, got.Items, 1)
	assert.Equal(t, arcaneupdater.StatusRestarted, got.Items[0].Status)
	assert.True(t, got.Items[0].UpdateApplied)
}

func TestUpdaterService_TriggerSelfUpdateViaCLIInternal(t *testing.T) {
	ctx := t.Context()

	t.Run("server label triggers upgrade with system user", func(t *testing.T) {
		mockUpgrade := &mockSystemUpgradeServiceInternal{}
		svc, svcErr := NewUpdaterService(nil, nil, nil, nil, nil, nil, nil, nil, nil, mockUpgrade, nil, nil, nil, nil, nil)
		require.NoError(t, svcErr)

		err := svc.TriggerSelfUpdateViaCLI(ctx, "test", "container-1", "arcane", map[string]string{
			labels.LabelArcane: "true",
		})

		require.NoError(t, err)
		assert.True(t, mockUpgrade.triggerCalled)
		require.NotNil(t, mockUpgrade.capturedUser)
		assert.Equal(t, usertypes.SystemUser.ID, mockUpgrade.capturedUser.ID)
		assert.Equal(t, usertypes.SystemUser.Username, mockUpgrade.capturedUser.Username)
		require.NotNil(t, mockUpgrade.capturedTarget)
		assert.Equal(t, "container-1", mockUpgrade.capturedTarget.ContainerID)
		assert.Equal(t, "arcane", mockUpgrade.capturedTarget.ContainerName)
		assert.Empty(t, mockUpgrade.capturedTarget.NewImageRef)
		assert.Empty(t, mockUpgrade.capturedTarget.PullImageRef)
	})

	t.Run("engine target keeps the selected tag and pulls the frozen digest", func(t *testing.T) {
		mockUpgrade := &mockSystemUpgradeServiceInternal{}
		svc, svcErr := NewUpdaterService(nil, nil, nil, nil, nil, nil, nil, nil, nil, mockUpgrade, nil, nil, nil, nil, nil)
		require.NoError(t, svcErr)

		frozen := svc.recovery.WithSingleTarget(ctx, &arcaneupdater.FrozenUpdateTarget{
			ContainerID:     "container-1",
			DesiredImageRef: "ghcr.io/getarcaneapp/arcane:next",
			DesiredDigest:   "sha256:frozen",
		}, func() error { return nil })
		err := svc.TriggerSelfUpdate(frozen, updater.SelfUpdateTarget{
			ContainerID:   "container-1",
			ContainerName: "arcane",
			InstanceType:  "agent",
			NewImageRef:   "ghcr.io/getarcaneapp/arcane:next",
		})

		require.NoError(t, err)
		require.NotNil(t, mockUpgrade.capturedTarget)
		assert.Equal(t, "ghcr.io/getarcaneapp/arcane:next", mockUpgrade.capturedTarget.NewImageRef)
		assert.Equal(t, "ghcr.io/getarcaneapp/arcane@sha256:frozen", mockUpgrade.capturedTarget.PullImageRef)
	})

	t.Run("agent label triggers upgrade", func(t *testing.T) {
		mockUpgrade := &mockSystemUpgradeServiceInternal{}
		svc, svcErr := NewUpdaterService(nil, nil, nil, nil, nil, nil, nil, nil, nil, mockUpgrade, nil, nil, nil, nil, nil)
		require.NoError(t, svcErr)

		err := svc.TriggerSelfUpdateViaCLI(ctx, "test", "container-1", "arcane-agent", map[string]string{
			labels.LabelArcaneAgent: "true",
		})

		require.NoError(t, err)
		assert.True(t, mockUpgrade.triggerCalled)
	})

	t.Run("non Arcane labels fail without triggering upgrade", func(t *testing.T) {
		mockUpgrade := &mockSystemUpgradeServiceInternal{}
		svc, svcErr := NewUpdaterService(nil, nil, nil, nil, nil, nil, nil, nil, nil, mockUpgrade, nil, nil, nil, nil, nil)
		require.NoError(t, svcErr)

		err := svc.TriggerSelfUpdateViaCLI(ctx, "test", "container-1", "demo", map[string]string{
			"com.example.app": "demo",
		})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "not an Arcane self-update target")
		assert.False(t, mockUpgrade.triggerCalled)
	})

	t.Run("missing upgrade service reports required hook", func(t *testing.T) {
		svc, svcErr := NewUpdaterService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
		require.NoError(t, svcErr)

		err := svc.TriggerSelfUpdateViaCLI(ctx, "test", "container-1", "arcane", map[string]string{
			labels.LabelArcane: "true",
		})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "self-update requires CLI upgrade service")
	})

	t.Run("upgrade errors are wrapped", func(t *testing.T) {
		mockUpgrade := &mockSystemUpgradeServiceInternal{triggerError: errors.New("upgrade failed")}
		svc, svcErr := NewUpdaterService(nil, nil, nil, nil, nil, nil, nil, nil, nil, mockUpgrade, nil, nil, nil, nil, nil)
		require.NoError(t, svcErr)

		err := svc.TriggerSelfUpdateViaCLI(ctx, "test", "container-1", "arcane", map[string]string{
			labels.LabelArcane: "true",
		})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "CLI upgrade failed")
	})
}

func TestUpdaterService_StatusTrackingInternal(t *testing.T) {
	svc, svcErr := NewUpdaterService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	require.NoError(t, svcErr)

	stopContainer := svc.BeginContainerUpdate("container-1")
	stopProject := svc.BeginProjectUpdate("project-a")

	status := svc.GetStatus()
	assert.Equal(t, 1, status.UpdatingContainers)
	assert.Equal(t, 1, status.UpdatingProjects)
	assert.Equal(t, []string{"container-1"}, status.ContainerIds)
	assert.Equal(t, []string{"project-a"}, status.ProjectIds)

	stopContainer()
	stopProject()

	status = svc.GetStatus()
	assert.Zero(t, status.UpdatingContainers)
	assert.Zero(t, status.UpdatingProjects)
	assert.Empty(t, status.ContainerIds)
	assert.Empty(t, status.ProjectIds)
}

func TestUpdaterService_DockerClientAdapterInternal(t *testing.T) {
	ctx := t.Context()
	t.Run("active engine updates correlate before the stop event is recorded", func(t *testing.T) {
		events := event.NewEventService(nil, nil, nil)
		svc, err := NewUpdaterService(nil, nil, nil, nil, nil, nil, events, nil, nil, nil, nil, nil, nil, nil, nil)
		require.NoError(t, err)
		end := svc.engineInternal().BeginContainerUpdate("updating-container")
		require.True(t, events.ShouldSuppressDaemonEvent("container", "updating-container", "", ""))
		require.False(t, events.ShouldSuppressDaemonEvent("container", "unrelated-container", "", ""))
		require.False(t, events.ShouldSuppressDaemonEvent("image", "updating-container", "", ""))
		end()
		require.False(t, events.ShouldSuppressDaemonEvent("container", "updating-container", "", ""))
	})

	t.Run("missing docker service returns unavailable error", func(t *testing.T) {
		svc, svcErr := NewUpdaterService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
		require.NoError(t, svcErr)

		cli, err := svc.DockerClient(ctx)

		require.Error(t, err)
		assert.Nil(t, cli)
		assert.Contains(t, err.Error(), "docker service unavailable")
	})

	t.Run("delegates to configured docker service", func(t *testing.T) {
		server := newImagePullServerWithObserverInternal(t, nil, nil)
		wantClient := newTestDockerClientInternal(t, server)
		dockerSvc := docker.NewDockerClientService(t.Context(), nil, nil, nil).WithClient(wantClient)
		svc, svcErr := NewUpdaterService(nil, nil, dockerSvc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
		require.NoError(t, svcErr)

		gotClient, err := svc.DockerClient(ctx)

		require.NoError(t, err)
		assert.Same(t, wantClient, gotClient)
	})
}

func TestUpdaterService_PullImageAdapterInternal(t *testing.T) {
	ctx := t.Context()

	t.Run("missing image service returns unavailable error", func(t *testing.T) {
		svc, svcErr := NewUpdaterService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
		require.NoError(t, svcErr)

		err := svc.PullImage(ctx, "registry.example.com/app:1.2.3", nil)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "image service unavailable")
	})

	t.Run("delegates to Arcane image puller", func(t *testing.T) {
		db := setupProjectTestDBInternal(t)
		server := newImagePullServerWithObserverInternal(t, nil, nil)
		dockerSvc := docker.NewDockerClientService(t.Context(), nil, nil, nil).WithClient(newTestDockerClientInternal(t, server))
		imageSvc := image.NewImageService(db, dockerSvc, nil, nil, nil, event.NewEventService(db, nil, nil), nil, nil)
		svc, svcErr := NewUpdaterService(db, nil, dockerSvc, nil, nil, nil, nil, imageSvc, nil, nil, nil, nil, nil, nil, nil)
		require.NoError(t, svcErr)
		runtime := francistest.New(t)
		gate := runs.NewAdmission(runtime.Service(), t.Name())
		require.NoError(t, gate.Register(runtime))
		francistest.Start(t, runtime)
		imageSvc.SetImageAdmission(gate, nil)
		var progress bytes.Buffer

		err := svc.PullImage(ctx, "nginx:latest", &progress)

		require.NoError(t, err)
		assert.Contains(t, progress.String(), "Pulling image")
	})

	t.Run("passes database registry credentials to Arcane image puller", func(t *testing.T) {
		db := setupProjectTestDBInternal(t)
		require.NoError(t, db.AutoMigrate(&registry.ContainerRegistry{}, &kv.KVEntry{}))
		crypto.InitEncryption(&crypto.Config{
			Environment:   string(config.AppEnvironmentTest),
			EncryptionKey: "test-encryption-key-for-testing-32bytes-min",
		})
		createTestPullRegistryInternal(t, db, "https://registry.example.com", "db-user", "db-token")

		imageRef := "registry.example.com/team/app:latest"
		var gotAuth string
		server := newImagePullServerWithObserverInternal(t, nil, func(fullRef, authHeader string) {
			if fullRef == imageRef {
				gotAuth = authHeader
			}
		})
		dockerSvc := docker.NewDockerClientService(t.Context(), nil, nil, nil).WithClient(newTestDockerClientInternal(t, server))
		registrySvc := registry.NewContainerRegistryService(db, nil, kv.NewKVService(db), nil)
		imageSvc := image.NewImageService(db, dockerSvc, registrySvc, nil, nil, event.NewEventService(db, nil, nil), nil, nil)
		envSvc := environment.NewEnvironmentService(db, nil, nil, nil, nil, nil)
		projectSvc := project.NewProjectService(db, nil, nil, nil, nil, nil, nil, nil, nil, nil)
		projectSvc.RegistryCredentialsProvider = envSvc.GetEnabledRegistryCredentials
		svc, svcErr := NewUpdaterService(db, nil, dockerSvc, projectSvc, nil, nil, nil, imageSvc, nil, nil, nil, nil, nil, nil, nil)
		require.NoError(t, svcErr)
		runtime := francistest.New(t)
		gate := runs.NewAdmission(runtime.Service(), t.Name())
		require.NoError(t, gate.Register(runtime))
		francistest.Start(t, runtime)
		imageSvc.SetImageAdmission(gate, nil)
		var progress bytes.Buffer

		err := svc.PullImage(ctx, imageRef, &progress)

		require.NoError(t, err)
		require.NotEmpty(t, gotAuth)
		authConfig := decodeRegistryAuthInternal(t, gotAuth)
		assert.Equal(t, "db-user", authConfig.Username)
		assert.Equal(t, "db-token", authConfig.Password)
		assert.Contains(t, progress.String(), "Pulling image")
	})
}

func TestUpdaterService_PendingImageUpdatesAdapterInternal(t *testing.T) {
	ctx := t.Context()
	db := setupProjectTestDBInternal(t)
	latest := "1.2.4"
	currentDigest := "sha256:1111111111111111111111111111111111111111111111111111111111111111"
	latestDigest := "sha256:2222222222222222222222222222222222222222222222222222222222222222"
	lastError := "previous check failed"
	checkTime := time.Now().Add(-time.Hour).UTC()
	require.NoError(t, db.Create(&imageupdate.ImageUpdateRecord{
		ID:             "pending",
		Repository:     "registry.example.com/team/app",
		Tag:            "1.2.3",
		HasUpdate:      true,
		UpdateType:     imageupdate.UpdateTypeTag,
		CurrentVersion: "1.2.3",
		LatestVersion:  &latest,
		CurrentDigest:  &currentDigest,
		LatestDigest:   &latestDigest,
		CheckTime:      checkTime,
		LastError:      &lastError,
	}).Error)
	require.NoError(t, db.Create(&imageupdate.ImageUpdateRecord{
		ID:         "not-pending",
		Repository: "registry.example.com/team/old",
		Tag:        "1.0.0",
		HasUpdate:  false,
		UpdateType: imageupdate.UpdateTypeDigest,
		CheckTime:  checkTime,
	}).Error)
	svc, svcErr := NewUpdaterService(db, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	require.NoError(t, svcErr)

	records, err := svc.PendingImageUpdates(ctx)

	require.NoError(t, err)
	require.Len(t, records, 1)
	assert.Equal(t, "pending", records[0].ID)
	assert.Equal(t, "registry.example.com/team/app", records[0].Repository)
	assert.Equal(t, "1.2.3", records[0].Tag)
	assert.True(t, records[0].HasUpdate)
	assert.Equal(t, updater.UpdateTypeTag, records[0].UpdateType)
	assert.Equal(t, "1.2.3", records[0].CurrentVersion)
	assert.Equal(t, &latest, records[0].LatestVersion)
	assert.Equal(t, &currentDigest, records[0].CurrentDigest)
	assert.Equal(t, &latestDigest, records[0].LatestDigest)
	assert.Equal(t, &lastError, records[0].LastError)
}

// The notification flush needs an empty container listing.
func newEmptyContainerListDockerServiceInternal(t *testing.T) *docker.DockerClientService {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/containers/json") {
			if !assert.NoError(t, json.MarshalWrite(w, []container.Summary{})) {
				return
			}
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(server.Close)
	return &docker.DockerClientService{Client: newTestDockerClientInternal(t, server)}
}

// PendingImageUpdates must not hand records to the engine when the flush could
// not determine what to send (#3532): consuming them would lose the pending
// notification for good instead of retrying once Docker is reachable again.
func TestUpdaterService_PendingImageUpdatesAbortsWhenNotificationEligibilityUnresolvedInternal(t *testing.T) {
	db := setupProjectTestDBInternal(t)
	require.NoError(t, db.AutoMigrate(&imageupdate.ImageUpdateRecord{}, &notification.NotificationSettings{}))
	notif := notification.NewNotificationService(db, nil, nil, nil)
	imageUpdates := imageupdate.NewImageUpdateService(db, nil, nil, nil, nil, notif, nil)
	svc, err := NewUpdaterService(db, nil, nil, nil, imageUpdates, nil, nil, nil, notif, nil, nil, nil, nil, nil, nil)
	require.NoError(t, err)
	require.NoError(t, db.Create(&imageupdate.ImageUpdateRecord{ID: "sha256:pending", Repository: "test/repo", Tag: "latest", HasUpdate: true}).Error)

	records, err := svc.PendingImageUpdates(t.Context())
	require.ErrorContains(t, err, "flush pending update notifications")
	require.Nil(t, records)
	var reloaded imageupdate.ImageUpdateRecord
	require.NoError(t, db.First(&reloaded, "id = ?", "sha256:pending").Error)
	require.True(t, reloaded.HasUpdate)
	require.False(t, reloaded.NotificationSent)
}

// issue #3132: the updater consumes and clears has_update records without checking
// notification_sent, so an "Updates Available" notification pending at consumption
// time was silently lost. PendingImageUpdates must flush it before the engine runs.
func TestUpdaterService_PendingImageUpdatesFlushesPendingNotificationsInternal(t *testing.T) {
	ctx := t.Context()
	db := setupProjectTestDBInternal(t)
	require.NoError(t, db.AutoMigrate(&imageupdate.ImageUpdateRecord{}, &notification.NotificationSettings{}))

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
	imageUpdates := imageupdate.NewImageUpdateService(db, nil, nil, newEmptyContainerListDockerServiceInternal(t), nil, notif, nil)
	svc, svcErr := NewUpdaterService(db, nil, nil, nil, imageUpdates, nil, nil, nil, notif, nil, nil, nil, nil, nil, nil)
	require.NoError(t, svcErr)

	require.NoError(t, db.Create(&imageupdate.ImageUpdateRecord{
		ID:               "sha256:pending-unnotified",
		Repository:       "test/repo",
		Tag:              "latest",
		HasUpdate:        true,
		NotificationSent: false,
	}).Error)

	records, err := svc.PendingImageUpdates(ctx)

	require.NoError(t, err)
	require.Len(t, records, 1)
	require.EqualValues(t, 1, calls.Load())
	var reloaded imageupdate.ImageUpdateRecord
	require.NoError(t, db.First(&reloaded, "id = ?", "sha256:pending-unnotified").Error)
	assert.True(t, reloaded.NotificationSent)
}

// With no providers configured, PendingImageUpdates still returns the records and
// leaves notification_sent false (preserves the #3079 "don't mark when nothing
// delivered" semantics).
func TestUpdaterService_PendingImageUpdatesNoProvidersLeavesUnnotifiedInternal(t *testing.T) {
	ctx := t.Context()
	db := setupProjectTestDBInternal(t)
	require.NoError(t, db.AutoMigrate(&imageupdate.ImageUpdateRecord{}, &notification.NotificationSettings{}))

	notif := notification.NewNotificationService(db, nil, nil, nil)
	imageUpdates := imageupdate.NewImageUpdateService(db, nil, nil, newEmptyContainerListDockerServiceInternal(t), nil, notif, nil)
	svc, svcErr := NewUpdaterService(db, nil, nil, nil, imageUpdates, nil, nil, nil, notif, nil, nil, nil, nil, nil, nil)
	require.NoError(t, svcErr)

	require.NoError(t, db.Create(&imageupdate.ImageUpdateRecord{
		ID:               "sha256:pending-no-provider",
		Repository:       "test/repo",
		Tag:              "latest",
		HasUpdate:        true,
		NotificationSent: false,
	}).Error)

	records, err := svc.PendingImageUpdates(ctx)

	require.NoError(t, err)
	require.Len(t, records, 1)
	var reloaded imageupdate.ImageUpdateRecord
	require.NoError(t, db.First(&reloaded, "id = ?", "sha256:pending-no-provider").Error)
	assert.False(t, reloaded.NotificationSent)
}

func TestUpdaterService_RecordUpdateRunAdapterInternal(t *testing.T) {
	ctx := t.Context()
	db := setupProjectTestDBInternal(t)
	require.NoError(t, db.AutoMigrate(&AutoUpdateRecord{}))
	svc, svcErr := NewUpdaterService(db, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	require.NoError(t, svcErr)

	err := svc.RecordUpdateRun(ctx, updater.ResourceResult{
		ResourceID:      "container-1",
		ResourceName:    "web",
		ResourceType:    updater.ResourceTypeContainer,
		Status:          updater.StatusUpdated,
		UpdateAvailable: true,
		UpdateApplied:   true,
		OldImage:        "nginx:1.2.3",
		NewImage:        "nginx:1.2.4",
		Details:         map[string]any{"source": "test"},
	})

	require.NoError(t, err)
	var record AutoUpdateRecord
	require.NoError(t, db.First(&record, "resource_id = ?", "container-1").Error)
	assert.Equal(t, "web", record.ResourceName)
	assert.Equal(t, "container", record.ResourceType)
	assert.Equal(t, AutoUpdateStatus(updater.StatusUpdated), record.Status)
	assert.True(t, record.UpdateAvailable)
	assert.True(t, record.UpdateApplied)
	assert.Equal(t, "nginx:1.2.3", record.OldImageVersions["main"])
	assert.Equal(t, "nginx:1.2.4", record.NewImageVersions["main"])
	assert.Equal(t, "test", record.Details["source"])
	progress := &updateProgressInternal{}
	ctx = context.WithValue(ctx, updateProgressKeyInternal{}, progress)
	ctx = jobcontext.WithExecution(ctx, scheduler.Run{AttemptCount: 1}, func(target scheduler.TargetOutcome) error {
		assert.Equal(t, scheduler.Failed, target.Status)
		return errors.New("progress unavailable")
	})
	err = svc.RecordUpdateRun(ctx, updater.ResourceResult{ResourceID: "failed-container", ResourceType: updater.ResourceTypeContainer, Status: updater.StatusFailed})
	require.ErrorContains(t, err, "progress unavailable")
	require.ErrorContains(t, progress.err, "progress unavailable")
}

func TestUpdaterService_RecordUpdateRunAppendsActivityMessageInternal(t *testing.T) {
	ctx := t.Context()
	db := setupProjectTestDBInternal(t)
	require.NoError(t, db.AutoMigrate(append(activity.Models(), &AutoUpdateRecord{})...))
	activityService := activity.NewActivityService(db, nil)
	svc, svcErr := NewUpdaterService(db, nil, nil, nil, nil, nil, nil, nil, nil, nil, activityService, nil, nil, nil, nil)
	require.NoError(t, svcErr)

	for _, tt := range []struct {
		name        string
		result      updater.ResourceResult
		wantLevel   activitytypes.MessageLevel
		wantMessage string
	}{
		{
			name: "failed with reason",
			result: updater.ResourceResult{
				ResourceID:   "c1",
				ResourceName: "sonarr",
				ResourceType: updater.ResourceTypeContainer,
				Status:       updater.StatusFailed,
				Error:        "pull failed: unexpected EOF",
			},
			wantLevel:   activitytypes.MessageLevelError,
			wantMessage: "sonarr: pull failed: unexpected EOF",
		},
		{
			name:        "failed without reason falls back to status",
			result:      updater.ResourceResult{ResourceID: "c2", ResourceName: "dozzle", ResourceType: updater.ResourceTypeContainer, Status: updater.StatusFailed},
			wantLevel:   activitytypes.MessageLevelError,
			wantMessage: "dozzle: failed",
		},
		{
			name: "updated includes image change",
			result: updater.ResourceResult{
				ResourceID:   "c3",
				ResourceName: "maintainerr",
				ResourceType: updater.ResourceTypeContainer,
				Status:       updater.StatusUpdated,
				OldImage:     "sha256:old",
				NewImage:     "ghcr.io/maintainerr/maintainerr:latest",
			},
			wantLevel:   activitytypes.MessageLevelInfo,
			wantMessage: "maintainerr: updated (sha256:old -> ghcr.io/maintainerr/maintainerr:latest)",
		},
		{
			name: "skipped unnamed uses resource id and reason",
			result: updater.ResourceResult{
				ResourceID:   "arcane-id",
				ResourceType: updater.ResourceTypeContainer,
				Status:       updater.StatusSkipped,
				Error:        "container excluded by settings",
			},
			wantLevel:   activitytypes.MessageLevelInfo,
			wantMessage: "arcane-id: skipped (container excluded by settings)",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			started, err := activityService.StartActivity(ctx, activitylib.StartRequest{EnvironmentID: "0", Type: activitytypes.TypeAutoUpdate})
			require.NoError(t, err)

			require.NoError(t, svc.RecordUpdateRun(contextWithActivityIDInternal(ctx, started.ID), tt.result))

			detail, err := activityService.GetActivityDetail(ctx, "0", started.ID, 10)
			require.NoError(t, err)
			require.Len(t, detail.Messages, 1)
			assert.Equal(t, tt.wantLevel, detail.Messages[0].Level)
			assert.Equal(t, tt.wantMessage, detail.Messages[0].Message)
		})
	}
}

func TestUpdaterService_AcceptSingleContainerUpdateUsesQueuedActivityInternal(t *testing.T) {
	for _, cancelActivity := range []bool{false, true} {
		name := kit.Ternary(cancelActivity, "activity cancellation while queued", "request disconnect")
		t.Run(name, func(t *testing.T) {
			db := setupProjectTestDBInternal(t)
			require.NoError(t, db.AutoMigrate(activity.Models()...))
			sqlDB, err := db.DB.DB()
			require.NoError(t, err)
			sqlDB.SetMaxOpenConns(1)
			activityService := activity.NewActivityService(db, nil)
			runtime := francistest.New(t)
			admission := runs.NewAdmission(runtime.Service(), t.Name())
			require.NoError(t, admission.Register(runtime))
			svc, err := NewUpdaterService(db, nil, nil, nil, nil, nil, nil, nil, nil, nil, activityService, nil, nil, admission, nil)
			require.NoError(t, err)
			require.NoError(t, svc.RegisterActors(runtime))
			francistest.Start(t, runtime)

			// Holding the update admission makes the executor decline the
			// dispatched job until the lease is released.
			held, admitted, err := admission.TryAcquire(t.Context(), scheduler.AdmissionKey{Scope: "updater"})
			require.NoError(t, err)
			require.True(t, admitted)
			var releaseOnce sync.Once
			release := func() { releaseOnce.Do(func() { held.Release(t.Context()) }) }
			defer release()

			requestCtx, cancelRequest := context.WithCancel(t.Context())
			workCtx := utils.ActivityRuntimeContext(requestCtx, t.Context())
			type acceptedInternal struct {
				activity *activitytypes.Activity
				err      error
			}
			accepted := make(chan acceptedInternal, 1)
			go func() {
				item, acceptErr := svc.AcceptSingleContainerUpdate(workCtx, "missing-container")
				accepted <- acceptedInternal{activity: item, err: acceptErr}
			}()
			var item *activitytypes.Activity
			select {
			case result := <-accepted:
				require.NoError(t, result.err)
				item = result.activity
			case <-time.After(time.Second):
				require.FailNow(t, "acceptance waited for executor work")
			}
			require.Equal(t, activitytypes.StatusQueued, item.Status)
			cancelRequest()
			if cancelActivity {
				_, err = activityService.CancelActivity(t.Context(), "0", item.ID, "test")
				require.NoError(t, err)
			}
			release()

			wantStatus := kit.Ternary(cancelActivity, activitytypes.StatusCancelled, activitytypes.StatusFailed)
			require.Eventually(t, func() bool {
				var saved activity.Activity
				return db.First(&saved, "id = ?", item.ID).Error == nil && saved.Status == wantStatus
			}, 5*time.Second, 10*time.Millisecond)
		})
	}
}
