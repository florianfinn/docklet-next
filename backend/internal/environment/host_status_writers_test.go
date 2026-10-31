// Source: docklet-next/issues/100
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original structural guard for the owners of the host status.

package environment

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/florianfinn/docklet-next/types/version"
	"github.com/stretchr/testify/require"
)

// Every statement that writes environments.status, by file. A transport-loss writer must
// keep pending and the update-required verdict inside its write; only the compatibility
// check (current registration) and registration/session transitions replace them.
var hostStatusWriters = map[string]int{
	"agent_access.go":    2, // compatibility verdict; Direct credential rejection (both credential-fenced)
	"service.go":         1, // credential link → pending (registration transition)
	"service_health.go":  6, // failed probe write + its two guards; hub heartbeat (id 0); connect → pending; disconnect CASE
	"service_runtime.go": 1, // startup reconcile (WHERE keeps pending/offline/update-required)
}

func TestHostStatusWritersAreTheReviewedSet(t *testing.T) {
	pattern := regexp.MustCompile(`"status"\s*:|Update\("status"|\["status"\]\s*=|status\s*=\s*\?`)
	files, err := filepath.Glob("*.go")
	require.NoError(t, err)
	found := map[string]int{}
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		body, err := os.ReadFile(file)
		require.NoError(t, err)
		if n := len(pattern.FindAll(body, -1)); n > 0 {
			found[file] = n
		}
	}
	require.Equal(t, hostStatusWriters, found, "a new host status writer needs a place in the reviewed writer table")
}

func TestGenericHostUpdateRefusesStatus(t *testing.T) {
	f := newAgentFixture(t, "edge", version.Info{})
	_, err := f.service.UpdateEnvironment(t.Context(), f.id, map[string]any{"status": "online"}, nil, nil)
	require.Error(t, err)
}
