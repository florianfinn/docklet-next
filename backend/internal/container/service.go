// Source: getarcaneapp/arcane/backend/internal/container/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; split lifecycle, editing, inventory and stream concerns; exclude Docklet agent inventory; M14 container and host operation contracts.

package container

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"strings"
	"time"

	containertypes "github.com/florianfinn/docklet-next/types/container"
	"github.com/florianfinn/docklet-next/types/containerregistry"
	updaterDTO "github.com/florianfinn/docklet-next/types/updater"
	usertypes "github.com/florianfinn/docklet-next/types/user"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
	"github.com/samber/hot"
	"go.getarcane.app/docker"
	"go.getarcane.app/docker/compat"
	"go.getarcane.app/docker/types"
	"go.getarcane.app/kit/pkg"
	"go.getarcane.app/sys/cgroup"
	"go.getarcane.app/updater/labels"

	"github.com/florianfinn/docklet-next/backend/internal/common"
	"github.com/florianfinn/docklet-next/backend/internal/container/children/stats"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	dockerInternal "github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/internal/event"
	"github.com/florianfinn/docklet-next/backend/internal/image"
	"github.com/florianfinn/docklet-next/backend/internal/project"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/timeouts"
	"github.com/florianfinn/docklet-next/backend/pkg/pagination"
	"github.com/florianfinn/docklet-next/backend/pkg/projects"
)

type ContainerService struct {
	operations      *containerOperations
	dockerService   *dockerInternal.DockerClientService
	eventService    *event.EventService
	imageService    *image.ImageService
	settingsService *settings.SettingsService
	projectService  *project.ProjectService
	iconMetaCache   *hot.HotCache[string, projects.ArcaneComposeMetadata]
	stats           *stats.Service
}

const (
	containerGroupByProject  = "project"
	containerNoProjectGroup  = "No Project"
	containerIconMetadataTTL = 5 * time.Second
)

type ContainerListResult struct {
	Items      []containertypes.Summary
	Groups     []containertypes.SummaryGroup
	Pagination pagination.Response
	Counts     containertypes.StatusCounts
}

func NewContainerService(
	eventService *event.EventService,
	dockerService *dockerInternal.DockerClientService,
	imageService *image.ImageService,
	settingsService *settings.SettingsService,
	projectService *project.ProjectService,
) *ContainerService {
	return &ContainerService{
		eventService:    eventService,
		dockerService:   dockerService,
		imageService:    imageService,
		settingsService: settingsService,
		projectService:  projectService,
		iconMetaCache: hot.NewHotCache[string, projects.ArcaneComposeMetadata](hot.LRU, 1024).
			WithTTL(containerIconMetadataTTL).
			WithJanitor().
			Build(),
		stats: stats.New(dockerService.GetClient, dockerService.DockerHost, stats.ContainerResourceSampleTTL, 0, 0),
	}
}

func buildCleanNetworkingConfigInternal(containerInspect container.InspectResponse, apiVersion string) *network.NetworkingConfig {
	if containerInspect.NetworkSettings == nil || len(containerInspect.NetworkSettings.Networks) == 0 {
		return nil
	}

	endpointsConfig := compat.SanitizeContainerCreateEndpointSettingsForDockerAPI(containerInspect.NetworkSettings.Networks, apiVersion)
	for networkName, endpoint := range endpointsConfig {
		if endpoint == nil {
			continue
		}

		endpointCopy := *endpoint
		endpointCopy.IPAMConfig = nil
		endpointsConfig[networkName] = &endpointCopy
	}

	if len(endpointsConfig) == 0 {
		return nil
	}

	return &network.NetworkingConfig{
		EndpointsConfig: endpointsConfig,
	}
}

func buildRedeployBackupNameInternal(containerName, containerID string) string {
	backupName := containerName
	if backupName == "" {
		backupName = "arcane-redeploy"
		if len(containerID) >= 12 {
			backupName = fmt.Sprintf("%s-%s", backupName, containerID[:12])
		}
	}

	return fmt.Sprintf("%s-arcane-redeploy-%d", backupName, time.Now().Unix())
}

