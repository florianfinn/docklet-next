// Source: docklet-next/docs/design/images-updates.md
// Revision: 5a7a7ad39c1817a3d222bc84c0cf91d433d48f0d
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Retired ingress refuses before legacy service dispatch on hub and agent.

package updater

import (
	"github.com/florianfinn/docklet-next/types/image"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestRetiredUpdaterHandlersDoNotDispatch(t *testing.T) {
	for _, env := range []string{"0", "example-remote"} {
		t.Run(env, func(t *testing.T) {
			h := &UpdaterHandler{}
			_, err := h.RunUpdater(t.Context(), &RunUpdaterInput{EnvironmentID: env})
			require.Equal(t, "update.path-retired", image.Neutral(err, "unexpected", 500).Problem.Code)
			_, err = h.updateContainerInternal(t.Context(), &UpdateContainerInput{EnvironmentID: env, ContainerID: "example-container", Async: true})
			require.Equal(t, "update.path-retired", image.Neutral(err, "unexpected", 500).Problem.Code)
		})
	}
}
