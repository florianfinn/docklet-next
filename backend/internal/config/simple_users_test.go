// Source: docklet-next/issues/322
// Revision: 6365cc5cebd04f199d15fe5f53c006a127d0a669
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original simple-user resource authorization.

package config

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestProxyLogoutURLValidation(t *testing.T) {
	for _, value := range []string{"javascript:alert(1)", "/logout", "https:/logout", "ftp://proxy.example.com/logout"} {
		c := &Config{AuthProxyLogoutURL: value}
		require.Error(t, c.ValidateSignIn(), value)
	}
	for _, value := range []string{"", "http://proxy.example.com/logout", "https://proxy.example.com/logout?next=%2F"} {
		c := &Config{AuthProxyLogoutURL: value}
		require.NoError(t, c.ValidateSignIn(), value)
	}
}
func TestProxyAdminListPreservesUnsetAndExactValues(t *testing.T) {
	t.Setenv("AUTH_MODE", "local")
	c := &Config{}
	loadFromEnv(c)
	require.Nil(t, c.AuthProxyAdminUsers)
	t.Setenv("AUTH_PROXY_ADMIN_USERS", "")
	loadFromEnv(c)
	require.NotNil(t, c.AuthProxyAdminUsers)
	require.Empty(t, *c.AuthProxyAdminUsers)
	t.Setenv("AUTH_PROXY_ADMIN_USERS", "Operator,viewer@example.com")
	loadFromEnv(c)
	require.Equal(t, "Operator,viewer@example.com", *c.AuthProxyAdminUsers)
}
