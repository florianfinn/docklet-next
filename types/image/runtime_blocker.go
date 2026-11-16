// Source: docklet-next/issues/465
// Revision: b4f894ff7f35186d4c39be1fc9fccd01b4b5934a
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Original bounded runtime identity blocker detail.

package image

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// RuntimeIdentityBlocker identifies a peer without forwarding filesystem diagnostics.
type RuntimeIdentityBlocker struct {
	Name     string `json:"name" minLength:"1" maxLength:"128"`
	Reason   string `json:"reason" enum:"compose.selection-unsupported,compose.identity-unreadable"`
	Archived bool   `json:"archived"`
}

// SafeRuntimeIdentityBlocker is the sole remote detail allowlist for this refusal.
func SafeRuntimeIdentityBlocker(code string, value *RuntimeIdentityBlocker) *RuntimeIdentityBlocker {
	if code != "runtime.target-unavailable" || value == nil || !utf8.ValidString(value.Name) || len(value.Name) > 128 || strings.TrimSpace(value.Name) == "" || strings.ContainsAny(value.Name, "/\\:") {
		return nil
	}
	for _, r := range value.Name {
		if unicode.IsControl(r) || (unicode.IsSpace(r) && r != ' ') {
			return nil
		}
	}
	switch value.Reason {
	case "compose.selection-unsupported", "compose.identity-unreadable":
	default:
		return nil
	}
	copy := *value
	return &copy
}
