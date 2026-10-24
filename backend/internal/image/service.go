// Source: getarcaneapp/arcane/backend/internal/image/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; split inventory reads.

package image

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	dockerInternal "github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/internal/event"
	"github.com/florianfinn/docklet-next/backend/internal/image/children/attestations"
	"github.com/florianfinn/docklet-next/backend/internal/image/children/patch"
	"github.com/florianfinn/docklet-next/backend/internal/image/guard"
	"github.com/florianfinn/docklet-next/backend/internal/imageupdate"
	"github.com/florianfinn/docklet-next/backend/internal/registry"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/backend/internal/vulnerability"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/registryauth"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/runs"
	"github.com/florianfinn/docklet-next/backend/pkg/utils"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/imageref"
	"github.com/florianfinn/docklet-next/types/containerregistry"
	imagetypes "github.com/florianfinn/docklet-next/types/image"
	"github.com/florianfinn/docklet-next/types/system"
	usertypes "github.com/florianfinn/docklet-next/types/user"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/image"
	"github.com/moby/moby/client"
	"github.com/samber/hot"

	"go.getarcane.app/updater"
	"go.getarcane.app/updater/pkg/utils/tagpolicy"
	"golang.org/x/sync/errgroup"
	"io"
	"log/slog"

	"strings"
	"time"
)

type ImageService struct {
	safety               *guard.Service
	db                   *database.DB
	dockerService        *dockerInternal.DockerClientService
	imageUpdateService   *imageupdate.ImageUpdateService
	registryService      *registry.ContainerRegistryService
	vulnerabilityService *vulnerability.VulnerabilityService
	eventService         *event.EventService

	patch          *patch.Service
	projectIDCache *hot.HotCache[struct{}, map[string]string]
}

func NewImageService(
	db *database.DB,
	dockerService *dockerInternal.DockerClientService,
	registryService *registry.ContainerRegistryService,
	imageUpdateService *imageupdate.ImageUpdateService,
	vulnerabilityService *vulnerability.VulnerabilityService,
	eventService *event.EventService,
	settingsService *settings.SettingsService,
	activityService *activity.ActivityService,
) *ImageService {
	return &ImageService{
		safety:               newImageGuard(db, dockerService),
		db:                   db,
		dockerService:        dockerService,
		registryService:      registryService,
		imageUpdateService:   imageUpdateService,
		vulnerabilityService: vulnerabilityService,
		eventService:         eventService,
		patch:                patch.NewService(db, dockerService, settingsService, activityService, registryService, vulnerabilityService),
		projectIDCache: hot.NewHotCache[struct{}, map[string]string](hot.LRU, 1).
			WithTTL(projectIDCacheTTL).
			Build(),
	}
}

// PatchFlaggedImages patches eligible images for the scheduled patch job.
func (s *ImageService) PatchFlaggedImages(ctx context.Context, environmentID string, user usertypes.Actor) (patched, skipped int, err error) {
	return s.patch.PatchFlaggedImages(ctx, environmentID, user)
}

// newAttestationsServiceInternal builds the attestation reader from the image
// service's Docker client and registry credentials.
func (s *ImageService) newAttestationsServiceInternal() *attestations.Service {
	var dockerClient func(context.Context) (*client.Client, error)
	if s.dockerService != nil {
		dockerClient = s.dockerService.GetClient
	}
	var registryAuth func(context.Context, string) (string, error)
	if s.registryService != nil {
		registryAuth = s.registryService.GetRegistryAuthForHost
	}
	return attestations.NewService(dockerClient, registryAuth)
}

