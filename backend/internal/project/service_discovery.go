// Source: getarcaneapp/arcane/backend/internal/project/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: SQLite service transaction policy; split the adopted service to keep files below 1,000 lines; local module namespace; functional cut: removed central variables and .env.global; functional cut: removed GitOps and saved Git repositories; SQLite-only local persistence.

package project

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"github.com/compose-spec/compose-go/v2/types"
	"github.com/florianfinn/docklet-next/backend/internal/common"
	"github.com/florianfinn/docklet-next/backend/internal/imageupdate"
	"github.com/florianfinn/docklet-next/backend/internal/project/children/listing"
	projectsync "github.com/florianfinn/docklet-next/backend/internal/project/children/sync"
	"github.com/florianfinn/docklet-next/backend/internal/project/children/tags"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane"
	"github.com/florianfinn/docklet-next/backend/pkg/pagination"
	"github.com/florianfinn/docklet-next/backend/pkg/projects"
	"github.com/florianfinn/docklet-next/backend/pkg/utils"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/iconcatalog"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/userctx"
	imagetypes "github.com/florianfinn/docklet-next/types/image"
	projecttypes "github.com/florianfinn/docklet-next/types/project"
	"github.com/moby/moby/api/types/container"
	"github.com/samber/mo"
	"go.getarcane.app/docker"
	kit "go.getarcane.app/kit/pkg"
	"golang.org/x/sync/errgroup"
	"gorm.io/gorm"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

// buildUpdateInfoSummary aggregates image checks into a project update summary.
func buildUpdateInfoSummary(
	imageRefs []string,
	updateInfoByRef map[string]*imagetypes.UpdateInfo,
) *projecttypes.UpdateInfo {
	imageCount := len(imageRefs)
	summary := &projecttypes.UpdateInfo{
		Status:     "unknown",
		HasUpdate:  false,
		ImageCount: imageCount,
		ImageRefs:  append([]string(nil), imageRefs...),
	}

	if imageCount == 0 {
		return summary
	}

	var latestCheckTime *time.Time

	for _, imageRef := range imageRefs {
		info := updateInfoByRef[imageRef]
		if info == nil {
			continue
		}

		summary.CheckedImageCount++
		if summary.UpdateInfoByRef == nil {
			summary.UpdateInfoByRef = make(map[string]imagetypes.UpdateInfo)
		}
		summary.UpdateInfoByRef[imageRef] = *info
		if info.HasUpdate {
			summary.HasUpdate = true
			summary.ImagesWithUpdates++
			summary.UpdatedImageRefs = append(summary.UpdatedImageRefs, imageRef)
		}
		if info.UpdateType == imageupdate.UpdateTypeNotPulled {
			summary.ImagesNotPulled++
			summary.NotPulledImageRefs = append(summary.NotPulledImageRefs, imageRef)
		}
		if strings.TrimSpace(info.Error) != "" {
			summary.ErrorCount++
			if summary.ErrorMessage == nil {
				summary.ErrorMessage = new(strings.TrimSpace(info.Error))
			}
		}
		if !info.CheckTime.IsZero() && (latestCheckTime == nil || info.CheckTime.After(*latestCheckTime)) {
			latestCheckTime = new(info.CheckTime)
		}
	}

	summary.LastCheckedAt = latestCheckTime

	switch {
	case summary.ImagesWithUpdates > 0:
		summary.Status = "has_update"
	case summary.ErrorCount > 0:
		summary.Status = "error"
	case summary.ImagesNotPulled > 0:
		summary.Status = "not_pulled"
	case summary.CheckedImageCount == imageCount:
		summary.Status = "up_to_date"
	default:
		summary.Status = "unknown"
	}

	return summary
}

