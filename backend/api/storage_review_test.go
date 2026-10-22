// Source: docklet-next/docs/design/storage-workspace.md
// Revision: ce3c4ea4af5b0bc1cdb2bb287d30281bbe20f0a8
// Retrieved: 2026-10-22
// License: Apache-2.0
// Deviations: Original R1 regression coverage for issue #179.
package api

import (
	"testing"

	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/stretchr/testify/require"
)

// Reviewer test: the hub proxy permission matcher, built from the registered
// producer, maps the new static prune preview to volumes:prune rather than the
// {volumeName} read neighbour, and rename to volumes:rename.
func TestReview179ProxyPermissionsForStorageRoutes(t *testing.T) {
	matcher := authz.NewPermissionMatcher()
	matcher.CollectFromHumaAPI(SetupAPIForSpec())
	for _, tc := range []struct{ method, suffix, perm string }{
		{"GET", "/volumes/prune", authz.PermVolumesPrune},
		{"POST", "/volumes/prune", authz.PermVolumesPrune},
		{"GET", "/volumes/prune-me", authz.PermVolumesRead},
		{"POST", "/volumes/data/rename", authz.PermVolumesRename},
	} {
		perm, ok := matcher.Lookup(tc.method, tc.suffix).Get()
		require.True(t, ok, tc.suffix)
		require.Equal(t, tc.perm, perm, tc.method+" "+tc.suffix)
	}
}
