// Source: docklet-next/docs/design/images-updates-identity.md
// Revision: fcf8c8d3a079154c299912a7e22e0e903e5e4e38
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Validate the executor baseline envelope before copying identity.

package updater

import (
	"github.com/florianfinn/docklet-next/types/base"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	dto "github.com/florianfinn/docklet-next/types/updater"
)

func checkedBaselineResponse(out base.ApiResponse[dto.BaselineStatus]) (dto.BaselineStatus, error) {
	if !out.Success || !out.Data.Valid() {
		return dto.BaselineStatus{}, imageDTO.Failure("update.check-unavailable", 503)
	}
	return out.Data, nil
}