func (s *ProjectService) enrichComposeDetails(ctx context.Context, proj *Project, opts projecttypes.DetailsOptions, resp *projecttypes.Details) {
	composeFile, err := s.ResolveProjectComposeFile(ctx, proj)
	if err != nil {
		if !errors.Is(err, common.ErrProjectEnvUnreadable) {
			return
		}
		// The env is unreadable, so only the compose file's identity is known:
		// name it for the UI and skip every enrichment that needs interpolation.
		projectsDirectory, dirErr := s.GetProjectsDirectory(ctx)
		if dirErr != nil {
			slog.DebugContext(ctx, "failed to resolve projects directory for compose identification", "projectId", proj.ID, "error", dirErr)
		}
		identified, detectErr := projects.DetectComposeFile(ctx, projectsDirectory, proj.Path)
		if detectErr != nil && (!errors.Is(detectErr, common.ErrProjectEnvUnreadable) || identified == "") {
			slog.WarnContext(ctx, "failed to identify project compose file", "projectId", proj.ID, "error", detectErr)
			return
		}
		if identified != "" {
			resp.ComposeFileName = filepath.Base(identified)
		}
		return
	}
	resp.ComposeFileName = filepath.Base(composeFile)
	if opts.IncludeIncludeFiles {
		resp.IncludeFiles = s.details.IncludeFiles(ctx, composeFile)
	}
	if !opts.IncludeServiceConfigs && !opts.IncludeUpdateInfo {
		return
	}

	composeProj, loadErr := s.getCachedComposeProject(ctx, proj, nil)
	if loadErr != nil {
		slog.WarnContext(ctx, "failed to load compose service configs", "path", composeFile, "error", loadErr)
		return
	}

	if composeProj == nil {
		return
	}

	// Convert map to slice
	svcList := make([]types.ServiceConfig, 0, len(composeProj.Services))
	hasBuildDirective := false
	for _, svc := range composeProj.Services {
		svcList = append(svcList, svc)
		if svc.Build != nil {
			hasBuildDirective = true
		}
	}
	resp.Services = svcList
	resp.HasBuildDirective = resp.HasBuildDirective || hasBuildDirective
}

func (s *ProjectService) ListAllProjects(ctx context.Context) ([]Project, error) {
	var items []Project
	if err := s.db.WithContext(ctx).Find(&items).Error; err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	return items, nil
}

