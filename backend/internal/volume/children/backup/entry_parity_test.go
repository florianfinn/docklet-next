// Source: docklet-next/issues/284
// Revision: 30d80ec7904e6faeeea28a36914e9d5a30b9d2da
// Retrieved: 2026-10-29
// License: Apache-2.0
// Deviations: Original backup entry and stored backup parity test.

package backup

import (
	"reflect"
	"strings"
	"testing"
	"time"

	backupmode "github.com/florianfinn/docklet-next/types/backup"
	"github.com/florianfinn/docklet-next/types/volume"
	"github.com/stretchr/testify/require"
)

// Every wire field the stored backup shares with its entry by name and type
// reaches the entry; status and error code are derived and checked elsewhere.
func TestBackupEntryCarriesEveryStoredWireField(t *testing.T) {
	activityID, policyID := "activity", "policy"
	stored := volume.Backup{
		ID: "backup", VolumeName: "data", Size: 24, CreatedAt: time.Now().UTC(), Status: "succeeded", Trigger: "manual",
		Destination: volume.BackupDestination("local"), Format: volume.BackupFormatRustic, LocalSnapshotID: "snapshot",
		PolicyID: policyID, StopContainers: true, Error: "error", ActivityID: &activityID, Type: backupmode.ManagementTypeVolume,
	}
	entry := entryInternal(&stored)
	source, target := reflect.ValueOf(stored), reflect.ValueOf(entry)
	derived := map[string]bool{"Status": true, "ErrorCode": true, "Error": true}
	compared := 0
	for i := range source.NumField() {
		field := source.Type().Field(i)
		name := strings.Split(field.Tag.Get("json"), ",")[0]
		other, ok := target.Type().FieldByName(field.Name)
		if name == "-" || derived[field.Name] || !ok || other.Type != field.Type {
			continue
		}
		require.Equal(t, source.Field(i).Interface(), target.FieldByIndex(other.Index).Interface(), field.Name)
		compared++
	}
	require.Greater(t, compared, 10, "parity covers the shared wire fields")
	require.Equal(t, "volume.backup.interrupted", entry.Error, "stored diagnostic text becomes a stable code")
}
