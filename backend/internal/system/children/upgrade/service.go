// Source: getarcaneapp/arcane/backend/internal/system/children/upgrade/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace

package upgrade

import (
	"cmp"
	"context"

	"errors"
	"fmt"

	digest "github.com/opencontainers/go-digest"
	"go.getarcane.app/kit/pkg"
	"io"

	"log/slog"

	"os"
	"regexp"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/distribution/reference"
	usertypes "github.com/florianfinn/docklet-next/types/user"
	versiontypes "github.com/florianfinn/docklet-next/types/version"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/client"

	"go.getarcane.app/docker"
	"go.getarcane.app/docker/compat"

	"go.getarcane.app/sys/cgroup"
	"go.getarcane.app/updater"
	"go.getarcane.app/updater/labels"
	"go.getarcane.app/updater/refs"
	updatertypes "go.getarcane.app/updater/types"
	"golang.org/x/mod/semver"

	"github.com/florianfinn/docklet-next/backend/internal/common"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	dockerInternal "github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/internal/event"
	"github.com/florianfinn/docklet-next/backend/internal/project"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/backend/internal/version"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/timeouts"
	"github.com/florianfinn/docklet-next/backend/pkg/projects"

	"github.com/florianfinn/docklet-next/backend/pkg/utils"
)

var upgradeLogNameInternal = regexp.MustCompile(`^arcane-upgrade-\d+\.log$`)

type Service struct {
	upgrading       atomic.Bool
	updatingAll     atomic.Bool
	db              *database.DB
	dockerService   *dockerInternal.DockerClientService
	versionService  *version.VersionService
	eventService    *event.EventService
	settingsService *settings.SettingsService
	projectService  *project.ProjectService
	// resolveRuntimeOptions determines how the upgrader container reaches the Docker daemon.
	resolveRuntimeOptions func(
		ctx context.Context,
		dockerHost string,
		currentContainer *container.InspectResponse,
		discoverHostPath func(context.Context, string) (string, error),
		isRunningInDocker func() bool,
		selectReachableNetwork func(context.Context, *container.InspectResponse, string) string,
	) ([]string, []mount.Mount, container.NetworkMode, error)
}

func NewService(
	db *database.DB,
	dockerService *dockerInternal.DockerClientService,
	versionService *version.VersionService,
	eventService *event.EventService,
	settingsService *settings.SettingsService,
	projectService *project.ProjectService,
	resolveRuntimeOptions func(
		ctx context.Context,
		dockerHost string,
		currentContainer *container.InspectResponse,
		discoverHostPath func(context.Context, string) (string, error),
		isRunningInDocker func() bool,
		selectReachableNetwork func(context.Context, *container.InspectResponse, string) string,
	) ([]string, []mount.Mount, container.NetworkMode, error),
) *Service {
	return &Service{
		db:                    db,
		dockerService:         dockerService,
		versionService:        versionService,
		eventService:          eventService,
		settingsService:       settingsService,
		projectService:        projectService,
		resolveRuntimeOptions: resolveRuntimeOptions,
	}
}

// CanUpgrade checks if self-upgrade is possible
func (s *Service) CanUpgrade(ctx context.Context) (bool, error) {
	// Check if running in Docker
	containerId, err := s.getCurrentContainerIDInternal(ctx)
	if err != nil {
		return false, err
	}

	// Verify we can access Docker
	_, err = s.dockerService.GetClient(ctx)
	if err != nil {
		return false, errors.New("docker socket is not accessible")
	}

	// Verify we can find our container
	_, err = s.findArcaneContainerInternal(ctx, containerId)
	if err != nil {
		return false, err
	}

	return true, nil
}

// AlreadyOnNewestImage reports whether this environment's version check is confident
// it already runs the newest image. A triggered upgrade still pulls, but will then
// find nothing to swap in and skip the recreate — so callers can use this to stop
// waiting for a restart that is not coming.
func (s *Service) AlreadyOnNewestImage(ctx context.Context) bool {
	return s.versionService.GetAppVersionInfo(ctx).AlreadyOnNewest()
}

// TriggerUpgradeViaCLI spawns the upgrade CLI command in a separate container and
// returns that upgrader container's ID. This avoids self-termination issues by running
// the upgrade from outside. A zero-value target resolves the image to upgrade to from
// the version check (see resolveSelfUpgradeTargetImageInternal); the updater engine
// passes an explicit target with the resolved new image, which is used as-is.
// Update-all uses the returned ID to tell an upgrade that recreated this
// container from one that found nothing to do — see watchManagerUpgraderInternal.
func (s *Service) TriggerUpgradeViaCLI(ctx context.Context, user usertypes.Actor, target updater.SelfUpdateTarget) (string, error) {
	prepared, err := s.prepareUpgradeInternal(ctx, user, target, "")
	if err != nil {
		return "", err
	}
	return s.runPreparedUpgradeInternal(ctx, prepared)
}

