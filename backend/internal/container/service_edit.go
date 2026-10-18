// Source: getarcaneapp/arcane/backend/internal/container/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; split lifecycle, editing, inventory and stream concerns; exclude Docklet agent inventory.

package container

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net/netip"
	"strings"
	"time"

	"github.com/containerd/errdefs"
	containertypes "github.com/florianfinn/docklet-next/types/container"
	"github.com/florianfinn/docklet-next/types/containerregistry"
	usertypes "github.com/florianfinn/docklet-next/types/user"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
	"go.getarcane.app/docker"
	"go.getarcane.app/docker/compat"
	"go.getarcane.app/docker/types"
	"go.getarcane.app/sys/cgroup"
	"go.getarcane.app/updater/labels"

	"github.com/florianfinn/docklet-next/backend/internal/common"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/event"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/timeouts"
)

func healthConfigFromCreateInternal(hc *containertypes.HealthcheckCreate) *container.HealthConfig {
	if hc == nil {
		return nil
	}

	return &container.HealthConfig{
		Test:          append([]string{}, hc.Test...),
		Interval:      time.Duration(hc.Interval) * time.Second,
		Timeout:       time.Duration(hc.Timeout) * time.Second,
		StartPeriod:   time.Duration(hc.StartPeriod) * time.Second,
		StartInterval: time.Duration(hc.StartInterval) * time.Second,
		Retries:       hc.Retries,
	}
}

// mergeEditMountsInternal builds the desired mount set for an edit. The edit
// form only carries the reduced {type, source, target, readOnly} view, so a
// requested mount that matches an existing one by type and target keeps the
// full original spec (bind/volume/tmpfs options, consistency, ...) with the
// source and read-only flag applied from the request; unmatched entries are
// new mounts built from the request.
func mergeEditMountsInternal(existing []mount.Mount, requested []containertypes.MountCreate) []mount.Mount {
	out := make([]mount.Mount, 0, len(requested))
	for _, req := range requested {
		merged := mount.Mount{
			Type:     mount.Type(req.Type),
			Source:   req.Source,
			Target:   req.Target,
			ReadOnly: req.ReadOnly,
		}
		for _, prior := range existing {
			if string(prior.Type) == req.Type && prior.Target == req.Target {
				merged = prior
				merged.Source = req.Source
				merged.ReadOnly = req.ReadOnly
				break
			}
		}
		out = append(out, merged)
	}
	return out
}

func mountsFromCreateInternal(mounts []containertypes.MountCreate) []mount.Mount {
	out := make([]mount.Mount, 0, len(mounts))
	for _, m := range mounts {
		out = append(out, mount.Mount{
			Type:     mount.Type(m.Type),
			Source:   m.Source,
			Target:   m.Target,
			ReadOnly: m.ReadOnly,
		})
	}
	return out
}

func portMapFromCreateInternal(bindings map[string][]containertypes.PortBindingCreate) (network.PortMap, error) {
	out := network.PortMap{}
	for portSpec, bindingList := range bindings {
		port, err := parsePortSpec(portSpec)
		if err != nil {
			return nil, fmt.Errorf("invalid port %s: %w", portSpec, err)
		}
		mapped := make([]network.PortBinding, 0, len(bindingList))
		for _, binding := range bindingList {
			pb := network.PortBinding{HostPort: binding.HostPort}
			if hostIP := strings.TrimSpace(binding.HostIP); hostIP != "" {
				parsedIP, parseAddrErr := netip.ParseAddr(hostIP)
				if parseAddrErr != nil {
					return nil, fmt.Errorf("invalid host IP %s: %w", hostIP, parseAddrErr)
				}
				pb.HostIP = parsedIP
			}
			mapped = append(mapped, pb)
		}
		out[port] = mapped
	}
	return out, nil
}

