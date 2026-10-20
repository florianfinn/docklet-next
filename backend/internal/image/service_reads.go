// Source: getarcaneapp/arcane/backend/internal/image/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; split inventory reads and hide dedicated agent images.

package image

import (
	"cmp"
	"context"
	"fmt"
	"github.com/distribution/reference"
	"github.com/florianfinn/docklet-next/backend/internal/imageupdate"
	"github.com/florianfinn/docklet-next/backend/pkg/inventory"
	"github.com/florianfinn/docklet-next/backend/pkg/pagination"
	"github.com/florianfinn/docklet-next/backend/pkg/projects"
	"github.com/florianfinn/docklet-next/backend/pkg/utils"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/imageref"
	imagetypes "github.com/florianfinn/docklet-next/types/image"
	vulnerabilitytypes "github.com/florianfinn/docklet-next/types/vulnerability"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/image"
	"github.com/samber/hot"
	"github.com/samber/mo"
	"go.getarcane.app/docker"
	"go.getarcane.app/kit/pkg"
	"golang.org/x/sync/errgroup"
	"log/slog"
	"maps"
	"slices"
	"strings"
	"time"
)

func (s *ImageService) ListImagesPaginated(ctx context.Context, params pagination.QueryParams) ([]imagetypes.Summary, pagination.Response, error) {
	var (
		dockerImages  []image.Summary
		containers    []container.Summary
		updateRecords []imageupdate.ImageUpdateRecord
	)

	g, groupCtx := errgroup.WithContext(ctx)

	// Fetch Docker images
	g.Go(func() (workerErr error) {
		defer utils.RecoverToError(&workerErr, "image worker")

		var err error
		imageList, err := s.dockerService.ListImages(groupCtx)
		if err != nil {
			return fmt.Errorf("failed to list Docker images: %w", err)
		}
		dockerImages = imageList
		return nil
	})

	// Fetch containers to determine usage
	g.Go(func() (workerErr error) {
		defer utils.RecoverToError(&workerErr, "image worker")

		var err error
		containerList, err := s.dockerService.ListContainers(groupCtx)
		if err != nil {
			return fmt.Errorf("failed to list containers: %w", err)
		}
		containers = containerList
		return nil
	})

	if err := g.Wait(); err != nil {
		return nil, pagination.Response{}, err
	}

	dockerImages = inventory.Images(dockerImages, containers)

	imageIDs := make([]string, 0, len(dockerImages))
	for _, img := range dockerImages {
		imageIDs = append(imageIDs, img.ID)
	}

	if s.db != nil && len(imageIDs) > 0 {
		if err := s.db.WithContext(ctx).Where("id IN ? OR image_id IN ?", imageIDs, imageIDs).Find(&updateRecords).Error; err != nil {
			return nil, pagination.Response{}, fmt.Errorf("failed to fetch image update records: %w", err)
		}
	}

	var vulnerabilityMap map[string]*vulnerabilitytypes.ScanSummary
	if s.vulnerabilityService != nil && len(imageIDs) > 0 {
		var err error
		vulnerabilityMap, err = s.vulnerabilityService.GetScanSummariesByImageIDs(ctx, imageIDs)
		if err != nil {
			return nil, pagination.Response{}, err
		}
	}

	projectIDByName := s.BuildProjectIDMap(ctx, containers)
	usageMap := BuildVolumeUsageMap(containers, projectIDByName)
	updateMap := buildUpdateMapInternal(updateRecords)

	items := MapDockerImagesToDTOs(dockerImages, containers, usageMap, updateMap, vulnerabilityMap)

	config := s.getImagePaginationConfig()

	result := config.SearchOrderAndPaginate(items, params)

	paginationResp := pagination.BuildResponse(result.TotalCount, result.TotalAvailable, params)

	return result.Items, paginationResp, nil
}