// TriggerUpgradeAsync validates synchronously, then pulls and spawns the upgrader in the background (#3628).
// The run follows the app lifecycle with a deadline so a stalled daemon cannot hold the upgrading guard forever.
func (s *Service) TriggerUpgradeAsync(ctx context.Context, user usertypes.Actor, targetVersion string) error {
	prepared, err := s.prepareUpgradeInternal(ctx, user, updater.SelfUpdateTarget{}, targetVersion)
	if err != nil {
		return err
	}
	localSettings := s.settingsService.GetSettingsConfig()
	runTimeout := timeouts.GetDuration(localSettings.DockerImagePullTimeout.AsInt(), timeouts.DefaultDockerImagePull) + time.Minute
	runCtx, cancel := context.WithTimeout(utils.ActivityRuntimeContext(ctx, nil), runTimeout)
	go func() {
		defer cancel()
		if _, runPreparedUpgradeErr := s.runPreparedUpgradeInternal(runCtx, prepared); runPreparedUpgradeErr != nil {
			slog.ErrorContext(ctx, "Background self-upgrade failed", "error", runPreparedUpgradeErr, "targetImage", prepared.targetImage)
		}
	}()
	return nil
}

type preparedUpgradeInternal struct {
	current       container.InspectResponse
	containerName string
	binaryPath    string
	// targetImage is the reference the recreated container runs as; pullImage
	// is the immutable reference actually pulled (the same unless frozen).
	targetImage string
	pullImage   string
	// saveCompose persists a tag change in the container's Compose project.
	saveCompose func(context.Context)
}

// prepareUpgradeInternal takes the upgrading guard, released by runPreparedUpgradeInternal (or here on error).
func (s *Service) prepareUpgradeInternal(ctx context.Context, user usertypes.Actor, target updater.SelfUpdateTarget, targetVersion string) (prepared *preparedUpgradeInternal, err error) {
	if !s.upgrading.CompareAndSwap(false, true) {
		return nil, common.Classify(common.ErrUpgradeInProgress, errors.New("an upgrade is already in progress"))
	}
	defer func() {
		if err != nil {
			s.upgrading.Store(false)
		}
	}()

	containerId := strings.TrimSpace(target.ContainerID)
	if containerId == "" {
		// Fall back to the container this process runs in
		containerId, err = s.getCurrentContainerIDInternal(ctx)
		if err != nil {
			return nil, fmt.Errorf("get current container: %w", err)
		}
	}

	currentContainer, err := s.findArcaneContainerInternal(ctx, containerId)
	if err != nil {
		return nil, fmt.Errorf("inspect container: %w", err)
	}

	containerName := strings.TrimPrefix(currentContainer.Name, "/")

	// Determine binary path based on container type (agent vs main)
	binaryPath := "/app/arcane"
	if currentContainer.Config != nil {
		binaryPath = kit.Ternary(labels.IsArcaneAgentContainer(currentContainer.Config.Labels), "/app/arcane-agent", "/app/arcane")
	}

	targetImage, err := s.resolveUpgradeTargetImageInternal(ctx, currentContainer, target.NewImageRef, targetVersion)
	if err != nil {
		return nil, err
	}
	targetImage, saveCompose, err := s.configuredTargetImageInternal(ctx, currentContainer, targetImage)
	if err != nil {
		return nil, err
	}
	pullImage := cmp.Or(strings.TrimSpace(target.PullImageRef), targetImage)

	// Log upgrade event
	metadata := database.JSON{
		"action":        "system_upgrade_cli",
		"containerId":   containerId,
		"containerName": containerName,
		"method":        "cli",
		"targetImage":   targetImage,
		"pullImage":     pullImage,
	}
	if logUserEventErr := s.eventService.LogUserEvent(ctx, event.EventTypeSystemUpgrade, user.ID, user.Username, metadata); logUserEventErr != nil {
		slog.WarnContext(ctx, "Failed to log upgrade event", "error", logUserEventErr)
	}

	return &preparedUpgradeInternal{
		current:       currentContainer,
		containerName: containerName,
		binaryPath:    binaryPath,
		targetImage:   targetImage,
		pullImage:     pullImage,
		saveCompose:   saveCompose,
	}, nil
}