func shouldStartRedeployedContainerInternal(containerInfo container.InspectResponse, wasRunning bool) bool {
	if !wasRunning && containerInfo.HostConfig == nil {
		return false
	}

	shouldStart := wasRunning
	if containerInfo.HostConfig != nil {
		rp := containerInfo.HostConfig.RestartPolicy.Name
		if rp == "always" || rp == "unless-stopped" || rp == "on-failure" {
			shouldStart = true
		}
	}

	return shouldStart
}

func (
	s *ContainerService,
) pullRedeployImageInternal(
	ctx context.Context,
	dockerClient *client.Client,
	imageName, containerID, containerName, action string,
	credentials []containerregistry.Credential,
	user usertypes.Actor,
) error {
	localSettings := s.settingsService.GetSettingsConfig()
	pullCtx, pullCancel := context.WithTimeout(ctx, timeouts.GetDuration(localSettings.DockerImagePullTimeout.AsInt(), timeouts.DefaultDockerImagePull))
	defer pullCancel()

	pullOptions, authErr := s.imageService.PullOptionsWithAuth(ctx, imageName, credentials)
	if authErr != nil {
		return authErr
	}

	defer s.eventService.BeginDockerResourceSuppressionWindow("image", "", imageName)()
	reader, pullErr := dockerClient.ImagePull(pullCtx, imageName, pullOptions)

	if pullErr != nil {
		if errors.Is(pullCtx.Err(), context.DeadlineExceeded) {
			s.eventService.LogErrorEvent(ctx, event.EventTypeContainerError, "container", containerID, containerName, user.ID, user.Username, "0", pullErr, database.JSON{
				"action": action,
				"step":   "pull_image_timeout",
				"image":  imageName,
			})
			return fmt.Errorf("image pull timed out for %s (increase DOCKER_IMAGE_PULL_TIMEOUT or setting)", imageName)
		}

		s.eventService.LogErrorEvent(ctx, event.EventTypeContainerError, "container", containerID, containerName, user.ID, user.Username, "0", pullErr, database.JSON{
			"action": action,
			"step":   "pull_image",
			"image":  imageName,
		})
		return fmt.Errorf("failed to pull image %s: %w", imageName, pullErr)
	}
	defer func() { _ = reader.Close() }()

	progressWriter, _ := ctx.Value(types.ProgressWriterKey{}).(io.Writer)
	logWriter := docker.NewLogLineWriter(progressWriter)
	defer func() { _ = logWriter.Close() }()

	streamErr := docker.RenderJSONMessageStream(reader, logWriter)
	if streamErr != nil {
		s.eventService.LogErrorEvent(ctx, event.EventTypeContainerError, "container", containerID, containerName, user.ID, user.Username, "0", streamErr, database.JSON{
			"action": action,
			"step":   "complete_pull",
			"image":  imageName,
		})
		return fmt.Errorf("failed to complete image pull: %w", streamErr)
	}

	return nil
}

func (
	s *ContainerService,
) prepareContainerForRedeployInternal(
	ctx context.Context,
	dockerClient *client.Client,
	containerID, containerName, backupName string,
	wasRunning bool,
	user usertypes.Actor,
) error {
	s.eventService.MarkDockerExpectation("container", containerID, containerName)
	if containerName != "" {
		if _, err := dockerClient.ContainerRename(ctx, containerID, client.ContainerRenameOptions{NewName: backupName}); err != nil {
			s.eventService.LogErrorEvent(ctx, event.EventTypeContainerError, "container", containerID, containerName, user.ID, user.Username, "0", err, database.JSON{
				"action":     "redeploy",
				"step":       "rename_old",
				"backupName": backupName,
			})
			return fmt.Errorf("failed to rename existing container: %w", err)
		}
	}

	if !wasRunning {
		return nil
	}

	_, err := dockerClient.ContainerStop(ctx, containerID, client.ContainerStopOptions{Timeout: new(30)})
	if err == nil {
		return nil
	}

	if containerName != "" {
		if _, renameErr := dockerClient.ContainerRename(ctx, containerID, client.ContainerRenameOptions{NewName: containerName}); renameErr != nil {
			s.eventService.LogErrorEvent(ctx, event.EventTypeContainerError, "container", containerID, containerName, user.ID, user.Username, "0", renameErr, database.JSON{
				"action": "redeploy",
				"step":   "restore_name_after_stop_failure",
			})
		}
	}

	s.eventService.LogErrorEvent(ctx, event.EventTypeContainerError, "container", containerID, containerName, user.ID, user.Username, "0", err, database.JSON{
		"action": "redeploy",
		"step":   "stop",
	})
	return fmt.Errorf("failed to stop container: %w", err)
}