func buildImageRefUpdateLookupsInternal(imageRefs []string) []imageRefUpdateLookup {
	lookups := make([]imageRefUpdateLookup, 0, len(imageRefs))
	seen := make(map[string]struct{}, len(imageRefs))

	for _, rawRef := range imageRefs {
		originalRef, tag, repositoryCandidates, ok := imageref.ParseUpdateLookup(rawRef)
		if !ok {
			continue
		}
		lookup := imageRefUpdateLookup{originalRef: originalRef, tag: tag, repositoryCandidates: repositoryCandidates}
		if _, exists := seen[lookup.originalRef]; exists {
			continue
		}
		seen[lookup.originalRef] = struct{}{}
		lookups = append(lookups, lookup)
	}

	return lookups
}

type imageUpdateRecordKeyInternal struct {
	repository string
	tag        string
}

func newImageUpdateRecordKeyInternal(repository, tag string) imageUpdateRecordKeyInternal {
	return imageUpdateRecordKeyInternal{repository: strings.TrimSpace(repository), tag: strings.ToLower(strings.TrimSpace(tag))}
}

// indexLatestImageUpdateRecordsInternal maps each repository and tag to the
// position of its latest record. Ties keep the earliest position, matching
// the order the records were fetched in.
func indexLatestImageUpdateRecordsInternal(updateRecords []imageupdate.ImageUpdateRecord) map[imageUpdateRecordKeyInternal]int {
	index := make(map[imageUpdateRecordKeyInternal]int, len(updateRecords))
	for i := range updateRecords {
		key := newImageUpdateRecordKeyInternal(updateRecords[i].Repository, updateRecords[i].Tag)
		if existing, ok := index[key]; !ok || updateRecords[i].CheckTime.After(updateRecords[existing].CheckTime) {
			index[key] = i
		}
	}
	return index
}

// selectLatestMatchingImageUpdateRecordInternal picks the latest record across
// the lookup's repository aliases; equal check times fall back to fetch order.
func selectLatestMatchingImageUpdateRecordInternal(
	lookup imageRefUpdateLookup,
	updateRecords []imageupdate.ImageUpdateRecord,
	index map[imageUpdateRecordKeyInternal]int,
) *imageupdate.ImageUpdateRecord {
	latest := -1
	for repository := range lookup.repositoryCandidates {
		i, ok := index[newImageUpdateRecordKeyInternal(repository, lookup.tag)]
		if !ok {
			continue
		}
		switch {
		case latest == -1,
			updateRecords[i].CheckTime.After(updateRecords[latest].CheckTime),
			updateRecords[i].CheckTime.Equal(updateRecords[latest].CheckTime) && i < latest:
			latest = i
		}
	}
	if latest == -1 {
		return nil
	}
	return &updateRecords[latest]
}

func convertLabels(labels map[string]string) map[string]any {
	if labels == nil {
		return nil
	}
	result, _ := kit.AsStringMap(labels)
	return result
}

// projectIDCacheTTL bounds how long a snapshot of all (name → id) project rows is reused.
// Dashboard polls frequently; the projects table changes rarely, so a short TTL is safe.
const projectIDCacheTTL = 5 * time.Second

func (s *ImageService) loadProjectIDByNameCachedInternal(ctx context.Context) map[string]string {
	if s.projectIDCache == nil {
		s.projectIDCache = hot.NewHotCache[struct{}, map[string]string](hot.LRU, 1).
			WithTTL(projectIDCacheTTL).
			Build()
	}
	stale, staleFound := s.projectIDCache.Peek(struct{}{})
	byName, found, err := s.projectIDCache.GetWithLoaders(struct{}{}, func(_ []struct{}) (map[struct{}]map[string]string, error) {
		var rows []struct {
			ID                 string
			Name               string
			ComposeProjectName *string
		}
		if err := s.db.WithContext(ctx).Table("projects").Select("id", "name", "compose_project_name").
			Order("CASE WHEN compose_project_name IS NULL OR compose_project_name = '' THEN 1 ELSE 0 END").Order("id").Find(&rows).Error; err != nil {
			return nil, err
		}

		loaded := make(map[string]string, len(rows))
		for _, row := range rows {
			name := row.Name
			if row.ComposeProjectName != nil && *row.ComposeProjectName != "" {
				name = *row.ComposeProjectName
			}
			name = projects.NormalizeProjectName(name)
			if name != "" {
				if _, exists := loaded[name]; !exists {
					loaded[name] = row.ID
				}
			}
		}
		return map[struct{}]map[string]string{{}: loaded}, nil
	})
	if err != nil {
		slog.WarnContext(ctx, "failed to load project ID map", "error", err)
		return kit.Ternary(staleFound, stale, map[string]string{})
	}
	return kit.Ternary(!found, map[string]string{}, byName)
}

