// Source: docklet-next/issues/496
// Revision: b4f894ff7f35186d4c39be1fc9fccd01b4b5934a
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Original deterministic download admission completion proof.

package backup

import (
	"context"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/backup"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestDownloadAdmissionOutlivesTerminalActivityWrite(t *testing.T) {
	for _, kind := range []string{"streamed", "buffered"} {
		t.Run(kind, func(t *testing.T) {
			r := newStreamRig(t, "h1")
			observed := make(chan bool, 1)
			db := r.f.service.deps.DB
			require.NoError(t, db.Callback().Update().Before("gorm:update").Register("test:download-terminal-admission", func(tx *gorm.DB) {
				fields, ok := tx.Statement.Dest.(map[string]any)
				if tx.Statement.Table != "activities" || !ok || fields["ended_at"] == nil {
					return
				}
				lease, acquired, err := r.f.service.deps.Engine.TryAcquireRun(t.Context(), backup.VolumeAdmissionScope, "data")
				if acquired {
					lease.Release(t.Context())
				}
				observed <- acquired && err == nil
			}))
			if kind == "streamed" {
				download, err := r.f.service.BeginStreamingDownload(t.Context(), "row", nil)
				require.NoError(t, err)
				download.release(context.Canceled)
			} else {
				reader, _, err := r.f.service.DownloadBackup(t.Context(), "row", nil)
				require.NoError(t, err)
				require.NoError(t, reader.Close())
			}
			require.False(t, <-observed, "download admission was released while its activity was still running")
			r.requireReleased()
		})
	}
}
