// Source: getarcaneapp/arcane/backend/internal/container/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; split lifecycle, editing, inventory and stream concerns; exclude Docklet agent inventory; M14 container and host operation contracts.

package container

import (
	"cmp"
	"context"
	"log/slog"
	"strings"

	containertypes "github.com/florianfinn/docklet-next/types/container"
	imagetypes "github.com/florianfinn/docklet-next/types/image"
	"github.com/moby/moby/api/types/container"
	"go.getarcane.app/docker"
	"go.getarcane.app/kit/pkg"
	"go.getarcane.app/sys/cgroup"
	"go.getarcane.app/updater"
	"go.getarcane.app/updater/labels"
	"go.getarcane.app/updater/pkg/utils/tagpolicy"

	"github.com/florianfinn/docklet-next/backend/internal/container/children/origin"
	"github.com/florianfinn/docklet-next/backend/internal/container/children/stats"
	"github.com/florianfinn/docklet-next/backend/internal/project"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane"
	"github.com/florianfinn/docklet-next/backend/pkg/pagination"
	"github.com/florianfinn/docklet-next/backend/pkg/projects"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/iconcatalog"
)

func (s *ContainerService) StreamStats(ctx context.Context, containerID string, statsChan chan<- any) error {
	return s.stats.Stream(ctx, containerID, statsChan)
}

func (s *ContainerService) ListContainersPaginated(
	ctx context.Context,
	params pagination.QueryParams,
	includeAll bool,
	includeInternal bool,
	includeHidden bool,
	groupBy string,
) (ContainerListResult, error) {
	dockerContainers, err := s.dockerService.ListContainers(ctx)
	if err != nil {
		return ContainerListResult{}, err
	}
	if !includeAll {
		running := make([]container.Summary, 0, len(dockerContainers))
		for _, dc := range dockerContainers {
			if dc.State == container.StateRunning || dc.State == container.StatePaused || dc.State == container.StateRestarting {
				running = append(running, dc)
			}
		}
		dockerContainers = running
	}

	dockerContainers = FilterExcludedContainers(dockerContainers, includeInternal, includeHidden)
	updateInfoMap := s.lookupContainerUpdateInfoInternal(ctx, dockerContainers)
	currentContainerID, currentContainerErr := cgroup.CurrentContainerID()
	items := s.BuildSummaries(ctx, dockerContainers, updateInfoMap, currentContainerID, currentContainerErr)

	config := s.buildContainerPaginationConfig()
	counts := s.CalculateStatusCounts(items)

	if groupBy == containerGroupByProject {
		ungroupedParams := params
		ungroupedParams.Start = 0
		ungroupedParams.Limit = -1

		result := config.SearchOrderAndPaginate(items, ungroupedParams)
		groups, paginationResp := paginateContainerProjectGroupsInternal(result, params)

		// Icons must be resolved before flattening: groups hold value copies,
		// so the flattened items only carry icons applied to the groups first.
		metadataByProject := map[string]projects.ArcaneComposeMetadata{}
		for gi := range groups {
			s.ApplySummaryIcons(ctx, groups[gi].Items, metadataByProject)
		}

		return ContainerListResult{
			Items:      flattenContainerProjectGroupsInternal(groups),
			Groups:     groups,
			Pagination: paginationResp,
			Counts:     counts,
		}, nil
	}

	if isContainerResourceSortRequestInternal(params, groupBy) {
		return s.listContainersByResourceInternal(ctx, config, items, counts, params)
	}

	result := config.SearchOrderAndPaginate(items, params)
	s.ApplySummaryIcons(ctx, result.Items, nil)
	paginationResp := pagination.BuildResponse(result.TotalCount, result.TotalAvailable, params)

	return ContainerListResult{
		Items:      result.Items,
		Pagination: paginationResp,
		Counts:     counts,
	}, nil
}