// BuildProjectIDMap returns a map of compose project name → project ID for any
// containers that carry the com.docker.compose.project label. The lookup uses a
// short-TTL cache shared across all callers of this ImageService instance.
func (s *ImageService) BuildProjectIDMap(ctx context.Context, containers []container.Summary) map[string]string {
	projectIDs := make(map[string]string)
	if s.db == nil {
		return projectIDs
	}

	projectNameSet := make(map[string]struct{})
	for _, c := range containers {
		if c.Labels == nil {
			continue
		}
		if projectName := docker.ComposeProjectLabel(c.Labels); projectName != "" {
			projectNameSet[projectName] = struct{}{}
		}
	}
	if len(projectNameSet) == 0 {
		return projectIDs
	}

	all := s.loadProjectIDByNameCachedInternal(ctx)
	for name := range projectNameSet {
		if id, ok := all[name]; ok {
			projectIDs[name] = id
		}
	}
	return projectIDs
}

func BuildVolumeUsageMap(containers []container.Summary, projectIDByName map[string]string) map[string][]imagetypes.UsedBy {
	usageMap := make(map[string][]imagetypes.UsedBy)
	projectSeen := make(map[string]map[string]bool)
	containerSeen := make(map[string]map[string]bool)

	for _, c := range containers {
		if c.ImageID == "" {
			continue
		}

		projectName := docker.ComposeProjectLabel(c.Labels)

		if projectName != "" {
			projectID := projectIDByName[projectName]
			if projectSeen[c.ImageID] == nil {
				projectSeen[c.ImageID] = make(map[string]bool)
			}
			if !projectSeen[c.ImageID][projectName] {
				usedBy := imagetypes.UsedBy{
					Type: "project",
					Name: projectName,
				}
				usedBy.ID = cmp.Or(projectID, usedBy.ID)
				usageMap[c.ImageID] = append(usageMap[c.ImageID], usedBy)
				projectSeen[c.ImageID][projectName] = true
			}
			continue
		}

		containerName := cmp.Or(docker.ContainerNameFromNames(c.Names), c.ID)

		if containerSeen[c.ImageID] == nil {
			containerSeen[c.ImageID] = make(map[string]bool)
		}
		if !containerSeen[c.ImageID][c.ID] {
			usageMap[c.ImageID] = append(usageMap[c.ImageID], imagetypes.UsedBy{
				Type: "container",
				Name: containerName,
				ID:   c.ID,
			})
			containerSeen[c.ImageID][c.ID] = true
		}
	}

	return usageMap
}

func buildUpdateMapInternal(records []imageupdate.ImageUpdateRecord) map[string]*imageupdate.ImageUpdateRecord {
	updateMap := make(map[string]*imageupdate.ImageUpdateRecord, len(records))
	for i := range records {
		if records[i].ContainerID == "" {
			updateMap[records[i].ID] = &records[i]
		}
	}
	for i := range records {
		record := &records[i]
		if record.ContainerID == "" || record.ImageID == "" || !record.HasUpdate {
			continue
		}
		// A shared image can have several container policies and target tags.
		aggregate := &imageupdate.ImageUpdateRecord{ID: record.ImageID, HasUpdate: true, CheckTime: record.CheckTime}
		if existing := updateMap[record.ImageID]; existing != nil && existing.CheckTime.After(aggregate.CheckTime) {
			aggregate.CheckTime = existing.CheckTime
		}
		updateMap[record.ImageID] = aggregate
	}
	return updateMap
}

