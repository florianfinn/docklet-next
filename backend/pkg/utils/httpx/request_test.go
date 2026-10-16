// Source: getarcaneapp/arcane/backend/pkg/utils/httpx/request_test.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Exact configured WebSocket origins and validated empty-Origin tests.

package httpx

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsWebSocketUpgradeRequest(t *testing.T) {
	tests := []struct {
		name       string
		connection string
		upgrade    string
		want       bool
	}{
		{"websocket upgrade", "Upgrade", "websocket", true},
		{"case insensitive with token list", "keep-alive, UPGRADE", "WebSocket", true},
		{"plain request", "", "", false},
		{"upgrade to h2c", "Upgrade", "h2c", false},
		{"only upgrade header from reverse proxy", "", "websocket", false},
		{"only connection upgrade from reverse proxy", "upgrade, keep-alive", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/", http.NoBody)
			if tt.connection != "" {
				r.Header.Set("Connection", tt.connection)
			}
			if tt.upgrade != "" {
				r.Header.Set("Upgrade", tt.upgrade)
			}
			{
				got := IsWebSocketUpgradeRequest(r)
				assert.Equal(t, tt.want, got,
					"IsWebSocketUpgradeRequest() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestValidateWebSocketOrigin(t *testing.T) {
	for _, tc := range []struct {
		origin              string
		authenticated, want bool
	}{
		{"https://hub.example.com", false, true},
		{"http://hub.example.com", true, false},
		{"https://hub.example.com/", true, false},
		{"https://hub.example.com?query", true, false},
		{"https://operator@hub.example.com", true, false},
		{"https://hub.example.com#fragment", true, false},
		{"https://other.example.com", true, false},
		{"http://localhost:8080", true, false},
		{"null", true, false},
		{"", false, false},
		{"", true, true},
	} {
		t.Run(tc.origin, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "http://other.example.com", http.NoBody)
			req.Header.Set("Origin", tc.origin)
			check := ValidateWebSocketOrigin([]string{"https://hub.example.com"}, func(*http.Request) bool { return tc.authenticated })
			assert.Equal(t, tc.want, check(req))
			req.Header.Add("Origin", "https://hub.example.com")
			assert.False(t, check(req))
		})
	}
	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	assert.False(t, ValidateWebSocketOrigin([]string{"https://hub.example.com"}, nil)(req))
}