// listContainersByResourceInternal applies search and filters, collects stats
// for the matching containers, then sorts and paginates.
func (s *ContainerService) listContainersByResourceInternal(
	ctx context.Context,
	config pagination.Config[containertypes.Summary],
	items []containertypes.Summary,
	counts containertypes.StatusCounts,
	params pagination.QueryParams,
) (ContainerListResult, error) {
	filtered := make([]containertypes.Summary, 0, len(items))
	for _, item := range items {
		if config.MatchesSearchAndFilters(item, params) {
			filtered = append(filtered, item)
		}
	}

	samples, err := s.stats.Collect(ctx, filtered)
	if err != nil {
		return ContainerListResult{}, err
	}
	for i := range filtered {
		filtered[i].ResourceSample = samples[filtered[i].ID]
	}

	page := config.OrderAndPaginate(filtered, params)
	s.ApplySummaryIcons(ctx, page, nil)

	return ContainerListResult{
		Items:      page,
		Pagination: pagination.BuildResponse(int64(len(filtered)), int64(len(items)), params),
		Counts:     counts,
	}, nil
}

func paginateContainerProjectGroupsInternal(
	result pagination.FilterResult[containertypes.Summary],
	params pagination.QueryParams,
) ([]containertypes.SummaryGroup, pagination.Response) {
	groups := groupContainersByProjectInternal(result.Items)
	totalCount := len(result.Items)

	if params.Limit <= 0 {
		return groups, pagination.BuildResponse(int64(totalCount), result.TotalAvailable, params)
	}

	requestedPage := max((params.Start/params.Limit)+1, 1)

	// Pages are contiguous runs of whole groups: a group is never split, so the
	// group that crosses the limit finishes its page. One walk over group sizes
	// finds the requested page's group range without materializing other pages.
	totalPages := 0
	pageStart, currentCount := 0, 0
	selStart, selEnd := 0, 0
	lastStart, lastEnd := 0, 0

	closePage := func(end int) {
		totalPages++
		if totalPages == requestedPage {
			selStart, selEnd = pageStart, end
		}
		lastStart, lastEnd = pageStart, end
		pageStart, currentCount = end, 0
	}

	for i := range groups {
		currentCount += len(groups[i].Items)
		if currentCount >= params.Limit {
			closePage(i + 1)
		}
	}
	if pageStart < len(groups) || totalPages == 0 {
		closePage(len(groups))
	}

	if requestedPage > totalPages {
		requestedPage = totalPages
		selStart, selEnd = lastStart, lastEnd
	}

	return groups[selStart:selEnd], pagination.Response{
		TotalPages:      int64(totalPages),
		TotalItems:      int64(totalCount),
		CurrentPage:     requestedPage,
		ItemsPerPage:    params.Limit,
		GrandTotalItems: result.TotalAvailable,
	}
}

func groupContainersByProjectInternal(items []containertypes.Summary) []containertypes.SummaryGroup {
	groups := make([]containertypes.SummaryGroup, 0)
	groupIndexes := make(map[string]int)

	for _, item := range items {
		groupName := getContainerProjectNameInternal(item)
		groupIndex, exists := groupIndexes[groupName]
		if !exists {
			groupIndex = len(groups)
			groupIndexes[groupName] = groupIndex
			groups = append(groups, containertypes.SummaryGroup{GroupName: groupName})
		}

		groups[groupIndex].Items = append(groups[groupIndex].Items, item)
	}

	return groups
}

func flattenContainerProjectGroupsInternal(groups []containertypes.SummaryGroup) []containertypes.Summary {
	flattened := make([]containertypes.Summary, 0)
	for _, group := range groups {
		flattened = append(flattened, group.Items...)
	}

	return flattened
}

func getContainerProjectNameInternal(localContainer containertypes.Summary) string {
	if localContainer.Labels == nil {
		return containerNoProjectGroup
	}

	projectName := docker.ComposeProjectLabel(localContainer.Labels)
	return kit.Ternary(projectName == "", containerNoProjectGroup, projectName)
}

// FilterExcludedContainers removes internal and hidden containers unless their corresponding inclusion flags are enabled.
func FilterExcludedContainers(containers []container.Summary, includeInternal, includeHidden bool) []container.Summary {

	filtered := make([]container.Summary, 0, len(containers))
	for _, dc := range containers {
		if dc.Labels["io.docklet.system"] == "agent" {
			continue
		}
		internal, _ := kit.ParseBool(dc.Labels[libarcane.InternalResourceLabel])
		hidden, _ := kit.ParseBool(dc.Labels[libarcane.HiddenResourceLabel])
		if (!includeInternal && internal) || (!includeHidden && hidden) {
			continue
		}
		filtered = append(filtered, dc)
	}
	return filtered
}