func parseRepoAndTagFromRepoTag(repoTag string) (repo, tag string) {
	if named, err := reference.ParseNormalizedNamed(repoTag); err == nil {
		repo = reference.FamiliarName(named)
		if tagged, ok := named.(reference.NamedTagged); ok {
			tag = tagged.Tag()
		} else {
			tag = "latest"
		}
		return repo, tag
	}

	if before, after, found := strings.CutLast(repoTag, ":"); found {
		return before, after
	}
	return repoTag, "latest"
}

func parseRepoFromDigests(repoDigests []string) mo.Option[string] {
	for _, rd := range repoDigests {
		if rd == "<none>@<none>" {
			continue
		}
		if candidateRepo, _, found := strings.CutLast(rd, "@"); found && candidateRepo != "" {
			return mo.Some(candidateRepo)
		}
	}
	return mo.None[string]()
}

func determineRepoAndTag(di image.Summary) (repo, tag string) {
	if len(di.RepoTags) > 0 {
		return parseRepoAndTagFromRepoTag(di.RepoTags[0])
	}

	if len(di.RepoDigests) > 0 {
		if localRepo, found := parseRepoFromDigests(di.RepoDigests).Get(); found {
			return localRepo, "<none>"
		}
	}

	return "<none>", "<none>"
}

func collectPinnedReferencesByImageIDInternal(containers []container.Summary) map[string][]string {
	if len(containers) == 0 {
		return nil
	}

	seen := make(map[string]map[string]struct{})
	for _, c := range containers {
		if c.ImageID == "" || c.Image == "" {
			continue
		}
		trimmed := strings.TrimSpace(c.Image)
		if trimmed == "" || strings.HasPrefix(trimmed, "sha256:") {
			continue
		}

		named, err := reference.ParseNormalizedNamed(trimmed)
		if err != nil {
			continue
		}
		if _, ok := named.(reference.Digested); !ok {
			continue
		}

		if seen[c.ImageID] == nil {
			seen[c.ImageID] = make(map[string]struct{})
		}
		seen[c.ImageID][trimmed] = struct{}{}
	}

	if len(seen) == 0 {
		return nil
	}

	result := make(map[string][]string, len(seen))
	for imageID, refsMap := range seen {
		refs := slices.Sorted(maps.Keys(refsMap))
		result[imageID] = refs
	}

	return result
}

func MapDockerImagesToDTOs(
	dockerImages []image.Summary,
	containers []container.Summary,
	usageMap map[string][]imagetypes.UsedBy,
	updateMap map[string]*imageupdate.ImageUpdateRecord,
	vulnerabilityMap map[string]*vulnerabilitytypes.ScanSummary,
) []imagetypes.Summary {
	pinnedRefsByImageID := collectPinnedReferencesByImageIDInternal(containers)
	items := make([]imagetypes.Summary, 0, len(dockerImages))
	for _, di := range dockerImages {
		repo, tag := determineRepoAndTag(di)

		usedBy := usageMap[di.ID]
		imageDto := imagetypes.Summary{
			ID:               di.ID,
			Repo:             repo,
			Tag:              tag,
			RepoTags:         di.RepoTags,
			RepoDigests:      di.RepoDigests,
			PinnedReferences: pinnedRefsByImageID[di.ID],
			Created:          di.Created,
			Size:             di.Size,
			VirtualSize:      di.SharedSize,
			Labels:           convertLabels(di.Labels),
			InUse:            len(usedBy) > 0,
			UsedBy:           usedBy,
		}

		if updateRecord, exists := updateMap[di.ID]; exists {
			imageDto.UpdateInfo = updateRecord.UpdateInfo()
		}

		if vulnerabilityMap != nil {
			if summary, exists := vulnerabilityMap[di.ID]; exists {
				imageDto.VulnerabilityScan = summary
			}
		}

		items = append(items, imageDto)
	}
	return items
}

