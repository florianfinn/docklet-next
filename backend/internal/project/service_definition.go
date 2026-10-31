// Source: docklet-next/issues/317
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-30
// License: Apache-2.0
// Deviations: Original bounded dependency-aware definition services for tracked stacks without containers.

package project

import (
	"context"
	projecttypes "github.com/florianfinn/docklet-next/types/project"
	"golang.org/x/sync/errgroup"
)

func (s *ProjectService) fillDefinitionServices(ctx context.Context, records []Project, rows []projecttypes.Details, env *projectMetadataEnv) {
	var group errgroup.Group
	group.SetLimit(4)
	for i := range records {
		if records[i].Path == "" || len(rows[i].RuntimeServices) != 0 || rows[i].Status == projecttypes.StatusUnknown {
			continue
		}
		group.Go(func() error {
			model, err := s.getCachedComposeProject(ctx, &records[i], env)
			if err != nil {
				rows[i].DefinitionHint = "definition-invalid"
				return nil
			}
			services := make([]projecttypes.RuntimeService, 0, len(model.Services))
			for _, name := range model.ServiceNames() {
				service := model.Services[name]
				services = append(services, projecttypes.RuntimeService{Name: name, Image: service.Image, Status: projecttypes.ServiceNotCreated})
			}
			rows[i].RuntimeServices = services
			rows[i].ServiceCount = len(services)
			return nil
		})
	}
	_ = group.Wait()
}
