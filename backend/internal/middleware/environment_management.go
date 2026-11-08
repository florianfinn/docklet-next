// Source: docklet-next/issues/242
// Revision: 89f69980c01d08714ce96c49365fb19065e3d6ba
// Retrieved: 2026-11-04
// License: Apache-2.0
// Deviations: Original hub policy cleanup after successful Direct or Edge stack destruction.

package middleware

import (
	"net/http"
	"strings"
)

func isManagementPathInternal(method, suffix string) bool {
	if isHistoryManagementPath(method, suffix) {
		return true
	}
	if method == http.MethodPost && suffix == "/agent/restart" {
		return true
	}
	partsRestart := strings.Split(suffix, "/")
	if method == http.MethodGet && len(partsRestart) == 4 && partsRestart[0] == "" && partsRestart[1] == "agent" && partsRestart[2] == "restarts" && partsRestart[3] != "" {
		return true
	}

	partsOrigin := strings.Split(strings.Trim(suffix, "/"), "/")
	if len(partsOrigin) == 3 && partsOrigin[0] == "containers" && partsOrigin[2] == "origin" {
		return true
	}
	if suffix == "/updater/system-image-hints" || suffix == "/updater/policies" || strings.HasPrefix(suffix, "/updater/policies/") {
		return true
	}
	if method == http.MethodPost && strings.HasPrefix(suffix, "/updater/projects/") && strings.HasSuffix(suffix, "/check") {
		return true
	}
	if method == http.MethodPost && strings.HasPrefix(suffix, "/updater/") && (strings.HasSuffix(suffix, "/update") || strings.HasSuffix(suffix, "/restore")) {
		return true
	}
	if strings.HasPrefix(suffix, "/containers/") && strings.HasSuffix(suffix, "/previous-image/restore") {
		return true
	}
	if strings.HasPrefix(suffix, "/image-updates/check") {
		return true
	}
	if suffix == "/runtime-runs" || strings.HasPrefix(suffix, "/runtime-runs/") || suffix == "/runtime-registration" || suffix == "/runtime-target" {
		return true
	}
	if method == http.MethodPost {
		parts := strings.Split(strings.Trim(suffix, "/"), "/")
		if len(parts) == 3 && parts[1] != "" {
			if parts[0] == "projects" && (parts[2] == "up" || parts[2] == "down" || parts[2] == "stop" || parts[2] == "restart" || parts[2] == "redeploy") {
				return true
			}
			if parts[0] == "containers" && (parts[2] == "start" || parts[2] == "stop" || parts[2] == "restart" || parts[2] == "recreate" || parts[2] == "redeploy" || parts[2] == "update") {
				return true
			}
		}
	}

	if suffix == "/jobs" || strings.HasPrefix(suffix, "/jobs/") {
		return true
	}
	if method == http.MethodPost {
		parts := strings.Split(strings.Trim(suffix, "/"), "/")
		if len(parts) == 3 && parts[0] == "volumes" && parts[1] != "" && parts[2] == "backups" {
			return true
		}
	}
	if isCentralVolumeBackupPolicyPathInternal(method, suffix) {
		return true
	}
	// Only the hub pushes backup target sets; clients never reach an agent's receiver.
	if (method == http.MethodPut && suffix == "/volumes/backups/targets") || (method == http.MethodPost && suffix == "/volumes/backups/targets/probe") {
		return true
	}
	if suffix == "/activities" || strings.HasPrefix(suffix, "/activities/") {
		return true
	}

	if suffix == "/auto-heal" {
		return true
	}
	if strings.HasPrefix(suffix, "/notifications") {
		return true
	}

	if strings.HasPrefix(suffix, "/deployment/mtls/") {
		return true
	}

	_, isManagement := managementEndpointSet[suffix]
	return isManagement
}