func endpointIPAMFromCreateInternal(ep containertypes.EndpointSettingsCreate) (*network.EndpointIPAMConfig, error) {
	ipv4 := strings.TrimSpace(ep.IPv4Address)
	ipv6 := strings.TrimSpace(ep.IPv6Address)
	if ipv4 == "" && ipv6 == "" {
		return nil, nil
	}

	cfg := &network.EndpointIPAMConfig{}
	if ipv4 != "" {
		addr, err := netip.ParseAddr(ipv4)
		if err != nil {
			return nil, fmt.Errorf("invalid IPv4 address %s: %w", ipv4, err)
		}
		cfg.IPv4Address = addr
	}
	if ipv6 != "" {
		addr, err := netip.ParseAddr(ipv6)
		if err != nil {
			return nil, fmt.Errorf("invalid IPv6 address %s: %w", ipv6, err)
		}
		cfg.IPv6Address = addr
	}
	return cfg, nil
}

// applyEditToContainerConfigInternal overwrites the non-nil form-owned Config
// sections of an edit request onto cfg. Settings the form does not own are
// left untouched. When port bindings are replaced, ExposedPorts is recomputed
// as the new binding keys plus any previously-exposed-but-unbound ports (image
// EXPOSE entries survive).
func applyEditToContainerConfigInternal(cfg *container.Config, previousBindings network.PortMap, req containertypes.Edit) error {
	if req.Image != nil && strings.TrimSpace(*req.Image) != "" {
		cfg.Image = strings.TrimSpace(*req.Image)
	}
	if req.WorkingDir != nil {
		cfg.WorkingDir = *req.WorkingDir
	}
	if req.User != nil {
		cfg.User = *req.User
	}
	if req.Command != nil {
		cfg.Cmd = append([]string{}, *req.Command...)
	}
	if req.Entrypoint != nil {
		cfg.Entrypoint = append([]string{}, *req.Entrypoint...)
	}
	if req.Environment != nil {
		cfg.Env = append([]string{}, *req.Environment...)
	}
	if req.Labels != nil {
		cfg.Labels = maps.Clone(*req.Labels)
	}

	switch {
	case req.ClearHealthcheck:
		cfg.Healthcheck = nil
	case req.Healthcheck != nil:
		cfg.Healthcheck = healthConfigFromCreateInternal(req.Healthcheck)
	}

	if req.HostConfig != nil && req.HostConfig.PortBindings != nil {
		newBindings, err := portMapFromCreateInternal(*req.HostConfig.PortBindings)
		if err != nil {
			return err
		}
		exposed := network.PortSet{}
		for port := range newBindings {
			exposed[port] = struct{}{}
		}
		for port := range cfg.ExposedPorts {
			if _, wasBound := previousBindings[port]; !wasBound {
				exposed[port] = struct{}{}
			}
		}
		cfg.ExposedPorts = exposed
	}

	return nil
}

// applyEditToHostConfigInternal overwrites the non-nil form-owned HostConfig
// sections of an edit request onto hc. Everything else (sysctls, ulimits, log
// drivers, devices, tmpfs, DNS, ...) carries over unchanged.
func applyEditToHostConfigInternal(hc *container.HostConfig, req containertypes.Edit) error {
	edit := req.HostConfig
	if edit == nil {
		return nil
	}

	if edit.Binds != nil {
		hc.Binds = append([]string{}, *edit.Binds...)
	}
	if edit.Mounts != nil {
		hc.Mounts = mergeEditMountsInternal(hc.Mounts, *edit.Mounts)
	}
	if edit.PortBindings != nil {
		newBindings, err := portMapFromCreateInternal(*edit.PortBindings)
		if err != nil {
			return err
		}
		hc.PortBindings = newBindings
	}
	if edit.RestartPolicy != nil {
		hc.RestartPolicy = container.RestartPolicy{
			Name:              container.RestartPolicyMode(edit.RestartPolicy.Name),
			MaximumRetryCount: edit.RestartPolicy.MaximumRetryCount,
		}
	}
	if edit.Privileged != nil {
		hc.Privileged = *edit.Privileged
	}
	if edit.CapAdd != nil {
		hc.CapAdd = append([]string{}, *edit.CapAdd...)
	}
	if edit.CapDrop != nil {
		hc.CapDrop = append([]string{}, *edit.CapDrop...)
	}
	if edit.AutoRemove != nil {
		hc.AutoRemove = *edit.AutoRemove
	}
	if edit.ReadonlyRootfs != nil {
		hc.ReadonlyRootfs = *edit.ReadonlyRootfs
	}
	if edit.Memory != nil {
		hc.Memory = *edit.Memory
	}
	if edit.MemorySwap != nil {
		hc.MemorySwap = *edit.MemorySwap
	}
	if edit.NanoCPUs != nil {
		hc.NanoCPUs = *edit.NanoCPUs
	}
	if edit.CPUShares != nil {
		hc.CPUShares = *edit.CPUShares
	}

	return nil
}