// GetImageDetail returns a DetailSummary for the given image ID. It fetches ImageInspect
// and ImageList concurrently so the size field reflects the same metric shown in the
// image table (docker image ls / docker system df).
func (s *ImageService) GetImageDetail(ctx context.Context, id string) (*imagetypes.DetailSummary, error) {
	dockerClient, err := s.dockerService.GetClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Docker: %w", err)
	}

	var (
		inspect    image.InspectResponse
		listSize   int64
		containers []container.Summary
	)

	g, gctx := errgroup.WithContext(ctx)

	g.Go(func() (workerErr error) {
		defer utils.RecoverToError(&workerErr, "image worker")

		var inspectImageErr error
		inspectResult, inspectImageErr := dockerClient.ImageInspect(gctx, id)
		if inspectImageErr != nil {
			return fmt.Errorf("inspect not found: %w", inspectImageErr)
		}
		inspect = inspectResult.InspectResponse
		return nil
	})

	g.Go(func() (workerErr error) {
		defer utils.RecoverToError(&workerErr, "image worker")

		imageList, imageListErr := dockerClient.ImageList(gctx, client.ImageListOptions{})
		if imageListErr != nil {
			return fmt.Errorf("failed to list images: %w", imageListErr)
		}
		for _, img := range imageList.Items {
			if img.ID == id {
				listSize = img.Size
				break
			}
		}
		return nil
	})

	g.Go(func() (workerErr error) {
		defer utils.RecoverToError(&workerErr, "image worker")

		containerList, listContainersErr := s.dockerService.ListContainers(gctx)
		if listContainersErr != nil {
			slog.DebugContext(gctx, "failed to list containers for image detail pinned references", "id", id, "error", listContainersErr)
			return nil
		}
		containers = containerList
		return nil
	})

	if waitErr := g.Wait(); waitErr != nil {
		return nil, waitErr
	}

	out := imagetypes.NewDetailSummary(&inspect)
	out.RepoTags = guard.PublicTags(out.RepoTags)
	out.UsageState = "known"
	out.ProtectionReasons = []imagetypes.Protection{}
	if pins, e := s.safety.Reasons(ctx, inspect.ID); e == nil {
		out.ProtectionReasons = append(out.ProtectionReasons, pins...)
	} else {
		out.UsageState = "unknown"
	}
	if listSize > 0 {
		out.Size = listSize
		out.Descriptor.Size = listSize
	}
	if len(containers) > 0 {
		pinnedRefsMap := collectPinnedReferencesByImageIDInternal(containers)
		out.PinnedReferences = pinnedRefsMap[inspect.ID]
	}
	return &out, nil
}

func (s *ImageService) SetImageAdmission(gate *runs.Admission, refs func() []string) {
	s.safety.Admission = gate
	s.safety.SystemReferences = refs
	if s.patch != nil {
		s.patch.SetImageGuard(s.safety)
	}
}
func newImageGuard(db *database.DB, d *dockerInternal.DockerClientService) *guard.Service {
	g := &guard.Service{DB: db}
	if d != nil {
		g.Docker = d.GetClient
	}
	return g
}
func (s *ImageService) ImageGuard() *guard.Service { return s.safety }
func (s *ImageService) RemoveImage(ctx context.Context, id string, force bool, user usertypes.Actor) error {
	release, err := s.safety.Acquire(ctx)
	if err != nil {
		return err
	}
	defer release()
	c, err := s.dockerService.GetClient(ctx)
	if err != nil {
		return imagetypes.Failure("image.remove-failed", 502)
	}
	_, err = s.safety.RemoveLocked(ctx, c, id)
	if err != nil {
		return err
	}
	s.cleanupImageUpdateRecordsInternal(ctx, []string{id})
	return nil
}
func (s *ImageService) PullImage(ctx context.Context, ref string, writer io.Writer, user usertypes.Actor, external []containerregistry.Credential) error {
	if len(external) > 0 {
		return imagetypes.Failure("registry.inline-credentials-not-allowed", 400)
	}
	if err := guard.ValidateReference(ref); err != nil {
		return err
	}
	release, err := s.safety.Acquire(ctx)
	if err != nil {
		return err
	}
	defer release()
	opts, err := s.PullOptionsWithAuth(ctx, ref, nil)
	if err != nil {
		return imagetypes.Neutral(err, "registry.credential-unavailable", 503)
	}
	c, err := s.dockerService.GetClient(ctx)
	if err != nil {
		return imagetypes.Failure("image.pull-failed", 502)
	}
	if s.eventService != nil {
		defer s.eventService.BeginDockerResourceSuppressionWindow("image", "", ref)()
	}
	reader, err := c.ImagePull(ctx, ref, opts)
	if err != nil {
		return imagetypes.Failure("image.pull-failed", 502)
	}
	defer reader.Close()
	decoder := json.NewDecoder(reader)
	for {
		var message struct {
			Status      string `json:"status"`
			Error       string `json:"error"`
			ErrorDetail *struct {
				Message string `json:"message"`
			} `json:"errorDetail"`
		}
		err = decoder.Decode(&message)
		if err == io.EOF {
			break
		}
		if err != nil || message.Error != "" || message.ErrorDetail != nil {
			return imagetypes.Failure("image.pull-failed", 502)
		}
		if writer != nil {
			if _, err = io.WriteString(writer, "{\"status\":\"Pulling image\"}\n"); err != nil {
				return imagetypes.Failure("image.pull-failed", 502)
			}
		}
	}
	if s.registryService != nil {
		_ = s.registryService.RecordImagePull(ctx, ref)
	}
	return nil
}