// lookupContainerUpdateInfoInternal resolves stored update checks keyed by
// container ID; lookup failures degrade to missing status rather than a failed list.
func (s *ContainerService) lookupContainerUpdateInfoInternal(ctx context.Context, containers []container.Summary) map[string]*imagetypes.UpdateInfo {
	if s.imageService == nil || len(containers) == 0 {
		return map[string]*imagetypes.UpdateInfo{}
	}
	updates, err := s.imageService.GetUpdateInfoByContainers(ctx, containers)
	if err != nil {
		slog.WarnContext(ctx, "Failed to fetch container update info", "error", err)
		return map[string]*imagetypes.UpdateInfo{}
	}
	return updates
}

func (
	s *ContainerService,
) BuildSummaries(
	ctx context.Context,
	containers []container.Summary,
	updateInfoMap map[string]*imagetypes.UpdateInfo,
	currentContainerID string,
	currentContainerErr error,
) []containertypes.Summary {
	items := make([]containertypes.Summary, 0, len(containers))
	var excluded map[string]bool
	if s.settingsService != nil {
		excluded = docker.ExcludedContainerNameSet(s.settingsService.GetStringSetting(ctx, "autoUpdateExcludedContainers", ""))
	}
	for _, dc := range containers {
		summary := containertypes.NewSummary(dc)
		summary.OriginFields = origin.Detect(dc.Labels)
		summary.Ports = mergePortBindings(summary.Ports)
		policy := updater.DefaultLabelPolicy().TagPolicy(dc.Labels)
		resolved, policyErr := tagpolicy.Resolve(dc.Image, policy)
		summary.UpdateStrategy = resolved.Strategy
		if policyErr != nil {
			summary.UpdateStrategy = cmp.Or(policy.Strategy, "auto")
		}
		summary.UpdateInfo = updateInfoMap[dc.ID]
		summary.RedeployDisabled = labels.ShouldDisableArcaneServerRedeploy(summary.Labels, summary.ID, currentContainerID, currentContainerErr)
		summary.AutoUpdateEnabled = !labels.IsUpdateDisabled(dc.Labels) && !docker.ContainerNameExcluded(dc.Names, excluded)
		summary.Hidden, _ = kit.ParseBool(dc.Labels[libarcane.HiddenResourceLabel])
		items = append(items, summary)
	}
	return items
}

// ApplySummaryIcons resolves icons for a page of summaries.
// Icon resolution is deferred until after pagination so the cost is bounded by
// page size rather than the full container list.
func (s *ContainerService) ApplySummaryIcons(ctx context.Context, summaries []containertypes.Summary, metadataByProject map[string]projects.ArcaneComposeMetadata) {
	if metadataByProject == nil {
		metadataByProject = map[string]projects.ArcaneComposeMetadata{}
	}
	for i := range summaries {
		s.applyContainerSummaryIconInternal(ctx, &summaries[i], metadataByProject)
	}
}

func (s *ContainerService) applyContainerSummaryIconInternal(ctx context.Context, summary *containertypes.Summary, metadataByProject map[string]projects.ArcaneComposeMetadata) {
	if summary == nil {
		return
	}
	resolvedIcon := s.resolveContainerIconInternal(ctx, summary.Labels, metadataByProject)
	summary.IconLightURL = resolvedIcon.IconLightURL
	summary.IconDarkURL = resolvedIcon.IconDarkURL
}

func (s *ContainerService) applyContainerDetailsIconInternal(ctx context.Context, details *containertypes.Details) {
	if details == nil {
		return
	}
	resolvedIcon := s.resolveContainerIconInternal(ctx, details.Labels, nil)
	details.IconLightURL = resolvedIcon.IconLightURL
	details.IconDarkURL = resolvedIcon.IconDarkURL
}