func (s *ProjectService) ListProjects(ctx context.Context, params pagination.QueryParams) ([]projecttypes.Details, pagination.Response, error) {
	statusFilter := strings.TrimSpace(params.Filters["status"])
	updatesFilter := strings.TrimSpace(params.Filters["updates"])
	labelFilter := strings.TrimSpace(params.Filters["label"])
	snapshot := s.listing.Snapshot(ctx)
	query := s.db.WithContext(ctx).Model(&Project{})
	systemNames := listing.SystemProjectNames(snapshot.Containers)
	if len(systemNames) > 0 {
		query = query.Where("name NOT IN ? AND COALESCE(compose_project_name, '') NOT IN ?", systemNames, systemNames)
	}
	if archivedFilter := strings.TrimSpace(params.Filters["archived"]); !strings.EqualFold(archivedFilter, "all") {
		archived, _ := kit.ParseBool(archivedFilter)
		query = query.Where("is_archived = ?", archived)
	}
	query = listing.ApplyProjectTagsDBFilter(query, strings.TrimSpace(params.Filters["tags"]))
	if term := strings.TrimSpace(params.Search); term != "" {
		pattern := "%" + term + "%"
		query = query.Where(
			"name LIKE ? OR path LIKE ? OR status LIKE ? OR COALESCE(dir_name, '') LIKE ? OR "+
				"EXISTS (SELECT 1 FROM project_tags WHERE project_tags.project_id = projects.id AND LOWER(project_tags.name) LIKE ?)",
			pattern, pattern, pattern, pattern, strings.ToLower(pattern),
		)
	}

	if statusFilter == "" && updatesFilter == "" && labelFilter == "" && !strings.EqualFold(strings.TrimSpace(params.Sort), "status") {
		query = pagination.ApplyFilter(query, "status", params.Filters["status"])
		var projectsArray []Project
		paginationResp, err := pagination.PaginateAndSortDB(params, query, &projectsArray)
		if err != nil {
			return nil, pagination.Response{}, fmt.Errorf("failed to paginate projects: %w", err)
		}
		slog.DebugContext(ctx, "Retrieved projects from database", "count", len(projectsArray))

		// The paginated page gets live status from one Docker snapshot plus the
		// compose-backed presentation fields, sharing one metadata env.
		env := s.newProjectMetadataEnv(ctx, projectsArray)
		result := s.projectListRows(ctx, env.projectsDirectory, projectsArray, snapshot)
		metas := make([]projects.ArcaneComposeMetadata, len(projectsArray))
		var metaGroup errgroup.Group
		metaGroup.SetLimit(maxConcurrentComposeReads)
		for i := range projectsArray {
			metaGroup.Go(func() (workerErr error) {
				defer utils.RecoverToError(&workerErr, "project metadata worker", "projectID", projectsArray[i].ID)
				metas[i] = s.ProjectMetadata(ctx, projectsArray[i], env)
				return nil
			})
		}
		if metaErr := metaGroup.Wait(); metaErr != nil {
			slog.WarnContext(ctx, "project metadata resolution failed", "error", metaErr)
		}
		listing.ApplyPresentation(ctx, env.projectsDirectory, IconCatalogForContext(ctx), projectRecords(projectsArray), result, metas)
		pageIDs := make([]string, len(projectsArray))
		for i, proj := range projectsArray {
			pageIDs[i] = proj.ID
		}
		var tagRows []projecttypes.TagAssignment
		if tagsErr := s.db.WithContext(ctx).Model(&ProjectTag{}).Where("project_id IN ?", pageIDs).Order("project_id, name, source, color").Find(&tagRows).Error; tagsErr != nil {
			return nil, pagination.Response{}, fmt.Errorf("load project tags: %w", tagsErr)
		}
		tagsByProject := tags.Group(tagRows)
		for i := range result {
			result[i].Tags = tagsByProject[result[i].ID]
		}
		s.enrichProjectsWithUpdateInfo(ctx, projectsArray, result, true, env)
		slog.DebugContext(ctx, "Completed ListProjects request", "resultCount", len(result))
		return result, paginationResp, nil
	}

	// Derived filters and status sorting read the container snapshot, so every
	// candidate gets a lean row and the compose-backed presentation fields are
	// resolved for the page alone.
	if params.Limit != -1 {
		params.Limit = kit.Ternary(params.Limit <= 0, 20, min(params.Limit, 100))
	}
	var projectsArray []Project
	if err := query.Find(&projectsArray).Error; err != nil {
		return nil, pagination.Response{}, fmt.Errorf("failed to list projects: %w", err)
	}
	env := s.newProjectMetadataEnv(ctx, nil)
	items := s.projectListRows(ctx, env.projectsDirectory, projectsArray, snapshot)
	byID := make(map[string]Project, len(projectsArray))
	for _, proj := range projectsArray {
		byID[proj.ID] = proj
	}
	var tagRows []projecttypes.TagAssignment
	if tagsErr := s.db.WithContext(ctx).Model(&ProjectTag{}).Where("project_id IN ?", slices.Collect(maps.Keys(byID))).Order("project_id, name, source, color").Find(&tagRows).Error; tagsErr != nil {
		return nil, pagination.Response{}, fmt.Errorf("load project tags: %w", tagsErr)
	}
	tagsByProject := tags.Group(tagRows)
	for i := range items {
		items[i].Tags = tagsByProject[items[i].ID]
	}
	if updatesFilter != "" {
		s.enrichProjectsWithUpdateInfo(ctx, projectsArray, items, true, env)
		// Untracked compose projects only join an unfiltered-by-tag has_update view.
		if strings.EqualFold(updatesFilter, "has_update") && strings.TrimSpace(params.Filters["tags"]) == "" {
			if snapshot.Err != nil {
				slog.WarnContext(ctx, "failed to list compose containers for project update rows", "error", snapshot.Err)
			} else {
				// Every tracked project name is known; the candidates cover a failed lookup.
				var knownProjects []Project
				if knownErr := s.db.WithContext(ctx).Select("name", "compose_project_name").Find(&knownProjects).Error; knownErr != nil {
					slog.WarnContext(ctx, "failed to load known project names for compose update discovery", "error", knownErr)
					knownProjects = projectsArray
				}
				known := listing.KnownComposeProjectNames(projectRecords(knownProjects))
				items = append(items, s.listing.DiscoveredUpdateRows(ctx, snapshot.Containers, known, IconCatalogForContext(ctx))...)
			}
		}
	}

	result, pageIndexes := listing.Page(items, params, func(id string) bool {
		_, tracked := byID[id]
		return tracked
	})
	pageProjects := make([]Project, 0, len(pageIndexes))
	pageDetails := make([]projecttypes.Details, 0, len(pageIndexes))
	for _, i := range pageIndexes {
		pageProjects = append(pageProjects, byID[result.Items[i].ID])
		pageDetails = append(pageDetails, result.Items[i])
	}
	if updatesFilter == "" {
		s.enrichProjectsWithUpdateInfo(ctx, pageProjects, pageDetails, true, env)
	}
	metas := make([]projects.ArcaneComposeMetadata, len(pageProjects))
	var metaGroup errgroup.Group
	metaGroup.SetLimit(maxConcurrentComposeReads)
	for i := range pageProjects {
		metaGroup.Go(func() (workerErr error) {
			defer utils.RecoverToError(&workerErr, "project metadata worker", "projectID", pageProjects[i].ID)
			metas[i] = s.ProjectMetadata(ctx, pageProjects[i], env)
			return nil
		})
	}
	if metaErr := metaGroup.Wait(); metaErr != nil {
		slog.WarnContext(ctx, "project metadata resolution failed", "error", metaErr)
	}
	listing.ApplyPresentation(ctx, env.projectsDirectory, IconCatalogForContext(ctx), projectRecords(pageProjects), pageDetails, metas)
	for k, i := range pageIndexes {
		result.Items[i] = pageDetails[k]
	}
	return result.Items, pagination.BuildResponse(result.TotalCount, result.TotalAvailable, params), nil
}

