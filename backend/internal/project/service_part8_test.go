// Source: getarcaneapp/arcane/backend/internal/project/service_test.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed central variables and .env.global; functional cut: removed GitOps and saved Git repositories; SQLite temp-file persistence fixture.

package project

import (
	"context"
	"encoding/json"
	"errors"

	"maps"
	"net/http"
	"net/http/httptest"

	"os"
	"path/filepath"

	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/compose-spec/compose-go/v2/types"

	imagetypes "github.com/florianfinn/docklet-next/types/image"
	projecttypes "github.com/florianfinn/docklet-next/types/project"
	usertypes "github.com/florianfinn/docklet-next/types/user"
	volumetypes "github.com/florianfinn/docklet-next/types/volume"

	"github.com/moby/moby/api/types/container"

	"github.com/moby/moby/api/types/volume"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.getarcane.app/updater/labels"
	updatertypes "go.getarcane.app/updater/types"

	"github.com/florianfinn/docklet-next/backend/internal/config"

	"github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/internal/event"
	"github.com/florianfinn/docklet-next/backend/internal/image"
	"github.com/florianfinn/docklet-next/backend/internal/imageupdate"
	"github.com/florianfinn/docklet-next/backend/internal/kv"

	projectdetails "github.com/florianfinn/docklet-next/backend/internal/project/children/details"

	"github.com/florianfinn/docklet-next/backend/internal/project/children/update"

	"github.com/florianfinn/docklet-next/backend/pkg/utils/imageref"
)

