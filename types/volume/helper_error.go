// Source: docklet-next/docs/design/storage-workspace.md
// Revision: 60d2471b62b12e1af64c54060a50102a2e39d1c1
// Retrieved: 2026-10-22
// License: Apache-2.0
// Deviations: Original typed helper incompatibility contract for issue #179.
package volume

// HelperIncompatibleError is returned before file access when the configured
// helper cannot serve the required ACFS protocol. Cause stays server-side.
type HelperIncompatibleError struct {
	Status int    `json:"status"`
	Title  string `json:"title"`
	Detail string `json:"detail"`
	Code   string `json:"code" enum:"volume.helper-incompatible"`
	Cause  error  `json:"-"`
}

func (e *HelperIncompatibleError) Error() string  { return e.Detail }
func (e *HelperIncompatibleError) Unwrap() error  { return e.Cause }
func (e *HelperIncompatibleError) GetStatus() int { return e.Status }
func (e *HelperIncompatibleError) ContentType(contentType string) string {
	if contentType == "application/json" {
		return "application/problem+json"
	}
	if contentType == "application/cbor" {
		return "application/problem+cbor"
	}
	return contentType
}

// HelperUnavailableError reports a transient Docker/exec failure without making
// a compatibility verdict about the configured image. Cause stays server-side.
type HelperUnavailableError struct {
	Status int    `json:"status"`
	Title  string `json:"title"`
	Detail string `json:"detail"`
	Code   string `json:"code" enum:"volume.helper-unavailable"`
	Cause  error  `json:"-"`
}

func (e *HelperUnavailableError) Error() string  { return e.Detail }
func (e *HelperUnavailableError) Unwrap() error  { return e.Cause }
func (e *HelperUnavailableError) GetStatus() int { return e.Status }
func (e *HelperUnavailableError) ContentType(contentType string) string {
	return (&HelperIncompatibleError{}).ContentType(contentType)
}
