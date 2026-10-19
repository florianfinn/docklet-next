// Source: docklet-next/docs/design/runtime-control.md
// Revision: d4229b124f079376c959142b63235f6a739be2a9711e0228ec5a63ab552f8cf0
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Migrated SQLite and synthetic Docker HTTP boundary for real services.

package activity_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/build"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/container"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/internal/event"
	"github.com/florianfinn/docklet-next/backend/internal/image"
	"github.com/florianfinn/docklet-next/backend/internal/project"
	"github.com/florianfinn/docklet-next/backend/internal/registry"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	usertypes "github.com/florianfinn/docklet-next/types/user"
	"github.com/google/uuid"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/require"
)

const runtimeContainerID = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
const runtimeSecondContainerID = "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
const runtimeReplacementContainerID = "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"

type runtimeHTTPEvent struct {
	Method, Path, Query, Body string
}

type runtimeFixtureContainer struct {
	name    string
	labels  map[string]string
	running bool
}

type runtimeFixtureVolume struct {
	name   string
	labels map[string]string
}

type runtimeHTTPFixture struct {
	t                *testing.T
	mu               sync.Mutex
	events           []runtimeHTTPEvent
	labels           map[string]string
	composeContainer bool
	running          bool
	activeID         string
	extraContainers  map[string]runtimeFixtureContainer
	volume           *runtimeFixtureVolume
	onCall           func(http.ResponseWriter, *http.Request) bool
	server           *httptest.Server
}

func newRuntimeHTTPFixture(t *testing.T) *runtimeHTTPFixture {
	t.Helper()
	f := &runtimeHTTPFixture{t: t, labels: map[string]string{}, activeID: runtimeContainerID}
	f.server = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.server.Close)
	return f
}