func (s *Service) runPreparedUpgradeInternal(ctx context.Context, prepared *preparedUpgradeInternal) (string, error) {
	defer s.upgrading.Store(false)

	// Run the upgrader from the image we are upgrading to, so the upgrade CLI
	// is the new version.
	upgraderImage := prepared.pullImage
	slog.DebugContext(ctx, "Using upgrader image", "image", upgraderImage)

	slog.InfoContext(ctx, "Spawning upgrade CLI command", "containerName", prepared.containerName, "upgraderImage", upgraderImage)

	// Spawn the upgrade command in a detached container
	// This will run independently of the current container
	dockerClient, err := s.dockerService.GetClient(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to connect to Docker: %w", err)
	}

	// Pull the upgrader image first to ensure it exists
	slog.InfoContext(ctx, "Pulling upgrader image", "image", upgraderImage)

	localSettings := s.settingsService.GetSettingsConfig()
	pullCtx, pullCancel := context.WithTimeout(ctx, timeouts.GetDuration(localSettings.DockerImagePullTimeout.AsInt(), timeouts.DefaultDockerImagePull))
	defer pullCancel()

	pullReader, err := dockerClient.ImagePull(pullCtx, upgraderImage, client.ImagePullOptions{})
	if err != nil {
		if errors.Is(pullCtx.Err(), context.DeadlineExceeded) {
			return "", fmt.Errorf("upgrader image pull timed out for %s (increase DOCKER_IMAGE_PULL_TIMEOUT or setting)", upgraderImage)
		}
		return "", fmt.Errorf("pull upgrader image: %w", err)
	}
	// Drain and validate the JSON stream to complete the pull.
	if renderJSONMessageStreamErr := docker.RenderJSONMessageStream(pullReader, io.Discard); renderJSONMessageStreamErr != nil {
		_ = pullReader.Close()
		return "", fmt.Errorf("failed to complete upgrader image pull: %w", renderJSONMessageStreamErr)
	}
	if closeErr := pullReader.Close(); closeErr != nil {
		slog.WarnContext(ctx, "Failed to close upgrader image pull reader", "error", closeErr)
	}
	slog.InfoContext(ctx, "Upgrader image pulled successfully", "image", upgraderImage)
	if prepared.saveCompose != nil {
		prepared.saveCompose(ctx)
	}

	// Try to get the /app/data mount from current container so upgrade logs persist.
	appDataMount := docker.MountForDestination(prepared.current.Mounts, libarcane.UpgradeLogDirectory, libarcane.UpgradeLogDirectory)
	if appDataMount == nil {
		slog.WarnContext(ctx, "Could not detect /app/data mount; upgrader logs may not persist")
	} else {
		slog.DebugContext(ctx, "Mounting /app/data into upgrader container", "type", appDataMount.Type, "source", appDataMount.Source)
	}

	// Create the upgrader container config
	containerEnv, runtimeMounts, networkMode, err := s.resolveRuntimeOptions(
		ctx,
		s.dockerService.DockerHost(),
		&prepared.current,
		func(ctx context.Context, containerPath string) (string, error) {
			return projects.GetHostPathForContainerPath(ctx, dockerClient, containerPath)
		},
		func() bool {
			_, currentContainerIDErr := cgroup.CurrentContainerID()
			return currentContainerIDErr == nil
		},
		func(ctx context.Context, inspect *container.InspectResponse, dockerHost string) string {
			return docker.SelectDockerHostReachableNetworkMode(ctx, dockerClient, inspect, dockerHost)
		},
	)
	if err != nil {
		return "", fmt.Errorf("resolve upgrader docker runtime: %w", err)
	}

	upgradeCmd := []string{prepared.binaryPath, "upgrade", "--container", prepared.containerName}
	if prepared.targetImage != "" {
		upgradeCmd = append(upgradeCmd, "--image", prepared.targetImage)
	}
	if prepared.pullImage != prepared.targetImage {
		upgradeCmd = append(upgradeCmd, "--pull-image", prepared.pullImage)
	}

	config := &container.Config{
		Image: upgraderImage,
		Cmd:   upgradeCmd,
		// The upgrader needs root for the Docker socket; unlike the server it
		// is short-lived and never goes through the runtime-identity drop, so
		// don't rely on the image's default user.
		User: "0:0",
		Env:  containerEnv,
		Labels: map[string]string{
			"com.getarcaneapp.arcane.upgrader": "true",
			"com.getarcaneapp.arcane":          "true",
		},
	}

	mounts := append([]mount.Mount{}, runtimeMounts...)
	if appDataMount != nil {
		mounts = append(mounts, *appDataMount)
	}

	keepUpgraderContainer := strings.EqualFold(strings.TrimSpace(os.Getenv("ARCANE_UPGRADE_KEEP_CONTAINER")), "true")
	if keepUpgraderContainer {
		slog.InfoContext(ctx, "Keeping upgrader container after exit (ARCANE_UPGRADE_KEEP_CONTAINER=true)")
	}

	hostConfig := &container.HostConfig{
		AutoRemove:  !keepUpgraderContainer, // default: clean up after completion
		Mounts:      mounts,
		NetworkMode: networkMode,
	}
	// Inherit the security context that lets the running Arcane container reach
	// the Docker socket (e.g. SELinux label=disable, privileged); the upgrader
	// needs the same access on hardened hosts.
	if prepared.current.HostConfig != nil {
		hostConfig.SecurityOpt = slices.Clone(prepared.current.HostConfig.SecurityOpt)
		hostConfig.Privileged = prepared.current.HostConfig.Privileged
	}
	// On SELinux-enforcing hosts the socket carries container_var_run_t, which
	// container processes cannot connect to regardless of UID; without an
	// explicit label opt the upgrader would exit with EACCES and auto-remove.
	if !hostConfig.Privileged && !hasSELinuxLabelOptInternal(hostConfig.SecurityOpt) && daemonHasSELinuxEnabledInternal(ctx, dockerClient) {
		hostConfig.SecurityOpt = append(hostConfig.SecurityOpt, "label=disable")
	}

	upgraderName := fmt.Sprintf("%s-upgrader-%d", prepared.containerName, time.Now().Unix())

	resp, err := dockerClient.ContainerCreate(ctx, client.ContainerCreateOptions{
		Config:     config,
		HostConfig: hostConfig,
		Name:       upgraderName,
	})
	if err != nil {
		return "", fmt.Errorf("create upgrader container: %w", err)
	}

	// Start the upgrader container - it will run the upgrade and auto-remove
	if _, containerStartErr := dockerClient.ContainerStart(ctx, resp.ID, client.ContainerStartOptions{}); containerStartErr != nil {
		_, _ = dockerClient.ContainerRemove(ctx, resp.ID, client.ContainerRemoveOptions{Force: true})
		return "", fmt.Errorf("start upgrader container: %w", containerStartErr)
	}

	slog.InfoContext(ctx, "Upgrade container started", "upgraderId", resp.ID[:12], "upgraderName", upgraderName)

	return resp.ID, nil
}

