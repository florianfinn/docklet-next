// Source: docklet-next/issues/315
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-30
// License: Apache-2.0
// Deviations: Original absent/null/value preservation for the nullable colour patch contract.
package environment

import "encoding/json"

// Color's outer pointer records presence; its inner nil restores the palette.
// Ordinary pointer decoding otherwise collapses an explicit null into absence.
func (u *Update) UnmarshalJSON(body []byte) error {
	type plain Update
	decoded := struct {
		*plain
		Color json.RawMessage `json:"color"`
	}{plain: new(plain)}
	if err := json.Unmarshal(body, &decoded); err != nil {
		return err
	}
	if decoded.Color != nil {
		var value *string
		if err := json.Unmarshal(decoded.Color, &value); err != nil {
			return err
		}
		decoded.plain.Color = &value
	}
	*u = Update(*decoded.plain)
	return nil
}
