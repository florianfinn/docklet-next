// Source: docklet-next/docs/design/images-updates.md
// Revision: 48915fde5d95ed00c757116410f76617e23e8a50
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Neutral authentication classification for saved and anonymous pulls.

package image

import (
	"github.com/containerd/errdefs"
	"github.com/florianfinn/docklet-next/backend/internal/image/guard"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	"strings"
)

// PullError classifies initial HTTP rejection and streamed daemon errors before
// discarding diagnostics. A registry rejection must never invalidate hub auth.
func PullError(err error, authenticated bool) *imageDTO.OperationError {
	if errdefs.IsUnauthorized(err) || errdefs.IsPermissionDenied(err) || isUnauthorizedPullErrorInternal(err) || strings.HasPrefix(strings.ToLower(strings.TrimSpace(err.Error())), "denied:") {
		return pullAuthenticationFailure(authenticated)
	}
	return guard.DaemonError(err, "image.pull-failed")
}

func pullAuthenticationFailure(authenticated bool) *imageDTO.OperationError {
	if authenticated {
		return imageDTO.Failure("registry.authentication-failed", 502)
	}
	return imageDTO.Failure("registry.authentication-required", 502)
}
