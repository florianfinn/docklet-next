// Source: docklet-next/docs/design/storage-workspace.md
// Revision: 60d2471b62b12e1af64c54060a50102a2e39d1c1
// Retrieved: 2026-10-22
// License: Apache-2.0
// Deviations: Original helper incompatibility factory for issue #179.
package volumehelper

import (
	"github.com/florianfinn/docklet-next/types/volume"
	"net/http"
)

func IncompatibleHelper(cause error) *volume.HelperIncompatibleError {
	return &volume.HelperIncompatibleError{Status: http.StatusServiceUnavailable, Title: http.StatusText(http.StatusServiceUnavailable), Detail: "The volume helper does not support the required ACFS protocol", Code: "volume.helper-incompatible", Cause: cause}
}
