// Source: getarcaneapp/arcane/backend/internal/version/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; removed APNS and Svelte-only metadata; added agent/hub contract metadata; reads the Docklet Next release feed instead of Arcane releases, digests and registry labels (#235).

package version

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/distribution/reference"
	"github.com/florianfinn/docklet-next/types/contract"
	"github.com/florianfinn/docklet-next/types/version"
	containertypes "github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"go.getarcane.app/docker/compat"
	"go.getarcane.app/kit/pkg"
	"go.getarcane.app/streams/agg"
	"go.getarcane.app/sys/cgroup"

	"github.com/florianfinn/docklet-next/backend/buildables"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/volumehelper"
)

const (
	versionStreamPollInterval = 5 * time.Minute
	defaultRequestTimeout     = 15 * time.Second
)

type VersionService struct {
	onboardingMu         sync.Mutex
	onboardingCache      *version.OnboardingChecks
	onboardingCachedAt   time.Time
	onboardingDockerHost string
	feed                 *releaseFeed
	disabled             bool
	version              string
	revision             string
	dockerService        *docker.DockerClientService
	settingsService      *settings.SettingsService
	streamHub            *agg.Hub[version.StreamEvent]
}

// NewVersionService reads releases from feedURL (empty: DefaultReleaseFeedURL).
// disabled suppresses every feed request.
func NewVersionService(
	httpClient *http.Client,
	disabled bool,
	feedURL string,
	appVersion, revision string,
	dockerService *docker.DockerClientService,
	settingsService *settings.SettingsService,
) *VersionService {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	service := &VersionService{
		feed:            newReleaseFeed(httpClient, disabled, feedURL),
		disabled:        disabled,
		version:         appVersion,
		revision:        revision,
		dockerService:   dockerService,
		settingsService: settingsService,
		streamHub:       agg.NewHub[version.StreamEvent](),
	}
	if !disabled && service.feed.invalid != nil {
		slog.Warn("Release feed URL is invalid; update checks report unavailable", "error", service.feed.invalid)
	}
	return service
}

// GetVersionInformation compares currentVersion (default: this build) with the
// newest published release. Non-SemVer builds make no feed request.
func (s *VersionService) GetVersionInformation(ctx context.Context, currentVersion string) (*version.Check, error) {
	if currentVersion == "" {
		currentVersion = s.version
	}
	check := &version.Check{CurrentVersion: s.normalizeVersion(currentVersion)}
	current, ok := version.ParseProduct(currentVersion)
	if s.disabled || !ok {
		return check, nil
	}
	release, found, err := s.NewestRelease(ctx)
	if err != nil {
		return check, err
	}
	// A release older than the running build (e.g. an rc ahead of stable) is no target.
	if found && release.parsed.Compare(current) >= 0 {
		check.NewestVersion = release.Tag
		check.UpdateAvailable = release.parsed.Compare(current) > 0
		check.ReleaseURL = release.URL
	}
	return check, nil
}

// normalizeVersion gives SemVer builds a "v" prefix for display; other values stay as they are.
func (s *VersionService) normalizeVersion(ver string) string {
	ver = strings.TrimSpace(ver)
	if ver == "" {
		return "v0.0.0"
	}
	if parsed, ok := version.ParseProduct(ver); ok {
		return "v" + parsed.String()
	}
	return ver
}

// isSemverVersion checks if the build version is a full SemVer product version.
func (s *VersionService) isSemverVersion() bool {
	_, ok := version.ParseProduct(s.version)
	return ok
}

// getDisplayVersion formats the version for display purposes
// Semver versions (including prereleases like 1.0.0-rc.1) display as v<version>
func (s *VersionService) getDisplayVersion() string {
	versionValue := strings.TrimPrefix(strings.TrimSpace(s.version), "v")
	return kit.Ternary(s.isSemverVersion(), "v"+versionValue, versionValue)
}

// GetAppVersionInfo returns application version information including display version
func (s *VersionService) GetAppVersionInfo(ctx context.Context) *version.Info {
	isSemver := s.isSemverVersion()
	ver := s.normalizeVersion(s.version)

	currentTag, currentDigest, currentImageRef, _ := s.detectCurrentImageInfo(ctx)

	info := &version.Info{
		CurrentImageRef:             currentImageRef,
		ToolsImage:                  volumehelper.DefaultToolsImage,
		AgentContractVersion:        contract.AgentHubVersion,
		AgentContractFingerprint:    contract.AgentHubFingerprint,
		MinimumAgentContractVersion: contract.MinimumAgentVersion,
		CurrentVersion:              ver,
		CurrentTag:                  currentTag,
		CurrentDigest:               currentDigest,
		DisplayVersion:              s.getDisplayVersion(),
		Revision:                    s.revision,
		ShortRevision:               config.ShortRevision(),
		GoVersion:                   config.GoVersion(),
		NodeVersion:                 config.NodeVersion,
		EnabledFeatures:             kit.Unique(kit.TrimNonEmpty(strings.Split(strings.ToLower(buildables.EnabledFeatures), ","))),
		BuildTime:                   config.BuildTime,
		IsSemverVersion:             isSemver,
		UpdateAvailable:             false,
	}

	if s.settingsService != nil {
		configured := s.settingsService.GetSettingsConfig().ToolsImage.Value
		info.ToolsImage = volumehelper.ToolsImage(configured)
		info.ToolsImageOverride = strings.TrimSpace(configured) != ""
	}
	current, ok := version.ParseProduct(s.version)
	if s.disabled || !ok {
		return info
	}
	if release, found, err := s.NewestRelease(ctx); err == nil && found && release.parsed.Compare(current) >= 0 {
		info.NewestVersion = release.Tag
		info.UpdateAvailable = release.parsed.Compare(current) > 0
		info.ReleaseURL = release.URL
		info.ReleaseNotes = release.Notes
		info.ReleasedAt = release.PublishedAt
	}
	return info
}

