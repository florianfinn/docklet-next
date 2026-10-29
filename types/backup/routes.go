// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original finite volume-backup transport classification.

package backup

import "strings"

// VolumeRouteResponseLimit excludes generic volume/workspace paths and includes
// the policy management path. Upload chunk limits belong to the upload service.
func VolumeRouteResponseLimit(method, path string) int {
	path = strings.TrimPrefix(path, "/api")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 4 || parts[0] != "environments" || parts[2] != "volumes" {
		return 0
	}
	isBackup := parts[3] == "backups" || len(parts) >= 5 && (parts[4] == "backups" || parts[4] == "backup-policy")
	if !isBackup {
		return 0
	}
	if method == "GET" && len(parts) == 6 && parts[3] == "backups" && parts[5] == "download" {
		return int(MaxEdgeDownloadBytes) + (1 << 20)
	}
	return MaxVolumeResponseBytes
}
