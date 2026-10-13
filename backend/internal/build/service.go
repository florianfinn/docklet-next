// Source: getarcaneapp/arcane/backend/internal/build/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed GitOps and saved Git repositories; functional cut: removed standalone build workshop.

package build

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	usertypes "github.com/florianfinn/docklet-next/types/user"
	"go.getarcane.app/builds/api"
	"go.getarcane.app/builds/pkg/contextsource"
	"go.getarcane.app/builds/types"
	"go.getarcane.app/docker"
	"go.getarcane.app/kit/pkg"
	gitkit "go.getarcane.app/kit/pkg/git"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	dockerInternal "github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/internal/event"
	"github.com/florianfinn/docklet-next/backend/internal/registry"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/backend/pkg/gitutil"
)

type BuildService struct {
	settings      *settings.SettingsService
	dockerService *dockerInternal.DockerClientService
	gitClient     *git.Client
	eventService  *event.EventService
	builder       types.Builder
	gitProbeFn    func(context.Context, string, git.AuthConfig) error
	gitCloneFn    func(context.Context, string, string, git.AuthConfig) (string, error)
	gitCleanupFn  func(string) error
}

func NewBuildService(
	localSettings *settings.SettingsService,
	dockerService *dockerInternal.DockerClientService,
	registryService *registry.ContainerRegistryService,
	eventService *event.EventService,
) *BuildService {
	svc := &BuildService{
		settings:      localSettings,
		dockerService: dockerService,
		gitClient:     git.NewClient(os.TempDir()),
		eventService:  eventService,
	}
	var registryAuthProvider types.RegistryAuthProvider
	if registryService != nil {
		registryAuthProvider = registryService
	}

	// registry.ContainerRegistryService already implements buildtypes.RegistryAuthProvider,
	// so the builder consumes it directly instead of through forwarding methods on BuildService.
	svc.builder = api.NewService(api.Config{
		SettingsProvider:     svc,
		DockerClientProvider: dockerService,
		RegistryAuthProvider: registryAuthProvider,
	})

	return svc
}

func (s *BuildService) BuildSettings() types.BuildSettings {
	if s.settings == nil {
		return types.BuildSettings{}
	}
	localSettings := s.settings.GetSettingsConfig()
	return types.BuildSettings{
		DepotProjectID:   localSettings.DepotProjectId.Value,
		DepotToken:       localSettings.DepotToken.Value,
		BuildProvider:    localSettings.BuildProvider.Value,
		BuildTimeoutSecs: localSettings.BuildTimeout.AsInt(),
	}
}

// BuildImage builds a Compose service image and forwards progress to the caller.
func (s *BuildService) BuildImage(ctx context.Context, environmentID string, req types.BuildRequest, progressWriter io.Writer, serviceName string, user *usertypes.Actor) (*types.BuildResult, error) {
	if s.builder == nil {
		return nil, errors.New("build service not available")
	}
	writer := io.Writer(io.Discard)
	var logWriter io.WriteCloser
	if progressWriter != nil {
		logWriter = docker.NewLogLineWriter(progressWriter)
		writer = logWriter
		defer logWriter.Close()
	}
	cleanup := func() error { return nil }
	defer func() {
		if err := cleanup(); err != nil {
			slog.WarnContext(ctx, "failed to cleanup temporary git build context", "error", err)
		}
	}()
	resolved, cleanupFn, err := s.resolveBuildRequestInternal(ctx, req, writer, serviceName)
	if err != nil {
		s.logBuildFailureEventInternal(ctx, environmentID, req, serviceName, err, user)
		return nil, err
	}
	cleanup = cleanupFn
	result, err := s.builder.BuildImage(ctx, resolved, writer, serviceName)
	if err != nil {
		s.logBuildFailureEventInternal(ctx, environmentID, req, serviceName, err, user)
	}
	return result, err
}

func (s *BuildService) logBuildFailureEventInternal(ctx context.Context, environmentID string, req types.BuildRequest, serviceName string, err error, user *usertypes.Actor) {
	if s.eventService == nil || err == nil {
		return
	}

	resourceName := cmp.Or(kit.TrimNonEmpty(req.Tags)...)
	if resourceName == "" {
		resourceName = strings.TrimSpace(serviceName)
	}
	if resourceName == "" {
		resourceName = sanitizeBuildContextForEventInternal(req.ContextDir)
	}

	userID := ""
	username := ""
	if user != nil {
		userID = user.ID
		username = user.Username
	}

	metadata := database.JSON{
		"action":     "build",
		"provider":   s.effectiveBuildProviderInternal(req.Provider),
		"contextDir": sanitizeBuildContextForEventInternal(req.ContextDir),
		"dockerfile": req.Dockerfile,
		"tags":       append([]string(nil), req.Tags...),
	}
	if service := strings.TrimSpace(serviceName); service != "" {
		metadata["serviceName"] = service
	}
	s.eventService.LogErrorEvent(ctx, event.EventTypeImageError, "image", "", resourceName, userID, username, environmentID, err, metadata)
}

