// Source: getarcaneapp/kit/updater/compose_groups.go
// Revision: 119f68956cee6d5aba8a5463561aaf9be9943b65
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: None

package updater

import (
	"context"
	"fmt"

	"go.getarcane.app/updater/internal/compose"
	"go.getarcane.app/updater/internal/deps"
	"go.getarcane.app/updater/types"
)

type composeGroup struct {
	images      map[string]types.ServiceImageChange
	tagChanges  bool
	err         error
	projectName string
	services    []string
	seen        map[string]struct{}
}

func (s *Service) buildComposeGroups(ctx context.Context, sorted []deps.ContainerWithDeps, plansByName map[string]*restartPlan) map[string]composeGroup {
	groups := map[string]composeGroup{}
	if s.config.ProjectUpdater == nil {
		return groups
	}

	for _, candidate := range sorted {
		plan := plansByName[candidate.Name]
		if plan == nil || plan.newRef == "" || plan.inspect == nil || plan.inspect.Config == nil {
			continue
		}
		labels := plan.inspect.Config.Labels
		if s.config.LabelPolicy.IsSelfUpdateTarget(labels) {
			continue
		}
		projectName := compose.ProjectLabel(labels)
		serviceName := compose.ServiceLabel(labels)
		if projectName == "" || serviceName == "" {
			continue
		}
		project, err := s.config.ProjectUpdater.ProjectByComposeName(ctx, projectName)
		if err != nil || project.ID == "" {
			continue
		}
		group := groups[project.ID]
		group.projectName = projectName
		if group.seen == nil {
			group.seen = map[string]struct{}{}
		}
		if group.images == nil {
			group.images = map[string]types.ServiceImageChange{}
		}
		change := types.ServiceImageChange{ExpectedRef: plan.inspect.Config.Image, TargetRef: plan.newRef}
		if previous, ok := group.images[serviceName]; ok && previous != change {
			group.err = fmt.Errorf("conflicting targets for compose service %s/%s", projectName, serviceName)
		}
		group.images[serviceName] = change
		group.tagChanges = group.tagChanges || isComposeTagChangeInternal(plan)
		if _, seen := group.seen[serviceName]; !seen {
			group.services = append(group.services, serviceName)
			group.seen[serviceName] = struct{}{}
		}
		groups[project.ID] = group
	}
	return groups
}

func composeProjectID(projectName string, groups map[string]composeGroup) string {
	if projectName == "" {
		return ""
	}
	for projectID, group := range groups {
		if group.projectName == projectName {
			return projectID
		}
	}
	return ""
}
