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
	v1 "github.com/google/go-containerregistry/pkg/v1"
)

func (s *UpdaterService) enrichPolicyObservation(ctx context.Context, ref, target string, record *imageupdate.ImageUpdateRecord) {
	if s.deps.ImageUpdates == nil {
		return
	}
	fail := func() {
		record.HasUpdate = false
		record.LastError = new(imageDTO.Failure("update.check-unavailable", 503).Error())
	}
	observed, err := s.deps.ImageUpdates.ReadUpdateInfo(ctx, ref)
	if err != nil || observed == nil {
		fail()
		return
	}
	if observed.ImageUpdate != nil {
		observed = observed.ImageUpdate
	}
	if observed.Platform == "" || observed.CurrentDigest == "" || observed.State == "error" || observed.State == "unknown" {
		fail()
		return
	}
	platform, err := v1.ParsePlatform(observed.Platform)
	if err != nil {
		fail()
		return
	}
	if target == "" {
		target = ref
	}
	candidate, err := s.deps.RegistryDigestResolver.PlatformImage(ctx, target, *platform)
	if err != nil {
		fail()
		return
	}
	record.Platform = new(observed.Platform)
	record.CurrentDigest = new(observed.CurrentDigest)
	record.LatestDigest = new(candidate.Digest)
	record.AuthMethod = new(candidate.AuthState)
	info := record.UpdateInfo()
	record.HasUpdate = info.HasUpdate
}