// CountProjectsWithPendingUpdates counts active projects and untracked compose
// projects with a pending update, without the full list pipeline. A nil
// allContainers is fetched here.
func (s *ProjectService) CountProjectsWithPendingUpdates(ctx context.Context, allContainers []container.Summary) (int, error) {
	if s.db == nil {
		return 0, nil
	}

	if allContainers == nil {
		var err error
		allContainers, err = s.details.ComposeContainers(ctx)
		if err != nil {
			return 0, fmt.Errorf("failed to list containers for project update count: %w", err)
		}
	}

	// One full scan: archived projects are excluded from the update count but
	// still mark their compose stacks as known during discovery, so loading
	// everything here saves the known-name pass its own table scan.
	var allProjects []Project
	if err := s.db.WithContext(ctx).Find(&allProjects).Error; err != nil {
		return 0, fmt.Errorf("failed to list projects for update count: %w", err)
	}

	activeProjects := make([]Project, 0, len(allProjects))
	for _, proj := range allProjects {
		if !proj.IsArchived {
			activeProjects = append(activeProjects, proj)
		}
	}

	// Runtime container IDs keep tag-policy updates scoped to their project.
	details := make([]projecttypes.Details, len(activeProjects))
	containersByProject := listing.GroupComposeContainersByProject(allContainers)
	for i, proj := range activeProjects {
		details[i].ID = proj.ID
		for _, c := range listing.ProjectContainers(projectRecord(proj), containersByProject) {
			details[i].RuntimeServices = append(
				details[i].RuntimeServices,
				projecttypes.RuntimeService{
					Name: docker.ComposeServiceLabel(
						c.Labels,
					),
					ContainerID:     c.ID,
					Image:           c.Image,
					ImageID:         c.ImageID,
					ContainerLabels: c.Labels,
				},
			)
		}
	}
	s.enrichProjectsWithUpdateInfo(ctx, activeProjects, details, false, nil)

	count := 0
	for i := range details {
		if details[i].UpdateInfo != nil && details[i].UpdateInfo.HasUpdate {
			count++
		}
	}

	visibleContainers := make([]container.Summary, 0, len(allContainers))
	for _, c := range allContainers {
		hidden, _ := kit.ParseBool(c.Labels[libarcane.HiddenResourceLabel])
		if !hidden {
			visibleContainers = append(visibleContainers, c)
		}
	}
	// Untracked compose projects with a pending update count too, so the
	// dashboard badge matches the projects table.
	known := listing.KnownComposeProjectNames(projectRecords(allProjects))
	return count + s.listing.CountDiscoveredUpdates(ctx, visibleContainers, known, IconCatalogForContext(ctx)), nil
}

// ProjectMetadata resolves a project's icon sets and service URLs, cached until
// a merged compose or env file changes. A nil env resolves the shared inputs here.
func (s *ProjectService) ProjectMetadata(ctx context.Context, p Project, env *projectMetadataEnv) projects.ArcaneComposeMetadata {
	empty := projects.ArcaneComposeMetadata{ServiceIconSets: map[string]projects.IconSet{}}

	if env == nil {
		env = s.newProjectMetadataEnv(ctx, []Project{p})
	}
	composeFile, err := s.resolveProjectComposeFile(ctx, &p, env)
	if err != nil {
		return empty
	}

	fingerprint := fmt.Sprintf("%q|%q|%t", composeFile, env.projectsDirectory, env.autoInjectEnv)
	if s.metaCache != nil && p.ID != "" {
		if cached, ok := s.metaCache.Get(p.ID, fingerprint); ok {
			return cached
		}
	}

	meta, err := projects.ParseArcaneComposeMetadata(ctx, composeFile, env.projectsDirectory, env.autoInjectEnv)
	if err != nil {
		slog.WarnContext(ctx, "failed to parse Arcane compose metadata", "path", composeFile, "error", err)
		return empty
	}

	if s.metaCache == nil || p.ID == "" {
		return meta
	}
	if setErr := s.metaCache.Set(p.ID, fingerprint, p.Path, env.projectsDirectory, composeFile, meta.ComposeFiles, meta.EnvFiles, meta); setErr != nil {
		slog.DebugContext(ctx, "failed to cache Compose metadata", "projectId", p.ID, "error", setErr)
	}

	return meta
}

