// Source: getarcaneapp/arcane/backend/internal/project/service_test.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed central variables and .env.global; functional cut: removed GitOps and saved Git repositories; SQLite temp-file persistence fixture.

package project

import (
	"encoding/json"

	"net/http"
	"net/http/httptest"

	"os"
	"path/filepath"

	"strings"
	"sync/atomic"
	"testing"

	projecttypes "github.com/florianfinn/docklet-next/types/project"

	volumetypes "github.com/florianfinn/docklet-next/types/volume"

	"github.com/moby/moby/api/types/container"

	"github.com/moby/moby/api/types/volume"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/florianfinn/docklet-next/backend/internal/config"

	"github.com/florianfinn/docklet-next/backend/internal/docker"

	"github.com/florianfinn/docklet-next/backend/internal/kv"
)

func TestProjectService_RecoverProjectRenameJournals_CompletesCommittedVolumeJournalWithoutHelperImage(t *testing.T) {
	db := setupProjectTestDB(t)
	require.NoError(t, db.AutoMigrate(&kv.KVEntry{}))
	ctx := t.Context()

	var imageInspectCalled atomic.Bool
	var oldVolumeRemoved atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/volumes/web_data"):
			w.Header().Set("Content-Type", "application/json")
			if !assert.NoError(t, json.NewEncoder(w).Encode(volume.Volume{Name: "web_data"})) {
				return
			}
		case r.Method == http.MethodDelete && strings.HasSuffix(r.URL.Path, "/volumes/nginx_data"):
			oldVolumeRemoved.Store(true)
			w.WriteHeader(http.StatusNoContent)
		case strings.Contains(r.URL.Path, "/images/"):
			imageInspectCalled.Store(true)
			http.NotFound(w, r)
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
		ID:      "proj-rename-committed-with-volumes",
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
		Phase:      projecttypes.RenameJournalPhaseOldVolumesRemoved,
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
	require.True(t, oldVolumeRemoved.Load(), "expected committed recovery to remove source volume")
	require.False(t, imageInspectCalled.Load(), "completion recovery should not inspect or pull the copy helper image")
}

