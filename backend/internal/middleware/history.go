// Source: docklet-next/issues/335
// Revision: 14e11ba9c022d57c29bd36c8ed24780763327d0c
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original hub-only history route classification.

package middleware

import "strings"

func isHistoryManagementPath(method, suffix string) bool {
	if method != "GET" {
		return false
	}
	if suffix == "/runtime-conditions" || suffix == "/history/host" {
		return true
	}
	parts := strings.Split(strings.Trim(suffix, "/"), "/")
	return len(parts) == 3 && (parts[0] == "containers" || parts[0] == "projects") && parts[1] != "" && parts[2] == "history"
}