// ImageLastTagTime returns when the local image was last tagged, zero when the
// engine doesn't report it. Pull-policy refresh windows compare against this
// timestamp, the same clock compose v5.5.0 uses.
func (s *ImageService) ImageLastTagTime(ctx context.Context, imageName string) (time.Time, error) {
	dockerClient, err := s.dockerService.GetClient(ctx)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to connect to Docker: %w", err)
	}

	inspect, err := dockerClient.ImageInspect(ctx, imageName)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to inspect image %s: %w", imageName, err)
	}
	return inspect.Metadata.LastTagTime, nil
}

func (s *ImageService) ReconcilePulledImageUpdate(ctx context.Context, imageName string) error {
	if s.imageUpdateService == nil {
		return nil
	}

	return s.imageUpdateService.MarkImageRefUpToDateAfterPull(ctx, imageName)
}

// TagImage adds a repository tag to an existing image.
func (s *ImageService) TagImage(ctx context.Context, source string, req imagetypes.TagRequest, user usertypes.Actor) error {
	target := strings.TrimSpace(req.Repository)
	if req.Tag != "" {
		target += ":" + strings.TrimSpace(req.Tag)
	}
	if err := guard.ValidateReference(target); err != nil {
		return err
	}
	release, err := s.safety.Acquire(ctx)
	if err != nil {
		return err
	}
	defer release()
	c, err := s.dockerService.GetClient(ctx)
	if err != nil {
		return imagetypes.Failure("image.tag-failed", 502)
	}
	if s.eventService != nil {
		defer s.eventService.BeginDockerResourceSuppressionWindow("image", "", target)()
	}
	if _, err = c.ImageTag(ctx, client.ImageTagOptions{Source: source, Target: target}); err != nil {
		return imagetypes.Failure("image.tag-failed", 502)
	}
	return nil
}

// GetImageHistory returns layer history for an image.
func (s *ImageService) GetImageHistory(ctx context.Context, imageName string) ([]imagetypes.HistoryItem, error) {
	imageName = strings.TrimSpace(imageName)
	if imageName == "" {
		return nil, errors.New("image name is required")
	}

	dockerClient, err := s.dockerService.GetClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Docker: %w", err)
	}

	result, err := dockerClient.ImageHistory(ctx, imageName)
	if err != nil {
		return nil, fmt.Errorf("failed to get image history: %w", err)
	}

	items := make([]imagetypes.HistoryItem, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, imagetypes.HistoryItem{
			ID:        item.ID,
			Created:   item.Created,
			CreatedBy: item.CreatedBy,
			Tags:      item.Tags,
			Size:      item.Size,
			Comment:   item.Comment,
		})
	}
	return items, nil
}