// hasSELinuxLabelOptInternal reports whether the security options already set
// an SELinux label policy (e.g. "label=disable", "label:disable", "label=type:...").
func hasSELinuxLabelOptInternal(securityOpts []string) bool {
	for _, opt := range securityOpts {
		if strings.HasPrefix(strings.TrimSpace(opt), "label") {
			return true
		}
	}
	return false
}

func daemonHasSELinuxEnabledInternal(ctx context.Context, dockerClient *client.Client) bool {
	infoResult, err := dockerClient.Info(ctx, client.InfoOptions{})
	if err != nil {
		slog.DebugContext(ctx, "Failed to query daemon info for SELinux detection", "error", err)
		return false
	}
	return slices.Contains(infoResult.Info.SecurityOptions, "name=selinux")
}

// configuredTargetImageInternal spells targetImage like the container's Compose
// service, or like its runtime image when Arcane does not manage that project.
// A tag change also returns the Compose edit to save before the upgrader starts;
// sources Arcane cannot edit keep upgrading without it, as they did before.
func (s *Service) configuredTargetImageInternal(ctx context.Context, current container.InspectResponse, targetImage string) (string, func(context.Context), error) {
	// Digest targets cannot be written as Compose tags.
	if refs.NormalizeImageUpdateRef(targetImage) == "" || current.Config == nil {
		return targetImage, nil, nil
	}
	runtimeRef := current.Config.Image
	projectName, serviceName := docker.ComposeProjectLabel(current.Config.Labels), docker.ComposeServiceLabel(current.Config.Labels)
	if projectName == "" || serviceName == "" {
		return refs.PreserveConfiguredRef(runtimeRef, targetImage), nil, nil
	}
	projectID, composeRef, err := s.projectService.ComposeServiceImage(ctx, projectName, serviceName)
	if errors.Is(err, common.ErrNotFound) {
		return refs.PreserveConfiguredRef(runtimeRef, targetImage), nil, nil
	}
	if err != nil {
		return "", nil, fmt.Errorf("resolve Compose service %s/%s: %w", projectName, serviceName, err)
	}
	selected := refs.PreserveConfiguredRef(composeRef, targetImage)
	normalizedCompose := refs.NormalizeImageUpdateRef(composeRef)
	if normalizedCompose == refs.NormalizeImageUpdateRef(targetImage) {
		return selected, nil, nil
	}
	if normalizedCompose != refs.NormalizeImageUpdateRef(runtimeRef) {
		return "", nil, fmt.Errorf("compose service %s/%s image %s does not match running image %s", projectName, serviceName, composeRef, runtimeRef)
	}
	changes := map[string]updatertypes.ServiceImageChange{serviceName: {ExpectedRef: runtimeRef, TargetRef: selected}}
	return selected, func(ctx context.Context) {
		if _, saveErr := s.projectService.SaveProjectServiceImages(ctx, projectID, changes); saveErr != nil {
			slog.WarnContext(ctx, "Compose image not updated for self-upgrade; update it manually", "project", projectName, "service", serviceName, "image", selected, "error", saveErr)
		}
	}, nil
}

