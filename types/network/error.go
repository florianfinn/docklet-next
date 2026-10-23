// Source: docklet-next/docs/design/network-management.md
// Revision: 09d1e632cb3a14bd48b87232acaf991b9d12145e
// Retrieved: 2026-10-23
// License: Apache-2.0
// Deviations: Original host-side network contract implementation for issue #203.

package network

// Error is the stable problem response for rejected network writes.
type Error struct {
	Status int    `json:"status"`
	Title  string `json:"title"`
	Detail string `json:"detail"`
	Code   string `json:"code" enum:"network.not-found,network.exists,network.invalid-network,network.builtin-network,network.compose-managed,network.in-use,network.network-mode,network.already-connected,network.not-connected,network.invalid-ip,network.overlap"`
}

func (e *Error) Error() string  { return e.Detail }
func (e *Error) GetStatus() int { return e.Status }
func (e *Error) ContentType(contentType string) string {
	if contentType == "application/json" {
		return "application/problem+json"
	}
	if contentType == "application/cbor" {
		return "application/problem+cbor"
	}
	return contentType
}