func TestProjectService_RecoverProjectRenameJournals_RollsBackCommittedJournalWhenTargetMissingAndSourceExists(t *testing.T) {
	db := setupProjectTestDB(t)
	require.NoError(t, db.AutoMigrate(&kv.KVEntry{}))
	ctx := t.Context()

	var oldVolumeRemoved atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/volumes/web_data"):
			http.NotFound(w, r)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/volumes/nginx_data"):
			w.Header().Set("Content-Type", "application/json")
			if !assert.NoError(t, json.NewEncoder(w).Encode(volume.Volume{Name: "nginx_data"})) {
				return
			}
		case r.Method == http.MethodDelete && strings.HasSuffix(r.URL.Path, "/volumes/nginx_data"):
			oldVolumeRemoved.Store(true)
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

	project := &Project{
		ID:      "proj-rename-missing-target-preserve-source",
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
		Phase:      projecttypes.RenameJournalPhaseOldVolumesRemoved,
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
	require.False(t, oldVolumeRemoved.Load(), "source volume is the only remaining copy and must not be deleted")

	_, ok, err := kvService.Get(ctx, projecttypes.RenameJournalKeyPrefix+project.ID)
	require.NoError(t, err)
	require.False(t, ok)
	require.DirExists(t, oldPath)
	require.NoDirExists(t, newPath)

	var fromDB Project
	require.NoError(t, db.First(&fromDB, "id = ?", project.ID).Error)
	require.Equal(t, "nginx", fromDB.Name)
	require.Equal(t, oldPath, fromDB.Path)
	require.NotNil(t, fromDB.DirName)
	require.Equal(t, oldDir, *fromDB.DirName)
}

func TestProjectService_RecoverProjectRenameJournals_ClearsJournalAfterDBRestoreWhenVolumeRollbackFails(t *testing.T) {
	db := setupProjectTestDB(t)
	require.NoError(t, db.AutoMigrate(&kv.KVEntry{}))
	ctx := t.Context()

	var targetRemoveAttempts atomic.Int32
	var targetExists atomic.Bool
	var allowTargetRemove atomic.Bool
	targetExists.Store(true)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/volumes/web_data"):
			http.NotFound(w, r)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/volumes/nginx_data"):
			w.Header().Set("Content-Type", "application/json")
			if !assert.NoError(t, json.NewEncoder(w).Encode(volume.Volume{Name: "nginx_data"})) {
				return
			}
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/volumes/web_cache"):
			if !targetExists.Load() {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			if !assert.NoError(t, json.NewEncoder(w).Encode(volume.Volume{Name: "web_cache"})) {
				return
			}
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/volumes/nginx_cache"):
			w.Header().Set("Content-Type", "application/json")
			if !assert.NoError(t, json.NewEncoder(w).Encode(volume.Volume{Name: "nginx_cache"})) {
				return
			}
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/containers/json"):
			w.Header().Set("Content-Type", "application/json")
			if !assert.NoError(t, json.NewEncoder(w).Encode([]container.Summary{})) {
				return
			}
		case r.Method == http.MethodDelete && strings.HasSuffix(r.URL.Path, "/volumes/web_cache"):
			targetRemoveAttempts.Add(1)
			if allowTargetRemove.Load() {
				targetExists.Store(false)
				w.WriteHeader(http.StatusNoContent)
				return
			}
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
	require.NoError(t, os.WriteFile(filepath.Join(newPath, "compose.yaml"), []byte("services: {}\n"), 0o600))

	project := &Project{
		ID:      "proj-rename-rollback-volume-fail",
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
		Phase:      projecttypes.RenameJournalPhaseProjectStateCommitted,
		Volumes: []volumetypes.JournalVolume{
			{
				Key:     "data",
				OldName: "nginx_data",
				NewName: "web_data",
			},
			{
				Key:     "cache",
				OldName: "nginx_cache",
				NewName: "web_cache",
			},
		},
	}
	payload, err := json.Marshal(journal)
	require.NoError(t, err)
	require.NoError(t, kvService.Set(ctx, projecttypes.RenameJournalKeyPrefix+project.ID, string(payload)))

	err = svc.RecoverProjectRenameJournals(ctx)
	require.Error(t, err)
	require.ErrorContains(t, err, "remove rollback target volume web_cache")

	require.Positive(t, targetRemoveAttempts.Load())
	_, ok, err := kvService.Get(ctx, projecttypes.RenameJournalKeyPrefix+project.ID)
	require.NoError(t, err)
	require.False(t, ok, "project-state journal should clear after database rollback succeeds")
	_, ok, err = kvService.Get(ctx, projecttypes.RenameRollbackCleanupKeyPrefix+project.ID)
	require.NoError(t, err)
	require.True(t, ok, "target cleanup should keep retry state when removal fails")
	require.FileExists(t, filepath.Join(oldPath, "compose.yaml"))
	require.NoDirExists(t, newPath)

	var fromDB Project
	require.NoError(t, db.First(&fromDB, "id = ?", project.ID).Error)
	require.Equal(t, "nginx", fromDB.Name)
	require.Equal(t, oldPath, fromDB.Path)
	require.NotNil(t, fromDB.DirName)
	require.Equal(t, oldDir, *fromDB.DirName)

	allowTargetRemove.Store(true)
	require.NoError(t, svc.RecoverProjectRenameJournals(ctx))

	_, ok, err = kvService.Get(ctx, projecttypes.RenameRollbackCleanupKeyPrefix+project.ID)
	require.NoError(t, err)
	require.False(t, ok)
	require.False(t, targetExists.Load())
}

func TestProjectService_RecoverProjectRenameJournals_KeepsRollbackCleanupWhenDockerUnavailable(t *testing.T) {
	db := setupProjectTestDB(t)
	require.NoError(t, db.AutoMigrate(&kv.KVEntry{}))
	ctx := t.Context()

	projectsDir := t.TempDir()
	oldDir := "nginx"
	oldPath := filepath.Join(projectsDir, oldDir)
	require.NoError(t, os.MkdirAll(oldPath, 0o755))

	project := &Project{
		ID:      "proj-rename-rollback-cleanup-docker-unavailable",
		Name:    "nginx",
		DirName: &oldDir,
		Path:    oldPath,
		Status:  ProjectStatusStopped,
	}
	require.NoError(t, db.Create(project).Error)

	kvService := kv.NewKVService(db)
	svc := NewProjectService(db, nil, nil, nil, nil, nil, nil, config.Load(), kvService, nil)
	cleanup := projecttypes.RenameRollbackCleanup{
		ProjectID: project.ID,
		OldName:   "nginx",
		OldPath:   oldPath,
		NewName:   "web",
		NewPath:   filepath.Join(projectsDir, "web"),
		Volumes: []volumetypes.JournalVolume{
			{
				Key:     "data",
				OldName: "nginx_data",
				NewName: "web_data",
			},
		},
	}
	payload, err := json.Marshal(cleanup)
	require.NoError(t, err)
	require.NoError(t, kvService.Set(ctx, projecttypes.RenameRollbackCleanupKeyPrefix+project.ID, string(payload)))

	err = svc.RecoverProjectRenameJournals(ctx)
	require.Error(t, err)
	require.ErrorContains(t, err, "docker service unavailable")

	_, ok, err := kvService.Get(ctx, projecttypes.RenameRollbackCleanupKeyPrefix+project.ID)
	require.NoError(t, err)
	require.True(t, ok)
}

func TestProjectService_RecoverProjectRenameJournals_ClearsCommittedJournalWhenSourceAndTargetMissing(t *testing.T) {
	db := setupProjectTestDB(t)
	require.NoError(t, db.AutoMigrate(&kv.KVEntry{}))
	ctx := t.Context()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/volumes/web_data"):
			http.NotFound(w, r)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/volumes/nginx_data"):
			http.NotFound(w, r)
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
		ID:      "proj-rename-missing-both-volumes",
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
		Phase:      projecttypes.RenameJournalPhaseProjectStateCommitted,
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
	require.False(t, ok)
	require.DirExists(t, newPath)
	require.NoDirExists(t, oldPath)

	var fromDB Project
	require.NoError(t, db.First(&fromDB, "id = ?", project.ID).Error)
	require.Equal(t, "web", fromDB.Name)
	require.Equal(t, newPath, fromDB.Path)
}

func TestProjectService_RecoverProjectRenameJournals_ClearsCommittedJournalAndCleansRemainingSourcesWhenSomeVolumesExternallyRemoved(t *testing.T) {
	db := setupProjectTestDB(t)
	require.NoError(t, db.AutoMigrate(&kv.KVEntry{}))
	ctx := t.Context()

	var cacheSourceRemoved atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/volumes/web_data"):
			http.NotFound(w, r)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/volumes/nginx_data"):
			http.NotFound(w, r)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/volumes/web_cache"):
			w.Header().Set("Content-Type", "application/json")
			if !assert.NoError(t, json.NewEncoder(w).Encode(volume.Volume{Name: "web_cache"})) {
				return
			}
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/volumes/nginx_cache"):
			w.Header().Set("Content-Type", "application/json")
			if !assert.NoError(t, json.NewEncoder(w).Encode(volume.Volume{Name: "nginx_cache"})) {
				return
			}
		case r.Method == http.MethodDelete && strings.HasSuffix(r.URL.Path, "/volumes/nginx_cache"):
			cacheSourceRemoved.Store(true)
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

	project := &Project{
		ID:      "proj-rename-mixed-missing-volumes",
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
		Phase:      projecttypes.RenameJournalPhaseProjectStateCommitted,
		Volumes: []volumetypes.JournalVolume{
			{
				Key:     "data",
				OldName: "nginx_data",
				NewName: "web_data",
			},
			{
				Key:     "cache",
				OldName: "nginx_cache",
				NewName: "web_cache",
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
	require.False(t, ok)
	require.True(t, cacheSourceRemoved.Load(), "source volume should still be cleaned up when the target exists")
	require.DirExists(t, newPath)
	require.NoDirExists(t, oldPath)

	var fromDB Project
	require.NoError(t, db.First(&fromDB, "id = ?", project.ID).Error)
	require.Equal(t, "web", fromDB.Name)
	require.Equal(t, newPath, fromDB.Path)
}

func TestProjectService_RecoverProjectRenameJournals_MarksSourceCleanupPendingWhenCommittedCleanupFails(t *testing.T) {
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
		ID:      "proj-rename-source-cleanup-fail",
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
		Phase:      projecttypes.RenameJournalPhaseProjectStateCommitted,
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

func TestProjectService_RecoverProjectRenameJournals_ClearsSourceCleanupPendingJournalAfterCleanup(t *testing.T) {
	db := setupProjectTestDB(t)
	require.NoError(t, db.AutoMigrate(&kv.KVEntry{}))
	ctx := t.Context()

	var sourceRemoved atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/volumes/web_data"):
			w.Header().Set("Content-Type", "application/json")
			if !assert.NoError(t, json.NewEncoder(w).Encode(volume.Volume{Name: "web_data"})) {
				return
			}
		case r.Method == http.MethodDelete && strings.HasSuffix(r.URL.Path, "/volumes/nginx_data"):
			sourceRemoved.Store(true)
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

	project := &Project{
		ID:      "proj-rename-source-cleanup-pending-clear",
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
	require.NoError(t, err)

	_, ok, err := kvService.Get(ctx, projecttypes.RenameJournalKeyPrefix+project.ID)
	require.NoError(t, err)
	require.False(t, ok)
	require.True(t, sourceRemoved.Load())
	require.DirExists(t, newPath)
	require.NoDirExists(t, oldPath)
}

func TestProjectService_RecoverProjectRenameJournals_RollsBackSourceCleanupPendingWhenTargetMissing(t *testing.T) {
	db := setupProjectTestDB(t)
	require.NoError(t, db.AutoMigrate(&kv.KVEntry{}))
	ctx := t.Context()

	var dataSourceRemoved atomic.Bool
	var dataTargetRemoved atomic.Bool
	var cacheTargetRemoved atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/volumes/web_data"):
			http.NotFound(w, r)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/volumes/nginx_data"):
			w.Header().Set("Content-Type", "application/json")
			if !assert.NoError(t, json.NewEncoder(w).Encode(volume.Volume{Name: "nginx_data"})) {
				return
			}
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/volumes/web_cache"):
			w.Header().Set("Content-Type", "application/json")
			if !assert.NoError(t, json.NewEncoder(w).Encode(volume.Volume{Name: "web_cache"})) {
				return
			}
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/volumes/nginx_cache"):
			w.Header().Set("Content-Type", "application/json")
			if !assert.NoError(t, json.NewEncoder(w).Encode(volume.Volume{Name: "nginx_cache"})) {
				return
			}
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/containers/json"):
			w.Header().Set("Content-Type", "application/json")
			if !assert.NoError(t, json.NewEncoder(w).Encode([]container.Summary{})) {
				return
			}
		case r.Method == http.MethodDelete && strings.HasSuffix(r.URL.Path, "/volumes/nginx_data"):
			dataSourceRemoved.Store(true)
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodDelete && strings.HasSuffix(r.URL.Path, "/volumes/web_data"):
			dataTargetRemoved.Store(true)
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodDelete && strings.HasSuffix(r.URL.Path, "/volumes/web_cache"):
			cacheTargetRemoved.Store(true)
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
		ID:      "proj-rename-source-cleanup-target-missing",
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
			{
				Key:     "cache",
				OldName: "nginx_cache",
				NewName: "web_cache",
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
	require.False(t, ok)
	_, ok, err = kvService.Get(ctx, projecttypes.RenameRollbackCleanupKeyPrefix+project.ID)
	require.NoError(t, err)
	require.False(t, ok)
	require.False(t, dataSourceRemoved.Load(), "source volume is the remaining data copy and must not be removed")
	require.False(t, dataTargetRemoved.Load(), "missing target should not be removed")
	require.True(t, cacheTargetRemoved.Load(), "safe target volume should still be cleaned during rollback")
	require.FileExists(t, filepath.Join(oldPath, "compose.yaml"))
	require.NoDirExists(t, newPath)

	var fromDB Project
	require.NoError(t, db.First(&fromDB, "id = ?", project.ID).Error)
	require.Equal(t, "nginx", fromDB.Name)
	require.Equal(t, oldPath, fromDB.Path)
	require.NotNil(t, fromDB.DirName)
	require.Equal(t, oldDir, *fromDB.DirName)
}
