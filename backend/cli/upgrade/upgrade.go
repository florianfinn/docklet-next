// Source: getarcaneapp/arcane/backend/cli/upgrade/upgrade.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; explicit --image required (no implicit latest), Docklet role-label detection and Compose drift warning (#362).

package upgrade

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"github.com/distribution/reference"
	"log/slog"
	"maps"
	"os"
	"strings"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/client"
	"github.com/spf13/cobra"
	"go.getarcane.app/docker"
	"go.getarcane.app/docker/compat"
	"go.getarcane.app/sys/cgroup"
	"go.getarcane.app/updater/refs"

	"github.com/florianfinn/docklet-next/backend/pkg/libarcane"
	"github.com/florianfinn/docklet-next/backend/pkg/projects"
)

var (
	containerName string
	targetImage   string
	pullImageRef  string
	autoDetect    bool
	targetRole    string
)

// dockletSystemLabel and dockletHelperLabel identify Docklet's own containers
// and its short-lived helpers, which are never upgrade targets.
const (
	dockletSystemLabel = "io.docklet.system"
	dockletHelperLabel = "io.docklet.helper"
	composeProjectKey  = "com.docker.compose.project"
)

// UpgradeCmd recreates a running Docklet Next hub or agent container with an
// explicitly given image while preserving its configuration.
var UpgradeCmd = &cobra.Command{
	Use:   "upgrade",
	Short: "Recreate the Docklet Next hub or agent container with a given image",
	Long: `Recreate a Docklet Next hub or agent container with the image given by --image.
There is no implicit target: the image reference is always explicit. Compose installations
should upgrade with their package instead (docs/upgrade.md); this command does not update
Compose or .env files. Run it from outside the target container.`,
	Example: `  # Recreate the hub container with a release image
  docklet-hub upgrade --auto --image ghcr.io/florianfinn/docklet-next-hub:1.0.0@sha256:<digest>

  # Recreate a named agent container
  docklet-hub upgrade --container docklet-agent --image ghcr.io/florianfinn/docklet-next-agent:1.0.0`,
	// Use background context to ignore signals during upgrade
	// This prevents the upgrade from being interrupted when the target container stops
	RunE: runUpgrade,
}

func init() {
	UpgradeCmd.Flags().StringVarP(&containerName, "container", "c", "", "Name of the container to upgrade")
	UpgradeCmd.Flags().StringVarP(&targetImage, "image", "i", "", "Target image reference (required)")
	UpgradeCmd.Flags().StringVar(&pullImageRef, "pull-image", "", "Immutable reference to pull and tag as --image (defaults to --image)")
	UpgradeCmd.Flags().BoolVarP(&autoDetect, "auto", "a", false, "Auto-detect the Docklet container by its role label")
	UpgradeCmd.Flags().StringVar(&targetRole, "role", "hub", "Role to auto-detect: hub or agent")
}

// validateUpgradeTargetInternal checks the operator input before any Engine
// request. Any valid reference is accepted; there is no implicit default.
func validateUpgradeTargetInternal(image, pull, role string) error {
	if strings.TrimSpace(image) == "" {
		return errors.New("--image is required: give the exact image reference to run (docs/upgrade.md)")
	}
	// The full parser keeps and validates a digest; refs.NormalizeReference drops it.
	if _, err := reference.ParseNormalizedNamed(strings.TrimSpace(image)); err != nil {
		return fmt.Errorf("invalid --image %q: %w", image, err)
	}
	if strings.TrimSpace(pull) != "" {
		if _, err := reference.ParseNormalizedNamed(strings.TrimSpace(pull)); err != nil {
			return fmt.Errorf("invalid --pull-image %q: %w", pull, err)
		}
	}
	if role != "hub" && role != "agent" {
		return fmt.Errorf("invalid --role %q: use hub or agent", role)
	}
	return nil
}

