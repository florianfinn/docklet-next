// Source: docklet-next/issues/438
// Revision: 57e835f7cce5778fd3d164298e5b488e11abb71d
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original scalar verified service request validation and retry identity proofs.

package updater

import (
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func TestVerifiedServiceRequestOwnsOneServiceAndDigest(t *testing.T) {
	request := LifecycleRequest{Service: "app", ExpectedManifest: "sha256:" + strings.Repeat("a", 64)}
	require.True(t, request.VerifiedService())
	require.Equal(t, []string{"app"}, request.SelectedServices())
	changed := request
	changed.ExpectedManifest = "sha256:" + strings.Repeat("b", 64)
	require.False(t, request.Equal(changed))
	changed = request
	changed.Service = "other"
	require.False(t, request.Equal(changed))
	for _, change := range []string{"invalid-digest", "missing-service", "services", "restore", "configuration"} {
		t.Run(change, func(t *testing.T) {
			r := request
			switch change {
			case "invalid-digest":
				r.ExpectedManifest = "sha256:invalid"
			case "missing-service":
				r.Service = ""
			case "services":
				r.Services = []string{"app", "other"}
			case "restore":
				r.PointID = "restore-point"
			case "configuration":
				r.ReplaceConfiguration = true
			}
			require.False(t, r.VerifiedService())
		})
	}
	ordinary := LifecycleRequest{Services: []string{"app", "other"}}
	require.True(t, ordinary.VerifiedService())
	require.Equal(t, ordinary.Services, ordinary.SelectedServices())
}