// resolveUpgradeTargetImageInternal picks the image the upgrade should move to.
// Explicit targets from the updater engine are authoritative. A blank target
// (manual trigger, update-all) resolves against the version check so a
// version-pinned install actually moves to the newest release (#3687).
func (s *Service) resolveUpgradeTargetImageInternal(ctx context.Context, currentContainer container.InspectResponse, explicitImageRef, targetVersion string) (string, error) {
	if targetImage := strings.TrimSpace(explicitImageRef); targetImage != "" {
		return targetImage, nil
	}

	currentImageRef := ""
	if currentContainer.Config != nil {
		currentImageRef = strings.TrimSpace(currentContainer.Config.Image)
	}
	info := s.versionService.GetAppVersionInfo(ctx)
	// A manager-supplied target version outranks this instance's own version check.
	if targetVersion = strings.TrimSpace(targetVersion); targetVersion != "" {
		merged := versiontypes.Info{}
		if info != nil {
			merged = *info
		}
		merged.NewestVersion = targetVersion
		info = &merged
	}
	resolved, err := resolveSelfUpgradeTargetImageInternal(currentImageRef, info)
	if err != nil {
		return "", fmt.Errorf("resolve upgrade target image: %w", err)
	}
	return resolved, nil
}

// resolveSelfUpgradeTargetImageInternal resolves the image for a blank-target
// self-upgrade from the running container's reference and the version check.
// Exact release tags (vX.Y.Z) move to the newest release; mutable channels and
// untagged references keep theirs; digest pins move only to a resolved newest
// digest. Unresolved or older exact-version targets fail instead of
// reinstalling or downgrading (#3687).
func resolveSelfUpgradeTargetImageInternal(currentImageRef string, info *versiontypes.Info) (string, error) {
	currentImageRef = strings.TrimSpace(currentImageRef)
	if currentImageRef == "" {
		return "", errors.New("running container has no image reference to upgrade from")
	}

	parsed, err := reference.Parse(currentImageRef)
	if err != nil {
		return "", fmt.Errorf("parse current image reference %q: %w", currentImageRef, err)
	}
	named, ok := parsed.(reference.Named)
	if !ok {
		return "", fmt.Errorf("current image reference %q is not a named image", currentImageRef)
	}

	newestDigest, newestVersion := newestTargetIdentifiersInternal(info)

	// Digest-pinned installs only move when the version check resolved a new
	// digest; otherwise there is nothing to point them at.
	if _, isDigested := parsed.(reference.Digested); isDigested {
		if newestDigest == "" {
			return "", errors.New("image is digest-pinned but no newest digest could be resolved; refusing an upgrade that could not move it")
		}
		targetDigest := digest.Digest(newestDigest)
		if validateErr := targetDigest.Validate(); validateErr != nil {
			return "", fmt.Errorf("resolved newest digest %q is not a valid digest: %w", newestDigest, validateErr)
		}
		withDigest, withDigestErr := reference.WithDigest(named, targetDigest)
		if withDigestErr != nil {
			return "", fmt.Errorf("build target reference for %q: %w", named.Name(), withDigestErr)
		}
		return withDigest.String(), nil
	}

	tagged, isTagged := parsed.(reference.Tagged)
	if !isTagged || !isExactReleaseTagInternal(tagged.Tag()) {
		// Mutable channel or untagged: the pull re-resolves whatever the name
		// points at, so keeping the reference advances the digest in place.
		return currentImageRef, nil
	}

	if newestVersion == "" {
		return "", fmt.Errorf("running exact release %q but the newest release could not be resolved", tagged.Tag())
	}
	newest := kit.EnsurePrefix(newestVersion, "v")
	if !semver.IsValid(newest) {
		return "", fmt.Errorf("resolved newest version %q is not a valid semver release", newestVersion)
	}
	if semver.Compare(newest, kit.EnsurePrefix(tagged.Tag(), "v")) < 0 {
		return "", fmt.Errorf("newest release %q is older than the running %q; refusing to downgrade", newestVersion, tagged.Tag())
	}

	withTag, err := reference.WithTag(named, newest)
	if err != nil {
		return "", fmt.Errorf("build target reference for %q: %w", named.Name(), err)
	}
	return withTag.String(), nil
}

// isExactReleaseTagInternal reports whether tag pins an exact release version
// (vX.Y.Z, prereleases included). Short channels like v2 or v2.9 are mutable.
// The x/mod/semver parser is deliberately lenient (it accepts v2 and v2.9), so
// the three numeric components are checked explicitly.
func isExactReleaseTagInternal(tag string) bool {
	core := strings.TrimPrefix(strings.TrimSpace(tag), "v")
	if i := strings.IndexAny(core, "-+"); i >= 0 {
		core = core[:i]
	}
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return false
	}
	for _, part := range parts {
		if part == "" {
			return false
		}
		for _, c := range part {
			if c < '0' || c > '9' {
				return false
			}
		}
	}
	return true
}

