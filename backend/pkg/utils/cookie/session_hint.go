// Source: docklet-next/issues/425
// Revision: 6b89ee3606392c8b65c6dbd6577ffe0773e581b1
// Retrieved: 2026-11-11
// License: Apache-2.0
// Deviations: Original non-secret session presence hint paired with token cookies.

package cookie

import (
	"net/http"
	"strings"
)

// Copy the actual primary token cookie so all lifetime and transport attributes stay paired.
// The hint is browser-readable and is never an authorization credential.
func appendSessionHint(headers []string, tokenName string, clear bool) []string {
	for _, header := range headers {
		primary, err := http.ParseSetCookie(header)
		if err != nil || primary.Name != tokenName {
			continue
		}
		primary.Name = "docklet_session"
		if strings.HasPrefix(tokenName, "__Host-") {
			primary.Name = "__Host-" + primary.Name
		}
		primary.Value = "1"
		if clear {
			primary.Value = ""
		}
		primary.HttpOnly = false
		return append(headers, primary.String())
	}
	return headers
}
