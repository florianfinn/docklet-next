// Source: docklet-next/docs/design/images-updates.md
// Revision: f6033a46c9ef4ad490661addd7dac41ab0ed5a45
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Neutral registry probe outcome classification for issue #229.

package registry

import (
	"errors"
	"github.com/containerd/errdefs"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"
	"strings"
)

// Classify before discarding diagnostics. Unknown/transport failures are not
// evidence of a bad password, and must never request hub sign-out.
func registryProbeError(err error, anonymous bool) *imageDTO.OperationError {
	var typed *imageDTO.OperationError
	if errors.As(err, &typed) {
		return typed
	}
	status := 0
	var remoteErr *transport.Error
	if errors.As(err, &remoteErr) {
		status = remoteErr.StatusCode
	}
	var httpErr interface{ HTTPStatusCode() int }
	if errors.As(err, &httpErr) {
		status = httpErr.HTTPStatusCode()
	}
	var apiErr interface{ ErrorCode() string }
	apiCode := ""
	if errors.As(err, &apiErr) {
		apiCode = strings.ToLower(apiErr.ErrorCode())
	}
	text := strings.ToLower(err.Error())
	if status == 429 || errdefs.IsResourceExhausted(err) || strings.Contains(apiCode, "throttl") || strings.Contains(apiCode, "toomanyrequest") || strings.Contains(text, "toomanyrequests") || strings.Contains(text, "too many requests") || strings.Contains(text, "rate limit") {
		return imageDTO.Failure("registry.rate-limited", 429)
	}
	if status == 401 || status == 403 || errdefs.IsUnauthorized(err) || errdefs.IsPermissionDenied(err) || strings.Contains(apiCode, "unrecognizedclient") || strings.Contains(apiCode, "invalidsignature") || strings.Contains(apiCode, "accessdenied") || strings.Contains(text, "unauthorized") || strings.Contains(text, "authentication required") || strings.Contains(text, "incorrect username or password") || strings.Contains(text, "no basic auth credentials") || strings.Contains(text, "access denied") {
		if anonymous {
			return imageDTO.Failure("registry.authentication-required", 401)
		}
		return imageDTO.Failure("registry.authentication-failed", 401)
	}
	return imageDTO.Failure("registry.unavailable", 503)
}