// newestTargetIdentifiersInternal reads the version-check result nil-safely: the
// service may be unavailable and the check itself may resolve neither identifier.
func newestTargetIdentifiersInternal(info *versiontypes.Info) (newestDigest, newestVersion string) {
	if info == nil {
		return "", ""
	}
	return strings.TrimSpace(info.NewestDigest), strings.TrimSpace(info.NewestVersion)
}

// getCurrentContainerID detects if we're running in Docker and returns container ID
func (s *Service) getCurrentContainerIDInternal(ctx context.Context) (string, error) {
	// cgroup detection fails on cgroupv2 with a private namespace, and with
	// network_mode: service:<sidecar> the hostname identifies the sidecar —
	// InspectCurrentArcaneContainer adds the Arcane-label fallback (#3544).
	dockerClient, err := s.dockerService.GetClient(ctx)
	if err != nil {
		return "", err
	}
	inspect, err := libarcane.InspectCurrentArcaneContainer(ctx, dockerClient)
	if err != nil {
		return "", errors.New("arcane is not running in a Docker container")
	}
	return inspect.ID, nil
}

// findArcaneContainer finds the container using the ID
func (s *Service) findArcaneContainerInternal(ctx context.Context, containerId string) (container.InspectResponse, error) {
	dockerClient, err := s.dockerService.GetClient(ctx)
	if err != nil {
		return container.InspectResponse{}, err
	}

	// Try to inspect the container directly
	inspectResult, err := compat.ContainerInspectWithCompatibility(ctx, dockerClient, containerId, client.ContainerInspectOptions{})
	if err == nil {
		return inspectResult.Container, nil
	}

	// Fallback: search for containers with arcane image
	filter := make(client.Filters)
	filter = filter.Add("ancestor", "ghcr.io/getarcaneapp/arcane")

	containers, err := dockerClient.ContainerList(ctx, client.ContainerListOptions{
		All:     true,
		Filters: filter,
	})
	if err != nil {
		return container.InspectResponse{}, err
	}

	for _, c := range containers.Items {
		if strings.HasPrefix(c.ID, containerId) {
			inspect, inspectErr := compat.ContainerInspectWithCompatibility(ctx, dockerClient, c.ID, client.ContainerInspectOptions{})
			if inspectErr != nil {
				return container.InspectResponse{}, inspectErr
			}
			return inspect.Container, nil
		}
	}

	// Try without filter - search all containers
	allContainers, err := dockerClient.ContainerList(ctx, client.ContainerListOptions{All: true})
	if err != nil {
		return container.InspectResponse{}, err
	}

	for _, c := range allContainers.Items {
		if strings.HasPrefix(c.ID, containerId) || c.ID == containerId {
			inspect, inspectErr := compat.ContainerInspectWithCompatibility(ctx, dockerClient, c.ID, client.ContainerInspectOptions{})
			if inspectErr != nil {
				return container.InspectResponse{}, inspectErr
			}
			return inspect.Container, nil
		}
	}

	return container.InspectResponse{}, common.Classify(common.ErrNotFound, errors.New("could not find Arcane container"))
}

const (
	// The run never finished observably: nothing may be concluded from it.
	upgraderExitUnobservedInternal upgraderExitInternal = 0
	// The upgrader exited 0.
	upgraderExitSucceededInternal upgraderExitInternal = 1
	// The upgrader exited non-zero.
	upgraderExitFailedInternal upgraderExitInternal = 2
	// The upgrader is gone, but its exit code was lost with it — success and failure
	// are indistinguishable.
	upgraderExitCodeLostInternal upgraderExitInternal = 3

	updateAllStaleThresholdInternal      = time.Hour
	updateAllAgentRequestTimeoutInternal = 15 * time.Second
	updateAllConfirmPollIntervalInternal = 10 * time.Second
	updateAllConfirmTimeoutInternal      = 5 * time.Minute
	updateAllErrorMaxLenInternal         = 500

	// How long to watch the manager's upgrader container for an exit. Generous: it
	// pulls the target image before recreating anything.
	updateAllManagerWatchTimeoutInternal = 15 * time.Minute
	// How long to let a recreate that is already underway stop this container before
	// concluding the manager was up to date and no restart is coming.
	updateAllManagerNoRestartGraceInternal = 15 * time.Second
)