// IconCatalogForContext resolves the icon catalog of the requesting
// user. On agent-proxied calls the caller is a synthetic user whose preference
// is populated from the X-Arcane-Icon-Catalog header the manager forwards.
// Background jobs have no user attached and fall back to the default catalog.
func IconCatalogForContext(ctx context.Context) string {
	if u, ok := userctx.CurrentUserFromContext(ctx); ok && u != nil && u.Preferences.IconCatalog != nil && *u.Preferences.IconCatalog != "" {
		return *u.Preferences.IconCatalog
	}
	return iconcatalog.DefaultCatalog
}

func (s *ProjectService) refreshProjectImageRefs(ctx context.Context, proj *Project) {
	if proj == nil || proj.ID == "" {
		return
	}

	s.invalidateProjectCaches(proj.ID)
	composeProject, err := s.getCachedComposeProject(ctx, proj, nil)
	if err != nil {
		if dbErr := s.db.WithContext(ctx).
			Model(&Project{}).
			Where("id = ?", proj.ID).
			Updates(map[string]any{
				"image_refs_json":       "",
				"build_image_refs_json": nil,
			}).Error; dbErr != nil {
			slog.WarnContext(ctx, "failed to clear stale project image refs", "projectId", proj.ID, "error", dbErr)
		}
		proj.ImageRefsJSON = ""
		proj.BuildImageRefsJSON = nil
		slog.WarnContext(ctx, "failed to refresh project image refs", "projectId", proj.ID, "projectName", proj.Name, "error", fmt.Errorf("load compose project: %w", err))
		return
	}
	imageRefsJSON := projects.MarshalImageRefsJSON(projects.ImageRefsFromComposeServices(composeProject.Services))
	buildImageRefsJSON := cmp.Or(projects.MarshalImageRefsJSON(projects.BuildImageRefsFromComposeProject(composeProject)), "[]")
	if persistImageRefsErr := s.db.WithContext(ctx).
		Model(&Project{}).
		Where("id = ?", proj.ID).
		Updates(map[string]any{
			"image_refs_json":       imageRefsJSON,
			"build_image_refs_json": buildImageRefsJSON,
		}).Error; persistImageRefsErr != nil {
		slog.WarnContext(ctx, "failed to persist project image refs", "projectId", proj.ID, "error", persistImageRefsErr)
		return
	}
	proj.ImageRefsJSON = imageRefsJSON
	proj.BuildImageRefsJSON = new(buildImageRefsJSON)
}

func (s *ProjectService) HandleProjectFilesChanged(ctx context.Context, paths []string) {
	if len(paths) == 0 || s.db == nil {
		return
	}

	var projectsList []Project
	if err := s.db.WithContext(ctx).Find(&projectsList).Error; err != nil {
		slog.WarnContext(ctx, "failed to resolve changed project files", "error", fmt.Errorf("list projects for changed paths: %w", err))
		return
	}
	for i := range projectsList {
		projectPath := filepath.Clean(projectsList[i].Path)
		if !slices.ContainsFunc(paths, func(changedPath string) bool {
			changedPath = filepath.Clean(changedPath)
			return changedPath == projectPath || strings.HasPrefix(changedPath, projectPath+string(os.PathSeparator))
		}) {
			continue
		}
		s.refreshProjectImageRefs(ctx, &projectsList[i])
		if err := s.reconcileComposeTagsForProject(ctx, &projectsList[i]); err != nil {
			slog.WarnContext(ctx, "failed to reconcile Compose project tags after file change", "projectId", projectsList[i].ID, "error", err)
		}
	}
}

func (s *ProjectService) BackfillProjectImageRefs(ctx context.Context) (int, error) {
	if s.db == nil {
		return 0, nil
	}

	var projectsList []Project
	if err := s.db.WithContext(ctx).
		Where("build_image_refs_json IS NULL").
		Find(&projectsList).Error; err != nil {
		return 0, fmt.Errorf("list projects for image ref backfill: %w", err)
	}
	for i := range projectsList {
		if err := ctx.Err(); err != nil {
			return i, err
		}
		s.refreshProjectImageRefs(ctx, &projectsList[i])
	}
	return len(projectsList), nil
}