func (f *runtimeHTTPFixture) serve(w http.ResponseWriter, r *http.Request) {
	body, readErr := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if readErr != nil {
		f.t.Errorf("read Docker HTTP request: %v", readErr)
		http.Error(w, "unreadable synthetic request", http.StatusBadRequest)
		return
	}
	f.mu.Lock()
	f.events = append(f.events, runtimeHTTPEvent{r.Method, r.URL.Path, r.URL.RawQuery, string(body)})
	labels := make(map[string]string, len(f.labels))
	for key, value := range f.labels {
		labels[key] = value
	}
	onCall := f.onCall
	composeContainer := f.composeContainer
	running := f.running
	activeID := f.activeID
	extra := make(map[string]runtimeFixtureContainer, len(f.extraContainers))
	for id, container := range f.extraContainers {
		extra[id] = container
	}
	var volume *runtimeFixtureVolume
	if f.volume != nil {
		copy := *f.volume
		volume = &copy
	}
	f.mu.Unlock()
	if onCall != nil && onCall(w, r) {
		return
	}
	w.Header().Set("Content-Type", "application/json")
	switch {
	case (r.Method == http.MethodHead || r.Method == http.MethodGet) && strings.HasSuffix(r.URL.Path, "/_ping"):
		w.WriteHeader(http.StatusOK)
	case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/version"):
		_ = json.NewEncoder(w).Encode(map[string]any{"Version": "28.0.0", "ApiVersion": "1.48", "MinAPIVersion": "1.41"})
	case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/containers/json"):
		if composeContainer && strings.Contains(r.URL.RawQuery, "com.docker.compose.project") && !strings.Contains(r.URL.RawQuery, "com.docker.compose.hook") {
			state, status := "exited", "Exited"
			if running {
				state, status = "running", "Up"
			}
			containers := []map[string]any{{"Id": activeID, "Names": []string{"/example_stack-app-1"}, "Image": "registry.example.com/app:1", "Labels": labels, "State": state, "Status": status}}
			for id, container := range extra {
				state, status := "exited", "Exited"
				if container.running {
					state, status = "running", "Up"
				}
				containers = append(containers, map[string]any{"Id": id, "Names": []string{container.name}, "Image": "registry.example.com/worker:1", "Labels": container.labels, "State": state, "Status": status})
			}
			_ = json.NewEncoder(w).Encode(containers)
		} else {
			_ = json.NewEncoder(w).Encode([]any{})
		}
	case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/volumes"):
		volumes := []any{}
		if volume != nil {
			volumes = append(volumes, map[string]any{"Name": volume.name, "Driver": "local", "Mountpoint": "/example/volume", "Labels": volume.labels})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"Volumes": volumes, "Warnings": []any{}})
	case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/volumes/"):
		if volume == nil || !strings.HasSuffix(r.URL.Path, "/"+volume.name) {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"Name": volume.name, "Driver": "local", "Mountpoint": "/example/volume", "Labels": volume.labels})
	case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/volumes/create"):
		var request struct {
			Name   string            `json:"Name"`
			Labels map[string]string `json:"Labels"`
		}
		if err := json.Unmarshal(body, &request); err != nil || request.Name == "" {
			f.t.Errorf("invalid volume create: %v", err)
			http.Error(w, "invalid volume", http.StatusBadRequest)
			return
		}
		f.mu.Lock()
		f.volume = &runtimeFixtureVolume{name: request.Name, labels: request.Labels}
		f.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{"Name": request.Name, "Driver": "local", "Mountpoint": "/example/volume", "Labels": request.Labels})
	case r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "/volumes/"):
		f.mu.Lock()
		f.volume = nil
		f.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/networks"):
		_ = json.NewEncoder(w).Encode([]any{})
	case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/images/") && strings.HasSuffix(r.URL.Path, "/json"):
		_ = json.NewEncoder(w).Encode(map[string]any{"Id": "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", "RepoTags": []string{"registry.example.com/app:1"}, "Config": map[string]any{}})
	case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/images/create"):
		_, _ = w.Write([]byte("{\"status\":\"Pull complete\"}\n"))
	case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/networks/"):
		http.NotFound(w, r)
	case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/networks/create"):
		_ = json.NewEncoder(w).Encode(map[string]any{"Id": "example-network-id"})
	case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/containers/create"):
		f.mu.Lock()
		if f.composeContainer {
			f.activeID = runtimeReplacementContainerID
		} else if f.activeID == "" {
			f.activeID = runtimeContainerID
		}
		f.composeContainer = true
		f.running = false
		createdID := f.activeID
		f.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{"Id": createdID, "Warnings": []string{}})
	case r.Method == http.MethodPost && (strings.HasSuffix(r.URL.Path, "/containers/"+runtimeContainerID+"/rename") || strings.HasSuffix(r.URL.Path, "/containers/"+runtimeReplacementContainerID+"/rename")):
		if r.URL.Query().Get("name") == "" {
			f.t.Error("missing replacement rename")
			http.Error(w, "missing name", http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/containers/"+runtimeContainerID+"/json"):
		state := "exited"
		if running {
			state = "running"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"Id": runtimeContainerID, "Name": "/example_stack-app-1", "Config": map[string]any{"Labels": labels}, "State": map[string]any{"Status": state, "Running": running}, "NetworkSettings": map[string]any{"Networks": map[string]any{}}})
	case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/containers/"+runtimeReplacementContainerID+"/json"):
		if activeID != runtimeReplacementContainerID {
			http.NotFound(w, r)
			return
		}
		state := "exited"
		if running {
			state = "running"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"Id": activeID, "Name": "/example_stack-app-1", "Config": map[string]any{"Labels": labels}, "State": map[string]any{"Status": state, "Running": running}, "NetworkSettings": map[string]any{"Networks": map[string]any{}}})
	case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/containers/"+runtimeSecondContainerID+"/json"):
		container, ok := extra[runtimeSecondContainerID]
		if !ok {
			http.NotFound(w, r)
			return
		}
		state := "exited"
		if container.running {
			state = "running"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"Id": runtimeSecondContainerID, "Name": container.name, "Config": map[string]any{"Labels": container.labels}, "State": map[string]any{"Status": state, "Running": container.running}, "NetworkSettings": map[string]any{"Networks": map[string]any{}}})
	case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/containers/") && strings.HasSuffix(r.URL.Path, "/json"):
		http.NotFound(w, r)
	case r.Method == http.MethodPost && (strings.HasSuffix(r.URL.Path, "/start") || strings.HasSuffix(r.URL.Path, "/stop") || strings.HasSuffix(r.URL.Path, "/restart")) && strings.Contains(r.URL.Path, "/containers/"):
		parts := strings.Split(r.URL.Path, "/")
		id := parts[len(parts)-2]
		if id != runtimeContainerID && id != runtimeSecondContainerID && id != runtimeReplacementContainerID {
			f.t.Errorf("unexpected container ID: %s", id)
			http.NotFound(w, r)
			return
		}
		f.mu.Lock()
		if id == runtimeContainerID || id == runtimeReplacementContainerID {
			if id == f.activeID {
				f.running = !strings.HasSuffix(r.URL.Path, "/stop")
			}
		} else {
			container := f.extraContainers[id]
			container.running = !strings.HasSuffix(r.URL.Path, "/stop")
			f.extraContainers[id] = container
		}
		f.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodDelete && strings.Contains(r.URL.Path, "/containers/"):
		f.mu.Lock()
		if strings.Contains(r.URL.Path, f.activeID) {
			f.composeContainer = false
			f.activeID = ""
		} else {
			delete(f.extraContainers, runtimeSecondContainerID)
		}
		f.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	default:
		f.t.Errorf("unexpected Docker HTTP request: %s %s", r.Method, r.URL.Path)
		http.Error(w, "unexpected synthetic request", http.StatusNotFound)
	}
}