// buildEditNetworkingConfigInternal builds the networking config for an edit
// recreate. A nil request preserves the existing attachments (same behavior as
// redeploy); otherwise the request is the full desired endpoint set, keeping
// the sanitized settings of endpoints that stay attached. Containers using
// host/none/container: network modes never get endpoint edits.
func buildEditNetworkingConfigInternal(containerInspect container.InspectResponse, req *containertypes.NetworkingConfigCreate, apiVersion string) (*network.NetworkingConfig, error) {
	if containerInspect.HostConfig != nil {
		mode := containerInspect.HostConfig.NetworkMode
		if mode.IsHost() || mode.IsNone() || mode.IsContainer() {
			return nil, nil
		}
	}

	if req == nil {
		return buildCleanNetworkingConfigInternal(containerInspect, apiVersion), nil
	}

	var existing map[string]*network.EndpointSettings
	if containerInspect.NetworkSettings != nil {
		existing = compat.SanitizeContainerCreateEndpointSettingsForDockerAPI(containerInspect.NetworkSettings.Networks, apiVersion)
	}

	endpoints := make(map[string]*network.EndpointSettings, len(req.EndpointsConfig))
	for name, epReq := range req.EndpointsConfig {
		endpoint := &network.EndpointSettings{}
		if prior := existing[name]; prior != nil {
			priorCopy := *prior
			priorCopy.IPAMConfig = nil
			endpoint = &priorCopy
		}
		endpoint.Aliases = append([]string{}, epReq.Aliases...)
		ipam, err := endpointIPAMFromCreateInternal(epReq)
		if err != nil {
			return nil, err
		}
		endpoint.IPAMConfig = ipam
		endpoints[name] = endpoint
	}

	if len(endpoints) == 0 {
		return nil, nil
	}
	return &network.NetworkingConfig{EndpointsConfig: endpoints}, nil
}

// GetContainerEditConfig returns the purpose-built DTO backing the container
// edit form.
func (s *ContainerService) GetContainerEditConfig(ctx context.Context, containerID string) (containertypes.EditConfig, error) {
	dockerClient, err := s.dockerService.GetClient(ctx)
	if err != nil {
		return containertypes.EditConfig{}, fmt.Errorf("failed to connect to Docker: %w", err)
	}

	containerJSON, err := compat.ContainerInspectWithCompatibility(ctx, dockerClient, containerID, client.ContainerInspectOptions{})
	if err != nil {
		return containertypes.EditConfig{}, fmt.Errorf("failed to inspect container: %w", err)
	}

	containerInfo := containerJSON.Container
	var containerLabels map[string]string
	if containerInfo.Config != nil {
		containerLabels = containerInfo.Config.Labels
	}

	isCompose := docker.ComposeProjectLabel(containerLabels) != ""
	currentContainerID, currentContainerErr := cgroup.CurrentContainerID()
	editDisabled := labels.ShouldDisableArcaneServerRedeploy(containerLabels, containerInfo.ID, currentContainerID, currentContainerErr)

	return containertypes.NewEditConfigFromInspect(&containerInfo, isCompose, editDisabled), nil
}