func runUpgrade(cmd *cobra.Command, args []string) error {
	// Use background context instead of command context to ignore signals
	// This prevents interruption when stopping the target container
	ctx := context.WithoutCancel(cmd.Context())
	if err := validateUpgradeTargetInternal(targetImage, pullImageRef, targetRole); err != nil {
		return err
	}
	targetImage = strings.TrimSpace(targetImage)

	logFile, err := SetupMessageOnlyLogFile(libarcane.UpgradeLogDirectory, "docklet-upgrade", slog.LevelInfo)
	if err != nil {
		slog.WarnContext(ctx, "Failed to setup file logging", "error", err)
	} else {
		slog.InfoContext(ctx, "Upgrade log file created", "path", logFile.Name())
	}

	// Connect to Docker
	dockerClient, err := client.New(client.FromEnv)
	if err != nil {
		return fmt.Errorf("failed to connect to Docker: %w", err)
	}
	defer func() { _ = dockerClient.Close() }()

	// Find the container
	var targetContainer container.InspectResponse
	if autoDetect || containerName == "" {
		slog.InfoContext(ctx, "Auto-detecting Docklet container...", "role", targetRole)
		targetContainer, err = findDockletContainer(ctx, dockerClient, targetRole)
		if err != nil {
			return fmt.Errorf("failed to find Docklet %s container: %w", targetRole, err)
		}
		containerName = strings.TrimPrefix(targetContainer.Name, "/")
		slog.InfoContext(ctx, "Found Docklet container", "name", containerName, "id", targetContainer.ID[:12])
	} else {
		inspectResult, inspectErr := compat.ContainerInspectWithCompatibility(ctx, dockerClient, containerName, client.ContainerInspectOptions{})
		if inspectErr != nil {
			return fmt.Errorf("failed to inspect container %s: %w", containerName, inspectErr)
		}
		targetContainer = inspectResult.Container
	}

	if project := composeProjectInternal(targetContainer); project != "" {
		slog.WarnContext(ctx, "This container belongs to a Compose project; its Compose and .env files are not updated and a later compose up reverts this change (docs/upgrade.md)",
			"container", containerName, "project", project)
	}

	// The reference the container should run as, and what to pull.
	newImage := targetImage
	imageToPull := cmp.Or(strings.TrimSpace(pullImageRef), newImage)

	// Pull the new image
	slog.InfoContext(ctx, "Pulling new image", "image", imageToPull)
	if pullImageErr := pullImage(ctx, dockerClient, imageToPull); pullImageErr != nil {
		return fmt.Errorf("failed to pull image: %w", pullImageErr)
	}
	// A frozen pull is tagged back as the selected reference so the recreated
	// container keeps its tag instead of an immutable digest.
	if imageToPull != newImage {
		if tagErr := tagPulledImage(ctx, dockerClient, imageToPull, newImage); tagErr != nil {
			return fmt.Errorf("failed to tag pulled image: %w", tagErr)
		}
	}

	// The pull always runs so a mutable tag gets re-resolved, but when it lands on
	// the image the container already runs there is nothing to swap in. Skipping the
	// recreate avoids needless downtime and, for agents, a dropped connection. A
	// container running the right image under a different reference is still
	// recreated so its reference is corrected.
	same, pulledID := pulledImageAlreadyRunning(ctx, dockerClient, imageToPull, targetContainer.Image)
	if pulledID == "" {
		return fmt.Errorf("could not resolve pulled image %s; refusing to recreate", imageToPull)
	}
	if same && targetContainer.Config != nil && strings.TrimSpace(targetContainer.Config.Image) == newImage {
		slog.InfoContext(ctx, "Image unchanged after pull; already up to date, skipping recreate",
			"container", containerName, "image", newImage, "imageId", pulledID)
		return nil
	}

	// Perform the upgrade
	slog.InfoContext(ctx, "Starting container upgrade", "container", containerName)
	if upgradeContainerErr := UpgradeContainer(ctx, dockerClient, targetContainer, newImage, pulledID, nil); upgradeContainerErr != nil {
		return fmt.Errorf("failed to upgrade container: %w", upgradeContainerErr)
	}

	slog.InfoContext(ctx, "Upgrade completed successfully", "container", containerName, "image", newImage)
	return nil
}

