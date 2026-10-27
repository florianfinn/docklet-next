// Source: getarcaneapp/arcane/backend/internal/system/children/prune/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; split image-prune guard integration for issue #229.

package prune

import (
	"context"
	"errors"
	"github.com/florianfinn/docklet-next/backend/internal/image/guard"
	"github.com/florianfinn/docklet-next/types/system"
)

func (s *Service) SetImageGuard(g *guard.Service) { s.imageGuard = g }

func (s *Service) pruneImagesInternal(ctx context.Context, options system.PruneImagesOptions, result *system.PruneAllResult) error {
	report, err := s.imageGuard.Prune(ctx, options)
	if err != nil {
		return err
	}
	result.ImagesDeleted = report.ImagesDeleted
	result.ImageSkipped = report.Skipped
	result.ImageSpaceReclaimedKnown = new(false)
	result.ImageSpaceReclaimed = 0
	if len(report.Failed) > 0 {
		return errors.New("image.prune-failed")
	}
	return nil
}
