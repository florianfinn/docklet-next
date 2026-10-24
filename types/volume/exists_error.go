// Source: docklet-next/issues/217
// Revision: 3cba55fd10c311e4a5b3361afabb8c93fca706a0
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Original typed volume name conflict response.

package volume

// ExistsError identifies an occupied target name without exposing daemon text.
type ExistsError struct {
	Status int    `json:"status"`
	Title  string `json:"title"`
	Detail string `json:"detail"`
	Code   string `json:"code" enum:"volume.exists"`
}

func (e *ExistsError) Error() string  { return e.Detail }
func (e *ExistsError) GetStatus() int { return e.Status }
func (e *ExistsError) ContentType(contentType string) string {
	return (&HelperIncompatibleError{}).ContentType(contentType)
}