// tagPulledImage tags the pulled immutable reference as the selected one and
// verifies the tag now resolves to the pulled image. Both must name the same
// repository.
func tagPulledImage(ctx context.Context, dockerClient *client.Client, pulled, selected string) error {
	pulledRef, err := refs.NormalizeReference(pulled)
	if err != nil {
		return err
	}
	selectedRef, err := refs.NormalizeReference(selected)
	if err != nil {
		return err
	}
	if pulledRef.RegistryHost != selectedRef.RegistryHost || pulledRef.Repository != selectedRef.Repository {
		return fmt.Errorf("pulled image %s does not belong to repository of %s", pulled, selected)
	}
	if _, tagErr := dockerClient.ImageTag(ctx, client.ImageTagOptions{Source: pulled, Target: selected}); tagErr != nil {
		return tagErr
	}
	pulledInspect, err := dockerClient.ImageInspect(ctx, pulled)
	if err != nil {
		return fmt.Errorf("inspect pulled image: %w", err)
	}
	selectedInspect, err := dockerClient.ImageInspect(ctx, selected)
	if err != nil {
		return fmt.Errorf("inspect tagged image: %w", err)
	}
	if pulledInspect.ID != selectedInspect.ID {
		return fmt.Errorf("%s resolves to %s after tagging, expected %s", selected, selectedInspect.ID, pulledInspect.ID)
	}
	return nil
}

// findDockletContainer returns the single running container labelled with the
// requested Docklet role, skipping this process and Docklet helpers.
func findDockletContainer(ctx context.Context, dockerClient *client.Client, role string) (container.InspectResponse, error) {
	filter := make(client.Filters)
	filter = filter.Add("status", "running")
	filter = filter.Add("label", dockletSystemLabel+"="+role)

	containers, err := dockerClient.ContainerList(ctx, client.ContainerListOptions{Filters: filter})
	if err != nil {
		return container.InspectResponse{}, err
	}
	selfID, _ := cgroup.CurrentContainerID()
	var matches []container.Summary
	for _, c := range containers.Items {
		if selfID != "" && strings.HasPrefix(c.ID, selfID) {
			continue
		}
		if _, helper := c.Labels[dockletHelperLabel]; helper {
			continue
		}
		matches = append(matches, c)
	}
	switch len(matches) {
	case 0:
		return container.InspectResponse{}, fmt.Errorf("no running container labelled %s=%s", dockletSystemLabel, role)
	case 1:
		inspectResult, inspectErr := compat.ContainerInspectWithCompatibility(ctx, dockerClient, matches[0].ID, client.ContainerInspectOptions{})
		if inspectErr != nil {
			return container.InspectResponse{}, inspectErr
		}
		return inspectResult.Container, nil
	default:
		names := make([]string, 0, len(matches))
		for _, c := range matches {
			names = append(names, strings.TrimPrefix(strings.Join(c.Names, ","), "/"))
		}
		return container.InspectResponse{}, fmt.Errorf("several running %s containers (%s); choose one with --container", role, strings.Join(names, ", "))
	}
}

// composeProjectInternal names the Compose project a container belongs to.
func composeProjectInternal(inspect container.InspectResponse) string {
	if inspect.Config == nil {
		return ""
	}
	return inspect.Config.Labels[composeProjectKey]
}

func refreshRecreatedImageLabelsInternal(containerLabels, previousImageLabels, imageLabels map[string]string, imageDigest string) map[string]string {
	refreshed := maps.Clone(containerLabels)
	for key, value := range containerLabels {
		if !strings.HasPrefix(strings.ToLower(key), "org.opencontainers.image.") {
			continue
		}
		for previousKey, previousValue := range previousImageLabels {
			if strings.EqualFold(previousKey, key) && previousValue == value {
				delete(refreshed, key)
				break
			}
		}
	}

	for key, value := range imageLabels {
		if !strings.HasPrefix(strings.ToLower(key), "org.opencontainers.image.") {
			continue
		}
		preserved := false
		for existingKey := range refreshed {
			if strings.EqualFold(existingKey, key) {
				preserved = true
				break
			}
		}
		if preserved {
			continue
		}
		if refreshed == nil {
			refreshed = make(map[string]string)
		}
		refreshed[key] = value
	}

	projects.RefreshComposeImageLabel(refreshed, imageDigest)

	return refreshed
}

