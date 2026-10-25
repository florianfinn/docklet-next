// Source: getarcaneapp/arcane/backend/pkg/scheduler/image_update_watcher.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; neutralise inherited triggers in favour of hub-owned stack policy jobs

package scheduler

import (
	"context"
	"errors"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/internal/imageupdate"
	"github.com/florianfinn/docklet-next/backend/internal/project"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/runs"
	"github.com/florianfinn/docklet-next/types/containerregistry"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	checkDTO "github.com/florianfinn/docklet-next/types/imageupdate"
)

type imageUpdateScannerInternal interface {
	CheckAllImages(context.Context, int, []containerregistry.Credential) (map[string]*checkDTO.Response, error)
}

// ImageUpdateWatcher remains inert for stale callers. Explicit hub requests use
// existing executor observation commands; no continuous/event trigger is installed.
type ImageUpdateWatcher struct{ imageUpdateService imageUpdateScannerInternal }

func NewImageUpdateWatcher(_ *config.Config, checks *imageupdate.ImageUpdateService, settings *settings.SettingsService, env *environment.EnvironmentService, docker *docker.DockerClientService, projects *project.ProjectService) (*ImageUpdateWatcher, error) {
	if checks == nil || settings == nil || env == nil || docker == nil || projects == nil {
		return nil, errors.New("image update watcher dependencies unavailable")
	}
	return &ImageUpdateWatcher{imageUpdateService: checks}, nil
}
func (*ImageUpdateWatcher) Name() string { return "image-polling" }
func (*ImageUpdateWatcher) Start(context.Context) error {
	return imageDTO.Failure("update.agent-local-job-disabled", 409)
}
func (*ImageUpdateWatcher) RunNow(context.Context) error {
	return imageDTO.Failure("update.agent-local-job-disabled", 409)
}
func (*ImageUpdateWatcher) Stop(context.Context) error       { return nil }
func (*ImageUpdateWatcher) Trigger()                         {}
func (*ImageUpdateWatcher) RefreshSchedule()                 {}
func (*ImageUpdateWatcher) SetCoordinator(*runs.Coordinator) {}
