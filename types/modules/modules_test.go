// Source: docklet-next/issues/472
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-15
// License: Apache-2.0
// Deviations: Original bounded typed module refusal classification checks.

package modules

import (
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func TestDisabledRefusalRequiresMatchingBoundedProblem(t *testing.T) {
	path := "/api/environments/0/modules/proxy/read"
	valid := []byte(`{"status":403,"code":"feature-disabled","feature":"proxy"}`)
	require.True(t, DisabledRefusal(403, path, valid))
	for _, body := range [][]byte{[]byte(`{"status":403,"code":"feature-disabled","feature":"unknown"}`), []byte(`{"code":"feature-disabled","feature":"proxy"}`), []byte(`{"status":403,"code":"access.admin-required","feature":"proxy"}`), append(valid, []byte(strings.Repeat(" ", 16<<10))...)} {
		require.False(t, DisabledRefusal(403, path, body))
	}
	require.False(t, DisabledRefusal(401, path, valid))
	require.False(t, DisabledRefusal(403, "/api/environments/0/containers", valid))
}