func (
	s *ContainerService,
) restoreContainerAfterRedeployFailureInternal(
	ctx context.Context,
	dockerClient *client.Client,
	containerID, containerName, backupName, failedStep string,
	wasRunning bool,
	user usertypes.Actor,
) {
	s.eventService.MarkDockerExpectation("container", containerID, containerName)
	if wasRunning {
		if _, startErr := dockerClient.ContainerStart(ctx, containerID, client.ContainerStartOptions{}); startErr != nil {
			s.eventService.LogErrorEvent(ctx, event.EventTypeContainerError, "container", containerID, containerName, user.ID, user.Username, "0", startErr, database.JSON{
				"action":     "redeploy",
				"step":       "restore_start_original",
				"failedStep": failedStep,
			})
		}
	}

	if containerName == "" {
		return
	}

	if _, renameErr := dockerClient.ContainerRename(ctx, containerID, client.ContainerRenameOptions{NewName: containerName}); renameErr != nil {
		s.eventService.LogErrorEvent(ctx, event.EventTypeContainerError, "container", containerID, backupName, user.ID, user.Username, "0", renameErr, database.JSON{
			"action":     "redeploy",
			"step":       "restore_name",
			"failedStep": failedStep,
		})
	}
}

// restoreAutoRemoveContainerAfterStartFailureInternal recreates and starts the
// original container when a deferred-stop auto-remove recreate fails to start
// the replacement. Stopping the original already made the daemon delete it, so
// the only rollback left is rebuilding from the inspected configuration. The
// failed replacement must be removed first so the original name is free again.
// A returned error means the original container is gone and could not be
// restored; the caller must surface it alongside the replacement failure.
func (
	s *ContainerService,
) restoreAutoRemoveContainerAfterStartFailureInternal(
	ctx context.Context,
	dockerClient *client.Client,
	containerInfo container.InspectResponse,
	apiVersion string,
	user usertypes.Actor,
) error {
	containerID := containerInfo.ID
	containerName := strings.TrimPrefix(containerInfo.Name, "/")

	originalConfig := *containerInfo.Config
	if len(containerID) >= 12 && originalConfig.Hostname == containerID[:12] {
		originalConfig.Hostname = ""
	}

	defer s.eventService.BeginDockerResourceSuppressionWindow("container", "", containerName)()
	createResp, err := compat.ContainerCreateWithCompatibilityForAPIVersion(ctx, dockerClient, client.ContainerCreateOptions{
		Config:           &originalConfig,
		HostConfig:       containerInfo.HostConfig,
		NetworkingConfig: buildCleanNetworkingConfigInternal(containerInfo, apiVersion),
		Name:             containerName,
	}, apiVersion)
	if err != nil {
		s.eventService.LogErrorEvent(ctx, event.EventTypeContainerError, "container", containerID, containerName, user.ID, user.Username, "0", err, database.JSON{
			"action": "redeploy",
			"step":   "restore_create_original",
		})
		return fmt.Errorf("failed to recreate original auto-remove container: %w", err)
	}

	defer s.eventService.BeginDockerResourceSuppressionWindow("container", createResp.ID, containerName)()
	if _, containerStartErr := dockerClient.ContainerStart(ctx, createResp.ID, client.ContainerStartOptions{}); containerStartErr != nil {
		s.eventService.LogErrorEvent(ctx, event.EventTypeContainerError, "container", createResp.ID, containerName, user.ID, user.Username, "0", containerStartErr, database.JSON{
			"action": "redeploy",
			"step":   "restore_start_original",
		})
		return fmt.Errorf("failed to restart original auto-remove container: %w", containerStartErr)
	}

	slog.InfoContext(
		ctx, "restored auto-remove container after failed recreate",
		"oldContainerId", containerID,
		"restoredContainerId", createResp.ID,
		"containerName", containerName,
	)
	return nil
}

