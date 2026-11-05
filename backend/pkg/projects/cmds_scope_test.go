// Source: docklet-next/issues/343
// Revision: 89f69980c01d08714ce96c49365fb19065e3d6ba
// Retrieved: 2026-11-05
// License: Apache-2.0
// Deviations: Original scoped orphan override and ordinary-deploy merge proof.

package projects

import (
	compose "github.com/compose-spec/compose-go/v2/types"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestComposeUpScopedOrphanOptions(t *testing.T) {
	for _, preserve := range []bool{false, true} {
		for _, remove := range []bool{false, true} {
			for _, environment := range []bool{false, true} {
				options, _ := composeUpOptionsInternal(&compose.Project{Name: "example"}, nil, remove, true, 0, ComposeEnvOptions{RemoveOrphans: environment}, preserve)
				require.Equal(t, !preserve && (remove || environment), options.RemoveOrphans, "preserve=%t caller=%t environment=%t", preserve, remove, environment)
				if preserve {
					require.True(t, options.IgnoreOrphans)
				}
			}
		}
	}
}
