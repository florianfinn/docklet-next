// Source: docklet-next/issues/438
// Revision: 57e835f7cce5778fd3d164298e5b488e11abb71d
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original verified one-service manifest recheck before lifecycle effects.

package project

import (
	"context"
	"github.com/florianfinn/docklet-next/backend/internal/imageupdate/children/identity"
	dto "github.com/florianfinn/docklet-next/types/updater"
)

func (s *ProjectService) verifyExpectedCandidate(ctx context.Context, intent *dto.LifecycleIntent) error {
	if intent.Request.ExpectedManifest == "" {
		return nil
	}
	if !intent.Request.VerifiedService() || len(intent.Selections) != 1 || intent.Selections[0].Service != intent.Request.Service {
		return lifecycleFailure("update.invalid-target")
	}
	selected := intent.Selections[0]
	if selected.Digest != intent.Request.ExpectedManifest {
		return lifecycleFailure("update.candidate-changed")
	}
	platform, err := identity.ParsePlatform(selected.Platform)
	if err != nil || s.containerRegistryService == nil {
		return lifecycleFailure("update.check-unavailable")
	}
	observed, err := s.containerRegistryService.PlatformImage(ctx, selected.Reference, platform.Registry())
	if err != nil {
		return err
	}
	if observed.Digest != intent.Request.ExpectedManifest {
		return lifecycleFailure("update.candidate-changed")
	}
	return nil
}