// rollbackFailedReplacementStartInternal removes the replacement that failed
// to start and rolls back to the original container. For a deferred-stop
// auto-remove original the rollback rebuilds it from the inspected
// configuration; a failed restore is combined into the start error so the
// caller learns the workload is down, not just that the replacement failed.
func (
	s *ContainerService,
) rollbackFailedReplacementStartInternal(
	ctx context.Context,
	dockerClient *client.Client,
	containerInfo container.InspectResponse,
	replacementID, action, backupName string,
	stopAfterCreate, wasRunning bool,
	apiVersion string,
	startErr error,
	user usertypes.Actor,
) error {
	containerName := strings.TrimPrefix(containerInfo.Name, "/")

	s.eventService.MarkDockerExpectation("container", replacementID, containerName)
	var cleanupErr error
	if _, removeErr := dockerClient.ContainerRemove(ctx, replacementID, client.ContainerRemoveOptions{Force: true}); removeErr != nil {
		cleanupErr = fmt.Errorf("failed to remove replacement container: %w", removeErr)
		s.eventService.LogErrorEvent(ctx, event.EventTypeContainerError, "container", replacementID, containerName, user.ID, user.Username, "0", removeErr, database.JSON{
			"action": action,
			"step":   "cleanup_failed_start",
		})
	}

	if stopAfterCreate {
		if restoreErr := s.restoreAutoRemoveContainerAfterStartFailureInternal(ctx, dockerClient, containerInfo, apiVersion, user); restoreErr != nil {
			return errors.Join(startErr, cleanupErr, restoreErr)
		}
		return startErr
	}

	s.restoreContainerAfterRedeployFailureInternal(ctx, dockerClient, containerInfo.ID, containerName, backupName, "start", wasRunning, user)
	return startErr
}

type containerLifecycleActionInternal struct {
	action             string
	eventType          event.EventType
	metadata           database.JSON
	warnOnLogError     bool
	runContainerAction func(*client.Client) error
}

func (s *ContainerService) runContainerLifecycleActionInternal(ctx context.Context, containerID string, user usertypes.Actor, cfg containerLifecycleActionInternal) error {
	dockerClient, err := s.dockerService.GetClient(ctx)
	if err != nil {
		s.eventService.LogErrorEvent(ctx, event.EventTypeContainerError, "container", containerID, "", user.ID, user.Username, "0", err, database.JSON{"action": cfg.action})
		return fmt.Errorf("failed to connect to Docker: %w", err)
	}

	metadata := database.JSON{
		"action":      cfg.action,
		"containerId": containerID,
	}
	maps.Copy(metadata, cfg.metadata)

	err = s.eventService.LogContainerEvent(ctx, cfg.eventType, containerID, "name", user.ID, user.Username, "0", metadata)
	if err != nil {
		if !cfg.warnOnLogError {
			return fmt.Errorf("failed to log action: %w", err)
		}
		slog.WarnContext(ctx, "could not log container action", "action", cfg.action, "error", err)
	}

	defer s.eventService.BeginDockerResourceSuppressionWindow("container", containerID, "")()
	err = cfg.runContainerAction(dockerClient)
	if err != nil {
		s.eventService.LogErrorEvent(ctx, event.EventTypeContainerError, "container", containerID, "", user.ID, user.Username, "0", err, database.JSON{"action": cfg.action})
	}
	return err
}

func (s *ContainerService) StartContainer(ctx context.Context, containerID string, user usertypes.Actor) error {
	return s.runContainerLifecycleActionInternal(ctx, containerID, user, containerLifecycleActionInternal{
		action:         "start",
		eventType:      event.EventTypeContainerStart,
		warnOnLogError: true,
		runContainerAction: func(dockerClient *client.Client) error {
			_, err := dockerClient.ContainerStart(ctx, containerID, client.ContainerStartOptions{})
			return err
		},
	})
}

func (s *ContainerService) StopContainer(ctx context.Context, containerID string, user usertypes.Actor) error {
	return s.runContainerLifecycleActionInternal(ctx, containerID, user, containerLifecycleActionInternal{
		action:    "stop",
		eventType: event.EventTypeContainerStop,
		runContainerAction: func(dockerClient *client.Client) error {
			_, err := dockerClient.ContainerStop(ctx, containerID, client.ContainerStopOptions{Timeout: new(30)})
			return err
		},
	})
}

func (s *ContainerService) RestartContainer(ctx context.Context, containerID string, user usertypes.Actor) error {
	return s.runContainerLifecycleActionInternal(ctx, containerID, user, containerLifecycleActionInternal{
		action:    "restart",
		eventType: event.EventTypeContainerRestart,
		runContainerAction: func(dockerClient *client.Client) error {
			_, err := dockerClient.ContainerRestart(ctx, containerID, client.ContainerRestartOptions{})
			return err
		},
	})
}

