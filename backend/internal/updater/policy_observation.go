// Source: docklet-next/docs/design/update-policies.md
// Revision: cdfe93c3dd2557cb21586ae542ebc05cae9bc0af
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Configured-service policy checks retain comparable platform candidate digests.

package updater

import (
	"context"
	"github.com/florianfinn/docklet-next/backend/internal/imageupdate"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
)

func (s *UpdaterService) enrichPolicyObservation(ctx context.Context, ref, target, platform, project string, record *imageupdate.ImageUpdateRecord) {
	fail := func() {
		record.HasUpdate = false
		record.Platform = nil
		record.LastError = new(imageDTO.Failure("update.check-unavailable", 503).Error())
	}
	if s.deps.ImageUpdates == nil {
		fail()
		return
	}
	observed, err := s.deps.ImageUpdates.NativeServiceObservation(ctx, ref, target, platform, project, record.ServiceName)
	if err != nil {
		fail()
		return
	}
	record.SetNativeObservation(observed)
	record.HasUpdate = record.UpdateInfo().HasUpdate
}