func refreshRecreatedContainerLabelsInternal(ctx context.Context, dockerClient *client.Client, containerLabels map[string]string, previousImage, targetImage string) map[string]string {
	// Manifest data is needed to record the platform manifest digest compose
	// v5.5.0 expects in com.docker.compose.image; older daemons may reject the
	// option, so retry without it and fall back to the top-level image ID.
	imageInspect, imageInspectErr := dockerClient.ImageInspect(ctx, targetImage, client.ImageInspectWithManifests(true))
	if imageInspectErr != nil {
		imageInspect, imageInspectErr = dockerClient.ImageInspect(ctx, targetImage)
	}
	if imageInspectErr != nil {
		slog.WarnContext(ctx, "Could not inspect target image labels; preserving container OCI overrides",
			"image", targetImage,
			"error", imageInspectErr,
		)
		return maps.Clone(containerLabels)
	}

	imageLabels := map[string]string(nil)
	previousImageLabels := map[string]string(nil)
	if imageInspect.Config != nil {
		imageLabels = imageInspect.Config.Labels
		if hasOCIImageLabelInternal(containerLabels) && strings.TrimSpace(previousImage) != "" {
			previousImageInspect, previousImageInspectErr := dockerClient.ImageInspect(ctx, previousImage)
			if previousImageInspectErr != nil {
				slog.WarnContext(ctx, "Could not inspect previous image labels; preserving existing OCI labels",
					"image", previousImage,
					"error", previousImageInspectErr,
				)
			} else if previousImageInspect.Config != nil {
				previousImageLabels = previousImageInspect.Config.Labels
			}
		}
	}
	return refreshRecreatedImageLabelsInternal(containerLabels, previousImageLabels, imageLabels, projects.ComposeImageDigest(imageInspect))
}

func hasOCIImageLabelInternal(containerLabels map[string]string) bool {
	for key := range containerLabels {
		if strings.HasPrefix(strings.ToLower(key), "org.opencontainers.image.") {
			return true
		}
	}
	return false
}

func pulledImageAlreadyRunning(ctx context.Context, dockerClient *client.Client, imageRef, runningImageID string) (bool, string) {
	if runningImageID == "" {
		return false, ""
	}
	inspect, err := dockerClient.ImageInspect(ctx, imageRef)
	if err != nil {
		slog.WarnContext(ctx, "Could not inspect pulled image", "image", imageRef, "error", err)
		return false, ""
	}
	return inspect.ID == runningImageID, inspect.ID
}

func pullImage(ctx context.Context, dockerClient *client.Client, imageName string) error {
	reader, err := dockerClient.ImagePull(ctx, imageName, client.ImagePullOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = reader.Close() }()

	return docker.RenderJSONMessageStream(reader, os.Stdout)
}