// KillContainer sends a signal to the container's main process (default SIGKILL
// when signal is empty) without removing the container.
func (s *ContainerService) KillContainer(ctx context.Context, containerID, signal string, user usertypes.Actor) error {
	return s.runContainerLifecycleActionInternal(ctx, containerID, user, containerLifecycleActionInternal{
		action:         "kill",
		eventType:      event.EventTypeContainerKill,
		metadata:       database.JSON{"signal": signal},
		warnOnLogError: true,
		runContainerAction: func(dockerClient *client.Client) error {
			_, err := dockerClient.ContainerKill(ctx, containerID, client.ContainerKillOptions{Signal: signal})
			return err
		},
	})
}

// PauseContainer suspends all processes in the container.
func (s *ContainerService) PauseContainer(ctx context.Context, containerID string, user usertypes.Actor) error {
	return s.runContainerLifecycleActionInternal(ctx, containerID, user, containerLifecycleActionInternal{
		action:         "pause",
		eventType:      event.EventTypeContainerPause,
		warnOnLogError: true,
		runContainerAction: func(dockerClient *client.Client) error {
			_, err := dockerClient.ContainerPause(ctx, containerID, client.ContainerPauseOptions{})
			return err
		},
	})
}

// UnpauseContainer resumes a previously paused container.
func (s *ContainerService) UnpauseContainer(ctx context.Context, containerID string, user usertypes.Actor) error {
	return s.runContainerLifecycleActionInternal(ctx, containerID, user, containerLifecycleActionInternal{
		action:         "unpause",
		eventType:      event.EventTypeContainerUnpause,
		warnOnLogError: true,
		runContainerAction: func(dockerClient *client.Client) error {
			_, err := dockerClient.ContainerUnpause(ctx, containerID, client.ContainerUnpauseOptions{})
			return err
		},
	})
}

// CommitContainer creates an image from a container's current filesystem.
func (s *ContainerService) CommitContainer(ctx context.Context, containerID string, req containertypes.CommitRequest, user usertypes.Actor) (*containertypes.CommitResult, error) {
	containerID = strings.TrimSpace(containerID)
	if containerID == "" {
		return nil, errors.New("container ID is required")
	}

	dockerClient, err := s.dockerService.GetClient(ctx)
	if err != nil {
		s.eventService.LogErrorEvent(ctx, event.EventTypeImageError, "container", containerID, "", user.ID, user.Username, "0", err, database.JSON{"action": "commit"})
		return nil, fmt.Errorf("failed to connect to Docker: %w", err)
	}

	repository := strings.TrimSpace(req.Repository)
	tag := strings.TrimSpace(req.Tag)
	reference := kit.Ternary(repository != "" && tag != "", repository+":"+tag, repository)

	result, err := dockerClient.ContainerCommit(ctx, containerID, client.ContainerCommitOptions{
		Reference: reference,
		Comment:   strings.TrimSpace(req.Comment),
		Author:    strings.TrimSpace(req.Author),
		Changes:   req.Changes,
		NoPause:   req.NoPause,
	})
	if err != nil {
		s.eventService.LogErrorEvent(ctx, event.EventTypeImageError, "container", containerID, reference, user.ID, user.Username, "0", err, database.JSON{"action": "commit", "reference": reference})
		return nil, fmt.Errorf("failed to commit container: %w", err)
	}

	metadata := database.JSON{
		"action":      "commit",
		"containerId": containerID,
		"imageId":     result.ID,
		"repository":  repository,
		"tag":         tag,
		"reference":   reference,
		"noPause":     req.NoPause,
	}
	if logErr := s.eventService.LogImageEvent(ctx, event.EventTypeImageCommit, containerID, reference, user.ID, user.Username, "0", metadata); logErr != nil {
		slog.WarnContext(ctx, "could not log container commit action", "container", containerID, "image", result.ID, "error", logErr)
	}

	return &containertypes.CommitResult{ID: result.ID}, nil
}