func (s *ProjectService) SyncProjectsFromFileSystem(ctx context.Context) error {
	// Serialized because the walk and the cleanup are two halves of one
	// decision: overlapping syncs let an older walk's cleanup delete a project a
	// newer walk had just upserted, because the older walk's `seen` set predates
	// it. Filesystem-watcher debounces fire these back to back.
	s.syncMu.Lock()
	defer s.syncMu.Unlock()

	followProjectSymlinks := s.settingsService.GetBoolSetting(ctx, "followProjectSymlinks", false)
	projectsDir, err := s.GetProjectsDirectory(ctx)
	if err != nil {
		slog.WarnContext(ctx, "unable to prepare projects directory", "error", err)
		return nil
	}

	discoveredProjects, discoveryErr := projects.DiscoverProjectDirectories(ctx, projectsDir, followProjectSymlinks, s.config.ProjectScanMaxDepth)
	if discoveryErr != nil {
		if os.IsNotExist(discoveryErr) {
			return nil
		}
		return fmt.Errorf("Failed to discover projects in %q: %w", projectsDir, discoveryErr) //nolint:staticcheck // Preserve the existing error message.
	}

	renameSyncState := s.updates.SyncState(ctx)
	seen := map[string]struct{}{}
	for _, discoveredProject := range discoveredProjects {
		if _, renameTarget := renameSyncState.SkipDiscoveredPaths[filepath.Clean(discoveredProject.Path)]; renameTarget {
			continue
		}
		if uerr := s.upsertProjectForDir(ctx, discoveredProject.DirName, discoveredProject.Path); uerr != nil {
			slog.WarnContext(ctx, "failed to sync project from folder", "dir", discoveredProject.Path, "error", uerr)
			continue
		}
		seen[discoveredProject.Path] = struct{}{}
	}
	maps.Copy(seen, renameSyncState.ProtectSeenPaths)

	// Decide deletions before performing any, so the mass-wipe guard can veto a
	// suspicious pass (e.g. an unmounted projects volume) as a whole.
	var all []Project
	if findErr := s.db.WithContext(ctx).Find(&all).Error; findErr != nil {
		slog.WarnContext(ctx, "error during DB cleanup of projects", "error", fmt.Errorf("list projects for cleanup failed: %w", findErr))
		return nil
	}
	candidates := 0
	var deletions, pendingDeletions []projectCleanupDecision
	for _, p := range all {
		if _, ok := seen[p.Path]; ok {
			continue
		}
		// Rows imported from Arcane's own scratch dirs (project-update preview/backup,
		// project update previews) or from filesystem snapshot/trash dirs are never real
		// projects. The decision is name-based, so it bypasses the mass-wipe guard.
		if projects.IsInternalScratchDirName(p.Name) || projects.IsInternalScratchDirName(filepath.Base(p.Path)) || (p.DirName != nil && projects.IsInternalScratchDirName(*p.DirName)) {
			deletions = append(deletions, projectCleanupDecision{project: p, reason: "removed internal Arcane scratch record (project-update temp dir)"})
			continue
		}
		if rel := listing.RelativePath(projectsDir, p.Path); rel != "" && projects.PathContainsSnapshotDirectory(rel) {
			deletions = append(deletions, projectCleanupDecision{project: p, reason: "removed project inside a filesystem snapshot/trash directory"})
			continue
		}
		candidates++
		if decision, remove := s.evaluateProjectCleanup(ctx, p, followProjectSymlinks, projectsDir, s.config.ProjectScanMaxDepth).Get(); remove {
			pendingDeletions = append(pendingDeletions, decision)
		}
	}
	// Pruning more than one project and over half the candidates usually means an
	// unmounted or mis-mapped projects directory, so that pass keeps every record.
	if len(pendingDeletions) <= 1 || len(pendingDeletions)*2 <= candidates {
		deletions = append(deletions, pendingDeletions...)
	} else {
		slog.WarnContext(ctx,
			"skipping project cleanup: this reconcile would delete most projects in a single pass, which usually "+
				"means the projects directory is empty, unmounted, or mis-mapped; preserving DB records — check the "+
				"projects volume is mounted and mapped correctly",
			"wouldDelete", len(pendingDeletions),
			"cleanupCandidates", candidates,
			"projectsDir", projectsDir,
		)
	}

	// Every removal is logged so this destructive reconcile leaves an audit trail.
	for _, decision := range deletions {
		p := decision.project
		if deleteErr := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error { return deleteProjectWithTags(tx, p.ID) }); deleteErr != nil {
			slog.ErrorContext(ctx, "failed to delete project during filesystem cleanup", "projectId", p.ID, "name", p.Name, "path", p.Path, "reason", decision.reason, "error", deleteErr)
			continue
		}
		slog.WarnContext(ctx, "deleted project during filesystem cleanup", "projectId", p.ID, "name", p.Name, "path", p.Path, "reason", decision.reason)
	}
	return nil
}