// EditContainer applies a user-supplied config diff onto the container's
// inspected config and recreates it. Sections not owned by the edit form are
// preserved from the existing container.
func (s *ContainerService) EditContainer(ctx context.Context, containerID string, req containertypes.Edit, user usertypes.Actor) (string, error) {
	dockerClient, err := s.dockerService.GetClient(ctx)
	if err != nil {
		s.eventService.LogErrorEvent(ctx, event.EventTypeContainerError, "container", containerID, "", user.ID, user.Username, "0", err, database.JSON{
			"action": "edit",
			"step":   "get_client",
		})
		return "", fmt.Errorf("failed to connect to Docker: %w", err)
	}

	containerJSON, err := compat.ContainerInspectWithCompatibility(ctx, dockerClient, containerID, client.ContainerInspectOptions{})
	if err != nil {
		s.eventService.LogErrorEvent(ctx, event.EventTypeContainerError, "container", containerID, "", user.ID, user.Username, "0", err, database.JSON{
			"action": "edit",
			"step":   "inspect",
		})
		return "", fmt.Errorf("failed to inspect container: %w", err)
	}

	containerInfo := containerJSON.Container
	if containerInfo.Config == nil {
		return "", errors.New("container config is nil")
	}

	containerName := strings.TrimPrefix(containerInfo.Name, "/")

	if localProject := docker.ComposeProjectLabel(containerInfo.Config.Labels); localProject != "" {
		return "", fmt.Errorf("compose project %s: %w", localProject, common.ErrContainerComposeManaged)
	}

	currentContainerID, currentContainerErr := cgroup.CurrentContainerID()
	if labels.ShouldDisableArcaneServerRedeploy(containerInfo.Config.Labels, containerInfo.ID, currentContainerID, currentContainerErr) {
		err = errors.New("arcane cannot edit itself; use the system upgrade flow (Settings -> Updates) instead")
		s.eventService.LogErrorEvent(ctx, event.EventTypeContainerError, "container", containerID, containerName, user.ID, user.Username, "0", err, database.JSON{
			"action": "edit",
			"step":   "self_edit_blocked",
		})
		return "", err
	}

	newName := containerName
	if req.Name != nil {
		if trimmed := strings.TrimPrefix(strings.TrimSpace(*req.Name), "/"); trimmed != "" {
			newName = trimmed
		}
	}
	if newName != containerName {
		if _, inspectErr := compat.ContainerInspectWithCompatibility(ctx, dockerClient, newName, client.ContainerInspectOptions{}); inspectErr == nil {
			return "", fmt.Errorf("container name %s: %w", newName, common.ErrContainerNameTaken)
		}
	}

	apiVersion := compat.DetectDockerAPIVersion(ctx, dockerClient)

	newConfig := *containerInfo.Config
	var newHostConfig container.HostConfig
	if containerInfo.HostConfig != nil {
		newHostConfig = *containerInfo.HostConfig
	}

	if applyEditToContainerConfigErr := applyEditToContainerConfigInternal(&newConfig, newHostConfig.PortBindings, req); applyEditToContainerConfigErr != nil {
		return "", common.Classify(common.ErrValidation, applyEditToContainerConfigErr)
	}
	if applyEditToHostConfigErr := applyEditToHostConfigInternal(&newHostConfig, req); applyEditToHostConfigErr != nil {
		return "", common.Classify(common.ErrValidation, applyEditToHostConfigErr)
	}
	networkingConfig, err := buildEditNetworkingConfigInternal(containerInfo, req.NetworkingConfig, apiVersion)
	if err != nil {
		return "", common.Classify(common.ErrValidation, err)
	}

	// Unlike redeploy, edit only pulls when the effective image is missing
	// locally so an unrelated config change never surprise-upgrades the image.
	imageName := newConfig.Image
	if imageName != "" {
		if _, inspectErr := dockerClient.ImageInspect(ctx, imageName); inspectErr != nil {
			if pullRedeployImageErr := s.pullRedeployImageInternal(ctx, dockerClient, imageName, containerID, containerName, "edit", req.Credentials, user); pullRedeployImageErr != nil {
				return "", pullRedeployImageErr
			}
		}
	}

	return s.recreateContainerInternal(ctx, dockerClient, containerInfo, "edit", newName, &newConfig, &newHostConfig, networkingConfig, apiVersion, event.EventTypeContainerUpdate, user)
}

func (s *ContainerService) GetContainerByReference(ctx context.Context, ref string) (*container.InspectResponse, error) {
	dockerClient, err := s.dockerService.GetClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Docker: %w", err)
	}

	containerInspect, err := compat.ContainerInspectWithCompatibility(ctx, dockerClient, ref, client.ContainerInspectOptions{})
	if err != nil {
		return nil, fmt.Errorf("container not found: %w", err)
	}

	return new(containerInspect.Container), nil
}

func (s *ContainerService) GetContainerByID(ctx context.Context, id string) (*container.InspectResponse, error) {
	return s.GetContainerByReference(ctx, id)
}