// tryRedeployViaComposeProjectInternal attempts to redeploy a compose-managed
// container by delegating to project.ProjectService.UpdateProjectServices, which loads
// the compose project with full project_directory / env-file / include context
// and runs pull/stop/up for just the target service.
//
// Return semantics:
//   - handled=false: this container is not eligible for the compose path (no
//     labels, project not registered in Arcane's DB, etc.). The caller should
//     fall back to the standalone Docker-API redeploy.
//   - handled=true, err==nil: compose path ran successfully; newContainerID is
//     the ID of the recreated container (or the original ID if it couldn't be
//     re-located by labels).
//   - handled=true, err!=nil: compose path was attempted and failed. The
//     caller MUST surface the error and MUST NOT fall back to the standalone
//     path, which would clobber whatever partial state ComposeUp left behind.
func (
	s *ContainerService,
) tryRedeployViaComposeProjectInternal(
	ctx context.Context,
	containerInfo container.InspectResponse,
	containerID, containerName string,
	user usertypes.Actor,
) (
	string,
	bool,
	error,
) {
	if s.projectService == nil || containerInfo.Config == nil {
		return "", false, nil
	}
	containerLabels := containerInfo.Config.Labels
	projectName := docker.ComposeProjectLabel(containerLabels)
	serviceName := docker.ComposeServiceLabel(containerLabels)
	if projectName == "" || serviceName == "" {
		return "", false, nil
	}

	proj, err := s.projectService.GetProjectByComposeName(ctx, projectName)
	if err != nil {
		// Distinguish "not found" (safe to fall back to standalone) from real DB
		// errors (should surface so a transient failure doesn't silently recreate
		// the container from stale cached config).
		if errors.Is(err, common.ErrNotFound) {
			slog.WarnContext(
				ctx, "RedeployContainer: compose project not registered, falling back to standalone redeploy",
				"containerId", containerID,
				"project", projectName,
				"service", serviceName,
			)
			return "", false, nil
		}
		return "", true, fmt.Errorf("failed to look up compose project %s: %w", projectName, err)
	}
	if proj == nil {
		slog.WarnContext(
			ctx, "RedeployContainer: compose project not registered, falling back to standalone redeploy",
			"containerId", containerID,
			"project", projectName,
			"service", serviceName,
		)
		return "", false, nil
	}

	slog.InfoContext(
		ctx, "RedeployContainer: detected compose container, using project-based redeploy",
		"containerId", containerID,
		"project", projectName,
		"service", serviceName,
	)

	if updateProjectServicesErr := s.projectService.UpdateProjectServices(ctx, proj.ID, []string{serviceName}, user, false); updateProjectServicesErr != nil {
		s.eventService.LogErrorEvent(ctx, event.EventTypeContainerError, "container", containerID, containerName, user.ID, user.Username, "0", updateProjectServicesErr, database.JSON{
			"action":      "redeploy",
			"step":        "compose_update_services",
			"project":     projectName,
			"service":     serviceName,
			"projectId":   proj.ID,
			"projectName": proj.Name,
		})
		return "", true, fmt.Errorf("compose redeploy failed for %s/%s: %w", projectName, serviceName, updateProjectServicesErr)
	}

	newID, lookupErr := projects.FindComposeServiceContainerID(ctx, s.dockerService.DockerHost(), projectName, serviceName)
	if lookupErr != nil {
		slog.WarnContext(ctx, "failed to resolve container via compose ps after redeploy", "project", projectName, "service", serviceName, "err", lookupErr)
	}
	if newID == "" {
		// Recreated successfully but couldn't locate the new container; return the
		// original ID so the handler can degrade gracefully.
		newID = containerID
	}

	if logErr := s.eventService.LogContainerEvent(ctx, event.EventTypeContainerDeploy, newID, containerName, user.ID, user.Username, "0", database.JSON{
		"action":        "redeploy",
		"containerId":   newID,
		"containerName": containerName,
		"project":       projectName,
		"service":       serviceName,
		"projectId":     proj.ID,
		"via":           "compose",
	}); logErr != nil {
		slog.WarnContext(ctx, "failed to log compose redeploy event", "err", logErr)
	}

	return newID, true, nil
}