func (f *runtimeHTTPFixture) snapshot() []runtimeHTTPEvent {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]runtimeHTTPEvent(nil), f.events...)
}

func (f *runtimeHTTPFixture) setLabels(labels map[string]string) {
	f.mu.Lock()
	f.labels = labels
	f.mu.Unlock()
}

func (f *runtimeHTTPFixture) setComposeContainer(present bool) {
	f.mu.Lock()
	f.composeContainer = present
	f.running = present
	f.activeID = runtimeContainerID
	f.mu.Unlock()
}

func (f *runtimeHTTPFixture) setSecondComposeContainer(labels map[string]string) {
	f.mu.Lock()
	f.extraContainers = map[string]runtimeFixtureContainer{runtimeSecondContainerID: {name: "/example_stack-worker-1", labels: labels, running: true}}
	f.mu.Unlock()
}

func (f *runtimeHTTPFixture) setVolumeHash(hash string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.volume == nil {
		f.t.Fatal("volume not created")
	}
	f.volume.labels["com.docker.compose.config-hash"] = hash
}

type runtimeServicesFixture struct {
	t          *testing.T
	db         *database.DB
	settings   *settings.SettingsService
	activity   *activity.ActivityService
	projects   *project.ProjectService
	containers *container.ContainerService
	adapter    *activity.RuntimeServiceAdapter
	runtime    *activity.RuntimeService
	http       *runtimeHTTPFixture
	dbPath     string
	root       string
	store      activity.RuntimeStoreBinding
}