// SearchImages searches the configured Docker registry for images.
func (s *ImageService) SearchImages(ctx context.Context, term string) ([]imagetypes.SearchResult, error) {
	if err := guard.ValidateReference(term); err != nil {
		return nil, err
	}
	term = strings.TrimSpace(term)
	if term == "" {
		return nil, errors.New("search term is required")
	}

	dockerClient, err := s.dockerService.GetClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Docker: %w", err)
	}

	result, err := dockerClient.ImageSearch(ctx, term, client.ImageSearchOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to search images: %w", err)
	}

	items := make([]imagetypes.SearchResult, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, imagetypes.SearchResult{
			Name:        item.Name,
			Description: item.Description,
			StarCount:   item.StarCount,
			Official:    item.IsOfficial,
		})
	}
	return items, nil
}

// ExportImage returns a tar stream for one Docker image.
func (s *ImageService) ExportImage(ctx context.Context, imageName string) (io.ReadCloser, error) {
	imageName = strings.TrimSpace(imageName)
	if imageName == "" {
		return nil, errors.New("image name is required")
	}

	dockerClient, err := s.dockerService.GetClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Docker: %w", err)
	}

	reader, err := dockerClient.ImageSave(ctx, []string{imageName})
	if err != nil {
		return nil, fmt.Errorf("failed to export image: %w", err)
	}
	return reader, nil
}

func (s *ImageService) LoadImageFromReader(ctx context.Context, reader io.Reader, fileName string, user usertypes.Actor, maxSizeBytes int64) (*imagetypes.LoadResult, error) {
	validated, cleanup, err := guard.ValidateArchive(reader, maxSizeBytes)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	release, err := s.safety.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	c, err := s.dockerService.GetClient(ctx)
	if err != nil {
		return nil, imagetypes.Failure("image.load-failed", 502)
	}
	resp, err := c.ImageLoad(ctx, validated)
	if err != nil {
		return nil, imagetypes.Failure("image.load-failed", 502)
	}
	defer resp.Close()
	decoder := json.NewDecoder(resp)
	for {
		var item struct {
			Error string `json:"error"`
		}
		err = decoder.Decode(&item)
		if err == io.EOF {
			break
		}
		if err != nil || item.Error != "" {
			return nil, imagetypes.Failure("image.load-failed", 502)
		}
	}
	return &imagetypes.LoadResult{Stream: "Image archive loaded."}, nil
}

func (s *ImageService) ImageExistsLocally(ctx context.Context, imageName string) (bool, error) {
	dockerClient, err := s.dockerService.GetClient(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to connect to Docker: %w", err)
	}

	_, err = dockerClient.ImageInspect(ctx, imageName)
	if err == nil {
		return true, nil
	}

	errLower := strings.ToLower(err.Error())
	if strings.Contains(errLower, "no such image") || strings.Contains(errLower, "not found") {
		return false, nil
	}
	return false, fmt.Errorf("failed to inspect image %s: %w", imageName, err)
}

func (s *ImageService) PullOptionsWithAuth(ctx context.Context, imageRef string, externalCreds []containerregistry.Credential) (client.ImagePullOptions, error) {
	pullOptions := client.ImagePullOptions{}

	registryHost := registryauth.ExtractRegistryHost(imageRef)

	// Legacy internal credential arguments never override saved host auth.

	if s.registryService == nil {
		return pullOptions, nil
	}

	authStr, err := s.registryService.GetRegistryAuthForHost(ctx, registryHost)
	if err != nil {
		return pullOptions, fmt.Errorf("failed to get registry credentials: %w", err)
	}
	if authStr != "" {
		pullOptions.RegistryAuth = authStr
		slog.DebugContext(ctx, "Using database credentials for image pull", "registry", registryHost)
	}

	return pullOptions, nil
}

func ShouldRetryAnonymousPull(pullOptions client.ImagePullOptions, pullErr error) bool {
	if pullOptions.RegistryAuth == "" || pullErr == nil {
		return false
	}
	return isUnauthorizedPullErrorInternal(pullErr)
}

