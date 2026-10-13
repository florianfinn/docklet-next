// Source: getarcaneapp/arcane/backend/pkg/scheduler/filesystem_watcher_job.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed template catalogue.

package scheduler

import (
	"context"
	"errors"
	"log/slog"
	"runtime"
	"sync"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/project"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/backend/pkg/fswatch"
	"github.com/florianfinn/docklet-next/backend/pkg/projects"
)

type FilesystemWatcherJob struct {
	projectService   *project.ProjectService
	settingsService  *settings.SettingsService
	projectScanDepth int
	lifecycleCtx     context.Context
	projectsWatcher  *fswatch.Watcher
	mu               sync.Mutex
	stopped          bool
	retired          []*fswatch.Watcher
}

func NewFilesystemWatcherJob(
	ctx context.Context,
	projectService *project.ProjectService,
	settingsService *settings.SettingsService,
	projectScanDepth int,
) (*FilesystemWatcherJob, error) {
	return &FilesystemWatcherJob{
		projectService:   projectService,
		settingsService:  settingsService,
		projectScanDepth: projectScanDepth,
		lifecycleCtx:     ctx,
	}, nil
}

func (j *FilesystemWatcherJob) Start(ctx context.Context) error {
	return j.RestartProjectsWatcher(ctx)
}

func (j *FilesystemWatcherJob) Stop(ctx context.Context) error {
	j.mu.Lock()
	j.stopped = true
	projectsWatcher := j.projectsWatcher
	j.projectsWatcher = nil
	retired := j.retired
	j.retired = nil
	j.mu.Unlock()
	var err error
	if projectsWatcher != nil {
		err = projectsWatcher.Stop()
	}
	for _, watcher := range retired {
		err = errors.Join(err, watcher.Stop())
	}
	return err
}

func (j *FilesystemWatcherJob) handleFilesystemChangeInternal(ctx context.Context) {
	slog.InfoContext(ctx, "Filesystem change detected, syncing projects")

	if err := j.projectService.SyncProjectsFromFileSystem(ctx); err != nil {
		slog.ErrorContext(ctx, "Failed to sync projects after filesystem change",
			"error", err)
	} else {
		slog.InfoContext(ctx, "Project sync completed after filesystem change")
	}
}

func (j *FilesystemWatcherJob) handleProjectFilePathsChangedInternal(ctx context.Context, paths []string) {
	if len(paths) == 0 || j.projectService == nil {
		return
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.stopped || ctx.Err() != nil {
		return
	}
	j.handleFilesystemChangeInternal(ctx)
	j.projectService.HandleProjectFilesChanged(ctx, paths)
}

func (j *FilesystemWatcherJob) RestartProjectsWatcher(ctx context.Context) error {
	slog.InfoContext(ctx, "Restarting projects filesystem watcher")
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.stopped {
		return errors.New("filesystem watcher stopped")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if j.projectsWatcher != nil {
		previous := j.projectsWatcher
		j.projectsWatcher = nil
		j.retired = append(j.retired, previous)
		// Callbacks need the domain mutex, so stop the watch loop separately from joining them.
		if err := previous.StopWatching(); err != nil {
			return err
		}
	}
	watcher, err := j.startProjectsWatcherInternal(ctx)
	if err != nil {
		if watcher != nil {
			_ = watcher.StopWatching()
		}
		return err
	}
	j.projectsWatcher = watcher
	return nil
}

func (j *FilesystemWatcherJob) startProjectsWatcherInternal(ctx context.Context) (*fswatch.Watcher, error) {
	localSettings, err := j.settingsService.GetSettings(ctx)
	if err != nil {
		return nil, err
	}
	projectsDirectory, err := projects.GetProjectsDirectory(ctx, localSettings.ProjectsDirectory.Value)
	if err != nil {
		return nil, err
	}
	j.logRecursiveProjectsWatchLimitWarningInternal(ctx, projectsDirectory)

	watcher, err := fswatch.NewWatcher(projectsDirectory, j.projectWatcherOptionsInternal(localSettings.FollowProjectSymlinks.IsTrue()))
	if err != nil {
		return nil, err
	}
	if startErr := watcher.Start(j.lifecycleCtx); startErr != nil { //nolint:contextcheck // watcher lifetime belongs to the application, not this replacement task.
		return watcher, startErr
	}

	slog.InfoContext(ctx, "Projects filesystem watcher started", "path", projectsDirectory)
	if j.projectService != nil {
		if syncProjectsFromFileSystemErr := j.projectService.SyncProjectsFromFileSystem(ctx); syncProjectsFromFileSystemErr != nil {
			slog.ErrorContext(ctx, "Initial project sync after watcher start failed", "error", syncProjectsFromFileSystemErr)
		}
	}
	return watcher, nil
}

func (j *FilesystemWatcherJob) logRecursiveProjectsWatchLimitWarningInternal(ctx context.Context, projectsDirectory string) {
	if runtime.GOOS != "linux" {
		return
	}

	slog.WarnContext(ctx,
		"Projects filesystem watcher is monitoring directories recursively; very deep trees may require increasing fs.inotify.max_user_watches",
		"path", projectsDirectory,
		"sysctl", "fs.inotify.max_user_watches")
}

func (j *FilesystemWatcherJob) projectWatcherOptionsInternal(followProjectSymlinks bool) fswatch.WatcherOptions {
	return fswatch.WatcherOptions{
		Debounce:          500 * time.Millisecond,
		OnChangePaths:     j.handleProjectFilePathsChangedInternal,
		MaxDepth:          j.projectScanDepth,
		FollowSymlinkDirs: followProjectSymlinks,
	}
}
