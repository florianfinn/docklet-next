// Source: docklet-next/docs/design/hub-backup.md
// Revision: 8f8c8f89bc6ebb7261862d0c35a6ddbc1513e9fa
// Retrieved: 2026-10-15
// License: Apache-2.0
// Deviations: Original route and schema contract tests for issue #32.

package api

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHubBackupArchiveContract(t *testing.T) {
	spec := SetupAPIForSpec().OpenAPI()
	for _, route := range []struct{ method, path string }{
		{"get", "/backups"}, {"post", "/backups"}, {"get", "/backups/history"},
		{"get", "/backups/policies"}, {"put", "/backups/policies"},
		{"get", "/backups/storage"}, {"get", "/backups/maintenance"},
		{"post", "/backups/upload"}, {"get", "/backups/{id}/download"},
		{"post", "/backups/{id}/restore"}, {"delete", "/backups/{id}"},
	} {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			path := spec.Paths[route.path]
			require.NotNil(t, path)
			switch route.method {
			case "get":
				require.NotNil(t, path.Get)
			case "post":
				require.NotNil(t, path.Post)
			case "put":
				require.NotNil(t, path.Put)
			case "delete":
				require.NotNil(t, path.Delete)
			}
		})
	}
	for _, path := range []string{
		"/backups/recovery-key", "/backups/recovery-key/generate",
		"/backups/{id}/files/browse", "/backups/{id}/restore-files",
	} {
		require.Nil(t, spec.Paths[path])
	}
	require.NotNil(t, spec.Paths["/backups/volumes/run"])
	download := spec.Paths["/backups/{id}/download"].Get.Responses["200"]
	require.NotNil(t, download.Content["application/octet-stream"])
	upload := spec.Paths["/backups/upload"].Post.RequestBody
	require.NotNil(t, upload)
	require.NotNil(t, upload.Content["application/octet-stream"])
	require.Nil(t, upload.Content["application/json"])
	require.Equal(t, "binary", upload.Content["application/octet-stream"].Schema.Format)
	require.Equal(t, 2147483648, *upload.Content["application/octet-stream"].Schema.MaxLength)

	run := spec.Components.Schemas.Map()["BackupSystemBackupRun"]
	require.NotNil(t, run)
	for _, field := range []string{"archiveName", "size", "activityId", "copyState", "copyTargetId", "migrationVersion"} {
		require.Contains(t, run.Properties, field)
	}
	for _, field := range []string{"localSnapshotId", "destination", "recoveryKey", "error"} {
		require.NotContains(t, run.Properties, field)
	}
	require.Equal(t, float64(2147483648), *run.Properties["size"].Maximum)
	storage := spec.Components.Schemas.Map()["BackupSystemBackupStorage"]
	require.NotNil(t, storage)
	for _, field := range []string{"available", "reasonCode", "usedBytes", "freeBytes", "maxArchiveBytes"} {
		require.Contains(t, storage.Properties, field)
	}
	require.Equal(t, float64(2147483648), *storage.Properties["maxArchiveBytes"].Maximum)
}