func isUnauthorizedPullErrorInternal(err error) bool {
	if err == nil {
		return false
	}

	errLower := strings.ToLower(err.Error())
	unauthorizedIndicators := []string{
		"unauthorized",
		"authentication required",
		"incorrect username or password",
		"no basic auth credentials",
		"access denied",
	}

	for _, indicator := range unauthorizedIndicators {
		if strings.Contains(errLower, indicator) {
			return true
		}
	}
	return false
}

func (s *ImageService) PruneImages(ctx context.Context, options system.PruneImagesOptions) (*imagetypes.PruneReport, error) {
	return s.safety.Prune(ctx, options)
}

func getDeletedImageIDsInternal(deletedImages []image.DeleteResponse) []string {
	if len(deletedImages) == 0 {
		return nil
	}

	idsToDelete := make([]string, 0, len(deletedImages))
	for _, img := range deletedImages {
		if img.Deleted != "" {
			idsToDelete = append(idsToDelete, img.Deleted)
			continue
		}
		if img.Untagged != "" {
			idsToDelete = append(idsToDelete, img.Untagged)
		}
	}
	return idsToDelete
}

func (s *ImageService) cleanupVulnerabilityRecordsAfterPruneInternal(ctx context.Context, idsToDelete []string) {
	if s.vulnerabilityService == nil || len(idsToDelete) == 0 {
		return
	}

	if err := s.vulnerabilityService.DeleteScanResultsByImageIDs(ctx, idsToDelete); err != nil {
		slog.WarnContext(ctx, "failed to delete vulnerability scan records after prune", "count", len(idsToDelete), "error", err)
	}
}

func (s *ImageService) cleanupImageUpdateRecordsInternal(ctx context.Context, idsToDelete []string) {
	if s.imageUpdateService == nil {
		return
	}

	if err := s.imageUpdateService.DeleteRecordsForImages(ctx, idsToDelete); err != nil {
		slog.WarnContext(ctx, "failed to clean up image update records", "error", err)
	}
	if err := s.imageUpdateService.CleanupOrphanedRecords(ctx); err != nil {
		slog.WarnContext(ctx, "failed to clean up orphaned image update records", "error", err)
	}
}

// GetUpdateInfoByImageIDs returns a map of image ID to UpdateInfo for the given image IDs.
// This is used by the container service to populate update info for containers.
func (s *ImageService) GetUpdateInfoByImageIDs(ctx context.Context, imageIDs []string) (map[string]*imagetypes.UpdateInfo, error) {
	if s.db == nil || len(imageIDs) == 0 {
		return make(map[string]*imagetypes.UpdateInfo), nil
	}

	var updateRecords []imageupdate.ImageUpdateRecord
	if err := s.db.WithContext(ctx).Where("container_id = ? AND project_id = ? AND id IN ?", "", "", imageIDs).Find(&updateRecords).Error; err != nil {
		return nil, fmt.Errorf("failed to fetch update records: %w", err)
	}

	result := make(map[string]*imagetypes.UpdateInfo, len(updateRecords))
	for i := range updateRecords {
		result[updateRecords[i].ID] = updateRecords[i].UpdateInfo()
	}

	return result, nil
}

