// Source: docklet-next/issues/322
// Revision: 6365cc5cebd04f199d15fe5f53c006a127d0a669
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original simple-user resource authorization.

package environment

import "encoding/json/v2"

func (e Environment) MarshalJSON() ([]byte, error) {
	if e.DisplayOnly {
		return json.Marshal(struct {
			ID     string  `json:"id"`
			Name   string  `json:"name"`
			Status string  `json:"status"`
			Color  *string `json:"color"`
		}{e.ID, e.Name, e.Status, e.Color})
	}
	type full Environment
	return json.Marshal(full(e))
}
