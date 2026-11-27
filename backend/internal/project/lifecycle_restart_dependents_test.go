// Source: docklet-next/issues/673
// Revision: 99c1b0b8c4dfe56182c72749e092b08202436fa5
// Retrieved: 2026-11-27
// License: Apache-2.0
// Deviations: Original regression proofs.

package project

import (
	"context"
	compose "github.com/compose-spec/compose-go/v2/types"
	activity "github.com/florianfinn/docklet-next/types/activity"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func restartDependentFixture(t *testing.T) *lifecycleFixture {
	t.Helper()
	f := newLifecycleFixture(t)
	require.NoError(t, os.WriteFile(filepath.Join(f.project.Path, "compose.yaml"), []byte(lifecycleOriginal+"  side:\n    image: registry.example.com/side:1.0.0\n    depends_on:\n      app:\n        condition: service_started\n        restart: true\n"), 0600))
	side := f.workloads["app"]
	side.ID = strings.Repeat("1", 64)
	side.Service = "side"
	side.Reference = "registry.example.com/side:1.0.0"
	f.tags[side.Reference] = lifecycleOldID
	f.workloads["side"] = side
	f.establishBaseline()
	return f
}

// A dependent whose tag moved since the baseline is refused before any effect.
func TestLifecycleRestartDependentMovedTagRefusedBeforeEffects(t *testing.T) {
	f := restartDependentFixture(t)
	f.tags["registry.example.com/side:1.0.0"] = "sha256:" + strings.Repeat("c", 64)
	intent := f.intent("rebuild")
	intent.Request.Services = []string{"app"}
	ups := 0
	f.up = func(context.Context, *compose.Project) error { ups++; return nil }
	result := f.wait(f.admit(intent))
	require.Zero(t, ups)
	require.Equal(t, activity.StatusFailed, result.Status)
	require.NotNil(t, result.Activity.ImageLifecycle)
	require.Equal(t, "update.target-changed", result.Activity.ImageLifecycle.Code)
}

// A recreated dependent that runs another image after the deploy is detected.
func TestLifecycleRestartDependentPostDeployMismatchDetected(t *testing.T) {
	f := restartDependentFixture(t)
	intent := f.intent("rebuild")
	intent.Request.Services = []string{"app"}
	// The dependent's tag moves during the deploy, so compose recreates it on another image.
	f.up = func(context.Context, *compose.Project) error {
		f.tags["registry.example.com/side:1.0.0"] = lifecycleNewID
		return nil
	}
	result := f.wait(f.admit(intent))
	require.Equal(t, activity.StatusFailed, result.Status)
	require.NotNil(t, result.Activity.ImageLifecycle)
	require.Equal(t, "update.target-changed", result.Activity.ImageLifecycle.Code)
}

func TestLifecycleSingleServiceRecreatesRestartDependent(t *testing.T) {
	f := newLifecycleFixture(t)
	require.NoError(t, os.WriteFile(filepath.Join(f.project.Path, "compose.yaml"), []byte(lifecycleOriginal+"  side:\n    image: registry.example.com/side:1.0.0\n    depends_on:\n      app:\n        condition: service_started\n        restart: true\n"), 0600))
	side := f.workloads["app"]
	side.ID = strings.Repeat("1", 64)
	side.Service = "side"
	side.Reference = "registry.example.com/side:1.0.0"
	f.tags[side.Reference] = lifecycleOldID
	f.workloads["side"] = side
	f.establishBaseline()
	intent := f.intent("rebuild")
	intent.Request.Services = []string{"app"}
	seen := false
	f.up = func(_ context.Context, model *compose.Project) error {
		seen = true
		require.Contains(t, model.Services, "side")
		return nil
	}
	result := f.wait(f.admit(intent))
	require.Equal(t, activity.StatusSuccess, result.Status, "%+v", result)
	require.True(t, seen)
}
