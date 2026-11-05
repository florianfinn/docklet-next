// Source: docklet-next/issues/252
// Revision: 89f69980c01d08714ce96c49365fb19065e3d6ba
// Retrieved: 2026-11-05
// License: Apache-2.0
// Deviations: Original real runtime startup reconciliation on reopened hub and agent stores.

package di

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	compose "github.com/compose-spec/compose-go/v2/types"
	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/container"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/internal/event"
	"github.com/florianfinn/docklet-next/backend/internal/image"
	"github.com/florianfinn/docklet-next/backend/internal/project"
	life "github.com/florianfinn/docklet-next/backend/internal/project/children/lifecycle"
	"github.com/florianfinn/docklet-next/backend/internal/registry"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/runs"
	francistest "github.com/florianfinn/docklet-next/backend/pkg/utils/francis/testing"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	activityDTO "github.com/florianfinn/docklet-next/types/activity"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/google/uuid"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/require"
	"go.getarcane.app/sys/crypto"
	"go.uber.org/fx/fxtest"
)

type startupLifecycleServices struct {
	db           *database.DB
	settings     *settings.SettingsService
	projects     *project.ProjectService
	containers   *container.ContainerService
	environments *environment.EnvironmentService
	activity     *activity.ActivityService
}

func startupLifecycleOpen(t *testing.T, path, root string, cfg *config.Config, daemonURL string) startupLifecycleServices {
	t.Helper()
	var db *database.DB
	var err error
	if cfg.AgentMode {
		db, err = database.Initialize(t.Context(), sqlitestore.URL(path), database.MigrationOptions{})
	} else {
		db, err = database.InitializeHub(t.Context(), sqlitestore.URL(path), database.MigrationOptions{})
	}
	require.NoError(t, err)
	settingsService, err := settings.NewSettingsService(t.Context(), db)
	require.NoError(t, err)
	require.NoError(t, settingsService.SetStringSetting(t.Context(), "projectsDirectory", root))
	cli, err := client.New(client.WithHost(daemonURL), client.WithAPIVersion("1.48"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, cli.Close()) })
	daemon := &docker.DockerClientService{Client: cli}
	events := event.NewEventService(db, cfg, nil)
	activities := activity.NewActivityService(db, settingsService)
	images := image.NewImageService(db, daemon, nil, nil, nil, events, settingsService, activities)
	framework := francistest.New(t)
	gate := runs.NewAdmission(framework.Service(), t.Name()+uuid.NewString())
	require.NoError(t, gate.Register(framework))
	francistest.Start(t, framework)
	images.SetImageAdmission(gate, nil)
	registries := registry.NewContainerRegistryService(db, nil, nil, settingsService)
	projects := project.NewProjectService(db, settingsService, events, images, daemon, nil, registries, cfg, nil, nil)
	containers := container.NewContainerService(events, daemon, images, settingsService, projects)
	environments := environment.NewEnvironmentService(db, nil, daemon, events, settingsService, nil)
	return startupLifecycleServices{db, settingsService, projects, containers, environments, activities}
}
func (s startupLifecycleServices) close(t *testing.T) {
	t.Helper()
	require.NoError(t, s.settings.Stop(context.WithoutCancel(t.Context())))
	require.NoError(t, s.db.Close())
}

