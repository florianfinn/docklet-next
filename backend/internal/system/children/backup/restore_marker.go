// Source: docklet-next/docs/design/hub-backup.md
// Revision: 8f8c8f89bc6ebb7261862d0c35a6ddbc1513e9fa
// Retrieved: 2026-10-15
// License: Apache-2.0
// Deviations: Original durable restore marker for issue #32.

package backup

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	backuptypes "github.com/florianfinn/docklet-next/types/backup"
)

const restoreMarkerName = "restore-state.json"

var ErrRestoreRecoveryRequired = errors.New("backup.recovery_required")

type restoreMarker struct {
	BackupID   string `json:"backupId"`
	ActivityID string `json:"activityId,omitempty"`
	Phase      string `json:"phase"`
}

func markerPath(dir string) string { return filepath.Join(dir, restoreMarkerName) }

// CheckRestoreMarker fails closed on a present, unreadable, or malformed marker.
// It runs before database migration or service startup.
func CheckRestoreMarker(dir string) error {
	_, err := os.Lstat(markerPath(dir))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return ErrRestoreRecoveryRequired
}

func readRestoreMarker(dir string) (restoreMarker, error) {
	var marker restoreMarker
	info, err := os.Lstat(markerPath(dir))
	if err != nil || !info.Mode().IsRegular() || info.Size() > 4096 {
		return marker, ErrRestoreRecoveryRequired
	}
	data, err := os.ReadFile(markerPath(dir))
	if err != nil || json.Unmarshal(data, &marker) != nil {
		return marker, ErrRestoreRecoveryRequired
	}
	if _, err := backuptypes.ArchiveNameForID(marker.BackupID); err != nil || (marker.Phase != "applying" && marker.Phase != "verifying") {
		return restoreMarker{}, ErrRestoreRecoveryRequired
	}
	return marker, nil
}

func writeRestoreMarker(dir string, marker restoreMarker) error {
	if err := ensureArchiveDir(dir); err != nil {
		return ErrRestoreRecoveryRequired
	}
	if marker.Phase != "applying" && marker.Phase != "verifying" {
		return ErrRestoreRecoveryRequired
	}
	if _, err := backuptypes.ArchiveNameForID(marker.BackupID); err != nil {
		return ErrRestoreRecoveryRequired
	}
	data, err := json.Marshal(marker)
	if err != nil {
		return ErrRestoreRecoveryRequired
	}
	file, err := os.CreateTemp(dir, ".restore-state-")
	if err != nil {
		return ErrRestoreRecoveryRequired
	}
	defer os.Remove(file.Name())
	defer file.Close()
	if err := file.Chmod(0600); err != nil {
		return ErrRestoreRecoveryRequired
	}
	if _, err := file.Write(data); err != nil {
		return ErrRestoreRecoveryRequired
	}
	if err := file.Sync(); err != nil {
		return ErrRestoreRecoveryRequired
	}
	if err := file.Close(); err != nil {
		return ErrRestoreRecoveryRequired
	}
	if err := os.Rename(file.Name(), markerPath(dir)); err != nil {
		return ErrRestoreRecoveryRequired
	}
	return syncArchiveDir(dir)
}

func clearRestoreMarker(dir string) error {
	if err := os.Remove(markerPath(dir)); err != nil {
		return ErrRestoreRecoveryRequired
	}
	return syncArchiveDir(dir)
}

func syncArchiveDir(dir string) error {
	file, err := os.Open(dir)
	if err != nil {
		return ErrRestoreRecoveryRequired
	}
	defer file.Close()
	if err := file.Sync(); err != nil {
		return ErrRestoreRecoveryRequired
	}
	return nil
}
