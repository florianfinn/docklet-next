// Source: docklet-next/issues/673
// Revision: 99c1b0b8c4dfe56182c72749e092b08202436fa5
// Retrieved: 2026-11-27
// License: Apache-2.0
// Deviations: Original regression proofs.

package projects

import (
	"context"
	"errors"
	compose "github.com/compose-spec/compose-go/v2/types"
	project "github.com/florianfinn/docklet-next/types/project"
	"github.com/stretchr/testify/require"
	build "go.getarcane.app/builds/types"
	"io"
	"testing"
)

// The real deploy path passes the defaultDeployPullPolicy setting; a builder without
// pull_policy whose image no registry serves is built, and its sibling is never pulled.
func TestSiblingBuildWithoutPullPolicyUnderDeployDefault(t *testing.T) {
	for _, override := range []string{"missing", "always"} {
		model := &compose.Project{Name: "example", WorkingDir: t.TempDir(), Services: compose.Services{
			"builder":  {Name: "builder", Image: "example.com/local:1", Build: &compose.BuildConfig{Context: "."}},
			"consumer": {Name: "consumer", Image: "example.com/local:1"},
		}}
		built := false
		var pulled []string
		err := NewCoordinator(project.ComposeCommands{}).(*composeCoordinatorInternal).PrepareImagesForDeploy(t.Context(), "example", model, nil, project.ComposeImageOperations{
			Exists: func(context.Context, string) (bool, error) { return built, nil },
			Pull: func(_ context.Context, image string, _ io.Writer) error {
				pulled = append(pulled, image)
				return errors.New("not found")
			},
			Build: func(context.Context, build.BuildRequest, io.Writer, string) error { built = true; return nil },
		}, override)
		require.NoError(t, err, override)
		require.True(t, built, override)
		require.Equal(t, []string{"example.com/local:1"}, pulled, "only the builder tries its registry once")
		require.Equal(t, "never", model.Services["consumer"].PullPolicy)
	}
}

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