// UpgradeContainer recreates an Arcane container with the supplied image and
// optional expected image ID and environment replacements. It runs from detached
// maintenance containers so stopping the target cannot interrupt the operation.
func UpgradeContainer(ctx context.Context, dockerClient *client.Client, oldContainer container.InspectResponse, newImage, expectedImageID string, environment map[string]string) error {
	originalName := strings.TrimPrefix(oldContainer.Name, "/")
	oldName := fmt.Sprintf("%s-old-%d", originalName, time.Now().UnixNano())

	if oldContainer.Config == nil {
		return fmt.Errorf("container %s inspection has no config; refusing to recreate", originalName)
	}

	// Create new container config
	config := *oldContainer.Config
	config.Image = newImage
	if len(environment) > 0 {
		config.Env = applyRecoveredEnvironmentInternal(config.Env, environment)
	}

	config.Labels = refreshRecreatedContainerLabelsInternal(ctx, dockerClient, config.Labels, oldContainer.Image, cmp.Or(expectedImageID, newImage))

	hostConfig, sanitizedMemorySwappiness, engineInfo, err := compat.PrepareRecreateHostConfigForEngine(ctx, dockerClient, oldContainer.HostConfig)
	if err != nil {
		return fmt.Errorf("prepare host config: %w", err)
	}
	if sanitizedMemorySwappiness {
		slog.InfoContext(ctx, "Stripped unsupported host config field for recreate",
			"container", originalName,
			"containerId", oldContainer.ID,
			"engine", engineInfo.Name,
			"cgroupVersion", engineInfo.CgroupVersion,
			"field", "memorySwappiness",
		)
	}

	// Pin every attached volume to its resolved name so the recreated container
	// keeps its data instead of receiving fresh anonymous volumes (#3953).
	if hostConfig == nil {
		hostConfig = &container.HostConfig{}
	}
	hostConfig.Binds, hostConfig.Mounts, err = docker.PreserveVolumeMounts(ctx, hostConfig.Binds, hostConfig.Mounts, oldContainer.Mounts)
	if err != nil {
		return fmt.Errorf("preserve volumes of container %s: %w", originalName, err)
	}

	// Fix for "conflicting options: hostname and the network mode"
	// When network mode is "host" or "container:...", Hostname must be empty
	nm := hostConfig.NetworkMode
	if nm.IsHost() || nm.IsContainer() {
		config.Hostname = ""
		config.Domainname = ""
	}

	// Clear hostname if it looks like a container ID (auto-generated by Docker)
	// This allows Docker to assign the new container's ID as the hostname
	if looksLikeContainerID(config.Hostname) {
		slog.DebugContext(ctx, "Clearing auto-generated hostname", "oldHostname", config.Hostname)
		config.Hostname = ""
	}

	// Fix for "conflicting options: port exposing and the container type network mode"
	// When network mode is "container:...", port mappings are not allowed
	if nm.IsContainer() {
		config.ExposedPorts = nil
		hostConfig.PortBindings = nil
		hostConfig.PublishAllPorts = false
	}

	// Build network config - preserve all network settings including IP addresses
	var (
		apiVersion    string
		networkConfig *network.NetworkingConfig
	)
	if !nm.IsContainer() {
		apiVersion = compat.DetectDockerAPIVersion(ctx, dockerClient)
		if apiVersion != "" && !compat.IsDockerAPIVersionAtLeast(apiVersion, compat.NetworkScopedMacAddressMinAPIVersion) {
			slog.InfoContext(ctx, "daemon API does not support per-network mac-address on create; stripping endpoint mac addresses",
				"dockerApiVersion", apiVersion,
				"minimumRequiredApiVersion", compat.NetworkScopedMacAddressMinAPIVersion,
			)
		}

		var endpoints map[string]*network.EndpointSettings
		if oldContainer.NetworkSettings != nil {
			endpoints = oldContainer.NetworkSettings.Networks
		}

		networkConfig = &network.NetworkingConfig{
			EndpointsConfig: compat.SanitizeContainerCreateEndpointSettingsForDockerAPI(endpoints, apiVersion),
		}
	}

	fmt.Println("PROGRESS:65:Renaming old container")
	slog.InfoContext(ctx, "Renaming old container", "from", originalName, "to", oldName)
	if _, containerRenameErr := dockerClient.ContainerRename(ctx, oldContainer.ID, client.ContainerRenameOptions{NewName: oldName}); containerRenameErr != nil {
		return fmt.Errorf("rename old container: %w", containerRenameErr)
	}

	fmt.Println("PROGRESS:70:Stopping old container")
	slog.InfoContext(ctx, "Stopping old container", "name", oldName)
	// Allow graceful shutdown and longer finite timeouts, but never wait forever.
	stopTimeout := 40
	if configured := oldContainer.Config.StopTimeout; configured != nil && *configured > stopTimeout {
		stopTimeout = *configured
	}
	if _, containerStopErr := dockerClient.ContainerStop(ctx, oldContainer.ID, client.ContainerStopOptions{Timeout: &stopTimeout}); containerStopErr != nil {
		_, _ = dockerClient.ContainerRename(ctx, oldContainer.ID, client.ContainerRenameOptions{NewName: originalName})
		return fmt.Errorf("stop old container: %w", containerStopErr)
	}

	fmt.Println("PROGRESS:75:Creating new container")
	slog.InfoContext(ctx, "Creating new container", "name", originalName)
	resp, err := compat.ContainerCreateWithCompatibilityForAPIVersion(ctx, dockerClient, client.ContainerCreateOptions{
		Config:           &config,
		HostConfig:       hostConfig,
		NetworkingConfig: networkConfig,
		Name:             originalName,
	}, apiVersion)
	if err != nil {
		// Try to restart and restore old container on failure
		_, _ = dockerClient.ContainerStart(ctx, oldContainer.ID, client.ContainerStartOptions{})
		_, _ = dockerClient.ContainerRename(ctx, oldContainer.ID, client.ContainerRenameOptions{NewName: originalName})
		return fmt.Errorf("create new container: %w", err)
	}

	// Docker resolves the tag at creation. Inspect the created container so a
	// concurrent pull cannot cause an unselected image to start.
	if expectedImageID != "" {
		created, inspectErr := compat.ContainerInspectWithCompatibility(ctx, dockerClient, resp.ID, client.ContainerInspectOptions{})
		if inspectErr != nil {
			err = fmt.Errorf("inspect new container image: %w", inspectErr)
		} else if created.Container.Image != expectedImageID {
			err = fmt.Errorf("new container uses image %s, expected %s", created.Container.Image, expectedImageID)
		}
	}
	if err == nil {
		fmt.Println("PROGRESS:80:Starting new container")
		slog.InfoContext(ctx, "Starting new container", "id", resp.ID[:12])
		if _, startErr := dockerClient.ContainerStart(ctx, resp.ID, client.ContainerStartOptions{}); startErr != nil {
			err = fmt.Errorf("start new container: %w", startErr)
		}
	}
	if err != nil {
		// Cleanup the rejected replacement and restore the original container.
		if _, removeErr := dockerClient.ContainerRemove(ctx, resp.ID, client.ContainerRemoveOptions{Force: true}); removeErr != nil {
			err = errors.Join(err, fmt.Errorf("remove new container: %w", removeErr))
		}
		if _, startErr := dockerClient.ContainerStart(ctx, oldContainer.ID, client.ContainerStartOptions{}); startErr != nil {
			err = errors.Join(err, fmt.Errorf("restart old container: %w", startErr))
		}
		if _, renameErr := dockerClient.ContainerRename(ctx, oldContainer.ID, client.ContainerRenameOptions{NewName: originalName}); renameErr != nil {
			err = errors.Join(err, fmt.Errorf("restore old container name: %w", renameErr))
		}
		return err
	}

	// Wait a moment for the new container to initialize
	// Wait a moment for the new container to initialize
	fmt.Println("PROGRESS:85:Waiting for container to start")
	time.Sleep(2 * time.Second)

	fmt.Println("PROGRESS:90:Removing old container")
	slog.InfoContext(ctx, "Removing old container", "id", oldContainer.ID[:12])
	if _, containerRemoveErr := dockerClient.ContainerRemove(ctx, oldContainer.ID, client.ContainerRemoveOptions{}); containerRemoveErr != nil {
		slog.WarnContext(ctx, "Failed to remove old container", "error", containerRemoveErr)
	}

	fmt.Println("PROGRESS:95:Upgrade complete")

	return nil
}

