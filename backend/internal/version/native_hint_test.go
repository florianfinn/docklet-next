// Source: docklet-next/docs/design/images-updates-identity.md
// Revision: fcf8c8d3a079154c299912a7e22e0e903e5e4e38
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Review correction with real producer and behavioral guard proofs.

package version

import (
	"github.com/florianfinn/docklet-next/backend/internal/imageupdate"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
	"time"
)

func TestNativeOwnVersionHintStores(t *testing.T) {
	for _, store := range []string{"classic", "containerd"} {
		for _, changed := range []bool{false, true} {
			t.Run(store+map[bool]string{true: "/changed", false: "/same"}[changed], func(t *testing.T) {
				db := setupImageUpdateTestDB(t)
				current := "sha256:" + strings.Repeat("a", 64)
				latest := current
				if changed {
					latest = "sha256:" + strings.Repeat("b", 64)
				}
				platform := "native-v1:" + store + ":linux/amd64"
				require.NoError(t, db.Create(&imageupdate.ImageUpdateRecord{ID: current, Repository: "registry.example.com/app", Tag: "next", Platform: &platform, CurrentDigest: &current, LatestDigest: &latest, CheckTime: time.Now().UTC(), HasUpdate: !changed}).Error)
				images := imageupdate.NewImageUpdateService(db, nil, nil, nil, nil, nil, nil)
				svc := NewVersionService(nil, false, "2.8.0-next.65", "revision", nil, nil, images, nil)
				available, ref := svc.storedOrDigestBasedUpdateInternal(t.Context(), current, "next", current, "registry.example.com/app", "registry.example.com/app")
				require.Equal(t, changed, available)
				require.Empty(t, ref, "native config/manifest keys cannot escape as legacy pull references")
			})
		}
	}
}
