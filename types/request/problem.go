// Source: docklet-next/issues/358
// Revision: 32e24a1b5260c98b6ce490ce2c1f6f50413c1e83
// Retrieved: 2026-11-07
// License: Apache-2.0
// Deviations: Original M20 removal and refusal contracts.

package request

// PathEncoded is a request refusal; it is not a runtime activity result.
const PathEncoded = "request.path-encoded"

type Problem struct {
	Code   string `json:"code" enum:"request.path-encoded"`
	Status int    `json:"status" enum:"400"`
}

func (p *Problem) Error() string  { return p.Code }
func (p *Problem) GetStatus() int { return p.Status }