func newRuntimeServicesFixture(t *testing.T) *runtimeServicesFixture {
	t.Helper()
	httpFixture := newRuntimeHTTPFixture(t)
	host, err := url.Parse(httpFixture.server.URL)
	require.NoError(t, err)
	t.Setenv("DOCKER_HOST", "tcp://"+host.Host)
	t.Setenv("DOCKER_CONFIG", t.TempDir())
	t.Setenv("DOCKER_CONTEXT", "default")
	t.Setenv("DOCKER_TLS_VERIFY", "")
	t.Setenv("DOCKER_CERT_PATH", "")
	ctx := t.Context()
	dbPath := filepath.Join(t.TempDir(), "runtime.db")
	db, err := database.InitializeHub(ctx, sqlitestore.URL(dbPath), database.MigrationOptions{})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	settingsService, err := settings.NewSettingsService(ctx, db)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, settingsService.Stop(context.WithoutCancel(ctx))) })
	root := filepath.Join(t.TempDir(), "projects")
	require.NoError(t, os.MkdirAll(root, 0755))
	require.NoError(t, settingsService.SetStringSetting(ctx, "projectsDirectory", root))
	cfg := &config.Config{DockerHost: "tcp://" + host.Host, ProjectsDirectory: root, Environment: config.AppEnvironmentTest}
	apiClient, err := client.New(client.WithHost(httpFixture.server.URL), client.WithAPIVersion("1.41"), client.WithHTTPClient(httpFixture.server.Client()))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, apiClient.Close()) })
	dockerService := docker.NewDockerClientService(ctx, db, cfg, settingsService).WithClient(apiClient)
	events := event.NewEventService(db, cfg, httpFixture.server.Client())
	activityService := activity.NewActivityService(db, settingsService)
	registries := registry.NewContainerRegistryService(db, nil, nil, settingsService)
	images := image.NewImageService(db, dockerService, registries, nil, nil, events, settingsService, activityService)
	builds := build.NewBuildService(settingsService, dockerService, registries, events)
	projects := project.NewProjectService(db, settingsService, events, images, dockerService, builds, registries, cfg, nil, nil)
	containers := container.NewContainerService(events, dockerService, images, settingsService, projects)
	adapter := activity.NewRuntimeServiceAdapter(projects, containers)
	bootstrap := activity.NewRuntimeService(activityService, adapter)
	generation, err := bootstrap.InitializeGeneration(ctx)
	require.NoError(t, err)
	store := activity.RuntimeStoreBinding{StoreGeneration: generation, ExecutorGeneration: generation, ManagerID: "example-manager", RegistrationID: "example-registration", EnvironmentID: "0"}
	runtime := activity.NewRuntimeService(activityService, adapter, activity.RuntimeLocalConfig{ApplicationContext: context.Background(), Store: store, Executor: adapter})
	return &runtimeServicesFixture{t: t, db: db, settings: settingsService, activity: activityService, projects: projects,
		containers: containers, adapter: adapter, runtime: runtime, http: httpFixture, dbPath: dbPath, root: root, store: store}
}

func (f *runtimeServicesFixture) command(operation activity.RuntimeOperation, target activity.RuntimeTarget) activity.RuntimeCommand {
	return activity.RuntimeCommand{Store: f.store, Key: uuid.NewString(), Actor: usertypes.Actor{ID: "example-owner", Username: "Operator"}, Operation: operation, Target: target}
}

func (f *runtimeServicesFixture) project(id, dir, body string) string {
	f.t.Helper()
	path := filepath.Join(f.root, dir)
	require.NoError(f.t, os.MkdirAll(path, 0755))
	require.NoError(f.t, os.WriteFile(filepath.Join(path, "compose.yaml"), []byte(body), 0644))
	row := &project.Project{Name: dir, DirName: &dir, Path: path, Status: project.ProjectStatusStopped}
	row.ID = id
	require.NoError(f.t, f.db.Create(row).Error)
	return path
}

func runtimeEffectEvents(events []runtimeHTTPEvent) []runtimeHTTPEvent {
	var effects []runtimeHTTPEvent
	for _, event := range events {
		if event.Method == http.MethodPost && (strings.Contains(event.Path, "/start") || strings.Contains(event.Path, "/stop") ||
			strings.Contains(event.Path, "/restart") || strings.Contains(event.Path, "/create") || strings.Contains(event.Path, "/remove")) {
			effects = append(effects, event)
		}
	}
	return effects
}

func (f *runtimeServicesFixture) run(command activity.RuntimeCommand) string {
	f.t.Helper()
	admitted, err := f.runtime.Admit(f.t.Context(), command)
	require.NoError(f.t, err)
	require.NotEmpty(f.t, admitted.ActivityID)
	snapshot, err := f.waitLocal(admitted.ActivityID)
	require.NoError(f.t, err)
	require.Equal(f.t, "success", string(snapshot.Status), f.debugEvents())
	return admitted.ActivityID
}

func (f *runtimeServicesFixture) waitLocal(runID string) (activitytypes.RuntimeSnapshot, error) {
	ctx, cancel := context.WithTimeout(f.t.Context(), 10*time.Second)
	defer cancel()
	return f.runtime.WaitLocal(ctx, activity.RuntimeRunReference{Store: f.store, RunID: runID})
}

func (f *runtimeServicesFixture) debugEvents() string {
	return fmt.Sprint(f.http.snapshot())
}