func (s *ProjectService) upsertProjectForDir(ctx context.Context, dirName, dirPath string) error {
	var existing Project
	err := s.db.WithContext(ctx).
		Where("path = ?", dirPath).
		First(&existing).Error

	cfg := s.settingsService.GetSettingsOrDefaults(ctx)
	composeMetadata := projecttypes.ComposeIdentity{ResolvedProjectName: projects.NormalizeProjectName(dirName)}
	projectsDirectory, serviceCountErr := projects.GetProjectsDirectory(ctx, cfg.ProjectsDirectory.Value)
	if serviceCountErr == nil {
		autoInjectEnv := kit.ParseOrDefault(cfg.AutoInjectEnv.Value, false, strconv.ParseBool)
		composeMetadata, serviceCountErr = projectsync.LoadComposeMetadata(ctx, dirPath, dirName, projectsDirectory, autoInjectEnv, s.projectPathMapper(ctx))
	}
	serviceCountLogLevel := kit.Ternary(errors.Is(serviceCountErr, common.ErrProjectEnvUnreadable), slog.LevelDebug, slog.LevelWarn)

	if errors.Is(err, gorm.ErrRecordNotFound) {
		// Create a minimal project entry
		reason := "Project discovered from filesystem, status pending Docker service query"
		proj := &Project{
			Name:               composeMetadata.ResolvedProjectName,
			DirName:            new(dirName),
			Path:               dirPath,
			Status:             ProjectStatusUnknown,
			StatusReason:       new(reason),
			ServiceCount:       composeMetadata.ServiceCount,
			RunningCount:       0,
			ComposeProjectName: composeMetadata.ComposeProjectName,
		}
		slog.InfoContext(ctx, "Discovered new project with unknown status",
			"project", dirName,
			"path", dirPath,
			"reason", reason)
		if serviceCountErr != nil {
			slog.Log(ctx, serviceCountLogLevel, "failed to read compose service count during project discovery", "project", dirName, "path", dirPath, "error", serviceCountErr)
		}
		if cerr := s.db.WithContext(ctx).Create(proj).Error; cerr != nil {
			return fmt.Errorf("create project for %q failed: %w", dirPath, cerr)
		}
		if composeName := composeMetadata.ResolvedProjectName; strings.TrimSpace(composeName) != "" {
			var duplicates int64
			if countErr := s.db.WithContext(ctx).Model(&Project{}).Where("name = ? AND path <> ? AND id <> ?", composeName, dirPath, proj.ID).Count(&duplicates).Error; countErr != nil {
				slog.WarnContext(ctx, "failed to check duplicate compose project names during project sync", "composeProjectName", composeName, "path", dirPath, "error", countErr)
			} else if duplicates > 0 {
				slog.WarnContext(ctx, "multiple project directories resolve to the same compose project name", "composeProjectName", composeName, "path", dirPath, "duplicates", duplicates)
			}
		}
		return s.reconcileComposeTagsForProject(ctx, proj)
	}
	if err != nil {
		return fmt.Errorf("query existing project for %q failed: %w", dirPath, err)
	}

	updates := map[string]any{}
	if existing.Path != dirPath {
		updates["path"] = dirPath
	}
	if existing.DirName == nil || *existing.DirName != dirName {
		updates["dir_name"] = dirName
	}
	if serviceCountErr == nil && existing.ServiceCount != composeMetadata.ServiceCount {
		updates["service_count"] = composeMetadata.ServiceCount
	} else if serviceCountErr != nil {
		slog.Log(ctx, serviceCountLogLevel, "failed to refresh compose service count during project sync", "projectId", existing.ID, "path", dirPath, "error", serviceCountErr)
	}
	if serviceCountErr == nil && mo.PointerToOption(existing.ComposeProjectName) != mo.PointerToOption(composeMetadata.ComposeProjectName) {
		updates["compose_project_name"] = composeMetadata.ComposeProjectName
	}
	if serviceCountErr == nil {
		if composeMetadata.ExplicitProjectName {
			if existing.Name != composeMetadata.ResolvedProjectName {
				updates["name"] = composeMetadata.ResolvedProjectName
			}
		} else if normalizedExistingName := projects.NormalizeProjectName(existing.Name); normalizedExistingName != existing.Name {
			updates["name"] = normalizedExistingName
		}
	}
	if len(updates) == 0 {
		return s.reconcileComposeTagsForProject(ctx, &existing)
	}

	updates["updated_at"] = time.Now()
	if uerr := s.db.WithContext(ctx).
		Model(&Project{}).
		Where("id = ?", existing.ID).
		Updates(updates).Error; uerr != nil {
		return fmt.Errorf("update project %s failed: %w", existing.ID, uerr)
	}
	if composeName := composeMetadata.ResolvedProjectName; serviceCountErr == nil && strings.TrimSpace(composeName) != "" {
		var duplicates int64
		if countErr := s.db.WithContext(ctx).Model(&Project{}).Where("name = ? AND path <> ? AND id <> ?", composeName, dirPath, existing.ID).Count(&duplicates).Error; countErr != nil {
			slog.WarnContext(ctx, "failed to check duplicate compose project names during project sync", "composeProjectName", composeName, "path", dirPath, "error", countErr)
		} else if duplicates > 0 {
			slog.WarnContext(ctx, "multiple project directories resolve to the same compose project name", "composeProjectName", composeName, "path", dirPath, "duplicates", duplicates)
		}
	}
	return s.reconcileComposeTagsForProject(ctx, &existing)
}

