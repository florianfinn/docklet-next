// Source: docklet-next/docs/design/images-updates-identity.md
// Revision: fcf8c8d3a079154c299912a7e22e0e903e5e4e38
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Review correction with real producer and behavioral guard proofs.

package project

import (
	"context"
	life "github.com/florianfinn/docklet-next/backend/internal/project/children/lifecycle"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPlatformCaptureBaselineStores(t *testing.T) {
	for _, store := range []string{"classic", "containerd"} {
		for _, requested := range []string{"linux/arm", "linux/arm/v7", "linux/arm64", "linux/arm64/v8"} {
			t.Run(store+"/"+requested, func(t *testing.T) {
				f := newLifecycleFixture(t)
				f.containerd = store == "containerd"
				f.platform = "linux/arm/v7"
				if requested == "linux/arm/v7" {
					f.platform = "linux/arm"
				}
				if strings.Contains(requested, "arm64") {
					f.platform = "linux/arm64/v8"
					if requested == "linux/arm64/v8" {
						f.platform = "linux/arm64"
					}
				}
				source := strings.Replace(lifecycleOriginal, "  app:\n", "  app:\n    platform: "+requested+"\n", 1)
				require.NoError(t, os.WriteFile(filepath.Join(f.project.Path, "compose.yaml"), []byte(source), 0600))
				_, snapshot, err := f.service.lifecycleCurrent(t.Context(), f.project.ID, f.registration)
				require.NoError(t, err)
				run := uuid.NewString()
				ctx := dto.WithExecutionIdentity(context.Background(), dto.ExecutionIdentity{RunID: run, RegistrationID: f.registration})
				f.service.captureDeployedBaseline(ctx, f.project.ID, snapshot)
				status, err := f.service.ImageBaselineStatus(t.Context(), f.project.ID, f.registration)
				require.NoError(t, err)
				require.True(t, status.Available)
				require.True(t, status.Valid())
				require.Equal(t, run, status.SourceRunID)
				canonical := "linux/arm/v7"
				if strings.Contains(requested, "arm64") {
					canonical = "linux/arm64"
				}
				require.Equal(t, canonical, snapshot.Services["app"].Platform)
				require.Zero(t, f.pulls)
				require.Zero(t, f.deploys)
			})
		}
	}
}
func TestPostDeployPlatformAndDigestStores(t *testing.T) {
	for _, store := range []string{"classic", "containerd"} {
		t.Run(store, func(t *testing.T) {
			expected := life.ServiceImage{ImageID: lifecycleOldID, Platform: "linux/arm64", Digest: ""}
			if store == "containerd" {
				expected.Digest = lifecycleDigest
			}
			actual := expected
			actual.Platform = "linux/arm64/v8"
			require.True(t, lifecycleMatchesExpected(actual, expected))
			actual.Platform = "linux/arm/v7"
			require.False(t, lifecycleMatchesExpected(actual, expected))
			actual = expected
			actual.Digest = "sha256:" + strings.Repeat("e", 64)
			require.False(t, lifecycleMatchesExpected(actual, expected))
		})
	}
}
func TestRestorePointNativeDigest(t *testing.T) {
	f := newLifecycleFixture(t)
	f.containerd = true
	snap := f.establishBaseline()
	run := uuid.NewString()
	point, err := f.service.lifecycleStore().Prepare(t.Context(), snap, run, "", "settled")
	require.NoError(t, err)
	require.NoError(t, f.service.lifecyclePinPoint(t.Context(), point, snap))
	require.NoError(t, f.service.verifyLifecyclePoint(t.Context(), point, snap))
	img := snap.Services["app"]
	img.Digest = lifecycleDigest
	snap.Services["app"] = img
	for key, img := range snap.Containers {
		img.Digest = lifecycleDigest
		snap.Containers[key] = img
	}
	require.Error(t, f.service.verifyLifecyclePoint(t.Context(), point, snap))
	require.Zero(t, f.pulls)
	require.Zero(t, f.deploys)
}
