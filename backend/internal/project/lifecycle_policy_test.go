// Source: docklet-next/docs/design/update-policies.md
// Revision: c2a8b95f3eae4f7d561bfc48d5dfe97c63bab6db
// Retrieved: 2026-10-25
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
