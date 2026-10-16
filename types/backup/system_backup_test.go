// Source: docklet-next/docs/design/hub-backup.md
// Revision: 8f8c8f89bc6ebb7261862d0c35a6ddbc1513e9fa
// Retrieved: 2026-10-15
// License: Apache-2.0
// Deviations: Original wire-isolation tests for issue #32.

package backup

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSystemArchiveWireExcludesRecoveryInputs(t *testing.T) {
	for _, value := range []any{
		CreateSystemBackupRequest{PolicyID: "policy-1", TargetID: "target-1"},
		RestoreSystemBackupRequest{ConfirmBackupID: "archive-1", RecoveryKey: "secret"},
		SystemBackupRun{ID: "archive-1", Size: 2147483648, Destination: SystemBackupDestinationLocal, LocalSnapshotID: "snapshot-1", Error: "secret"},
	} {
		body, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		for _, old := range []string{"secret", "recoveryKey", "localSnapshotId", "destination", "snapshot-1"} {
			if strings.Contains(string(body), old) {
				t.Fatalf("system archive wire includes legacy field: %s", old)
			}
		}
	}
}

func TestSystemArchiveSizeIsAnIntegerByteCount(t *testing.T) {
	body, err := json.Marshal(SystemBackupRun{ID: "archive-1", Size: 2147483648})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `"size":2147483648`) {
		t.Fatalf("archive byte size was not preserved: %s", body)
	}
}

func TestArchiveNameForIDIsIsolatedFromPaths(t *testing.T) {
	name, err := ArchiveNameForID("backup_01-ABC")
	if err != nil || name != "backup_01-ABC.tar" {
		t.Fatalf("safe backup ID did not map to a fixed archive name: %q, %v", name, err)
	}
	for _, id := range []string{"", "../archive", "folder/archive", "folder\\archive", ".hidden", "a.tar", strings.Repeat("a", 129)} {
		if _, err := ArchiveNameForID(id); err == nil {
			t.Fatalf("unsafe backup ID accepted: %q", id)
		}
	}
}