func (s *ContainerService) RedeployContainer(ctx context.Context, containerID string, user usertypes.Actor) (string, error) {
	dockerClient, err := s.dockerService.GetClient(ctx)
	if err != nil {
		s.eventService.LogErrorEvent(ctx, event.EventTypeContainerError, "container", containerID, "", user.ID, user.Username, "0", err, database.JSON{
			"action": "redeploy",
			"step":   "get_client",
		})
		return "", fmt.Errorf("failed to connect to Docker: %w", err)
	}

	containerJSON, err := compat.ContainerInspectWithCompatibility(ctx, dockerClient, containerID, client.ContainerInspectOptions{})
	if err != nil {
		s.eventService.LogErrorEvent(ctx, event.EventTypeContainerError, "container", containerID, "", user.ID, user.Username, "0", err, database.JSON{
			"action": "redeploy",
			"step":   "inspect",
		})
		return "", fmt.Errorf("failed to inspect container: %w", err)
	}

	containerInfo := containerJSON.Container
	if err := effectOriginGuard(containerInfo); err != nil {
		return "", err
	}
	if containerInfo.Config == nil {
		err = errors.New("container config is nil")
		s.eventService.LogErrorEvent(ctx, event.EventTypeContainerError, "container", containerID, "", user.ID, user.Username, "0", err, database.JSON{
			"action": "redeploy",
			"step":   "validate_config",
		})
		return "", fmt.Errorf("failed to redeploy container: %w", err)
	}

	containerName := strings.TrimPrefix(containerInfo.Name, "/")
	imageName := containerInfo.Config.Image
	apiVersion := compat.DetectDockerAPIVersion(ctx, dockerClient)

	currentContainerID, currentContainerErr := cgroup.CurrentContainerID()
	if labels.ShouldDisableArcaneServerRedeploy(containerInfo.Config.Labels, containerInfo.ID, currentContainerID, currentContainerErr) {
		err = errors.New("arcane cannot redeploy itself; use the system upgrade flow (Settings -> Updates) instead")
		s.eventService.LogErrorEvent(ctx, event.EventTypeContainerError, "container", containerID, containerName, user.ID, user.Username, "0", err, database.JSON{
			"action": "redeploy",
			"step":   "self_redeploy_blocked",
		})
		return "", err
	}

	if imageName != "" {
		if pullRedeployImageErr := s.pullRedeployImageInternal(ctx, dockerClient, imageName, containerID, containerName, "redeploy", nil, user); pullRedeployImageErr != nil {
			return "", pullRedeployImageErr
		}
	}

	networkingConfig := buildCleanNetworkingConfigInternal(containerInfo, apiVersion)
	newConfig := *containerInfo.Config

	return s.recreateContainerInternal(
		ctx,
		dockerClient,
		containerInfo,
		"redeploy",
		containerName,
		&newConfig,
		containerInfo.HostConfig,
		networkingConfig,
		apiVersion,
		event.EventTypeContainerDeploy,
		user,
	)
}