func (s *ContainerService) resolveContainerIconInternal(ctx context.Context, localLabels map[string]string, metadataByProject map[string]projects.ArcaneComposeMetadata) iconcatalog.ResolvedIconSet {
	explicitIcon := projects.FindArcaneIconSet(localLabels)
	if !explicitIcon.IsEmpty() {
		return iconcatalog.Resolve(project.IconCatalogForContext(ctx), explicitIcon)
	}

	projectName := docker.ComposeProjectLabel(localLabels)
	if projectName == "" || s == nil || s.projectService == nil {
		return iconcatalog.Resolve(project.IconCatalogForContext(ctx), explicitIcon)
	}

	meta := s.getCachedProjectIconMetadataInternal(ctx, projectName, metadataByProject)

	serviceName := docker.ComposeServiceLabel(localLabels)
	return iconcatalog.Resolve(project.IconCatalogForContext(ctx), cmp.Or(
		explicitIcon,
		meta.ServiceIconSets[serviceName],
		meta.ProjectIcon,
	))
}

func (s *ContainerService) getCachedProjectIconMetadataInternal(ctx context.Context, projectName string, metadataByProject map[string]projects.ArcaneComposeMetadata) projects.ArcaneComposeMetadata {
	if metadataByProject != nil {
		if meta, ok := metadataByProject[projectName]; ok {
			return meta
		}
	}

	if s.iconMetaCache != nil {
		if meta, ok, _ := s.iconMetaCache.Get(projectName); ok {
			if metadataByProject != nil {
				metadataByProject[projectName] = meta
			}
			return meta
		}
	}

	meta := projects.ArcaneComposeMetadata{ServiceIconSets: map[string]projects.IconSet{}}
	proj, err := s.projectService.GetProjectByComposeName(ctx, projectName)
	if err == nil && proj != nil {
		meta = s.projectService.ProjectMetadata(ctx, *proj, nil)
	}
	if s.iconMetaCache != nil {
		s.iconMetaCache.Set(projectName, meta)
	}
	if metadataByProject != nil {
		metadataByProject[projectName] = meta
	}
	return meta
}

func (s *ContainerService) buildContainerPaginationConfig() pagination.Config[containertypes.Summary] {
	return pagination.Config[containertypes.Summary]{
		SearchAccessors: []pagination.SearchAccessor[containertypes.Summary]{
			func(c containertypes.Summary) (string, error) {
				if len(c.Names) > 0 {
					return c.Names[0], nil
				}
				return "", nil
			},
			func(c containertypes.Summary) (string, error) { return c.Image, nil },
			func(c containertypes.Summary) (string, error) { return c.State, nil },
			func(c containertypes.Summary) (string, error) { return c.Status, nil },
		},
		SortBindings:    s.buildContainerSortBindings(),
		FilterAccessors: s.buildContainerFilterAccessors(),
	}
}

func (s *ContainerService) buildContainerSortBindings() []pagination.SortBinding[containertypes.Summary] {
	return []pagination.SortBinding[containertypes.Summary]{
		{
			Key: "name",
			Fn: func(a, b containertypes.Summary) int {
				nameA, nameB := "", ""
				if len(a.Names) > 0 {
					nameA = a.Names[0]
				}
				if len(b.Names) > 0 {
					nameB = b.Names[0]
				}
				return strings.Compare(nameA, nameB)
			},
		},
		{
			Key: "image",
			Fn: func(a, b containertypes.Summary) int {
				return strings.Compare(a.Image, b.Image)
			},
		},
		{
			Key: "state",
			Fn: func(a, b containertypes.Summary) int {
				return strings.Compare(a.State, b.State)
			},
		},
		{
			Key: "status",
			Fn: func(a, b containertypes.Summary) int {
				return strings.Compare(a.Status, b.Status)
			},
		},
		{
			Key:    "ports",
			Fn:     compareContainerPortsForSortInternal,
			DescFn: compareContainerPortsForSortDescInternal,
		},
		{
			Key: "created",
			Fn: func(a, b containertypes.Summary) int {
				if a.Created < b.Created {
					return -1
				}
				return kit.Ternary(a.Created > b.Created, 1, 0)
			},
		},
		{
			Key:    containertypes.SortCPUUsage,
			Fn:     stats.ContainerResourceSampleSort(containertypes.SortCPUUsage, false),
			DescFn: stats.ContainerResourceSampleSort(containertypes.SortCPUUsage, true),
		},
		{
			Key:    containertypes.SortMemoryUsage,
			Fn:     stats.ContainerResourceSampleSort(containertypes.SortMemoryUsage, false),
			DescFn: stats.ContainerResourceSampleSort(containertypes.SortMemoryUsage, true),
		},
	}
}

