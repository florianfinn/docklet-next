// Source: docklet-next/issues/316
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-30
// License: Apache-2.0
// Deviations: Original explicit-service Compose stop with success-only status projection.

package project

import (
	"context"
	"fmt"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/event"
	usertypes "github.com/florianfinn/docklet-next/types/user"
)

func (s *ProjectService) StopProject(ctx context.Context, projectID string, actor usertypes.Actor) error {
	stored, err := s.getMutableProject(ctx, projectID)
	if err != nil {
		return err
	}
	model, _, err := s.loadComposeProjectForProject(ctx, stored, nil)
	if err != nil {
		return fmt.Errorf("failed to load compose project: %w", err)
	}
	defer s.eventService.BeginComposeSuppressionWindow(model.Name)()
	if err := composeStopProjectServices(ctx, model, model.ServiceNames()); err != nil {
		return fmt.Errorf("failed to stop project: %w", err)
	}
	s.logProjectEvent(ctx, event.EventTypeProjectStop, projectID, stored.Name, actor, database.JSON{"action": "stop", "projectID": projectID, "projectName": stored.Name}, "could not log project stop action")
	return s.updateProjectStatusAndCounts(ctx, projectID, ProjectStatusStopped)
}