// StartUpdateAll begins a fleet-wide update. The agents phase runs first in the
// background (while the manager is up); the agents goroutine triggers the manager
// self-upgrade as its final step (job left pending_restart, finalized at next
// boot). Every environment pulls the latest image, whether or not it reports an
// update available.
func (s *Service) StartUpdateAll(ctx context.Context, user usertypes.Actor, env *environment.EnvironmentService) (*EnvironmentUpdateJob, error) {
	// Guard the check-then-create against concurrent callers (e.g. a double-click).
	// A dedicated flag rather than s.upgrading, because the manager branch below
	// calls TriggerUpgradeViaCLI which acquires s.upgrading itself. The persisted
	// job row is the durable guard once committed; this only closes the in-process
	// window before that row exists.
	if !s.updatingAll.CompareAndSwap(false, true) {
		return nil, common.Classify(common.ErrUpdateAllInProgress, errors.New("an update-all job is already in progress"))
	}
	defer s.updatingAll.Store(false)

	active, err := s.activeUpdateAllJobInternal(ctx)
	if err != nil {
		return nil, fmt.Errorf("check for active update-all job: %w", err)
	}
	if active != nil {
		return nil, common.Classify(common.ErrUpdateAllInProgress, errors.New("an update-all job is already in progress"))
	}

	info := s.versionService.GetAppVersionInfo(ctx)

	managerResult := EnvironmentUpdateResult{
		EnvironmentID:   environment.LocalEnvironmentID,
		EnvironmentName: env.ResolveEnvironmentName(ctx, environment.LocalEnvironmentID),
		FromVersion:     info.CurrentVersion,
	}

	// Seed a pending row for every remote environment up front so the dialog can
	// show the whole fleet immediately instead of popping rows in as each finishes.
	// Best effort: the agents phase re-lists authoritatively and fills any gaps.
	remoteResults := s.seedRemoteResultsInternal(ctx, env)

	job := &EnvironmentUpdateJob{
		UserID:                user.ID,
		Username:              user.Username,
		ManagerVersionAtStart: info.CurrentVersion,
		ManagerDigestAtStart:  info.CurrentDigest,
		ManagerTargetVersion:  updateAllTargetVersionInternal(info),
	}

	// Manager upgrades LAST. Seed its row pending; the agents-phase goroutine
	// flips it to updating right before triggering the self-upgrade.
	managerResult.Status = EnvironmentUpdateResultStatusPending
	managerResult.ToVersion = job.ManagerTargetVersion

	// Running from the start: the agents phase happens first, while the backend is
	// up and can report progress. The manager self-upgrade fires at the very end of
	// the agents goroutine.
	job.Status = EnvironmentUpdateJobStatusRunning
	job.Results = append(EnvironmentUpdateResults{managerResult}, remoteResults...)

	if createUpgradeJobErr := s.db.WithContext(ctx).Create(job).Error; createUpgradeJobErr != nil {
		return nil, fmt.Errorf("create update-all job: %w", createUpgradeJobErr)
	}

	slog.InfoContext(ctx, "Update-all started; upgrading agents first", "jobId", job.ID, "user", user.Username)
	go s.runAgentsPhaseInternal(context.WithoutCancel(ctx), job.ID, env, user)

	return job, nil
}

// ResumeUpdateAllOnStartup is called once at manager startup. When the manager
// self-upgraded as the final step of an update-all (job left pending_restart), the
// agents phase already ran before the restart — so this finalizes the manager's own
// result and closes the job. It is a no-op when there is nothing pending.
func (s *Service) ResumeUpdateAllOnStartup(ctx context.Context) {
	job, err := s.activeUpdateAllJobInternal(ctx)
	if err != nil {
		slog.WarnContext(ctx, "Failed to load pending update-all job on startup", "error", err)
		return
	}
	if job == nil {
		return
	}

	// A job left running means the manager died mid-agents-phase, before it reached
	// the manager step; we can't safely resume partial progress, so fail it.
	if job.Status == EnvironmentUpdateJobStatusRunning {
		s.markUpdateAllFailedInternal(ctx, job, "interrupted by manager restart")
		return
	}

	info := s.versionService.GetAppVersionInfo(ctx)
	action := resolveResumeActionInternal(job, info.CurrentVersion, info.CurrentDigest, time.Now())

	if action.markStale {
		s.markUpdateAllFailedInternal(ctx, job, "update-all job is stale; manager did not restart in time")
		return
	}

	// The agents phase already ran before the restart. Finalize the manager's own
	// result and close the job — do not re-run the agents phase.
	managerStatus := kit.Ternary(action.managerSucceeded, EnvironmentUpdateResultStatusUpdated, EnvironmentUpdateResultStatusFailed)
	s.recordManagerResultInternal(job, managerStatus, info.CurrentVersion)
	s.finalizeUpdateAllJobInternal(ctx, job)

	slog.InfoContext(ctx, "Finalized update-all job after manager restart", "jobId", job.ID, "managerUpgraded", action.managerSucceeded)
}

type resumeActionInternal struct {
	markStale        bool
	managerSucceeded bool
}

