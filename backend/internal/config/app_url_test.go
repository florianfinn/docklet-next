// Source: docklet-next/issues/68
// Revision: af5fa9df01e730f7691d650985abd33d74f51ae5
// Retrieved: 2026-10-16
// License: Apache-2.0
// Deviations: Original APP_URL canonicalization regressions.

package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSignInNormalizesAppURL(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{"https://Hub.example.com", "https://hub.example.com"},
		{"HTTPS://Hub.example.com:443", "https://hub.example.com"},
		{"http://Hub.example.com:80", "http://hub.example.com"},
		{"https://Hub.example.com:0443", "https://hub.example.com"},
		{"https://Hub.example.com:8443/", "https://hub.example.com:8443/"},
		{"https://[2001:DB8:0:0::1]:443", "https://[2001:db8::1]"},
		{"Hub.example.com", "https://hub.example.com"},
	} {
		t.Run(tc.input, func(t *testing.T) {
			cfg := &Config{AppUrl: tc.input}
			require.Equal(t, tc.want, cfg.GetAppURL())
			require.NoError(t, cfg.ValidateSignIn())
			require.Equal(t, tc.want, cfg.AppUrl)
			require.Equal(t, tc.want, cfg.GetAppURL())
		})
	}
}