// isContainerResourceSortRequestInternal limits resource sorting to the
// ungrouped list: project-grouped mode keeps the existing group pagination.
func isContainerResourceSortRequestInternal(params pagination.QueryParams, groupBy string) bool {
	return groupBy != containerGroupByProject && containertypes.IsResourceSort(params.Sort)
}

func compareContainerPortsForSortInternal(a, b containertypes.Summary) int {
	hasPortsA, portA := lowestContainerPortSortValueInternal(a.Ports)
	hasPortsB, portB := lowestContainerPortSortValueInternal(b.Ports)

	switch {
	case !hasPortsA && !hasPortsB:
		return stats.CompareContainerNamesForSort(a, b)
	case !hasPortsA:
		return 1
	case !hasPortsB:
		return -1
	case portA < portB:
		return -1
	case portA > portB:
		return 1
	default:
		return stats.CompareContainerNamesForSort(a, b)
	}
}

func compareContainerPortsForSortDescInternal(a, b containertypes.Summary) int {
	hasPortsA, portA := lowestContainerPortSortValueInternal(a.Ports)
	hasPortsB, portB := lowestContainerPortSortValueInternal(b.Ports)

	switch {
	case !hasPortsA && !hasPortsB:
		return stats.CompareContainerNamesForSort(a, b)
	case !hasPortsA:
		return 1
	case !hasPortsB:
		return -1
	case portA > portB:
		return -1
	case portA < portB:
		return 1
	default:
		return stats.CompareContainerNamesForSort(a, b)
	}
}

func lowestContainerPortSortValueInternal(ports []containertypes.Port) (bool, int) {
	if len(ports) == 0 {
		return false, 0
	}

	lowestPublished := 0
	lowestPrivate := 0
	for _, port := range ports {
		if port.PublicPort > 0 && (lowestPublished == 0 || port.PublicPort < lowestPublished) {
			lowestPublished = port.PublicPort
		}
		if port.PrivatePort > 0 && (lowestPrivate == 0 || port.PrivatePort < lowestPrivate) {
			lowestPrivate = port.PrivatePort
		}
	}

	switch {
	case lowestPublished > 0:
		return true, lowestPublished
	case lowestPrivate > 0:
		return true, lowestPrivate
	default:
		return false, 0
	}
}

func (s *ContainerService) buildContainerFilterAccessors() []pagination.FilterAccessor[containertypes.Summary] {
	return []pagination.FilterAccessor[containertypes.Summary]{
		{
			Key: "updates",
			Fn: func(c containertypes.Summary, filterValue string) bool {
				switch filterValue {
				case "has_update":
					return c.UpdateInfo != nil && c.UpdateInfo.HasUpdate
				case "up_to_date":
					return c.UpdateInfo != nil && !c.UpdateInfo.HasUpdate && c.UpdateInfo.Error == ""
				case "error":
					return c.UpdateInfo != nil && c.UpdateInfo.Error != ""
				case "unknown":
					return c.UpdateInfo == nil
				default:
					return true
				}
			},
		},
		{
			Key: "standalone",
			Fn: func(c containertypes.Summary, filterValue string) bool {
				isStandalone := docker.ComposeProjectLabel(c.Labels) == ""
				value, valid := kit.ParseBool(filterValue)
				return !valid || isStandalone == value
			},
		},
		{
			Key:     "label",
			NoSplit: true,
			Fn: func(c containertypes.Summary, filterValue string) bool {
				key, value, hasValue := strings.Cut(filterValue, "=")
				key = strings.TrimSpace(key)
				if key == "" {
					return true
				}
				actual, ok := c.Labels[key]
				return ok && (!hasValue || actual == value)
			},
		},
	}
}

func (s *ContainerService) CalculateStatusCounts(items []containertypes.Summary) containertypes.StatusCounts {
	counts := containertypes.StatusCounts{
		TotalContainers: len(items),
	}
	for _, c := range items {
		if c.State == "running" {
			counts.RunningContainers++
		} else {
			counts.StoppedContainers++
		}
	}
	return counts
}
