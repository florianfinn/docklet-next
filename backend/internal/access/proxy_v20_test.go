// Source: docklet-next/issues/492
// Revision: 52bae4d0610f04e66451664e2c0741699c901873
// Retrieved: 2026-11-20
// License: Apache-2.0
// Deviations: Original exact M28 admin declarations and simple-user refusal proof.

package access

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestM28ProxyAdminGates(t *testing.T) {
	for operation, path := range map[string]string{
		"check-proxy-certificate":      "/api/modules/proxy/instances/example/certificate-check",
		"create-proxy-resolver":        "/api/modules/proxy/instances/example/resolvers",
		"check-host-proxy-certificate": "/api/environments/example/modules/proxy/certificate-check",
		"create-host-proxy-resolver":   "/api/environments/example/modules/proxy/resolvers",
	} {
		t.Run(operation, func(t *testing.T) {
			actual, env, params := Match("POST", path)
			require.Equal(t, operation, actual)
			t.Run("admin allowed with an explicit declaration", func(t *testing.T) {
				// Administrators bypass resource filtering; require the declaration independently.
				rule, listed := Rules[actual]
				require.True(t, listed)
				require.Equal(t, Admin, rule.Kind)
				require.Equal(t, Allow, Decide(t.Context(), Principal{ID: "example-admin", Administrator: true}, actual, env, params))
			})
			t.Run("simple user refused", func(t *testing.T) {
				require.Equal(t, AdminRequired, Decide(t.Context(), Principal{ID: "example-user"}, actual, env, params))
			})
		})
	}
	t.Run("unlisted operation refused", func(t *testing.T) {
		operation, env, params := Match("POST", "/api/modules/proxy/instances/example/unlisted")
		require.Empty(t, operation)
		require.Equal(t, AdminRequired, Decide(t.Context(), Principal{ID: "example-user"}, operation, env, params))
	})
}

// M38 #595: the fresh container check is administrator-only like the container update.
func TestM38ContainerCheckAdminGate(t *testing.T) {
	actual, env, params := Match("POST", "/api/environments/example/updater/containers/example-container/check")
	require.Equal(t, "check-container-update", actual)
	require.Equal(t, Admin, Rules[actual].Kind)
	require.Equal(t, AdminRequired, Decide(t.Context(), Principal{ID: "example-user"}, actual, env, params))
}
