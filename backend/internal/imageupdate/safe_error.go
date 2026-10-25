// Source: docklet-next/docs/design/images-updates.md
// Revision: 361c54cb8d087b7d531ff8bb4dd1acae82f5f534
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Original image and registry safety contract for issue #229.

package imageupdate

import (
	"github.com/florianfinn/docklet-next/backend/internal/registry"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	checkDTO "github.com/florianfinn/docklet-next/types/imageupdate"
)

func checkCode(err error) string {
	if err != nil && registry.IsRateLimitErrorString(err.Error()) {
		return "registry.rate-limited"
	}
	return "update.check-unavailable"
}
func sanitizeCheckResult(result *checkDTO.Response) {
	if result == nil {
		return
	}
	result.AuthUsername = ""
	if result.Error != "" {
		if registry.IsRateLimitErrorString(result.Error) {
			result.Code = "registry.rate-limited"
		}
		if result.Code == "" {
			result.Code = "update.check-unavailable"
		}
		result.Error = imageDTO.Failure(result.Code, 503).Error()
	}
}
