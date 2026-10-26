// Source: docklet-next/docs/design/update-policies.md
// Revision: cdfe93c3dd2557cb21586ae542ebc05cae9bc0af
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Actual Compose labels/exclusions and saved edits reach automatic-policy baseline DTOs.

package project

import (
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLifecyclePolicyProjectionExclusionsAndDrift(t *testing.T) {
	for _, test := range []string{"configured label", "excluded name", "saved drift"} {
		t.Run(test, func(t *testing.T) {
			f := newLifecycleFixture(t)
			f.establishBaseline()
			if test == "configured label" {
				source := strings.Replace(lifecycleOriginal, "    env_file:", "    labels:\n      com.getarcaneapp.arcane.updater: 'false'\n    env_file:", 1)
				require.NoError(t, os.WriteFile(filepath.Join(f.project.Path, "compose.yaml"), []byte(source), 0600))
			} else if test == "excluded name" {
				require.NoError(t, f.service.settingsService.SetStringSetting(t.Context(), "autoUpdateExcludedContainers", "app"))
			} else {
				require.NoError(t, os.WriteFile(filepath.Join(f.project.Path, "compose.yaml"), []byte(lifecycleOriginal+"# saved edit\n"), 0600))
			}
			status, err := f.service.ImageBaselineStatus(t.Context(), f.project.ID, f.registration)
			require.NoError(t, err)
			require.Len(t, status.TargetIdentity, 64)
			if test == "saved drift" {
				require.False(t, status.Available)
				require.Equal(t, "update.config-drift", status.Code)
			} else {
				require.True(t, status.AutoExcluded)
			}
			require.Zero(t, f.pulls)
			require.Zero(t, f.deploys)
		})
	}
}
