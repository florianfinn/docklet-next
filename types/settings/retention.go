// Source: docklet-next/issues/355
// Revision: b5068aff3da7eac11f208d1f4d8c9068d4a8c073
// Retrieved: 2026-11-06
// License: Apache-2.0
// Deviations: Original activity retention settings problem.

package settings

type Problem struct {
	Code   string `json:"code" enum:"settings.activity-limit-invalid"`
	Field  string `json:"field"`
	Status int    `json:"status"`
}

func (p *Problem) Error() string  { return p.Code }
func (p *Problem) GetStatus() int { return p.Status }
