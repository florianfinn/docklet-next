// Source: docklet-next/issues/341
// Revision: 89f69980c01d08714ce96c49365fb19065e3d6ba
// Retrieved: 2026-11-04
// License: Apache-2.0
// Deviations: Original zero-forwarding refusal of retired update controls.

package job

import (
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	"strings"
)

func retiredUpdateControl(id string) error {
	if id == "auto-update" || id == "image-polling" || strings.HasPrefix(id, "image-policy:") {
		return imageDTO.Failure("update.agent-local-job-disabled", 409)
	}
	return nil
}
