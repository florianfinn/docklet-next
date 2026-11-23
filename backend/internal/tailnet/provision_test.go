// Source: docklet-next/issues/573
// Revision: b4807f4912bc07f3e51abaf552a69f2223cabd27
// Retrieved: 2026-11-22
// License: Apache-2.0
// Deviations: Original Tailscale API error table and key payload tests.

package tailnet_test

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/florianfinn/docklet-next/backend/internal/tailnet"
	"github.com/florianfinn/docklet-next/backend/internal/tailnet/tailnettest"
	tailnettypes "github.com/florianfinn/docklet-next/types/tailnet"
)

var clientKey = tailnettest.Key("client", "example-secret")

func TestMintHostKeyRequestsAOneUseTaggedOneDayKey(t *testing.T) {
	var body map[string]any
	var auth string
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/oauth/token":
			require.NoError(t, r.ParseForm())
			require.Equal(t, "client_credentials", r.PostForm.Get("grant_type"))
			require.Equal(t, clientKey, r.PostForm.Get("client_secret"))
			_, _ = w.Write([]byte(`{"access_token":"test-only","expires_in":3600}`))
		case "/api/v2/tailnet/-/keys":
			auth = r.Header.Get("Authorization")
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			_, _ = w.Write([]byte(`{"id":"k1","key":"tskey-auth-example-host"}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	t.Cleanup(provider.Close)
	key, err := (&tailnet.Provisioner{BaseURL: provider.URL}).MintHostKey(t.Context(), "example-client", clientKey)
	require.NoError(t, err)
	require.Equal(t, "tskey-auth-example-host", key)
	require.Equal(t, "Bearer test-only", auth)
	create := body["capabilities"].(map[string]any)["devices"].(map[string]any)["create"].(map[string]any)
	require.Equal(t, false, create["reusable"])
	require.Equal(t, false, create["ephemeral"])
	require.Equal(t, true, create["preauthorized"])
	require.Equal(t, []any{tailnet.AgentTag}, create["tags"])
	require.EqualValues(t, 86400, body["expirySeconds"])
}

func TestProviderErrorsMapToCodesAndNeverReturnAKey(t *testing.T) {
	big := strings.Repeat("x", 70<<10)
	for _, tc := range []struct {
		name           string
		token, mint    func(w http.ResponseWriter, r *http.Request)
		code           string
		providerStatus int
	}{
		{"token 401 invalid client", status(http.StatusUnauthorized, `{"error":"invalid_client"}`), nil, tailnettypes.CodeCredentialRejected, 401},
		{"mint 401 revoked", nil, status(http.StatusUnauthorized, `{}`), tailnettypes.CodeCredentialRejected, 401},
		{"mint 403 tag not owned", nil, status(http.StatusForbidden, `{"message":"requested tags are invalid or not permitted"}`), tailnettypes.CodeCredentialRejected, 403},
		{"mint 429", nil, status(http.StatusTooManyRequests, `{}`), tailnettypes.CodeKeyMintFailed, 429},
		{"mint 500", nil, status(http.StatusInternalServerError, `{}`), tailnettypes.CodeKeyMintFailed, 500},
		{"mint 503", nil, status(http.StatusServiceUnavailable, `{}`), tailnettypes.CodeKeyMintFailed, 503},
		{"mint malformed body", nil, status(http.StatusOK, `{"key":`), tailnettypes.CodeKeyMintFailed, 200},
		{"mint body without key", nil, status(http.StatusOK, `{"key":"not-a-key"}`), tailnettypes.CodeKeyMintFailed, 200},
		{"mint oversized body", nil, status(http.StatusOK, `{"key":"tskey-auth-x","pad":"`+big+`"}`), tailnettypes.CodeKeyMintFailed, 200},
		{"token oversized body", status(http.StatusOK, jsonObject("access_token", big)), nil, tailnettypes.CodeKeyMintFailed, 200},
		{"mint ambiguous: connection dropped after the request", nil, hangUp, tailnettypes.CodeKeyMintFailed, 0},
		{"mint timeout", nil, func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }, tailnettypes.CodeKeyMintFailed, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				if r.URL.Path == "/api/v2/oauth/token" {
					if tc.token != nil {
						tc.token(w, r)
						return
					}
					_, _ = w.Write([]byte(`{"access_token":"test-only"}`))
					return
				}
				tc.mint(w, r)
			}))
			t.Cleanup(provider.Close)
			p := &tailnet.Provisioner{BaseURL: provider.URL, Client: &http.Client{Timeout: 300 * time.Millisecond}}
			key, err := p.MintHostKey(t.Context(), "example-client", clientKey)
			require.Empty(t, key)
			var problem *tailnettypes.Problem
			require.ErrorAs(t, err, &problem)
			require.Equal(t, tc.code, problem.Code)
			require.Equal(t, tc.providerStatus, problem.ProviderStatus)
			require.NotContains(t, err.Error(), "tskey-client")
		})
	}
}

func status(code int, body string) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(code)
		_, _ = w.Write([]byte(body))
	}
}

func jsonObject(name, value string) string {
	data, _ := json.Marshal(map[string]string{name: value})
	return string(data)
}

func hangUp(w http.ResponseWriter, _ *http.Request) {
	conn, _, err := w.(http.Hijacker).Hijack()
	if err == nil {
		_ = conn.(*net.TCPConn).SetLinger(0)
		_ = conn.Close()
	}
}

func TestPolicySnippetGrantsOnlyAgentToHubIngress(t *testing.T) {
	var policy struct {
		TagOwners map[string][]string `json:"tagOwners"`
		Grants    []struct {
			Src []string `json:"src"`
			Dst []string `json:"dst"`
			IP  []string `json:"ip"`
		} `json:"grants"`
	}
	require.NoError(t, json.Unmarshal([]byte(tailnet.PolicySnippet()), &policy))
	require.Contains(t, policy.TagOwners, tailnet.HubTag)
	require.Contains(t, policy.TagOwners, tailnet.AgentTag)
	require.Len(t, policy.Grants, 1)
	require.Equal(t, []string{tailnet.AgentTag}, policy.Grants[0].Src)
	require.Equal(t, []string{tailnet.HubTag}, policy.Grants[0].Dst)
	require.Equal(t, []string{"tcp:3552"}, policy.Grants[0].IP)
}

// The web setup steps derive tags, owner and grant from this text; their fixture must stay the producer's exact text.
func TestPolicySnippetMatchesWebFixture(t *testing.T) {
	source, err := os.ReadFile("../../../web/src/features/connections/tailnet.test-support.ts")
	require.NoError(t, err)
	_, rest, found := strings.Cut(string(source), "export const PRODUCER_POLICY = `")
	require.True(t, found)
	fixture, _, found := strings.Cut(rest, "`")
	require.True(t, found)
	require.Equal(t, tailnet.PolicySnippet(), fixture)
}
