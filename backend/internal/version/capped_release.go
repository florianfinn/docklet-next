// Source: docklet-next/docs/design/images-updates.md
// Revision: f6033a46c9ef4ad490661addd7dac41ab0ed5a45
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Published stable release selection below the running hub for issue #229; reads the product feed (#235).

package version

import (
	"context"

	imageDTO "github.com/florianfinn/docklet-next/types/image"
)

// PublishedReleaseAtMost keeps the update-all contract: no observed release at
// or below the hub is a requires-hub-update refusal, a feed failure is 503.
func (s *VersionService) PublishedReleaseAtMost(ctx context.Context, hub string) (string, error) {
	release, ok, err := s.ReleaseAtMost(ctx, hub)
	switch {
	case failureCodeInternal(err) == "update.system-version-unknown":
		return "", err
	case failureCodeInternal(err) == "feature-disabled":
		return "", imageDTO.Failure("update.requires-hub-update", 409)
	case err != nil:
		return "", imageDTO.Failure("update.check-unavailable", 503)
	case !ok:
		return "", imageDTO.Failure("update.requires-hub-update", 409)
	}
	return release.Version, nil
}