func (s *ContainerService) GetContainerDetails(ctx context.Context, id string) (containertypes.Details, error) {
	containerInspect, err := s.GetContainerByID(ctx, id)
	if err != nil {
		return containertypes.Details{}, err
	}

	details := containertypes.NewDetails(containerInspect)
	details.Ports = mergePortBindings(details.Ports)
	currentContainerID, currentContainerErr := cgroup.CurrentContainerID()
	details.RedeployDisabled = labels.ShouldDisableArcaneServerRedeploy(details.Labels, details.ID, currentContainerID, currentContainerErr)
	var excluded map[string]bool
	if s.settingsService != nil {
		excluded = docker.ExcludedContainerNameSet(s.settingsService.GetStringSetting(ctx, "autoUpdateExcludedContainers", ""))
	}
	details.AutoUpdateEnabled = !labels.IsUpdateDisabled(details.Labels) && !docker.ContainerNameExcluded([]string{details.Name}, excluded)
	updates := s.lookupContainerUpdateInfoInternal(ctx, []container.Summary{{ID: details.ID, Image: details.Image, ImageID: details.ImageID, Labels: details.Labels}})
	details.UpdateInfo = updates[details.ID]
	s.applyContainerDetailsIconInternal(ctx, &details)

	return details, nil
}

// containerProcessesPsArgs lists ps argument sets in preference order: procps first, then
// BusyBox, which rejects %cpu/%mem. Windows daemons ignore the arguments.
var containerProcessesPsArgs = [][]string{
	{"-eo", "pid,user,%cpu,%mem,etime,cmd"},
	{"-o", "pid,user,etime,args"},
}

// GetContainerProcesses returns Docker's `top` snapshot for a container.
func (s *ContainerService) GetContainerProcesses(ctx context.Context, containerID string) (containertypes.Processes, error) {
	dockerClient, err := s.dockerService.GetClient(ctx)
	if err != nil {
		return containertypes.Processes{}, fmt.Errorf("failed to connect to Docker: %w", err)
	}

	timeout := timeouts.DefaultDockerAPI
	if s.settingsService != nil {
		timeout = timeouts.GetDuration(s.settingsService.GetSettingsConfig().DockerAPITimeout.AsInt(), timeouts.DefaultDockerAPI)
	}
	topCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var result client.ContainerTopResult
	for _, args := range containerProcessesPsArgs {
		result, err = dockerClient.ContainerTop(topCtx, containerID, client.ContainerTopOptions{Arguments: args})
		if err == nil || errdefs.IsNotFound(err) || errdefs.IsConflict(err) || topCtx.Err() != nil {
			break
		}
	}
	if err != nil {
		return containertypes.Processes{}, fmt.Errorf("failed to list container processes: %w", err)
	}
	processes := containertypes.Processes{Titles: result.Titles, Processes: result.Processes}
	if processes.Titles == nil {
		processes.Titles = []string{}
	}
	if processes.Processes == nil {
		processes.Processes = [][]string{}
	}
	return processes, nil
}

// GetContainerNameByReference resolves a container's clean name from a Docker ID or name.
func (s *ContainerService) GetContainerNameByReference(ctx context.Context, ref string) (string, error) {
	info, err := s.GetContainerByReference(ctx, ref)
	if err != nil {
		return "", err
	}
	return strings.TrimPrefix(info.Name, "/"), nil
}

// GetContainerNameByID resolves a container's clean name from its Docker ID.
func (s *ContainerService) GetContainerNameByID(ctx context.Context, id string) (string, error) {
	return s.GetContainerNameByReference(ctx, id)
}