func applyRecoveredEnvironmentInternal(current []string, recovered map[string]string) []string {
	seen := make(map[string]struct{}, len(recovered))
	updated := make([]string, 0, len(current)+len(recovered))
	for _, entry := range current {
		name, _, found := strings.Cut(entry, "=")
		if !found {
			updated = append(updated, entry)
			continue
		}
		if baseName, ok := strings.CutSuffix(name, "__FILE"); ok {
			if _, localOk := recovered[baseName]; localOk {
				continue
			}
		}
		if baseName, ok := strings.CutSuffix(name, "_FILE"); ok {
			if _, localOk2 := recovered[baseName]; localOk2 {
				continue
			}
		}
		if replacement, ok := recovered[name]; ok {
			updated = append(updated, name+"="+replacement)
			seen[name] = struct{}{}
		} else {
			updated = append(updated, entry)
		}
	}
	for name, value := range recovered {
		if _, ok := seen[name]; !ok {
			updated = append(updated, name+"="+value)
		}
	}
	return updated
}

// looksLikeContainerID checks if a string looks like a Docker container ID
// (12 or 64 lowercase hex characters, which Docker auto-generates as hostnames)
func looksLikeContainerID(s string) bool {
	if len(s) != 12 && len(s) != 64 {
		return false
	}
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}
