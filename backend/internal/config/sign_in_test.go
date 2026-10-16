// Source: docklet-next/issues/68
// Revision: 92e896e926aaaf0189c72dcec6e3b15a52ddbd5d
// Retrieved: 2026-10-16
// License: Apache-2.0
// Deviations: Original startup validation tests.

package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/florianfinn/docklet-next/types/auth"
	"github.com/stretchr/testify/require"
)

func TestSignInStartupValidation(t *testing.T) {
	for _, tc := range []struct {
		name, mode, peers, header string
		valid                     bool
	}{
		{"default", "", "", "", true},
		{"local", "local", "", "", true},
		{"none", "none", "", "", true},
		{"unknown", "other", "", "", false},
		{"multiple", "local,proxy", "192.0.2.10", "", false},
		{"case", "LOCAL", "", "", false},
		{"proxy missing peers", "proxy", "", "", false},
		{"proxy bad peer", "proxy", "proxy.example.com", "", false},
		{"proxy partial bad list", "proxy", "192.0.2.10,invalid", "", false},
		{"proxy empty member", "proxy", "192.0.2.10,", "", false},
		{"proxy bad header", "proxy", "192.0.2.10", "Remote User", false},
		{"proxy IPs", "proxy", "192.0.2.10,2001:db8::10", "", true},
		{"proxy CIDRs", "proxy", "192.0.2.0/24,2001:db8::/32", "X-Identity", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &Config{AuthMode: tc.mode, AuthTrustedProxies: tc.peers, AuthProxyHeader: tc.header}
			if tc.valid {
				require.NoError(t, cfg.ValidateSignIn())
			} else {
				require.Error(t, cfg.ValidateSignIn())
			}
		})
	}
}

func TestSignInEnvAndFileVariants(t *testing.T) {
	for _, suffix := range []string{"", "_FILE", "__FILE"} {
		t.Run(suffix, func(t *testing.T) {
			for key, value := range map[string]string{"AUTH_MODE": "proxy", "AUTH_PROXY_HEADER": "X-Identity", "AUTH_TRUSTED_PROXIES": "192.0.2.10", "AUTH_RESET_ADMIN_PASSWORD": "example"} {
				t.Setenv(key, "")
				t.Setenv(key+"_FILE", "")
				t.Setenv(key+"__FILE", "")
				if suffix != "" {
					path := filepath.Join(t.TempDir(), key)
					require.NoError(t, os.WriteFile(path, []byte(value), 0o600))
					t.Setenv(key+suffix, path)
				} else {
					t.Setenv(key, value)
				}
			}
			cfg := Load()
			require.NoError(t, cfg.ValidateSignIn())
			require.Equal(t, auth.SignInModeProxy, cfg.SignInMode())
			require.Equal(t, "X-Identity", cfg.ProxyIdentityHeader())
			require.Equal(t, "192.0.2.10", cfg.AuthTrustedProxies)
			require.Equal(t, "example", cfg.AuthResetAdminPassword)
			require.Equal(t, "****", cfg.MaskSensitive()["AUTH_RESET_ADMIN_PASSWORD"])
		})
	}
}

func TestSignInAgentDoesNotUseHubPolicy(t *testing.T) {
	cfg := &Config{AgentMode: true, AuthMode: "proxy"}
	require.NoError(t, cfg.ValidateSignIn())
}