func TestProjectService_RecoverProjectRenameJournals_KeepsSourceCleanupPendingJournalWhenCleanupFails(t *testing.T) {
	db := setupProjectTestDB(t)
	require.NoError(t, db.AutoMigrate(&kv.KVEntry{}))
	ctx := t.Context()

	var sourceRemoveAttempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/volumes/web_data"):
			w.Header().Set("Content-Type", "application/json")
			if !assert.NoError(t, json.NewEncoder(w).Encode(volume.Volume{Name: "web_data"})) {
				return
			}
		case r.Method == http.MethodDelete && strings.HasSuffix(r.URL.Path, "/volumes/nginx_data"):
			sourceRemoveAttempts.Add(1)
			http.Error(w, "volume busy", http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	projectsDir := t.TempDir()
	oldDir := "nginx"
	newDir := "web"
	oldPath := filepath.Join(projectsDir, oldDir)
	newPath := filepath.Join(projectsDir, newDir)
	require.NoError(t, os.MkdirAll(newPath, 0o755))

	project := &Project{
		ID:      "proj-rename-source-cleanup-pending-fail",
		Name:    "web",
		DirName: &newDir,
		Path:    newPath,
		Status:  ProjectStatusStopped,
	}
	require.NoError(t, db.Create(project).Error)

	kvService := kv.NewKVService(db)
	dockerService := &docker.DockerClientService{Client: newTestDockerClient(t, server)}
	svc := NewProjectService(db, nil, nil, nil, dockerService, nil, nil, config.Load(), kvService, nil)
	journal := projecttypes.RenameJournal{
		ProjectID:  project.ID,
		OldName:    "nginx",
		NewName:    "web",
		OldPath:    oldPath,
		NewPath:    newPath,
		OldDirName: &oldDir,
		NewDirName: newDir,
		Phase:      projecttypes.RenameJournalPhaseSourceCleanupPending,
		Volumes: []volumetypes.JournalVolume{
			{
				Key:     "data",
				OldName: "nginx_data",
				NewName: "web_data",
			},
		},
	}
	payload, err := json.Marshal(journal)
	require.NoError(t, err)
	require.NoError(t, kvService.Set(ctx, projecttypes.RenameJournalKeyPrefix+project.ID, string(payload)))

	err = svc.RecoverProjectRenameJournals(ctx)
	require.Error(t, err)
	var cleanupErr *volumetypes.SourceCleanupError
	require.ErrorAs(t, err, &cleanupErr)
	require.Equal(t, "nginx_data", cleanupErr.SourceVolume)
	require.Positive(t, sourceRemoveAttempts.Load())

	raw, ok, err := kvService.Get(ctx, projecttypes.RenameJournalKeyPrefix+project.ID)
	require.NoError(t, err)
	require.True(t, ok)

	var updatedJournal projecttypes.RenameJournal
	require.NoError(t, json.Unmarshal([]byte(raw), &updatedJournal))
	require.Equal(t, projecttypes.RenameJournalPhaseSourceCleanupPending, updatedJournal.Phase)
	require.DirExists(t, newPath)
	require.NoDirExists(t, oldPath)

	var fromDB Project
	require.NoError(t, db.First(&fromDB, "id = ?", project.ID).Error)
	require.Equal(t, "web", fromDB.Name)
	require.Equal(t, newPath, fromDB.Path)
}

func TestProjectService_RecoverProjectRenameJournals_ClearsStartedJournalWhenDirectoriesAreMissing(t *testing.T) {
	db := setupProjectTestDB(t)
	require.NoError(t, db.AutoMigrate(&kv.KVEntry{}))
	ctx := t.Context()

	projectsDir := t.TempDir()
	oldDir := "nginx"
	newDir := "web"
	oldPath := filepath.Join(projectsDir, oldDir)
	newPath := filepath.Join(projectsDir, newDir)

	project := &Project{
		ID:      "proj-rename-missing-directories",
		Name:    "nginx",
		DirName: &oldDir,
		Path:    oldPath,
		Status:  ProjectStatusStopped,
	}
	require.NoError(t, db.Create(project).Error)

	kvService := kv.NewKVService(db)
	svc := NewProjectService(db, nil, nil, nil, nil, nil, nil, config.Load(), kvService, nil)
	journal := projecttypes.RenameJournal{
		ProjectID:  project.ID,
		OldName:    "nginx",
		NewName:    "web",
		OldPath:    oldPath,
		NewPath:    newPath,
		OldDirName: &oldDir,
		NewDirName: newDir,
		Phase:      projecttypes.RenameJournalPhaseStarted,
	}
	payload, err := json.Marshal(journal)
	require.NoError(t, err)
	require.NoError(t, kvService.Set(ctx, projecttypes.RenameJournalKeyPrefix+project.ID, string(payload)))

	require.NoError(t, svc.RecoverProjectRenameJournals(ctx))

	require.NoDirExists(t, oldPath)
	require.NoDirExists(t, newPath)
	_, ok, err := kvService.Get(ctx, projecttypes.RenameJournalKeyPrefix+project.ID)
	require.NoError(t, err)
	require.False(t, ok)
}

func TestProjectService_RecoverProjectRenameJournals_ClearsMissingPathJournalWhenTargetPreserved(t *testing.T) {
	db := setupProjectTestDB(t)
	require.NoError(t, db.AutoMigrate(&kv.KVEntry{}))
	ctx := t.Context()

	var targetRemoved atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/volumes/web_data"):
			w.Header().Set("Content-Type", "application/json")
			if !assert.NoError(t, json.NewEncoder(w).Encode(volume.Volume{Name: "web_data"})) {
				return
			}
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/volumes/nginx_data"):
			http.NotFound(w, r)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/containers/json"):
			w.Header().Set("Content-Type", "application/json")
			if !assert.NoError(t, json.NewEncoder(w).Encode([]container.Summary{})) {
				return
			}
		case r.Method == http.MethodDelete && strings.HasSuffix(r.URL.Path, "/volumes/web_data"):
			targetRemoved.Store(true)
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	projectsDir := t.TempDir()
	oldDir := "nginx"
	newDir := "web"
	oldPath := filepath.Join(projectsDir, oldDir)
	newPath := filepath.Join(projectsDir, newDir)

	project := &Project{
		ID:      "proj-rename-missing-path-preserved-target",
		Name:    "nginx",
		DirName: &oldDir,
		Path:    oldPath,
		Status:  ProjectStatusStopped,
	}
	require.NoError(t, db.Create(project).Error)

	kvService := kv.NewKVService(db)
	dockerService := &docker.DockerClientService{Client: newTestDockerClient(t, server)}
	svc := NewProjectService(db, nil, nil, nil, dockerService, nil, nil, config.Load(), kvService, nil)
	journal := projecttypes.RenameJournal{
		ProjectID:  project.ID,
		OldName:    "nginx",
		NewName:    "web",
		OldPath:    oldPath,
		NewPath:    newPath,
		OldDirName: &oldDir,
		NewDirName: newDir,
		Phase:      projecttypes.RenameJournalPhaseTargetsCopied,
		Volumes: []volumetypes.JournalVolume{
			{
				Key:     "data",
				OldName: "nginx_data",
				NewName: "web_data",
			},
		},
	}
	payload, err := json.Marshal(journal)
	require.NoError(t, err)
	require.NoError(t, kvService.Set(ctx, projecttypes.RenameJournalKeyPrefix+project.ID, string(payload)))

	require.NoError(t, svc.RecoverProjectRenameJournals(ctx))

	_, ok, err := kvService.Get(ctx, projecttypes.RenameJournalKeyPrefix+project.ID)
	require.NoError(t, err)
	require.False(t, ok)
	require.False(t, targetRemoved.Load(), "target volume may be the only complete copy and must stay when source restore fails")
	require.NoDirExists(t, oldPath)
	require.NoDirExists(t, newPath)

	var fromDB Project
	require.NoError(t, db.First(&fromDB, "id = ?", project.ID).Error)
	require.Equal(t, "nginx", fromDB.Name)
	require.Equal(t, oldPath, fromDB.Path)
}

func TestProjectService_RecoverProjectRenameJournals_ClearsJournalWhenRollbackSourceInspectFails(t *testing.T) {
	db := setupProjectTestDB(t)
	require.NoError(t, db.AutoMigrate(&kv.KVEntry{}))
	ctx := t.Context()

	var targetRemoved atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/volumes/nginx_data"):
			http.Error(w, "temporary docker error", http.StatusInternalServerError)
		case r.Method == http.MethodDelete && strings.HasSuffix(r.URL.Path, "/volumes/web_data"):
			targetRemoved.Store(true)
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	projectsDir := t.TempDir()
	oldDir := "nginx"
	newDir := "web"
	oldPath := filepath.Join(projectsDir, oldDir)
	newPath := filepath.Join(projectsDir, newDir)
	require.NoError(t, os.MkdirAll(oldPath, 0o755))

	project := &Project{
		ID:      "proj-rename-source-inspect-preserve",
		Name:    "nginx",
		DirName: &oldDir,
		Path:    oldPath,
		Status:  ProjectStatusStopped,
	}
	require.NoError(t, db.Create(project).Error)

	kvService := kv.NewKVService(db)
	dockerService := &docker.DockerClientService{Client: newTestDockerClient(t, server)}
	svc := NewProjectService(db, nil, nil, nil, dockerService, nil, nil, config.Load(), kvService, nil)
	journal := projecttypes.RenameJournal{
		ProjectID:  project.ID,
		OldName:    "nginx",
		NewName:    "web",
		OldPath:    oldPath,
		NewPath:    newPath,
		OldDirName: &oldDir,
		NewDirName: newDir,
		Phase:      projecttypes.RenameJournalPhaseTargetsCopied,
		Volumes: []volumetypes.JournalVolume{
			{
				Key:     "data",
				OldName: "nginx_data",
				NewName: "web_data",
			},
		},
	}
	payload, err := json.Marshal(journal)
	require.NoError(t, err)
	require.NoError(t, kvService.Set(ctx, projecttypes.RenameJournalKeyPrefix+project.ID, string(payload)))

	require.NoError(t, svc.RecoverProjectRenameJournals(ctx))

	_, ok, err := kvService.Get(ctx, projecttypes.RenameJournalKeyPrefix+project.ID)
	require.NoError(t, err)
	require.False(t, ok, "inspect uncertainty should not permanently block future renames")
	require.False(t, targetRemoved.Load(), "target volume must not be deleted when source inspection is uncertain")
	require.DirExists(t, oldPath)
	require.NoDirExists(t, newPath)

	var fromDB Project
	require.NoError(t, db.First(&fromDB, "id = ?", project.ID).Error)
	require.Equal(t, "nginx", fromDB.Name)
	require.Equal(t, oldPath, fromDB.Path)
}

func TestProjectService_RecoverProjectRenameJournals_ClearsJournalWhenRollbackTargetInspectFails(t *testing.T) {
	db := setupProjectTestDB(t)
	require.NoError(t, db.AutoMigrate(&kv.KVEntry{}))
	ctx := t.Context()

	var targetRemoved atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/volumes/nginx_data"):
			w.Header().Set("Content-Type", "application/json")
			if !assert.NoError(t, json.NewEncoder(w).Encode(volume.Volume{Name: "nginx_data"})) {
				return
			}
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/volumes/web_data"):
			http.Error(w, "temporary docker error", http.StatusInternalServerError)
		case r.Method == http.MethodDelete && strings.HasSuffix(r.URL.Path, "/volumes/web_data"):
			targetRemoved.Store(true)
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	projectsDir := t.TempDir()
	oldDir := "nginx"
	newDir := "web"
	oldPath := filepath.Join(projectsDir, oldDir)
	newPath := filepath.Join(projectsDir, newDir)
	require.NoError(t, os.MkdirAll(oldPath, 0o755))

	project := &Project{
		ID:      "proj-rename-target-inspect-preserve",
		Name:    "nginx",
		DirName: &oldDir,
		Path:    oldPath,
		Status:  ProjectStatusStopped,
	}
	require.NoError(t, db.Create(project).Error)

	kvService := kv.NewKVService(db)
	dockerService := &docker.DockerClientService{Client: newTestDockerClient(t, server)}
	svc := NewProjectService(db, nil, nil, nil, dockerService, nil, nil, config.Load(), kvService, nil)
	journal := projecttypes.RenameJournal{
		ProjectID:  project.ID,
		OldName:    "nginx",
		NewName:    "web",
		OldPath:    oldPath,
		NewPath:    newPath,
		OldDirName: &oldDir,
		NewDirName: newDir,
		Phase:      projecttypes.RenameJournalPhaseTargetsCopied,
		Volumes: []volumetypes.JournalVolume{
			{
				Key:     "data",
				OldName: "nginx_data",
				NewName: "web_data",
			},
		},
	}
	payload, err := json.Marshal(journal)
	require.NoError(t, err)
	require.NoError(t, kvService.Set(ctx, projecttypes.RenameJournalKeyPrefix+project.ID, string(payload)))

	require.NoError(t, svc.RecoverProjectRenameJournals(ctx))

	_, ok, err := kvService.Get(ctx, projecttypes.RenameJournalKeyPrefix+project.ID)
	require.NoError(t, err)
	require.False(t, ok, "inspect uncertainty should not permanently block future renames")
	require.False(t, targetRemoved.Load(), "target volume must not be deleted when target inspection is uncertain")
	require.DirExists(t, oldPath)
	require.NoDirExists(t, newPath)

	var fromDB Project
	require.NoError(t, db.First(&fromDB, "id = ?", project.ID).Error)
	require.Equal(t, "nginx", fromDB.Name)
	require.Equal(t, oldPath, fromDB.Path)
}

func TestProjectService_RecoverProjectRenameJournals_ClearsJournalWhenTargetPreservedAndDirectoryRolledBack(t *testing.T) {
	db := setupProjectTestDB(t)
	require.NoError(t, db.AutoMigrate(&kv.KVEntry{}))
	ctx := t.Context()

	var targetRemoved atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/volumes/web_data"):
			w.Header().Set("Content-Type", "application/json")
			if !assert.NoError(t, json.NewEncoder(w).Encode(volume.Volume{Name: "web_data"})) {
				return
			}
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/volumes/nginx_data"):
			http.NotFound(w, r)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/containers/json"):
			w.Header().Set("Content-Type", "application/json")
			if !assert.NoError(t, json.NewEncoder(w).Encode([]container.Summary{})) {
				return
			}
		case r.Method == http.MethodDelete && strings.HasSuffix(r.URL.Path, "/volumes/web_data"):
			targetRemoved.Store(true)
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	projectsDir := t.TempDir()
	oldDir := "nginx"
	newDir := "web"
	oldPath := filepath.Join(projectsDir, oldDir)
	newPath := filepath.Join(projectsDir, newDir)
	require.NoError(t, os.MkdirAll(newPath, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(newPath, "compose.yaml"), []byte("services: {}\n"), 0o600))

	project := &Project{
		ID:      "proj-rename-preserved-target-retry",
		Name:    "nginx",
		DirName: &oldDir,
		Path:    oldPath,
		Status:  ProjectStatusStopped,
	}
	require.NoError(t, db.Create(project).Error)

	kvService := kv.NewKVService(db)
	dockerService := &docker.DockerClientService{Client: newTestDockerClient(t, server)}
	svc := NewProjectService(db, nil, nil, nil, dockerService, nil, nil, config.Load(), kvService, nil)
	journal := projecttypes.RenameJournal{
		ProjectID:  project.ID,
		OldName:    "nginx",
		NewName:    "web",
		OldPath:    oldPath,
		NewPath:    newPath,
		OldDirName: &oldDir,
		NewDirName: newDir,
		Phase:      projecttypes.RenameJournalPhaseTargetsCopied,
		Volumes: []volumetypes.JournalVolume{
			{
				Key:     "data",
				OldName: "nginx_data",
				NewName: "web_data",
			},
		},
	}
	payload, err := json.Marshal(journal)
	require.NoError(t, err)
	require.NoError(t, kvService.Set(ctx, projecttypes.RenameJournalKeyPrefix+project.ID, string(payload)))

	err = svc.RecoverProjectRenameJournals(ctx)
	require.NoError(t, err)

	_, ok, err := kvService.Get(ctx, projecttypes.RenameJournalKeyPrefix+project.ID)
	require.NoError(t, err)
	require.False(t, ok, "preserved target data should not leave the project permanently blocked")
	require.False(t, targetRemoved.Load(), "target volume may be the only complete copy and must stay when source restore fails")
	require.FileExists(t, filepath.Join(oldPath, "compose.yaml"))
	require.NoDirExists(t, newPath)
}

func TestProjectPathMapperUsesCurrentSettings(t *testing.T) {
	db := setupProjectTestDB(t)
	settingsService, err := newSettingsServiceForTest(t, t.Context(), db)
	require.NoError(t, err)
	service := &ProjectService{settingsService: settingsService}
	service.initChildren(nil, nil)
	containerDir := t.TempDir()
	for _, hostDir := range []string{t.TempDir(), t.TempDir()} {
		require.NoError(t, settingsService.SetStringSetting(t.Context(), "projectsDirectory", containerDir+":"+hostDir))
		mapper := service.projectPathMapper(t.Context())
		require.NotNil(t, mapper)
		mapped, _, containerToHostErr := mapper.ContainerToHost(filepath.Join(containerDir, "data"))
		require.NoError(t, containerToHostErr)
		require.Equal(t, filepath.Join(hostDir, "data"), mapped)
	}
}

func TestPrepareProjectServiceImages(t *testing.T) {
	source := []byte(
		"\ufeff# operator configuration\r\n\r\nservices:\r\n  web:\r\n    image: \"app:${VERSION}\"   # keep this comment\r\n " +
			"   environment:\r\n      VERSION: ${VERSION}\r\n\r\n  worker:\r\n    image: 'app:1.2.0'  \r\n  tagged:\r\n    " +
			"image: !!str app:1.2.0\r\n  escaped:\r\n    image: \"app\\u003a1.2.0\"\r\n  untouched:\r\n    image: app:1.2.0 " +
			"# stays\r\n\r\r  plain:\r\n    image: app:1.2.0\r\n  qualified:\r\n    image: docker.io/library/app:1.2.0\r\n  " +
			"omitted:\r\n    image: registry.example.com:5000/team/app",
	)
	expected := "\ufeff# operator configuration\r\n\r\nservices:\r\n  web:\r\n    image: \"app:1.3.0\"   # keep this comment\r\n    " +
		"environment:\r\n      VERSION: ${VERSION}\r\n\r\n  worker:\r\n    image: 'app:1.3.0'  \r\n  " +
		"tagged:\r\n    image: !!str app:1.3.0\r\n  escaped:\r\n    image: \"app:1.3.0\"\r\n  untouched:\r\n    image: " +
		"app:1.2.0 # stays\r\n\r\r  plain:\r\n    image: app:1.3.0\r\n  qualified:\r\n    image: docker.io/library/app:1.3.0\r\n  " +
		"omitted:\r\n    image: registry.example.com:5000/team/app:1.3.0"
	effective := &types.Project{
		Services: types.Services{
			"web": {
				Image: "app:1.2.0",
			},
			"worker": {
				Image: "app:1.2.0",
			},
			"tagged": {
				Image: "app:1.2.0",
			},
			"escaped": {
				Image: "app:1.2.0",
			},
			"untouched": {
				Image: "app:1.2.0",
			},
			"plain": {
				Image: "app:1.2.0",
			},
			"qualified": {
				Image: "docker.io/library/app:1.2.0",
			},
			"omitted": {
				Image: "registry.example.com:5000/team/app",
			},
		},
	}
	updated, names, err := update.PrepareProjectServiceImages(source, effective, map[string]updatertypes.ServiceImageChange{
		"web":       {ExpectedRef: "docker.io/library/app:1.2.0", TargetRef: "app:1.3.0"},
		"worker":    {ExpectedRef: "app:1.2.0", TargetRef: "docker.io/library/app:1.3.0"},
		"tagged":    {ExpectedRef: "app:1.2.0", TargetRef: "app:1.3.0"},
		"escaped":   {ExpectedRef: "app:1.2.0", TargetRef: "app:1.3.0"},
		"plain":     {ExpectedRef: "app:1.2.0", TargetRef: "app:1.3.0"},
		"qualified": {ExpectedRef: "app:1.2.0", TargetRef: "app:1.3.0"},
		"omitted":   {ExpectedRef: "registry.example.com:5000/team/app", TargetRef: "registry.example.com:5000/team/app:1.3.0"},
	})
	require.NoError(t, err)
	require.Equal(t, []string{"escaped", "omitted", "plain", "qualified", "tagged", "web", "worker"}, names)
	require.Equal(t, expected, string(updated))
	require.Equal(t, "app:1.2.0", effective.Services["web"].Image)
}

func TestPrepareProjectServiceImagesRejectsUnsupportedSource(t *testing.T) {
	for _, tt := range []struct{ name, source, expected string }{
		{"stale effective image", "services:\n  web:\n    image: app:1.2.0\n", "app:1.1.0"},
		{"stale source image", "services:\n  web:\n    image: app:1.1.0\n", "app:1.2.0"},
		{"included source", "include: other.yaml\nservices:\n  web:\n    image: app:1.2.0\n", "app:1.2.0"},
		{"extended service", "services:\n  web:\n    image: app:1.2.0\n    extends: base\n", "app:1.2.0"},
		{"image alias", "x-image: &image app:1.2.0\nservices:\n  web:\n    image: *image\n", "app:1.2.0"},
		{"missing image", "services:\n  web:\n    build: .\n", "app:1.2.0"},
		{"multiple documents", "services:\n  web:\n    image: app:1.2.0\n---\nservices: {}\n", "app:1.2.0"},
		{"literal block image", "services:\n  web:\n    image: |\n      app:1.2.0\n", "app:1.2.0"},
		{"multi-line plain image", "services:\n  web:\n    image: app:${VERSION}\n      tail\n", "app:1.2.0"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			effective := &types.Project{Services: types.Services{"web": {Image: "app:1.2.0"}}}
			updated, _, err := update.PrepareProjectServiceImages([]byte(tt.source), effective, map[string]updatertypes.ServiceImageChange{"web": {ExpectedRef: tt.expected, TargetRef: "app:1.3.0"}})
			require.Error(t, err)
			require.Nil(t, updated)
		})
	}
}

