// Source: getarcaneapp/arcane/backend/internal/project/children/deployment/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace

// Package deployment runs the image and bind-directory side of project
// deployments: pulls, builds, update reconciliation and dependents.
package deployment

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"maps"
	"slices"

	composetypes "github.com/compose-spec/compose-go/v2/types"
	"github.com/florianfinn/docklet-next/types"
	"github.com/florianfinn/docklet-next/types/containerregistry"
	projecttypes "github.com/florianfinn/docklet-next/types/project"
	usertypes "github.com/florianfinn/docklet-next/types/user"
	"go.getarcane.app/acfs"
	buildtypes "go.getarcane.app/builds/types"

	"github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/internal/image"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/timeouts"
	"github.com/florianfinn/docklet-next/backend/pkg/projects"
	"github.com/florianfinn/docklet-next/backend/pkg/utils"
)

// Builder builds Compose service images.
type Builder interface {
	BuildImage(ctx context.Context, environmentID string, req buildtypes.BuildRequest, progressWriter io.Writer, serviceName string, user *usertypes.Actor) (*buildtypes.BuildResult, error)
	BuildSettings() buildtypes.BuildSettings
}

// Service runs the image side of deployments: pulls, update reconciliation,
// builds and namespace dependent discovery.
type Service struct {
	settingsService *settings.SettingsService
	imageService    *image.ImageService
	dockerService   *docker.DockerClientService
	buildService    Builder
}

func New(settingsService *settings.SettingsService, imageService *image.ImageService, dockerService *docker.DockerClientService, buildService Builder) *Service {
	return &Service{settingsService: settingsService, imageService: imageService, dockerService: dockerService, buildService: buildService}
}

// Pull pulls one image within the configured pull timeout and reconciles its
// update state.
func (s *Service) Pull(
	ctx context.Context,
	imageRef string,
	progressWriter io.Writer,
	user usertypes.Actor,
	credentials []containerregistry.Credential,
) error {
	if s == nil || s.imageService == nil {
		return errors.New("image service not available")
	}

	cfg := s.settingsService.GetSettingsConfig()

	pullCtx, pullCancel := context.WithTimeout(ctx, timeouts.GetDuration(cfg.DockerImagePullTimeout.AsInt(), timeouts.DefaultDockerImagePull))
	defer pullCancel()

	if err := s.imageService.PullComposeImage(pullCtx, imageRef, progressWriter, user, credentials); err != nil {
		if errors.Is(pullCtx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("image pull timed out for %s (increase DOCKER_IMAGE_PULL_TIMEOUT or setting)", imageRef)
		}
		return fmt.Errorf("failed to pull image %s: %w", imageRef, err)
	}

	if err := s.imageService.ReconcilePulledImageUpdate(ctx, imageRef); err != nil {
		slog.WarnContext(ctx, "failed to reconcile pulled image update state", "image", imageRef, "error", err)
	}
	return nil
}

// NamespaceDependents splits projects.NamespaceDependents into
// services with a running container and services with only stopped ones, so a
// stopped dependent is recreated but not started, and a declared-but-never-
// created dependent is left alone. A ComposePs failure treats every dependent
// as running: a dangling namespace reference is the worse outcome.
func (s *Service) NamespaceDependents(ctx context.Context, compProj *composetypes.Project, services []string) (running, stopped []string) {
	if len(services) == 0 {
		return nil, nil
	}
	deps := projects.NamespaceDependents(compProj, services)
	if len(deps) == 0 {
		return nil, nil
	}
	summaries, err := projects.ComposePs(ctx, s.dockerService.DockerHost(), compProj, deps, true)
	if err != nil {
		slog.WarnContext(ctx, "failed to list namespace dependent containers; recreating all dependents", "project", compProj.Name, "dependents", deps, "error", err)
		return deps, nil
	}
	for _, name := range deps {
		var found, isRunning bool
		for _, summary := range summaries {
			if summary.Service == name {
				found = true
				isRunning = isRunning || summary.State == "running"
			}
		}
		switch {
		case isRunning:
			running = append(running, name)
		case found:
			stopped = append(stopped, name)
		}
	}
	return running, stopped
}

// ImageOperations wires image pulls, existence checks and builds for Compose.
func (s *Service) ImageOperations(user *usertypes.Actor, credentials []containerregistry.Credential) projecttypes.ComposeImageOperations {
	operations := projecttypes.ComposeImageOperations{
		Pull: func(ctx context.Context, image string, progress io.Writer) error {
			actor := usertypes.SystemUser
			if user != nil {
				actor = *user
			}
			return s.Pull(ctx, image, progress, actor, credentials)
		},
	}
	if s.imageService != nil {
		operations.Exists = s.imageService.ImageExistsLocally
		operations.LastTagged = s.imageService.ImageLastTagTime
	}
	if s.buildService != nil {
		operations.BuildProvider = s.buildService.BuildSettings().BuildProvider
		operations.Build = func(ctx context.Context, request buildtypes.BuildRequest, progress io.Writer, service string) error {
			_, err := s.buildService.BuildImage(ctx, types.LocalDockerEnvironmentID, request, progress, service, user)
			return err
		}
	}
	return operations
}

// PrepareProjectBindDirectories creates missing bind-mount sources inside
// projectPath as Arcane's runtime user before deploys; Docker would create them as root (#4132).
//
// Only sources with Compose's automatic host path creation are considered.
// Existing paths, sources escaping the project lexically or through a symlink
// (#4195), and sources Arcane cannot inspect are left to Docker.
func PrepareProjectBindDirectories(projectPath string) projects.PrepareProjectFunc {
	return func(ctx context.Context, project *composetypes.Project) error {
		for _, serviceName := range slices.Sorted(maps.Keys(project.Services)) {
			for _, volume := range project.Services[serviceName].Volumes {
				if volume.Type != composetypes.VolumeTypeBind || volume.Source == "" {
					continue
				}
				if volume.Bind != nil && !bool(volume.Bind.CreateHostPath) {
					continue
				}
				logicalPath, err := acfs.LogicalPath(projectPath, volume.Source)
				if err == nil && logicalPath != "/" {
					exists, existsErr := acfs.Exists(ctx, projectPath, logicalPath)
					switch {
					case exists || errors.Is(existsErr, acfs.ErrOutsideRoot) || errors.Is(existsErr, fs.ErrPermission):
						// Leave existing, escaping, and uninspectable sources to the Docker daemon.
					case existsErr != nil:
						err = existsErr
					default:
						if err = acfs.MkdirAll(ctx, projectPath, logicalPath, utils.DirPerm); err == nil {
							slog.InfoContext(ctx, "created missing bind directory for project deployment", "projectPath", projectPath, "source", volume.Source)
						}
					}
				}
				if err != nil && !errors.Is(err, acfs.ErrOutsideRoot) {
					return fmt.Errorf("bind source %s for service %s: %w", volume.Source, serviceName, err)
				}
			}
		}
		return nil
	}
}
