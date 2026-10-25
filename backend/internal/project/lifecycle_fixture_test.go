// Source: docklet-next/backend/internal/image/safety_test.go
// Revision: 5a7a7ad39c1817a3d222bc84c0cf91d433d48f0d
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Migrated SQLite, real project files, shared image lease and controlled Docker effects.

package project

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	compose "github.com/compose-spec/compose-go/v2/types"
	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/internal/event"
	"github.com/florianfinn/docklet-next/backend/internal/image"
	life "github.com/florianfinn/docklet-next/backend/internal/project/children/lifecycle"
	registryService "github.com/florianfinn/docklet-next/backend/internal/registry"
	"github.com/florianfinn/docklet-next/backend/pkg/projects"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/runs"
	francistest "github.com/florianfinn/docklet-next/backend/pkg/utils/francis/testing"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	activityDTO "github.com/florianfinn/docklet-next/types/activity"
	projectDTO "github.com/florianfinn/docklet-next/types/project"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/google/uuid"
	"github.com/moby/moby/api/types/registry"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.getarcane.app/sys/crypto"
)

const lifecycleOldID = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
const lifecycleNewID = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
const lifecycleDigest = "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
const lifecycleContainerID = "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
const lifecycleOriginal = "# operator comment\nservices:\n  app:\n    image: registry.example.com/app:1.0.0 # preserve comment\n    env_file: service.env\n"

type lifecycleWorkload struct {
	ID, Service, Project, Reference, ImageID string
	Running                                  bool
}
type lifecycleFixture struct {
	t                         *testing.T
	stopSignals               []string
	service                   *ProjectService
	db                        *database.DB
	activity                  *activity.ActivityService
	runtime                   *activity.RuntimeService
	binding                   activity.RuntimeStoreBinding
	project                   *Project
	root, registration        string
	mu                        sync.Mutex
	tags                      map[string]string
	workloads                 map[string]lifecycleWorkload
	pulls, tagWrites, deploys int
	up                        func(context.Context, *compose.Project) error
	onPull                    func()
	onInspect                 func(string)
	normalDeploy              bool
}

