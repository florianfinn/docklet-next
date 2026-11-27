// Source: docklet-next/issues/680
// Revision: 9590d35e3340294b3d329053d5846656b5791371
// Retrieved: 2026-11-27
// License: Apache-2.0
// Deviations: Original minimum image age implementation.

package updater

import (
	"context"
	policy "github.com/florianfinn/docklet-next/backend/internal/updater/children/policies"
	st "github.com/florianfinn/docklet-next/types/scheduler"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
	"time"
)

func TestMinimumAgeGateAndDisabledBypass(t *testing.T) {
	for _, which := range []string{"young", "eligible", "unknown", "invalid", "disabled", "old", "persistence"} {
		t.Run(which, func(t *testing.T) {
			f := newPolicyFixture(t)
			one := f.save(t, "example-stack", "scheduled")
			info := f.check.ServiceUpdates["app"].UpdateInfo
			info.LatestManifest = "sha256:" + strings.Repeat("d", 64)
			days := "2"
			if which == "disabled" {
				days = "0"
			}
			require.NoError(t, f.s.deps.Settings.UpdateSetting(t.Context(), "updateMinimumAgeDays", days))
			if which == "unknown" {
				info.LatestManifest = ""
			}
			if which == "invalid" {
				info.LatestManifest = "invalid"
			}
			if which == "eligible" || which == "old" {
				c := dto.UpdateCandidate{ImageRef: f.check.ServiceUpdates["app"].ImageRef, Platform: info.Platform, Manifest: info.LatestManifest}
				require.NoError(t, f.p.store.ObserveFirstSeen(t.Context(), c, time.Now().UTC().Add(-72*time.Hour)))
			}
			if which == "persistence" {
				require.NoError(t, f.db.Migrator().DropTable(&policy.FirstSeen{}))
			}
			if which == "old" {
				one.EnvironmentID = "example-remote"
				f.p.ports.contractVersion = func(_ context.Context, _ string) (int, error) { return 25, nil }
			}
			out, _ := f.s.executeImagePolicyTarget(t.Context(), f.p, one, "window", "auto-update")
			if which == "eligible" || which == "disabled" {
				require.Equal(t, st.Succeeded, out.Status)
				require.Equal(t, 1, f.admissions)
				if which == "eligible" {
					require.Equal(t, []dto.ExpectedCandidate{{Service: "app", Manifest: info.LatestManifest}}, f.bindings[0].Expected)
				} else {
					require.Empty(t, f.bindings[0].Expected)
					var n int64
					require.NoError(t, f.db.Model(&policy.FirstSeen{}).Count(&n).Error)
					require.Zero(t, n)
				}
			} else {
				require.Zero(t, f.admissions)
				code := "update.minimum-age"
				if which == "persistence" {
					code = "update.persistence-failed"
				}
				if which == "invalid" {
					code = "update.check-unavailable"
				}
				require.Equal(t, code, out.Code)
			}
		})
	}
}