// GetUpdateInfoByContainers returns the stored check matching each container's
// current policy, keyed by container ID. Digest policies read the image-level
// record for the container's runtime image ID, so a check recorded under another
// local tag of the same image still applies. Tag policies read the container's
// own record and require an exact policy key. Containers that opted out of update
// checks are omitted; disabling automatic installation does not hide results.
func (s *ImageService) GetUpdateInfoByContainers(ctx context.Context, containers []container.Summary) (map[string]*imagetypes.UpdateInfo, error) {
	result := make(map[string]*imagetypes.UpdateInfo)
	if s.db == nil || len(containers) == 0 {
		return result, nil
	}
	digestImageIDs := make(map[string]string, len(containers))
	tagContainerIDs := make([]string, 0, len(containers))
	policies := make(map[string]string, len(containers))
	for _, cnt := range containers {
		if cnt.ID == "" || imageref.IsUpdateCheckDisabled(cnt.Labels) {
			continue
		}
		policy, policyErr := tagpolicy.Resolve(cnt.Image, updater.DefaultLabelPolicy().TagPolicy(cnt.Labels))
		if policyErr == nil && policy.Strategy == "digest" {
			if imageID := strings.TrimSpace(cnt.ImageID); imageID != "" {
				digestImageIDs[cnt.ID] = imageID
			}
			continue
		}
		tagContainerIDs = append(tagContainerIDs, cnt.ID)
		policies[cnt.ID] = imageref.UpdatePolicyKey(cnt.Image, cnt.Labels)
	}

	if len(digestImageIDs) > 0 {
		imageIDs := make([]string, 0, len(digestImageIDs))
		seen := make(map[string]struct{}, len(digestImageIDs))
		for _, imageID := range digestImageIDs {
			if _, exists := seen[imageID]; exists {
				continue
			}
			seen[imageID] = struct{}{}
			imageIDs = append(imageIDs, imageID)
		}
		byImageID, err := s.GetUpdateInfoByImageIDs(ctx, imageIDs)
		if err != nil {
			return nil, err
		}
		for containerID, imageID := range digestImageIDs {
			if info := byImageID[imageID]; info != nil {
				// Each container gets its own copy so aggregation never mutates a shared record.
				scoped := *info
				result[containerID] = &scoped
			}
		}
	}

	if len(tagContainerIDs) == 0 {
		return result, nil
	}
	var records []imageupdate.ImageUpdateRecord
	if err := s.db.WithContext(ctx).Where("container_id IN ?", tagContainerIDs).Find(&records).Error; err != nil {
		return nil, fmt.Errorf("failed to fetch container update records: %w", err)
	}
	for i := range records {
		if records[i].PolicyKey == policies[records[i].ContainerID] {
			result[records[i].ContainerID] = records[i].UpdateInfo()
		}
	}
	return result, nil
}

type imageRefUpdateLookup struct {
	originalRef          string
	tag                  string
	repositoryCandidates map[string]struct{}
}

// GetUpdateInfoByImageRefs returns persisted update information keyed by the
// original image reference string.
func (s *ImageService) GetUpdateInfoByImageRefs(ctx context.Context, imageRefs []string) (map[string]*imagetypes.UpdateInfo, error) {
	result := make(map[string]*imagetypes.UpdateInfo)
	if s.db == nil || len(imageRefs) == 0 {
		return result, nil
	}

	lookups := buildImageRefUpdateLookupsInternal(imageRefs)
	if len(lookups) == 0 {
		return result, nil
	}

	repositoryCandidates := make([]string, 0)
	repositorySeen := make(map[string]struct{})
	tags := make([]string, 0, len(lookups))
	tagSeen := make(map[string]struct{})

	for _, lookup := range lookups {
		if _, exists := tagSeen[lookup.tag]; !exists {
			tagSeen[lookup.tag] = struct{}{}
			tags = append(tags, lookup.tag)
		}
		for repo := range lookup.repositoryCandidates {
			if _, exists := repositorySeen[repo]; exists {
				continue
			}
			repositorySeen[repo] = struct{}{}
			repositoryCandidates = append(repositoryCandidates, repo)
		}
	}

	if len(repositoryCandidates) == 0 || len(tags) == 0 {
		return result, nil
	}

	var updateRecords []imageupdate.ImageUpdateRecord
	if err := s.db.WithContext(ctx).
		Where("container_id = ? AND project_id = ? AND tag IN ? AND repository IN ?", "", "", tags, repositoryCandidates).
		Order("check_time DESC").
		Find(&updateRecords).Error; err != nil {
		return nil, fmt.Errorf("failed to fetch update records by image refs: %w", err)
	}

	index := indexLatestImageUpdateRecordsInternal(updateRecords)
	for _, lookup := range lookups {
		if record := selectLatestMatchingImageUpdateRecordInternal(lookup, updateRecords, index); record != nil {
			result[lookup.originalRef] = record.UpdateInfo()
		}
	}

	return result, nil
}