func (s *ContainerService) DeleteContainer(ctx context.Context, containerID string, force, removeVolumes bool, user usertypes.Actor) error {
	dockerClient, err := s.dockerService.GetClient(ctx)
	if err != nil {
		s.eventService.LogErrorEvent(
			ctx,
			event.EventTypeContainerError,
			"container",
			containerID,
			"",
			user.ID,
			user.Username,
			"0",
			err,
			database.JSON{
				"action":        "delete",
				"force":         force,
				"removeVolumes": removeVolumes,
			},
		)
		return fmt.Errorf("failed to connect to Docker: %w", err)
	}

	// Get container mounts before deletion if we need to remove volumes
	var volumesToRemove []string
	if removeVolumes {
		containerJSON, inspectErr := compat.ContainerInspectWithCompatibility(ctx, dockerClient, containerID, client.ContainerInspectOptions{})
		if inspectErr == nil {
			for _, mount := range containerJSON.Container.Mounts {
				// Only collect named volumes (not bind mounts or tmpfs)
				if mount.Type == "volume" && mount.Name != "" {
					volumesToRemove = append(volumesToRemove, mount.Name)
				}
			}
		}
	}

	defer s.eventService.BeginDockerResourceSuppressionWindow("container", containerID, "")()
	volumeReleases := make([]func(), 0, len(volumesToRemove))
	defer func() {
		for i := len(volumeReleases) - 1; i >= 0; i-- {
			volumeReleases[i]()
		}
	}()
	for _, volumeName := range volumesToRemove {
		volumeReleases = append(volumeReleases, s.eventService.BeginDockerResourceSuppressionWindow("volume", volumeName, volumeName))
	}
	_, err = dockerClient.ContainerRemove(ctx, containerID, client.ContainerRemoveOptions{
		Force:         force,
		RemoveVolumes: removeVolumes,
		RemoveLinks:   false,
	})
	if err != nil {
		s.eventService.LogErrorEvent(
			ctx,
			event.EventTypeContainerError,
			"container",
			containerID,
			"",
			user.ID,
			user.Username,
			"0",
			err,
			database.JSON{
				"action":        "delete",
				"force":         force,
				"removeVolumes": removeVolumes,
			},
		)
		return fmt.Errorf("failed to delete container: %w", err)
	}

	// Remove named volumes if requested
	if removeVolumes && len(volumesToRemove) > 0 {
		for _, volumeName := range volumesToRemove {
			if _, removeErr := dockerClient.VolumeRemove(ctx, volumeName, client.VolumeRemoveOptions{Force: false}); removeErr != nil {
				// Log but don't fail if volume removal fails (might be in use by another container)
				s.eventService.LogErrorEvent(ctx, event.EventTypeVolumeError, "volume", volumeName, "", user.ID, user.Username, "0", removeErr, database.JSON{"action": "delete", "container": containerID})
			}
		}
	}

	metadata := database.JSON{
		"action":      "delete",
		"containerId": containerID,
	}

	err = s.eventService.LogContainerEvent(ctx, event.EventTypeContainerDelete, containerID, "name", user.ID, user.Username, "0", metadata)
	if err != nil {
		return fmt.Errorf("failed to log action: %w", err)
	}

	return nil
}

