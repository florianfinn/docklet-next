// Source: docklet-next/docs/design/images-updates-identity.md
// Revision: fcf8c8d3a079154c299912a7e22e0e903e5e4e38
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Stored legacy values cannot acquire native comparison authority.

package imageupdate

import (
	"github.com/florianfinn/docklet-next/backend/internal/imageupdate/children/identity"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
	"time"
)

func TestLegacyTopLevelProjection(t *testing.T) {
	db := setupImageUpdateRegistryTestDBInternal(t)
	record := ImageUpdateRecord{ID: "legacy", Repository: "registry.example.com/app", Tag: "stable", HasUpdate: true, CheckTime: time.Now().UTC(), CurrentDigest: new("sha256:" + strings.Repeat("a", 64)), LatestDigest: new("sha256:" + strings.Repeat("b", 64))}
	for _, p := range []string{"", "linux/amd64"} {
		record.Platform = &p
		require.NoError(t, db.Save(&record).Error)
		var reopened ImageUpdateRecord
		require.NoError(t, db.First(&reopened, "id = ?", record.ID).Error)
		require.Equal(t, "unknown", reopened.UpdateInfo().State)
		require.False(t, reopened.NeedsUpdate())
		svc := &ImageUpdateService{db: db}
		stored, found, e := svc.StoredUpdateByImageID(t.Context(), record.ID)
		require.NoError(t, e)
		require.True(t, found)
		require.False(t, stored.HasUpdate)
		require.Equal(t, *record.LatestDigest, *stored.LatestDigest)
	}
	p, e := identity.ParsePlatform("linux/amd64")
	require.NoError(t, e)
	n, e := identity.DecodeObservation("native-v1:classic:"+p.String(), *record.CurrentDigest, *record.LatestDigest)
	require.NoError(t, e)
	record.SetNativeObservation(n)
	require.Equal(t, "rebuild", record.UpdateInfo().State)
}