func TestPersistProjectServiceImages(t *testing.T) {
	for _, name := range []string{"success", "concurrent edit", "symlink", "cancellation"} {
		t.Run(name, func(t *testing.T) {
			directory := t.TempDir()
			path := filepath.Join(directory, "compose.yaml")
			original := []byte("services: {}\n")
			updated := []byte("services: {web: {image: app:1.3.0}}\n")
			require.NoError(t, os.WriteFile(path, original, 0o600))
			ctx := t.Context()
			switch name {
			case "concurrent edit":
				require.NoError(t, os.WriteFile(path, []byte("# operator edit\n"), 0o600))
			case "symlink":
				require.NoError(t, os.Rename(path, filepath.Join(directory, "actual.yaml")))
				require.NoError(t, os.Symlink("actual.yaml", path))
			case "cancellation":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			err := update.PersistProjectServiceImages(ctx, directory, "/compose.yaml", original, updated)
			content, readErr := os.ReadFile(path)
			require.NoError(t, readErr)
			if name == "success" {
				require.NoError(t, err)
				require.Equal(t, updated, content)
				return
			}
			require.Error(t, err)
			require.NotEqual(t, updated, content)
		})
	}
}

type serviceImageCoordinator struct {
	projecttypes.ComposeCoordinator
	requests []projecttypes.ComposeServiceUpdate
	err      error
}

func (c *serviceImageCoordinator) UpdateServices(_ context.Context, request projecttypes.ComposeServiceUpdate) error {
	c.requests = append(c.requests, request)
	return c.err
}

func TestUpdateProjectServiceImagesPersistsBeforeDeploymentAndRetries(t *testing.T) {
	ctx := t.Context()
	db := setupProjectTestDB(t)
	directory := t.TempDir()
	t.Setenv("PROJECTS_DIRECTORY", directory)
	settingsService, err := newSettingsServiceForTest(t, ctx, db)
	require.NoError(t, err)
	require.NoError(t, settingsService.SetStringSetting(ctx, "projectsDirectory", directory))
	projectPath := createComposeProjectDir(t, directory, "image-tags")
	source := "# keep\n\nservices:\n  app:\n    image: app:${VERSION}\n\n  worker:\n    image: app:${VERSION}\n"
	require.NoError(t, os.WriteFile(filepath.Join(projectPath, "compose.yaml"), []byte(source), 0o600))
	require.NoError(t, os.Chmod(filepath.Join(projectPath, "compose.yaml"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(projectPath, ".env"), []byte("VERSION=1.2.0\n"), 0o600))
	proj := &Project{ID: "image-tags", Name: "image-tags", Path: projectPath, Status: ProjectStatusRunning}
	require.NoError(t, db.Create(proj).Error)
	deploymentError := errors.New("deployment failed after source persistence")
	coordinator := &serviceImageCoordinator{err: deploymentError}
	service := &ProjectService{db: db, settingsService: settingsService, eventService: event.NewEventService(db, nil, nil), composeCoordinator: coordinator}
	service.initChildren(nil, nil)
	changes := map[string]updatertypes.ServiceImageChange{"app": {ExpectedRef: "app:1.2.0", TargetRef: "app:1.3.0"}}
	for range 2 {
		updateProjectServiceImagesErr := service.UpdateProjectServiceImages(ctx, proj.ID, changes, usertypes.SystemUser)
		require.ErrorIs(t, updateProjectServiceImagesErr, deploymentError)
		content, readComposeErr := os.ReadFile(filepath.Join(projectPath, "compose.yaml"))
		require.NoError(t, readComposeErr)
		require.Equal(t, "# keep\n\nservices:\n  app:\n    image: app:1.3.0\n\n  worker:\n    image: app:${VERSION}\n", string(content))
		mode, statComposeErr := os.Stat(filepath.Join(projectPath, "compose.yaml"))
		require.NoError(t, statComposeErr)
		require.Equal(t, os.FileMode(0o600), mode.Mode().Perm())
	}
	require.Len(t, coordinator.requests, 2)
	for _, request := range coordinator.requests {
		require.Equal(t, []string{"app"}, request.Services)
		require.Equal(t, "app:1.3.0", request.Project.Services["app"].Image)
	}
	env, err := os.ReadFile(filepath.Join(projectPath, ".env"))
	require.NoError(t, err)
	require.Equal(t, "VERSION=1.2.0\n", string(env))
}

func TestUpdateProjectServiceImagesRejectsOverrideBeforeWriting(t *testing.T) {
	ctx := t.Context()
	db := setupProjectTestDB(t)
	directory := t.TempDir()
	t.Setenv("PROJECTS_DIRECTORY", directory)
	settingsService, err := newSettingsServiceForTest(t, ctx, db)
	require.NoError(t, err)
	require.NoError(t, settingsService.SetStringSetting(ctx, "projectsDirectory", directory))
	projectPath := createComposeProjectDir(t, directory, "image-overrides")
	original := []byte("services:\n  app:\n    image: app:1.0.0\n")
	override := []byte("services:\n  app:\n    image: app:1.2.0\n")
	require.NoError(t, os.WriteFile(filepath.Join(projectPath, "compose.yaml"), original, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(projectPath, "compose.override.yaml"), override, 0o600))
	proj := &Project{ID: "image-overrides", Name: "image-overrides", Path: projectPath}
	require.NoError(t, db.Create(proj).Error)
	service := &ProjectService{db: db, settingsService: settingsService}
	service.initChildren(nil, nil)
	err = service.UpdateProjectServiceImages(ctx, proj.ID, map[string]updatertypes.ServiceImageChange{"app": {ExpectedRef: "app:1.2.0", TargetRef: "app:1.3.0"}}, usertypes.SystemUser)
	require.ErrorContains(t, err, "overrides")
	content, err := os.ReadFile(filepath.Join(projectPath, "compose.yaml"))
	require.NoError(t, err)
	require.Equal(t, original, content)
	content, err = os.ReadFile(filepath.Join(projectPath, "compose.override.yaml"))
	require.NoError(t, err)
	require.Equal(t, override, content)
}

func TestDiscoveredProjectTagUpdatesRemainScoped(t *testing.T) {
	db := setupProjectTestDB(t)
	target := "3.2.0"
	tagLabels := map[string]string{labels.LabelUpdateStrategy: "tag"}
	require.NoError(
		t,
		db.Create(
			&imageupdate.ImageUpdateRecord{Platform: new("native-v1:classic:linux/amd64"), CurrentDigest: new("sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"), LatestDigest: new("sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"),
				ID: "container::first",
				PolicyKey: imageref.UpdatePolicyKey(
					"example:3.1.0",
					tagLabels,
				),
				ContainerID:   "first",
				ImageID:       "shared",
				Repository:    "docker.io/library/example",
				Tag:           "3.1.0",
				HasUpdate:     true,
				UpdateType:    "tag",
				LatestVersion: &target,
				CheckTime:     time.Now(),
			},
		).Error,
	)
	containers := []container.Summary{
		{
			ID:      "first-replica",
			Image:   "example:3.1.0",
			ImageID: "shared",
			Labels: map[string]string{
				labels.LabelUpdateStrategy:   "tag",
				"com.docker.compose.project": "first-project",
				"com.docker.compose.service": "web",
			},
		},
		{
			ID:      "first",
			Image:   "example:3.1.0",
			ImageID: "shared",
			Labels: map[string]string{
				labels.LabelUpdateStrategy:   "tag",
				"com.docker.compose.project": "first-project",
				"com.docker.compose.service": "web",
			},
		},
		{ID: "second", Image: "example:3.1.0", ImageID: "shared", Labels: map[string]string{"com.docker.compose.project": "second-project", "com.docker.compose.service": "web"}},
	}
	imageSvc := image.NewImageService(db, nil, nil, nil, nil, nil, nil, nil)
	rows := discoveryListingForTest(imageSvc).DiscoveredUpdateRows(t.Context(), containers, nil, "")
	require.Len(t, rows, 1)
	require.Equal(t, "first-project", rows[0].Name)
	require.True(t, rows[0].UpdateInfo.HasUpdate)
	require.Equal(t, target, rows[0].UpdateInfo.UpdateInfoByRef["example:3.1.0"].LatestVersion)
	service := &ProjectService{imageService: image.NewImageService(db, nil, nil, nil, nil, nil, nil, nil)}
	service.initChildren(nil, nil)
	detail := projecttypes.Details{ID: "tracked", RuntimeServices: []projecttypes.RuntimeService{{ContainerID: "first", Image: "example:3.1.0", ContainerLabels: tagLabels}}}
	service.enrichProjectUpdateInfo(t.Context(), &detail)
	require.True(t, detail.UpdateInfo.HasUpdate)
	require.Equal(t, target, detail.UpdateInfo.UpdateInfoByRef["example:3.1.0"].LatestVersion)
	delete(containers[1].Labels, labels.LabelUpdateStrategy)
	rows = discoveryListingForTest(imageSvc).DiscoveredUpdateRows(t.Context(), containers, nil, "")
	require.Empty(t, rows, "removing the strategy label falls back to digest checks")
	detail.RuntimeServices[0].ContainerLabels = containers[1].Labels
	service.enrichProjectUpdateInfo(t.Context(), &detail)
	require.False(t, detail.UpdateInfo.HasUpdate, "an undeclared strategy no longer inherits a tag record")
	// Turning off automatic installation keeps the check current (#3532).
	containers[1].Labels = map[string]string{labels.LabelUpdateStrategy: "tag", labels.LabelUpdater: "off", "com.docker.compose.project": "first-project", "com.docker.compose.service": "web"}
	rows = discoveryListingForTest(imageSvc).DiscoveredUpdateRows(t.Context(), containers, nil, "")
	require.Len(t, rows, 1, "updater=false containers keep their check results")
	for _, policy := range []map[string]string{
		{
			labels.LabelUpdateStrategy: "digest",
		},
		{
			labels.LabelUpdateStrategy:   "tag",
			labels.LabelUpdateConstraint: "3.1.x",
		},
		{
			labels.LabelUpdateStrategy:   "tag",
			labels.LabelUpdateTagPattern: ".*",
		},
		{
			labels.LabelUpdateStrategy: "tag",
			imageref.UpdateCheckLabel:  "off",
		},
	} {
		current := map[string]string{"com.docker.compose.project": "first-project", "com.docker.compose.service": "web"}
		maps.Copy(current, policy)
		containers[1].Labels = current
		rows = discoveryListingForTest(imageSvc).DiscoveredUpdateRows(t.Context(), containers, nil, "")
		require.Empty(t, rows, "stale records must not mark discovered projects updated")
		detail.RuntimeServices[0].ContainerLabels = current
		service.enrichProjectUpdateInfo(t.Context(), &detail)
		require.False(t, detail.UpdateInfo.HasUpdate, "runtime fallback must not apply a stale policy")
	}
}

func TestProjectTagSummaryDoesNotMutateSharedReferenceResults(t *testing.T) {
	base := map[string]*imagetypes.UpdateInfo{"example:3.1.0": {HasUpdate: false, UpdateType: "digest"}}
	scoped := map[string]*imagetypes.UpdateInfo{
		"first":  {HasUpdate: true, UpdateType: "tag", LatestVersion: "3.2.0"},
		"second": {HasUpdate: true, UpdateType: "tag", LatestVersion: "4.0.0"},
	}
	merged := projectdetails.MergeProjectContainerUpdateInfo(base, []projecttypes.RuntimeService{{ContainerID: "first", Image: "example:3.1.0"}, {ContainerID: "second", Image: "example:3.1.0"}}, scoped)
	require.True(t, merged["example:3.1.0"].HasUpdate)
	require.Empty(t, merged["example:3.1.0"].LatestVersion)
	require.False(t, base["example:3.1.0"].HasUpdate)
	require.Equal(t, "3.2.0", scoped["first"].LatestVersion)

	// A shared image result is dropped when every service using the reference
	// opted out of update checks, and kept while one still monitors it.
	base = map[string]*imagetypes.UpdateInfo{"example:3.1.0": {HasUpdate: true, UpdateType: "digest"}, "other:1.0": {HasUpdate: true, UpdateType: "digest"}}
	unmonitored := map[string]string{imageref.UpdateCheckLabel: "false"}
	merged = projectdetails.MergeProjectContainerUpdateInfo(
		base,
		[]projecttypes.RuntimeService{
			{
				ContainerID:     "a",
				Image:           "example:3.1.0",
				ContainerLabels: unmonitored,
			},
			{
				ContainerID: "b",
				Image:       "example:3.1.0",
				ContainerLabels: map[string]string{
					labels.LabelUpdater: "false",
				},
			},
		},
		nil,
	)
	require.True(t, merged["example:3.1.0"].HasUpdate, "updater=false keeps the shared result")
	require.True(t, merged["other:1.0"].HasUpdate, "references without runtime services are untouched")
	merged = projectdetails.MergeProjectContainerUpdateInfo(base, []projecttypes.RuntimeService{{ContainerID: "a", Image: "example:3.1.0", ContainerLabels: unmonitored}}, nil)
	require.NotContains(t, merged, "example:3.1.0")
	require.True(t, base["example:3.1.0"].HasUpdate)
}