func vulnerabilitySortCountsInternal(i imagetypes.Summary) vulnerabilitytypes.SeveritySummary {
	scan := i.VulnerabilityScan
	if scan == nil || scan.Status != vulnerabilitytypes.ScanStatusCompleted || scan.Summary == nil {
		return vulnerabilitytypes.SeveritySummary{Total: -1}
	}
	return *scan.Summary
}

func (s *ImageService) getImagePaginationConfig() pagination.Config[imagetypes.Summary] {
	return pagination.Config[imagetypes.Summary]{
		SearchAccessors: []pagination.SearchAccessor[imagetypes.Summary]{
			func(i imagetypes.Summary) (string, error) { return i.Repo, nil },
			func(i imagetypes.Summary) (string, error) { return i.Tag, nil },
			func(i imagetypes.Summary) (string, error) { return i.ID, nil },
			func(i imagetypes.Summary) (string, error) {
				if len(i.RepoTags) > 0 {
					return i.RepoTags[0], nil
				}
				return "", nil
			},
			func(i imagetypes.Summary) (string, error) {
				return strings.Join(i.PinnedReferences, " "), nil
			},
		},
		SortBindings: []pagination.SortBinding[imagetypes.Summary]{
			{
				Key: "repo",
				Fn: func(a, b imagetypes.Summary) int {
					if localCmp := strings.Compare(a.Repo, b.Repo); localCmp != 0 {
						return localCmp
					}
					if localCmp2 := strings.Compare(a.Tag, b.Tag); localCmp2 != 0 {
						return localCmp2
					}
					return strings.Compare(a.ID, b.ID)
				},
			},
			{
				Key: "tag",
				Fn: func(a, b imagetypes.Summary) int {
					return strings.Compare(a.Tag, b.Tag)
				},
			},
			{
				Key: "size",
				Fn: func(a, b imagetypes.Summary) int {
					if a.Size < b.Size {
						return -1
					}
					return kit.Ternary(a.Size > b.Size, 1, 0)
				},
			},
			{
				Key: "created",
				Fn: func(a, b imagetypes.Summary) int {
					if a.Created < b.Created {
						return -1
					}
					return kit.Ternary(a.Created > b.Created, 1, 0)
				},
			},
			{
				Key: "inUse",
				Fn: func(a, b imagetypes.Summary) int {
					if a.InUse == b.InUse {
						return 0
					}
					return kit.Ternary(a.InUse, -1, 1)
				},
			},
			{
				Key: "vulnerabilities",
				Fn: func(a, b imagetypes.Summary) int {
					ac, bc := vulnerabilitySortCountsInternal(a), vulnerabilitySortCountsInternal(b)
					return cmp.Or(
						cmp.Compare(ac.Total, bc.Total),
						cmp.Compare(ac.Critical, bc.Critical),
						cmp.Compare(ac.High, bc.High),
					)
				},
			},
		},
		FilterAccessors: []pagination.FilterAccessor[imagetypes.Summary]{
			{
				Key: "inUse",
				Fn: func(i imagetypes.Summary, filterValue string) bool {
					return kit.Ternary(filterValue == "true", i.InUse, filterValue != "false" || !i.InUse)
				},
			},
			{
				Key: "updates",
				Fn: func(i imagetypes.Summary, filterValue string) bool {
					switch filterValue {
					case "has_update":
						return i.UpdateInfo != nil && i.UpdateInfo.HasUpdate
					case "up_to_date":
						return i.UpdateInfo != nil && !i.UpdateInfo.HasUpdate && i.UpdateInfo.Error == ""
					case "error":
						return i.UpdateInfo != nil && i.UpdateInfo.Error != ""
					case "unknown":
						return i.UpdateInfo == nil
					default:
						value, valid := kit.ParseBool(filterValue)
						hasUpdate := i.UpdateInfo != nil && i.UpdateInfo.HasUpdate
						return !valid || hasUpdate == value
					}
				},
			},
		},
	}
}
