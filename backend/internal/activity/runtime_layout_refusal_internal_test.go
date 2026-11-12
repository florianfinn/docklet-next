// Source: docklet-next/issues/406
// Revision: 6e3afa6a78f4f60ffd74858d9a1eaefbb1b73bda
// Retrieved: 2026-11-12
// License: Apache-2.0
// Deviations: Original operation scope proof for the layout refusal.

package activity

import (
	"errors"
	"fmt"
	"testing"

	"github.com/florianfinn/docklet-next/backend/pkg/projects"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	"github.com/stretchr/testify/require"
)

// Deploy and other stack operations keep runtime.target-unavailable; only image
// updates and restores answer the layout refusal, local or relayed.
func TestLayoutRefusalOnlyForImageOperations(t *testing.T) {
	local := fmt.Errorf("read runtime project identity: %w", projects.ErrComposeFileNestedBase)
	relayed := fmt.Errorf("resolve: %w", imageDTO.Failure(runtimeLayoutUnsupported, 409))
	for _, err := range []error{local, relayed} {
		for _, op := range []RuntimeOperation{RuntimeUpdateProject, RuntimeRestoreProject} {
			var problem *imageDTO.OperationError
			require.True(t, errors.As(runtimeTargetFailure(op, err), &problem))
			require.Equal(t, runtimeLayoutUnsupported, problem.Problem.Code)
		}
		failure := runtimeTargetFailure(RuntimeDeployProject, err)
		var target *RuntimeTargetError
		require.True(t, errors.As(failure, &target))
		require.Equal(t, "runtime.target-unavailable", target.Code)
		var problem *imageDTO.OperationError
		require.False(t, errors.As(failure, &problem), "deploy must not relay the update refusal")
	}
	require.Nil(t, runtimeLayoutProblem("container", local))
	require.Nil(t, runtimeLayoutProblem("stack", errors.New("other")))
}
