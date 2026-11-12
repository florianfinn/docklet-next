// Source: docklet-next/issues/322
// Revision: 6365cc5cebd04f199d15fe5f53c006a127d0a669
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original simple-user resource authorization.

package environment

import (
	"encoding/json/v2"
	dto "github.com/florianfinn/docklet-next/types/environment"
)

// The embedded environment marshaler must not hide the one-time pairing key.
func (e EnvironmentWithApiKey) MarshalJSON() ([]byte, error) {
	type fields dto.Environment
	return json.Marshal(struct {
		fields
		APIKey *string `json:"apiKey,omitempty"`
	}{fields(e.Environment), e.ApiKey})
}
