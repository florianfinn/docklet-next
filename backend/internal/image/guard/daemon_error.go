// Source: docklet-next/docs/design/images-updates.md
// Revision: 7df52f8f3b912be751e2283ec8d990933f5dc4b8
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Original image/registry UI foundation follow-up for issue #229.

package guard

import (
	"errors"
	"net"

	"github.com/containerd/errdefs"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	"github.com/moby/moby/client"
)

// Classify daemon errors before dropping private diagnostics. A missing source
// and unavailable transport remain distinct from a refused upstream operation.
func DaemonError(err error, code string) *imageDTO.OperationError {
	var typed *imageDTO.OperationError
	if errors.As(err, &typed) {
		return typed
	}
	if errdefs.IsNotFound(err) {
		return imageDTO.Failure("image.not-found", 404)
	}
	var transport net.Error
	if errdefs.IsUnavailable(err) || client.IsErrConnectionFailed(err) || errors.As(err, &transport) {
		return imageDTO.Failure("image.usage-unavailable", 503)
	}
	return imageDTO.Failure(code, 502)
}
