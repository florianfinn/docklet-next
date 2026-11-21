// Source: docklet-next/docs/design/stack-env-and-peers.md
// Revision: be6efa6e1470df78a90c7d485bc781be1206737a
// Retrieved: 2026-11-21
// License: Apache-2.0
// Deviations: Request-scoped fresh recorded Compose labels; M34 #548.

package project

import (
	"context"
	"time"

	"github.com/florianfinn/docklet-next/backend/pkg/projects"
	"github.com/moby/moby/client"
)

func (s *ProjectService) composeInputContext(ctx context.Context) context.Context {
	if projects.HasComposeContainerLabels(ctx) || s.dockerService == nil {
		return ctx
	}
	return projects.WithComposeContainerLabels(ctx, func(ctx context.Context) ([]map[string]string, error) {
		ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		c, err := s.dockerService.GetClient(ctx)
		if err != nil {
			return nil, err
		}
		listed, err := c.ContainerList(ctx, client.ContainerListOptions{All: true, Filters: make(client.Filters).Add("label", "com.docker.compose.project.working_dir")})
		if err != nil {
			return nil, err
		}
		labels := make([]map[string]string, 0, len(listed.Items))
		for _, item := range listed.Items {
			labels = append(labels, item.Labels)
		}
		return labels, nil
	})
}

func (s *ProjectService) resolvedProjectInputContext(ctx context.Context, dir string) (context.Context, *projects.ResolvedInputs, error) {
	ctx = s.composeInputContext(ctx)
	root, err := s.GetProjectsDirectory(ctx)
	if err != nil {
		root = ""
	}
	input, err := projects.ResolveComposeInputs(ctx, root, dir, s.settingsService.GetBoolSetting(ctx, "autoInjectEnv", false))
	if err != nil {
		return ctx, input, err
	}
	return projects.WithResolvedInputs(ctx, input), input, nil
}

func projectsDirectoryForProjection(s *ProjectService, ctx context.Context) string {
	root, _ := s.GetProjectsDirectory(ctx)
	return root
}