func sanitizeBuildContextForEventInternal(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}

	base, fragment, hasFragment := strings.Cut(trimmed, "#")
	parsed, err := url.Parse(base)
	if err != nil {
		return kit.Ternary(strings.Contains(base, "@"), "[unparseable URL]", trimmed)
	}
	if parsed.User == nil {
		return trimmed
	}

	parsed.User = url.User("redacted")
	sanitized := parsed.String()
	if hasFragment {
		sanitized += "#" + fragment
	}
	return sanitized
}

func (s *BuildService) effectiveBuildProviderInternal(provider string) string {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider != "" {
		return provider
	}
	if s.settings != nil {
		provider = strings.ToLower(strings.TrimSpace(s.settings.GetSettingsConfig().BuildProvider.Value))
	}
	return kit.Ternary(provider == "", "local", provider)
}

func (s *BuildService) resolveBuildRequestInternal(
	ctx context.Context,
	req types.BuildRequest,
	progressWriter io.Writer,
	serviceName string,
) (types.BuildRequest, func() error, error) {
	source, ok, err := contextsource.ParseGitBuildContextSource(req.ContextDir)
	if err != nil {
		return types.BuildRequest{}, func() error { return nil }, err
	}
	if !ok || source == nil {
		if contextsource.IsPotentialRemoteBuildContextSource(req.ContextDir) {
			return types.BuildRequest{}, func() error { return nil }, fmt.Errorf("unsupported remote build context source %q: only git repository URLs are supported", req.ContextDir)
		}
		return req, func() error { return nil }, nil
	}

	writeBuildProgressStatusInternal(ctx, progressWriter, serviceName, "resolving remote git context "+source.RepositoryURL)

	authConfig := git.AuthConfig{}
	if gitkit.RequiresRemoteProbe(source.RepositoryURL) {
		writeBuildProgressStatusInternal(ctx, progressWriter, serviceName, "verifying remote git repository "+source.RepositoryURL)
		if probeGitContextErr := s.probeGitContextInternal(ctx, source.RepositoryURL, authConfig); probeGitContextErr != nil {
			return types.BuildRequest{}, func() error { return nil }, fmt.Errorf("failed to verify remote git repository %q: %w", source.RepositoryURL, probeGitContextErr)
		}
	}

	repoPath, err := s.cloneGitContextInternal(ctx, source.RepositoryURL, source.Ref, authConfig)
	if err != nil {
		return types.BuildRequest{}, func() error { return nil }, err
	}

	contextDir := repoPath
	if source.Subdir != "" {
		if validatePathErr := git.ValidatePath(repoPath, filepath.FromSlash(source.Subdir)); validatePathErr != nil {
			_ = s.cleanupGitContextInternal(repoPath)
			return types.BuildRequest{}, func() error { return nil }, fmt.Errorf("invalid git build context subdir: %w", validatePathErr)
		}
		contextDir = filepath.Join(repoPath, filepath.FromSlash(source.Subdir))
	}

	// os.* rather than acfs: the build context path comes from the user-provided
	// source and may point anywhere on the host, so no confinement root exists.
	info, err := os.Stat(contextDir)
	if err != nil {
		_ = s.cleanupGitContextInternal(repoPath)
		return types.BuildRequest{}, func() error { return nil }, fmt.Errorf("failed to stat resolved git build context: %w", err)
	}
	if !info.IsDir() {
		_ = s.cleanupGitContextInternal(repoPath)
		return types.BuildRequest{}, func() error { return nil }, errors.New("resolved git build context is not a directory")
	}

	writeBuildProgressStatusInternal(ctx, progressWriter, serviceName, "using remote build context "+source.Raw)

	resolvedReq := req
	resolvedReq.ContextDir = contextDir

	return resolvedReq, func() error { return s.cleanupGitContextInternal(repoPath) }, nil
}

func (s *BuildService) probeGitContextInternal(ctx context.Context, repositoryURL string, authConfig git.AuthConfig) error {
	if s.gitProbeFn != nil {
		return s.gitProbeFn(ctx, repositoryURL, authConfig)
	}

	return s.gitClient.ProbeRemote(ctx, repositoryURL, authConfig)
}

func (s *BuildService) cloneGitContextInternal(ctx context.Context, repositoryURL, ref string, authConfig git.AuthConfig) (string, error) {
	if s.gitCloneFn != nil {
		return s.gitCloneFn(ctx, repositoryURL, ref, authConfig)
	}

	return s.gitClient.Clone(ctx, repositoryURL, ref, authConfig)
}

func (s *BuildService) cleanupGitContextInternal(repoPath string) error {
	if repoPath == "" {
		return nil
	}
	if s.gitCleanupFn != nil {
		return s.gitCleanupFn(repoPath)
	}
	return s.gitClient.Cleanup(repoPath)
}

func writeBuildProgressStatusInternal(ctx context.Context, progressWriter io.Writer, serviceName, status string) {
	if progressWriter == nil || strings.TrimSpace(status) == "" {
		return
	}

	line := status
	if service := strings.TrimSpace(serviceName); service != "" {
		line = service + ": " + status
	}
	if _, err := io.WriteString(progressWriter, line+"\n"); err != nil {
		slog.DebugContext(ctx, "failed to write build progress status", "error", err)
	}
}