func newLifecycleFixture(t *testing.T) *lifecycleFixture {
	t.Helper()
	crypto.InitEncryption(&crypto.Config{Environment: "test", EncryptionKey: "synthetic-lifecycle-encryption-key-32bytes"})
	db, err := database.Initialize(t.Context(), sqlitestore.URL(filepath.Join(t.TempDir(), "lifecycle.db")), database.MigrationOptions{})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	f := &lifecycleFixture{t: t, db: db, tags: map[string]string{"registry.example.com/app:1.0.0": lifecycleOldID, lifecycleOldID: lifecycleOldID, lifecycleNewID: lifecycleNewID}, workloads: map[string]lifecycleWorkload{}}
	server := httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(server.Close)
	cli, err := client.New(client.WithHost(server.URL), client.WithAPIVersion("1.48"), client.WithHTTPClient(server.Client()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = cli.Close() })
	daemon := &docker.DockerClientService{Client: cli}
	settingsService, err := newSettingsServiceForTest(t, t.Context(), db)
	require.NoError(t, err)
	f.root = t.TempDir()
	require.NoError(t, settingsService.SetStringSetting(t.Context(), "projectsDirectory", f.root))
	f.registration = settingsService.GetSettingsConfig().InstanceID.Value
	events := event.NewEventService(db, nil, nil)
	f.activity = activity.NewActivityService(db, nil)
	images := image.NewImageService(db, daemon, nil, nil, nil, events, settingsService, f.activity)
	framework := francistest.New(t)
	gate := runs.NewAdmission(framework.Service(), t.Name())
	require.NoError(t, gate.Register(framework))
	francistest.Start(t, framework)
	images.SetImageAdmission(gate, nil)
	registries := registryService.NewContainerRegistryService(db, nil, nil, nil)
	cfg := config.Load()
	f.service = NewProjectService(db, settingsService, events, images, daemon, nil, registries, cfg, nil, nil)
	path := filepath.Join(f.root, "example-stack")
	require.NoError(t, os.Mkdir(path, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(path, "compose.yaml"), []byte(lifecycleOriginal), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(path, "service.env"), []byte("SYNTHETIC_VALUE=original\n"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(path, ".env"), []byte("STACK_INPUT=original\n"), 0600))
	f.project = &Project{BaseModel: database.BaseModel{ID: uuid.NewString()}, Name: "example-stack", Path: path, DirName: new("example-stack")}
	require.NoError(t, db.Create(f.project).Error)
	f.workloads["app"] = lifecycleWorkload{ID: lifecycleContainerID, Service: "app", Project: "example-stack", Reference: "registry.example.com/app:1.0.0", ImageID: lifecycleOldID, Running: true}
	f.service.composeCoordinator = projects.NewCoordinator(projectDTO.ComposeCommands{Up: func(ctx context.Context, model *compose.Project, _ []string, _ bool, _ bool, _ bool, _ map[string]registry.AuthConfig, _ time.Duration) error {
		f.mu.Lock()
		f.deploys++
		hook := f.up
		f.mu.Unlock()
		if hook != nil {
			if err := hook(ctx, model); err != nil {
				return err
			}
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		for key, svc := range model.Services {
			if !f.normalDeploy {
				assert.Equal(t, "never", svc.PullPolicy)
			}
			assert.Nil(t, svc.Build)
			id := lifecycleContainerID
			if key != "app" {
				id = fmt.Sprintf("%x", sha256.Sum256([]byte(key)))
			}
			f.workloads[key] = lifecycleWorkload{ID: id, Service: key, Project: model.Name, Reference: svc.Image, ImageID: f.tags[svc.Image], Running: true}
		}
		return nil
	}})
	adapter := activity.NewRuntimeServiceAdapter(f.service, nil)
	f.runtime = activity.NewRuntimeService(f.activity, adapter)
	generation, err := f.runtime.InitializeGeneration(t.Context())
	require.NoError(t, err)
	f.binding, err = f.runtime.LocalBinding(t.Context(), generation, f.registration)
	require.NoError(t, err)
	require.NoError(t, f.runtime.ConfigureLocal(activity.RuntimeLocalConfig{ApplicationContext: t.Context(), Store: f.binding, Executor: adapter}))
	return f
}
func (f *lifecycleFixture) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	path := r.URL.Path
	if len(path) > 2 && path[1] == 'v' {
		if i := strings.Index(path[1:], "/"); i >= 0 {
			path = path[i+1:]
		}
	}
	switch {
	case path == "/_ping":
		w.Header().Set("API-Version", "1.48")
		_, _ = fmt.Fprint(w, "OK")
	case path == "/containers/json":
		rows := []map[string]any{}
		for _, c := range f.workloads {
			state := "exited"
			if c.Running {
				state = "running"
			}
			rows = append(rows, map[string]any{"Id": c.ID, "Image": c.Reference, "ImageID": c.ImageID, "State": state, "Labels": map[string]string{"com.docker.compose.project": c.Project, "com.docker.compose.service": c.Service}})
		}
		_ = json.NewEncoder(w).Encode(rows)
	case strings.HasPrefix(path, "/containers/") && strings.HasSuffix(path, "/json"):
		id := strings.TrimSuffix(strings.TrimPrefix(path, "/containers/"), "/json")
		for _, c := range f.workloads {
			if c.ID == id {
				_ = json.NewEncoder(w).Encode(map[string]any{"Id": c.ID, "Image": c.ImageID, "State": map[string]any{"Running": c.Running}, "Config": map[string]any{"Image": c.Reference, "Labels": map[string]string{"com.docker.compose.project": c.Project, "com.docker.compose.service": c.Service}}})
				return
			}
		}
		w.WriteHeader(404)
	case strings.HasPrefix(path, "/images/") && strings.HasSuffix(path, "/json"):
		ref, _ := url.PathUnescape(strings.TrimSuffix(strings.TrimPrefix(path, "/images/"), "/json"))
		hook := f.onInspect
		if hook != nil {
			f.mu.Unlock()
			hook(ref)
			f.mu.Lock()
		}
		id, ok := f.tags[ref]
		if !ok {
			w.WriteHeader(404)
			_, _ = fmt.Fprint(w, `{"message":"not found"}`)
			return
		}
		tags := []string{}
		for tag, target := range f.tags {
			if target == id && !strings.HasPrefix(tag, "sha256:") {
				tags = append(tags, tag)
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"Id": id, "Os": "linux", "Architecture": "amd64", "RepoTags": tags, "RepoDigests": []string{}})
	case strings.HasPrefix(path, "/images/") && strings.HasSuffix(path, "/tag"):
		source, _ := url.PathUnescape(strings.TrimSuffix(strings.TrimPrefix(path, "/images/"), "/tag"))
		target := r.URL.Query().Get("repo")
		if tag := r.URL.Query().Get("tag"); tag != "" {
			target += ":" + tag
		}
		id, ok := f.tags[source]
		if !ok {
			w.WriteHeader(404)
			return
		}
		f.tags[target] = id
		f.tagWrites++
		w.WriteHeader(201)
	case path == "/images/create":
		f.pulls++
		ref := r.URL.Query().Get("fromImage")
		tag := r.URL.Query().Get("tag")
		if tag != "" {
			if strings.HasPrefix(tag, "sha256:") {
				ref += "@" + tag
			} else {
				ref += ":" + tag
			}
		}
		f.tags[ref] = lifecycleNewID
		hook := f.onPull
		if hook != nil {
			f.mu.Unlock()
			hook()
			f.mu.Lock()
		}
		_, _ = fmt.Fprint(w, `{"status":"downloaded"}`)
	case r.Method == http.MethodDelete && strings.HasPrefix(path, "/images/"):
		ref, _ := url.PathUnescape(strings.TrimPrefix(path, "/images/"))
		delete(f.tags, ref)
		_, _ = fmt.Fprintf(w, `[{"Untagged":%q}]`, ref)
	case strings.HasPrefix(path, "/containers/") && strings.HasSuffix(path, "/stop"):
		f.stopSignals = append(f.stopSignals, r.URL.Query().Get("signal"))
		for key, c := range f.workloads {
			if !strings.Contains(path, c.ID) {
				continue
			}
			c.Running = false
			f.workloads[key] = c
		}
		w.WriteHeader(204)
	case strings.HasPrefix(path, "/containers/") && r.Method == http.MethodDelete:
		id := strings.TrimPrefix(path, "/containers/")
		for key, c := range f.workloads {
			if c.ID == id {
				delete(f.workloads, key)
			}
		}
		w.WriteHeader(204)
	default:
		f.t.Errorf("unexpected Docker call %s %s", r.Method, path)
		w.WriteHeader(500)
	}
}
func (f *lifecycleFixture) establishBaseline() *life.Snapshot {
	f.t.Helper()
	_, snap, err := f.service.lifecycleCurrent(f.t.Context(), f.project.ID, f.registration)
	require.NoError(f.t, err)
	snap.RunID = uuid.NewString()
	require.NoError(f.t, f.service.lifecycleStore().SaveBaseline(f.t.Context(), snap))
	return snap
}
func (f *lifecycleFixture) intent(kind string) *dto.LifecycleIntent {
	f.t.Helper()
	_, current, err := f.service.lifecycleCurrent(f.t.Context(), f.project.ID, f.registration)
	require.NoError(f.t, err)
	_, revision, err := f.service.lifecycleStore().Baseline(f.t.Context(), current)
	require.NoError(f.t, err)
	ref := "registry.example.com/app:1.0.0"
	if kind == "version" {
		ref = "registry.example.com/app:1.1.0"
	}
	return &dto.LifecycleIntent{Request: dto.LifecycleRequest{ExpectedRevision: current.SavedRevision}, TargetIdentity: current.TargetIdentity, SavedRevision: current.SavedRevision, ObservedRevision: lifecycleObserved(current), BaselineRevision: revision, Selections: []dto.ImageSelection{{Service: "app", SourceReference: "registry.example.com/app:1.0.0", Reference: ref, Digest: lifecycleDigest, ImageID: lifecycleNewID, Platform: "linux/amd64", Kind: kind}}}
}
func (f *lifecycleFixture) admit(intent *dto.LifecycleIntent) activity.RuntimeAdmissionResult {
	f.t.Helper()
	operation := activity.RuntimeUpdateProject
	if intent.Request.PointID != "" {
		operation = activity.RuntimeRestoreProject
	}
	result, err := f.runtime.Admit(f.t.Context(), activity.RuntimeCommand{Store: f.binding, Key: uuid.NewString(), Actor: user.Actor{ID: "operator", Username: "operator"}, Operation: operation, Target: activity.RuntimeTarget{Kind: "stack", ID: f.project.ID, Name: new("example-stack")}, Options: activity.RuntimeOptions{Lifecycle: intent}})
	require.NoError(f.t, err)
	return result
}
func (f *lifecycleFixture) wait(result activity.RuntimeAdmissionResult) activityDTO.RuntimeSnapshot {
	f.t.Helper()
	out, err := f.runtime.WaitLocal(f.t.Context(), activity.RuntimeRunReference{Store: f.binding, RunID: result.ActivityID})
	require.NoError(f.t, err)
	return out
}

func clearLifecycleKeyForTest(t *testing.T) {
	t.Helper()
	crypto.InitEncryption(&crypto.Config{Environment: "test", EncryptionKey: ""})
	t.Cleanup(func() {
		crypto.InitEncryption(&crypto.Config{Environment: "test", EncryptionKey: "synthetic-lifecycle-encryption-key-32bytes"})
	})
}
