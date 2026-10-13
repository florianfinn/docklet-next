// Source: getarcaneapp/arcane/types/httpx/client.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: None

package httpx

import "time"

// ClientOptions configures timeouts for an outbound HTTP client.
// Durations are used as provided; a zero Timeout disables the request timeout.
type ClientOptions struct {
	Timeout             time.Duration
	TLSHandshakeTimeout time.Duration
}