func TestRuntimeStartupReconcilesDeployPointsOnHubAndAgent(t *testing.T) {
	for _, mode := range []string{"hub", "agent", "edge"} {
		for _, hadEffect := range []bool{false, true} {
			t.Run(mode+map[bool]string{false: "/before-effect", true: "/after-effect"}[hadEffect], func(t *testing.T) {
				crypto.InitEncryption(&crypto.Config{Environment: "test", EncryptionKey: "synthetic-lifecycle-encryption-key-32bytes"})
				imageID := "sha256:" + strings.Repeat("a", 64)
				tags := map[string]string{imageID: imageID}
				daemon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					path := r.URL.Path
					if i := strings.Index(path[1:], "/"); i >= 0 {
						path = path[i+1:]
					}
					w.Header().Set("Content-Type", "application/json")
					switch {
					case strings.HasPrefix(path, "/images/") && strings.HasSuffix(path, "/json"):
						ref, _ := url.PathUnescape(strings.TrimSuffix(strings.TrimPrefix(path, "/images/"), "/json"))
						id, ok := tags[ref]
						if !ok {
							w.WriteHeader(404)
							return
						}
						_ = json.NewEncoder(w).Encode(map[string]any{"Id": id, "Os": "linux", "Architecture": "amd64", "RepoTags": []string{}})
					case strings.HasSuffix(path, "/tag"):
						tags[r.URL.Query().Get("repo")+":"+r.URL.Query().Get("tag")] = imageID
						w.WriteHeader(201)
					default:
						http.NotFound(w, r)
					}
				}))
				defer daemon.Close()
				root := t.TempDir()
				path := filepath.Join(t.TempDir(), "executor.db")
				cfg := &config.Config{AgentMode: mode != "hub", EdgeAgent: mode == "edge", ProjectsDirectory: root, Environment: config.AppEnvironmentTest}
				prior := startupLifecycleOpen(t, path, root, cfg, daemon.URL)
				adapter := activity.NewRuntimeServiceAdapter(prior.projects, prior.containers)
				oldRuntime := activity.NewRuntimeService(prior.activity, adapter)
				generation, err := oldRuntime.InitializeGeneration(t.Context())
				require.NoError(t, err)
				if cfg.AgentMode {
					require.NoError(t, prior.db.Create(&activity.RuntimeExecutorBinding{ID: 1, ManagerID: "example-manager", RegistrationID: "example-registration"}).Error)
				}
				binding, err := oldRuntime.LocalBinding(t.Context(), generation, prior.settings.GetSettingsConfig().InstanceID.Value)
				require.NoError(t, err)
				projectPath := filepath.Join(root, "example-stack")
				require.NoError(t, os.Mkdir(projectPath, 0700))
				source := "services:\n  app:\n    image: registry.example.com/app:1.0.0\n"
				require.NoError(t, os.WriteFile(filepath.Join(projectPath, "compose.yaml"), []byte(source), 0600))
				p := project.Project{BaseModel: database.BaseModel{ID: uuid.NewString()}, Name: "example-stack", Path: projectPath, DirName: new("example-stack")}
				require.NoError(t, prior.db.Create(&p).Error)
				command := activity.RuntimeCommand{Store: binding, Key: uuid.NewString(), Actor: user.Actor{ID: "operator", Username: "operator"}, Operation: activity.RuntimeDeployProject, Target: activity.RuntimeTarget{Kind: "stack", ID: p.ID, Name: &p.Name}}
				admitted, err := oldRuntime.Admit(t.Context(), command)
				require.NoError(t, err)
				require.NoError(t, prior.db.First(&p, "id = ?", p.ID).Error)
				snapshot := &life.Snapshot{TargetID: p.ID, TargetIdentity: life.Hash([]any{p.ID, p.CreatedAt, p.Path}), RegistrationID: binding.RegistrationID, RunID: uuid.NewString(), SavedRevision: strings.Repeat("b", 64), Files: []life.File{{Path: "compose.yaml", Content: []byte(source), Present: true}}, Model: &compose.Project{Name: p.Name, WorkingDir: projectPath, ComposeFiles: []string{filepath.Join(projectPath, "compose.yaml")}, Services: compose.Services{"app": {Name: "app", Image: "registry.example.com/app:1.0.0"}}}, Services: map[string]life.ServiceImage{"app": {Reference: "registry.example.com/app:1.0.0", ImageID: imageID, Platform: "linux/amd64", Present: true, Running: true}}}
				store := life.Store{DB: prior.db}
				require.NoError(t, store.SaveBaseline(t.Context(), snapshot))
				prior.projects.PrepareDeployPoint(t.Context(), p.ID, admitted.ActivityID, binding.RegistrationID)
				var run life.Run
				require.NoError(t, prior.db.First(&run, "id = ?", admitted.ActivityID).Error)
				require.Nil(t, run.PointCode)
				require.NotEmpty(t, run.PointID)
				if hadEffect {
					require.NoError(t, store.Effect(t.Context(), admitted.ActivityID))
				}
				held, err := store.Held(t.Context(), run.PointID)
				require.NoError(t, err)
				require.True(t, held)
				prior.close(t)
				// New services, settings, SQLite handle and runtime lifetime; no prior sink survives.
				next := startupLifecycleOpen(t, path, root, cfg, daemon.URL)
				defer next.close(t)
				lifecycle := fxtest.NewLifecycle(t)
				result, err := provideRuntimeInternal(t.Context(), lifecycle, next.activity, next.projects, next.containers, next.environments, next.settings, cfg, http.DefaultClient, edge.NewTunnelRegistry(), next.db)
				require.NoError(t, err)
				require.NoError(t, lifecycle.Start(t.Context()))
				defer func() {
					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()
					require.NoError(t, lifecycle.Stop(ctx))
				}()
				require.Equal(t, binding, result.Service.LocalStore())
				var repaired life.Run
				require.NoError(t, next.db.First(&repaired, "id = ?", admitted.ActivityID).Error)
				require.True(t, repaired.Settled)
				var count int64
				require.NoError(t, next.db.Model(&life.Point{}).Where("id = ?", run.PointID).Count(&count).Error)
				require.Equal(t, map[bool]int64{false: 0, true: 1}[hadEffect], count, "startup reconciliation must remove only no-effect points")
				held, err = (life.Store{DB: next.db}).Held(t.Context(), run.PointID)
				require.NoError(t, err)
				require.False(t, held)
				out, err := result.Service.WaitLocal(t.Context(), activity.RuntimeRunReference{Store: binding, RunID: admitted.ActivityID})
				require.NoError(t, err)
				require.Equal(t, activityDTO.StatusFailed, out.Status)
				require.Equal(t, "runtime.interrupted", out.MessageCode)
				require.NotNil(t, out.ImageLifecycle)
				require.Equal(t, run.PointID, out.ImageLifecycle.RestorePointID)
			})
		}
	}
}
