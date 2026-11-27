// Source: docklet-next/issues/673
// Revision: 99c1b0b8c4dfe56182c72749e092b08202436fa5
// Retrieved: 2026-11-27
// License: Apache-2.0
// Deviations: Original regression proofs.

package projects

import (
	"context"
	compose "github.com/compose-spec/compose-go/v2/types"
	project "github.com/florianfinn/docklet-next/types/project"
	"github.com/stretchr/testify/require"
	build "go.getarcane.app/builds/types"
	"io"
	"testing"
)

func TestSiblingBuildNeverPulled(t *testing.T) {
	for range 30 {
		model := &compose.Project{Name: "example", WorkingDir: t.TempDir(), Services: compose.Services{
			"builder":  {Name: "builder", Image: "example.com/local:1", Build: &compose.BuildConfig{Context: "."}, PullPolicy: "build"},
			"consumer": {Name: "consumer", Image: "example.com/local:1", PullPolicy: "always"},
		}}
		built := false
		pulls := 0
		err := NewCoordinator(project.ComposeCommands{}).(*composeCoordinatorInternal).PrepareImagesForDeploy(t.Context(), "example", model, nil, project.ComposeImageOperations{
			Exists: func(context.Context, string) (bool, error) { return built, nil },
			Pull:   func(context.Context, string, io.Writer) error { pulls++; return nil },
			Build:  func(context.Context, build.BuildRequest, io.Writer, string) error { built = true; return nil },
		}, "always")
		require.NoError(t, err)
		require.True(t, built)
		require.Zero(t, pulls)
		require.Equal(t, "never", model.Services["consumer"].PullPolicy)
	}
}