// recreateContainerInternal replaces an existing container with one created
// from the supplied config, using rename-as-backup so the original can be
// restored when create or start fails. Shared by redeploy and edit.
func (
	s *ContainerService,
) recreateContainerInternal(
	ctx context.Context,
	dockerClient *client.Client,
	containerInfo container.InspectResponse,
	action, newName string,
	newConfig *container.Config,
	newHostConfig *container.HostConfig,
	networkingConfig *network.NetworkingConfig,
	apiVersion string,
	eventType event.EventType,
	user usertypes.Actor,
) (
	string,
	error,
) {
	if err := effectOriginGuard(containerInfo); err != nil {
		return "", err
	}
	if updaterDTO.ExecutionIdentityFromContext(ctx).RunID != "" {
		return s.recreateDurable(ctx, dockerClient, containerInfo, newName, newConfig, newHostConfig, networkingConfig, apiVersion)
	}
	containerID := containerInfo.ID
	containerName := strings.TrimPrefix(containerInfo.Name, "/")
	wasRunning := containerInfo.State != nil && containerInfo.State.Running
	imageName := newConfig.Image

	// A running auto-remove container is deleted by the daemon the moment it
	// stops, which would make rollback impossible. For that case the stop is
	// deferred until the replacement has been created: the rename alone frees
	// the name, and a create failure rolls back to the untouched, still-running
	// original. Only a start failure can still lose the original.
	stopAfterCreate := wasRunning && containerInfo.HostConfig != nil && containerInfo.HostConfig.AutoRemove

	defer s.eventService.BeginDockerResourceSuppressionWindow("container", containerID, containerName)()
	backupName := buildRedeployBackupNameInternal(containerName, containerID)
	if err := s.prepareContainerForRedeployInternal(ctx, dockerClient, containerID, containerName, backupName, wasRunning && !stopAfterCreate, user); err != nil {
		return "", err
	}

	if len(containerID) >= 12 && newConfig.Hostname == containerID[:12] {
		newConfig.Hostname = ""
	}

	defer s.eventService.BeginDockerResourceSuppressionWindow("container", "", newName)()
	createResp, err := compat.ContainerCreateWithCompatibilityForAPIVersion(ctx, dockerClient, client.ContainerCreateOptions{
		Config:           newConfig,
		HostConfig:       newHostConfig,
		NetworkingConfig: networkingConfig,
		Name:             newName,
	}, apiVersion)
	if err != nil {
		s.restoreContainerAfterRedeployFailureInternal(ctx, dockerClient, containerID, containerName, backupName, "create", wasRunning && !stopAfterCreate, user)
		s.eventService.LogErrorEvent(ctx, event.EventTypeContainerError, "container", containerID, containerName, user.ID, user.Username, "0", err, database.JSON{
			"action": action,
			"step":   "create",
			"image":  imageName,
		})
		return "", fmt.Errorf("failed to recreate container: %w", err)
	}

	defer s.eventService.BeginDockerResourceSuppressionWindow("container", createResp.ID, newName)()

	if stopAfterCreate {
		if _, containerStopErr := dockerClient.ContainerStop(ctx, containerID, client.ContainerStopOptions{Timeout: new(30)}); containerStopErr != nil {
			if _, removeErr := dockerClient.ContainerRemove(ctx, createResp.ID, client.ContainerRemoveOptions{Force: true}); removeErr != nil {
				s.eventService.LogErrorEvent(ctx, event.EventTypeContainerError, "container", createResp.ID, containerName, user.ID, user.Username, "0", removeErr, database.JSON{
					"action": action,
					"step":   "cleanup_failed_stop",
				})
			}
			s.restoreContainerAfterRedeployFailureInternal(ctx, dockerClient, containerID, containerName, backupName, "stop", false, user)
			s.eventService.LogErrorEvent(ctx, event.EventTypeContainerError, "container", containerID, containerName, user.ID, user.Username, "0", containerStopErr, database.JSON{
				"action": action,
				"step":   "stop",
			})
			return "", fmt.Errorf("failed to stop container: %w", containerStopErr)
		}
	}

	if shouldStartRedeployedContainerInternal(containerInfo, wasRunning) {
		_, err = dockerClient.ContainerStart(ctx, createResp.ID, client.ContainerStartOptions{})
		if err != nil {
			err = s.rollbackFailedReplacementStartInternal(ctx, dockerClient, containerInfo, createResp.ID, action, backupName, stopAfterCreate, wasRunning, apiVersion, err, user)
			s.eventService.LogErrorEvent(ctx, event.EventTypeContainerError, "container", createResp.ID, containerName, user.ID, user.Username, "0", err, database.JSON{
				"action": action,
				"step":   "start",
				"image":  imageName,
			})
			return "", fmt.Errorf("failed to start new container: %w", err)
		}
	}

	slog.InfoContext(
		ctx, "container recreated successfully",
		"action", action,
		"oldContainerId", containerID,
		"newContainerId", createResp.ID,
		"containerName", newName,
		"image", imageName,
	)

	// After a deferred stop the daemon has already auto-removed the original.
	if !stopAfterCreate {
		if _, containerRemoveErr := dockerClient.ContainerRemove(ctx, containerID, client.ContainerRemoveOptions{
			Force:         true,
			RemoveVolumes: false,
			RemoveLinks:   false,
		}); containerRemoveErr != nil {
			slog.WarnContext(
				ctx, "failed to remove old container after successful recreate",
				"containerId", containerID,
				"backupName", backupName,
				"error", containerRemoveErr,
			)
		}
	}

	metadata := database.JSON{
		"action":        action,
		"containerId":   containerID,
		"containerName": newName,
		"image":         imageName,
	}
	if logErr := s.eventService.LogContainerEvent(ctx, eventType, createResp.ID, newName, user.ID, user.Username, "0", metadata); logErr != nil {
		slog.WarnContext(ctx, "failed to log recreate event", "err", logErr)
	}

	return createResp.ID, nil
}