// detectCurrentImageInfo attempts to detect the current container's image tag and digest
func (s *VersionService) detectCurrentImageInfo(ctx context.Context) (tag, digest, imageRef, imageID string) {
	if s.dockerService == nil {
		slog.DebugContext(ctx, "detectCurrentImageInfo: dockerService is nil")
		return "", "", "", ""
	}

	dockerClient, err := s.dockerService.GetClient(ctx)
	if err != nil {
		slog.DebugContext(ctx, "detectCurrentImageInfo: failed to get docker client", "error", err)
		return "", "", "", ""
	}

	containerId := s.detectContainerID(ctx, dockerClient)
	if containerId == "" {
		slog.DebugContext(ctx, "detectCurrentImageInfo: could not detect container ID")
		return "", "", "", ""
	}
	slog.DebugContext(ctx, "detectCurrentImageInfo: detected container", "containerId", containerId)

	inspectResult, err := compat.ContainerInspectWithCompatibility(ctx, dockerClient, containerId, client.ContainerInspectOptions{})
	if err != nil {
		slog.DebugContext(ctx, "detectCurrentImageInfo: failed to inspect container", "containerId", containerId, "error", err)
		return "", "", "", ""
	}
	container := inspectResult.Container
	imageID = container.Image

	configImage := ""
	if container.Config != nil {
		configImage = container.Config.Image
	}

	// Parse tag from container config image (user-specified reference)
	tag = s.extractTagFromImageRef(configImage)

	// Get digest and normalized imageRef from container image
	imageRef, digest = s.extractImageDetails(ctx, dockerClient, container)

	// Fallback to container config image if RepoDigests didn't provide imageRef
	if imageRef == "" {
		imageRef = s.normalizeImageRef(configImage)
	}

	return tag, digest, imageRef, imageID
}

// detectContainerID tries to get the current container ID, falling back to label-based detection
func (s *VersionService) detectContainerID(ctx context.Context, dockerClient *client.Client) string {
	containerId, err := s.getCurrentContainerID()
	if err == nil {
		slog.DebugContext(ctx, "detectContainerID: found via getCurrentContainerID", "containerId", containerId)
		return containerId
	}
	slog.DebugContext(ctx, "detectContainerID: getCurrentContainerID failed, trying label fallback", "error", err)

	// Fallback: locate the Arcane container by label (works even when cgroup/hostname detection fails)
	return libarcane.FindArcaneContainerIDByLabel(ctx, dockerClient)
}

// extractImageDetails extracts digest and imageRef from a container's image
func (s *VersionService) extractImageDetails(ctx context.Context, dockerClient *client.Client, container containertypes.InspectResponse) (imageRef, digest string) {
	if container.Image == "" {
		return "", ""
	}

	imageInspect, err := dockerClient.ImageInspect(ctx, container.Image)
	if err != nil {
		return "", ""
	}

	// Extract digest and repository from first RepoDigest using reference library
	for _, repoDigest := range imageInspect.RepoDigests {
		named, parseNormalizedNamedErr := reference.ParseNormalizedNamed(repoDigest)
		if parseNormalizedNamedErr != nil {
			continue
		}
		if digested, ok := named.(reference.Digested); ok {
			return named.Name(), string(digested.Digest())
		}
	}

	return "", ""
}

// normalizeImageRef extracts just the repository name from an image reference
func (s *VersionService) normalizeImageRef(configImage string) string {
	if named, err := reference.ParseNormalizedNamed(configImage); err == nil {
		return named.Name()
	}
	return configImage
}

// getCurrentContainerID detects if we're running in Docker via cgroup, mountinfo, or hostname
func (s *VersionService) getCurrentContainerID() (string, error) {
	return cgroup.CurrentContainerID()
}

// extractTagFromImageRef extracts the tag from an image reference using distribution/reference
func (s *VersionService) extractTagFromImageRef(imageRef string) string {
	named, err := reference.ParseNormalizedNamed(imageRef)
	if err != nil {
		return "latest"
	}

	tagged, ok := named.(reference.Tagged)
	if ok {
		return tagged.Tag()
	}

	return "latest"
}

// RunStreamProducer shares one version poll loop across every connected client and emits a snapshot whenever it changes.
func (s *VersionService) RunStreamProducer(ctx context.Context, events chan<- version.StreamEvent) {
	s.streamHub.Subscribe(ctx, "local",
		func(runCtx context.Context, publish func(version.StreamEvent)) {
			var last *version.Info
			poll := func() {
				info := s.GetAppVersionInfo(runCtx)
				if runCtx.Err() != nil {
					return
				}
				if last != nil {
					if last.UpdateAvailable == info.UpdateAvailable &&
						last.NewestVersion == info.NewestVersion &&
						last.NewestDigest == info.NewestDigest &&
						last.CurrentTag == info.CurrentTag &&
						last.CurrentDigest == info.CurrentDigest &&
						last.ReleaseURL == info.ReleaseURL &&
						last.ReleaseNotes == info.ReleaseNotes &&
						last.ReleasedAt == info.ReleasedAt {
						return
					}
				}
				last = info
				publish(version.StreamEvent{Type: "snapshot", Info: publicVersionInfo(info), Timestamp: time.Now()})
			}

			poll()

			ticker := time.NewTicker(versionStreamPollInterval)
			defer ticker.Stop()

			for {
				select {
				case <-runCtx.Done():
					return
				case <-ticker.C:
					poll()
				}
			}
		},
		func(event version.StreamEvent) bool {
			return agg.Send(ctx, events, event)
		})
}