// resolveResumeActionInternal is the pure decision for a resumed pending_restart
// job: stale if it has waited too long, otherwise the manager upgrade is considered
// successful when either the version or the digest changed from the at-start values
// (digest covers non-semver, digest-pinned installs), or when the manager is already
// on the recorded target — a force-update of an up-to-date manager recreates the
// container without changing either.
func resolveResumeActionInternal(job *EnvironmentUpdateJob, currentVersion, currentDigest string, now time.Time) resumeActionInternal {
	if now.Sub(job.CreatedAt) > updateAllStaleThresholdInternal {
		return resumeActionInternal{markStale: true}
	}

	versionChanged := job.ManagerVersionAtStart != "" && currentVersion != job.ManagerVersionAtStart
	digestChanged := job.ManagerDigestAtStart != "" && currentDigest != job.ManagerDigestAtStart
	// ManagerTargetVersion holds the newest version tag when known, otherwise the
	// newest digest — compare against both current identifiers.
	onTarget := job.ManagerTargetVersion != "" &&
		(strings.TrimPrefix(currentVersion, "v") == strings.TrimPrefix(job.ManagerTargetVersion, "v") ||
			currentDigest == job.ManagerTargetVersion)

	return resumeActionInternal{managerSucceeded: versionChanged || digestChanged || onTarget}
}

// runAgentsPhaseInternal upgrades every online remote environment sequentially,
// persisting progress after each one so the status endpoint can report live
// progress, then triggers the manager's own self-upgrade as the final step
// (leaving the job pending_restart for the next boot to finalize).
func (s *Service) runAgentsPhaseInternal(ctx context.Context, jobID string, env *environment.EnvironmentService, user usertypes.Actor) {
	job, err := s.getUpdateAllJobByIDInternal(ctx, jobID)
	if err != nil || job == nil {
		slog.WarnContext(ctx, "update-all: failed to reload job for agents phase", "jobId", jobID, "error", err)
		return
	}

	envs, err := env.ListRemoteEnvironments(ctx)
	if err != nil {
		s.markUpdateAllFailedInternal(ctx, job, fmt.Sprintf("failed to list remote environments: %v", err))
		return
	}

	for _, remote := range envs {
		// Find the row seeded at job start (or append one if seeding missed this
		// environment) and mark it updating so the dialog shows a live indicator on
		// the row currently being processed.
		idx := upsertPendingResultInternal(job, remote.ID, remote.Name)
		job.Results[idx].Status = EnvironmentUpdateResultStatusUpdating
		if persistUpdateAllJobErr := s.persistUpdateAllJobInternal(ctx, job); persistUpdateAllJobErr != nil {
			slog.WarnContext(ctx, "update-all: failed to persist updating status", "jobId", job.ID, "environmentId", remote.ID, "error", persistUpdateAllJobErr)
		}

		s.upgradeAgentInternal(ctx, env, remote.ID, job, &job.Results[idx])

		if saveRemoteProgressErr := s.persistUpdateAllJobInternal(ctx, job); saveRemoteProgressErr != nil {
			slog.WarnContext(ctx, "update-all: failed to persist progress", "jobId", job.ID, "environmentId", remote.ID, "error", saveRemoteProgressErr)
		}
	}

	// All remote agents processed. Handle the manager LAST: flip its row pending ->
	// updating and move the job to pending_restart, persisting BEFORE the trigger so
	// that if the manager dies the instant the upgrader starts, the next boot sees
	// pending_restart and finalizes it.
	manager := managerResultInternal(job)
	if manager != nil {
		manager.Status = EnvironmentUpdateResultStatusUpdating
		manager.Stage = EnvironmentUpdateStageStarting
		manager.StageStartedAt = new(time.Now())
	}
	job.Status = EnvironmentUpdateJobStatusPendingRestart
	if savePendingRestartErr := s.persistUpdateAllJobInternal(ctx, job); savePendingRestartErr != nil {
		slog.WarnContext(ctx, "update-all: failed to persist pending_restart before manager upgrade", "jobId", job.ID, "error", savePendingRestartErr)
	}

	// Snapshot whether the manager was already on the newest image BEFORE triggering:
	// only that makes a no-op upgrader run an expected outcome. Checked after the fact
	// the same answer proves nothing, since it was already true going in.
	wasAlreadyNewest := s.AlreadyOnNewestImage(ctx)

	upgraderID, err := s.TriggerUpgradeViaCLI(ctx, user, updater.SelfUpdateTarget{})
	if err != nil {
		// Agents already ran and no restart is coming, so finalize now. This flips
		// the manager's updating row to failed with the reason.
		s.markUpdateAllFailedInternal(ctx, job, fmt.Sprintf("manager upgrade trigger failed: %v", err))
		return
	}

	if manager != nil {
		s.setUpdateStageInternal(ctx, job, manager, EnvironmentUpdateStageReconnecting)
	}

	slog.InfoContext(ctx, "Update-all: agents done, manager self-upgrade triggered", "jobId", job.ID, "upgraderId", upgraderID)
	s.watchManagerUpgraderInternal(ctx, job.ID, upgraderID, wasAlreadyNewest)
}