func (
	s *ContainerService,
) CreateContainer(
	ctx context.Context,
	config *container.Config,
	hostConfig *container.HostConfig,
	networkingConfig *network.NetworkingConfig,
	containerName string,
	user usertypes.Actor,
	credentials []containerregistry.Credential,
) (
	*container.InspectResponse,
	error,
) {
	dockerClient, err := s.dockerService.GetClient(ctx)
	if err != nil {
		s.eventService.LogErrorEvent(ctx, event.EventTypeContainerError, "container", "", containerName, user.ID, user.Username, "0", err, database.JSON{"action": "create", "image": config.Image})
		return nil, fmt.Errorf("failed to connect to Docker: %w", err)
	}

	_, err = dockerClient.ImageInspect(ctx, config.Image)
	if err != nil {
		// Image not found locally, need to pull it
		pullOptions, authErr := s.imageService.PullOptionsWithAuth(ctx, config.Image, credentials)
		if authErr != nil {
			slog.WarnContext(ctx, "Failed to get registry authentication for container image; proceeding without auth",
				"image", config.Image,
				"error", authErr.Error())
			pullOptions = client.ImagePullOptions{}
		}

		localSettings := s.settingsService.GetSettingsConfig()
		pullCtx, pullCancel := context.WithTimeout(ctx, timeouts.GetDuration(localSettings.DockerImagePullTimeout.AsInt(), timeouts.DefaultDockerImagePull))
		defer pullCancel()

		defer s.eventService.BeginDockerResourceSuppressionWindow("image", "", config.Image)()
		reader, pullErr := dockerClient.ImagePull(pullCtx, config.Image, pullOptions)
		if pullErr != nil {
			if errors.Is(pullCtx.Err(), context.DeadlineExceeded) {
				s.eventService.LogErrorEvent(
					ctx,
					event.EventTypeContainerError,
					"container",
					"",
					containerName,
					user.ID,
					user.Username,
					"0",
					pullErr,
					database.JSON{
						"action": "create",
						"image":  config.Image,
						"step":   "pull_image_timeout",
					},
				)
				return nil, fmt.Errorf("image pull timed out for %s (increase DOCKER_IMAGE_PULL_TIMEOUT or setting)", config.Image)
			}
			s.eventService.LogErrorEvent(
				ctx,
				event.EventTypeContainerError,
				"container",
				"",
				containerName,
				user.ID,
				user.Username,
				"0",
				pullErr,
				database.JSON{
					"action": "create",
					"image":  config.Image,
					"step":   "pull_image",
				},
			)
			return nil, fmt.Errorf("failed to pull image %s: %w", config.Image, pullErr)
		}
		defer func() { _ = reader.Close() }()

		progressWriter, _ := ctx.Value(types.ProgressWriterKey{}).(io.Writer)
		logWriter := docker.NewLogLineWriter(progressWriter)
		streamErr := docker.RenderJSONMessageStream(reader, logWriter)
		_ = logWriter.Close()
		if streamErr != nil {
			s.eventService.LogErrorEvent(
				ctx,
				event.EventTypeContainerError,
				"container",
				"",
				containerName,
				user.ID,
				user.Username,
				"0",
				streamErr,
				database.JSON{
					"action": "create",
					"image":  config.Image,
					"step":   "complete_pull",
				},
			)
			return nil, fmt.Errorf("failed to complete image pull: %w", streamErr)
		}
	}

	defer s.eventService.BeginDockerResourceSuppressionWindow("container", "", containerName)()
	resp, err := compat.ContainerCreateWithCompatibility(ctx, dockerClient, client.ContainerCreateOptions{
		Config:           config,
		HostConfig:       hostConfig,
		NetworkingConfig: networkingConfig,
		Name:             containerName,
	})
	if err != nil {
		s.eventService.LogErrorEvent(
			ctx,
			event.EventTypeContainerError,
			"container",
			"",
			containerName,
			user.ID,
			user.Username,
			"0",
			err,
			database.JSON{
				"action": "create",
				"image":  config.Image,
				"step":   "create",
			},
		)
		return nil, fmt.Errorf("failed to create container: %w", err)
	}

	defer s.eventService.BeginDockerResourceSuppressionWindow("container", resp.ID, containerName)()
	metadata := database.JSON{
		"action":      "create",
		"containerId": resp.ID,
	}

	if logErr := s.eventService.LogContainerEvent(ctx, event.EventTypeContainerCreate, resp.ID, "name", user.ID, user.Username, "0", metadata); logErr != nil {
		slog.WarnContext(ctx, "could not log container stop action", "error", logErr)
	}

	if _, containerStartErr := dockerClient.ContainerStart(ctx, resp.ID, client.ContainerStartOptions{}); containerStartErr != nil {
		_, _ = dockerClient.ContainerRemove(ctx, resp.ID, client.ContainerRemoveOptions{Force: true})
		s.eventService.LogErrorEvent(
			ctx,
			event.EventTypeContainerError,
			"container",
			resp.ID,
			containerName,
			user.ID,
			user.Username,
			"0",
			containerStartErr,
			database.JSON{
				"action": "create",
				"image":  config.Image,
				"step":   "start",
			},
		)
		return nil, fmt.Errorf("failed to start container: %w", containerStartErr)
	}

	containerJSON, err := compat.ContainerInspectWithCompatibility(ctx, dockerClient, resp.ID, client.ContainerInspectOptions{})
	if err != nil {
		s.eventService.LogErrorEvent(
			ctx,
			event.EventTypeContainerError,
			"container",
			resp.ID,
			containerName,
			user.ID,
			user.Username,
			"0",
			err,
			database.JSON{
				"action": "create",
				"image":  config.Image,
				"step":   "inspect",
			},
		)
		return nil, fmt.Errorf("failed to inspect created container: %w", err)
	}

	return new(containerJSON.Container), nil
}