// evaluateProjectCleanup decides whether a project missing from the current
// filesystem pass should be pruned. It only reads; the caller defers deletion
// so the mass-wipe guard can veto the whole pass.
func (s *ProjectService) evaluateProjectCleanup(ctx context.Context, p Project, followProjectSymlinks bool, projectsDir string, maxDepth int) mo.Option[projectCleanupDecision] {
	// Projects still on disk but beyond a lowered scan depth are no longer
	// discovered; root-level and outside paths fall through to the disk checks.
	if rel := listing.RelativePath(projectsDir, p.Path); maxDepth > 0 && rel != "" && strings.Count(rel, "/") >= maxDepth {
		return mo.Some(projectCleanupDecision{project: p, reason: "removed project: directory is beyond the configured scan depth"})
	}

	validDir, err := projects.IsProjectDirectoryPath(p.Path, followProjectSymlinks)
	switch {
	case os.IsNotExist(err):
		return mo.Some(projectCleanupDecision{project: p, reason: "removed project: directory no longer exists"})
	case err != nil:
		slog.WarnContext(ctx, "stat error during cleanup; keeping DB record", "path", p.Path, "error", err)
		return mo.None[projectCleanupDecision]()
	case !validDir:
		return mo.Some(projectCleanupDecision{project: p, reason: "removed project: path is no longer a valid project directory"})
	}

	// Only a directory with no compose file is pruned. An ambiguous match, an
	// unreadable directory, or a parse error keeps a possibly deployable project.
	_, err = s.ResolveProjectComposeFile(ctx, &p)
	switch {
	case err == nil:
		return mo.None[projectCleanupDecision]()
	case !errors.Is(err, common.ErrComposeFileNotFound):
		slog.WarnContext(ctx, "project directory present but compose file unresolved during cleanup; keeping DB record", "projectId", p.ID, "path", p.Path, "error", err)
		return mo.None[projectCleanupDecision]()
	}
	return mo.Some(projectCleanupDecision{project: p, reason: "removed orphaned project: directory present but contains no compose file"})
}

func (s *ProjectService) reconcileComposeTagsForProject(ctx context.Context, projectModel *Project) error {
	if projectModel == nil {
		return nil
	}
	composeFile, err := s.ResolveProjectComposeFile(ctx, projectModel)
	if err != nil {
		return fmt.Errorf("resolve Compose file for tag reconciliation: %w", err)
	}
	projectsDirectory, err := s.GetProjectsDirectory(ctx)
	if err != nil {
		return err
	}
	meta, err := projects.ParseArcaneComposeMetadata(ctx, composeFile, projectsDirectory, s.settingsService.GetBoolSetting(ctx, "autoInjectEnv", false))
	if err != nil {
		return err
	}
	if !meta.ProjectTagsAuthoritative {
		return nil
	}
	return s.reconcileComposeProjectTags(ctx, projectModel.ID, meta.ProjectTags)
}
