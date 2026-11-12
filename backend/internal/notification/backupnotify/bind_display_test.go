// Source: docklet-next/issues/289
// Revision: b9b59ae92f1b09eb828940860971002eab9267b6
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original M16 producer and transport regression coverage; M16 bind mounts and backup completion.

package backupnotify

import (
	"context"
	dto "github.com/florianfinn/docklet-next/types/notification"
	"github.com/florianfinn/docklet-next/types/volume"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestBindNotificationDisplaysSource(t *testing.T) {
	key := volume.BindKey("/srv/app.conf")
	var request dto.DispatchRequest
	n := Notifier{Agent: true, Dispatch: func(_ context.Context, r dto.DispatchRequest) error { request = r; return nil }}
	n.Terminal(t.Context(), Run{ID: key, Operation: "backup", Volume: key, Status: "failed", Reason: "volume.backup.engine-unavailable"})
	require.NotNil(t, request.BackupFailed)
	require.Equal(t, "/srv/app.conf", request.BackupFailed.Volume)
	require.NotContains(t, request.BackupFailed.Volume, "bind:")
	require.Equal(t, key, volume.BindKey(request.BackupFailed.Volume))
}
