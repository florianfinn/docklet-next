// Source: docklet-next/issues/493
// Revision: 54fd54b6bf259e09c1d4fa66ab1cd7ebdebf09e6
// Retrieved: 2026-11-20
// License: Apache-2.0
// Deviations: Original bounded probe client and certificate observation integration.

package hostio

import (
	"encoding/json/v2"
	"strings"
	"testing"

	dto "github.com/florianfinn/docklet-next/types/proxy"
	"github.com/stretchr/testify/require"
)

func tlsHTTP() string {
	return strings.TrimSuffix(helperHTTP(401), "}") + `,"tls":{"version":"TLS 1.3","sni":"tls.example.test","peer":{"subject":"CN=tls.example.test","issuer":"CN=Example CA","sans":["tls.example.test"],"notBefore":"2026-01-01T00:00:00Z","notAfter":"2027-01-01T00:00:00Z","sha256":"` + strings.Repeat("b", 64) + `"},"chain":["` + strings.Repeat("b", 64) + `"],"verify":{"chainTrusted":false,"hostnameMatch":true,"validity":"valid","error":"chain-untrusted"}}}`
}
func TestP1P2ExactArgumentsAndFirstResponse(t *testing.T) {
	args, tls, err := ProbeArgs("host.example.test:443", "https://127.0.0.1:8443/api?key=value", "tls.example.test")
	require.NoError(t, err)
	require.True(t, tls)
	require.Equal(t, []string{"/usr/bin/docklet-probe", "http", "--connect", "127.0.0.1:8443", "--host", "host.example.test:443", "--target", "/api?key=value", "--timeout", "10s", "--max-body", "65536", "--tls", "--sni", "tls.example.test"}, args)
	r, err := ParseHelperProbe(ExecResult{Stdout: []byte(helperHTTP(302))}, false)
	require.NoError(t, err)
	require.Equal(t, 302, r.Status)
	require.False(t, r.Followed)
	for _, url := range []string{"http://192.0.2.1/", "http://example.test/", "http://user@127.0.0.1/", "http://127.0.0.1/#fragment"} {
		_, _, err := ProbeArgs("example.test", url, "")
		require.Error(t, err)
	}
}
func TestP3IndependentVerificationPreserved(t *testing.T) {
	for _, chain := range []bool{false, true} {
		for _, name := range []bool{false, true} {
			for _, validity := range []dto.CertificateValidity{dto.CertificateValid, dto.CertificateExpired, dto.CertificateNotYetValid} {
				var raw map[string]any
				require.NoError(t, json.Unmarshal([]byte(tlsHTTP()), &raw))
				verify := raw["tls"].(map[string]any)["verify"].(map[string]any)
				verify["chainTrusted"] = chain
				verify["hostnameMatch"] = name
				verify["validity"] = validity
				b, err := json.Marshal(raw)
				require.NoError(t, err)
				r, err := ParseHelperProbe(ExecResult{Stdout: b}, true)
				require.NoError(t, err)
				require.Equal(t, 401, r.Status)
				require.Equal(t, chain, r.Certificate.ChainTrusted)
				require.Equal(t, name, r.Certificate.HostnameMatch)
				require.Equal(t, validity, r.Certificate.Validity)
			}
		}
	}
}
func TestP10RejectsMalformedAndContradictoryJSON(t *testing.T) {
	for _, tt := range []struct {
		name, body string
		tls        bool
		exit       int
	}{
		{name: "oversize", body: strings.Repeat(" ", MaxProbeJSON) + helperHTTP(200)},
		{name: "wrong-version", body: strings.Replace(helperHTTP(200), `"v":1`, `"v":2`, 1)},
		{name: "status-and-error", body: strings.TrimSuffix(helperHTTP(200), "}") + `,"error":"timeout"}`},
		{name: "exit3-with-status", body: helperHTTP(200), exit: 3},
		{name: "missing-status", body: `{"v":1}`},
		{name: "tls-unrequested", body: tlsHTTP()},
		{name: "tls-missing", body: helperHTTP(200), tls: true},
		{name: "flag-missing", body: strings.Replace(tlsHTTP(), `"chainTrusted":false,`, "", 1), tls: true},
		{name: "subject-bound", body: strings.Replace(tlsHTTP(), "CN=tls.example.test", strings.Repeat("x", 257), 1), tls: true},
		{name: "bad-hash", body: strings.Replace(helperHTTP(200), strings.Repeat("a", 64), "bad", 1)},
		{name: "two-values", body: helperHTTP(200) + helperHTTP(200)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseHelperProbe(ExecResult{Stdout: []byte(tt.body), ExitCode: tt.exit}, tt.tls)
			require.Error(t, err)
		})
	}
	body := strings.Replace(helperHTTP(200), `"headers":{}`, `"headers":{"WWW-Authenticate":" Basic realm=example "}`, 1)
	r, err := ParseHelperProbe(ExecResult{Stdout: []byte(body)}, false)
	require.NoError(t, err)
	require.Equal(t, "Basic realm=example", r.Headers["www-authenticate"])
	transport := `{"v":1,"error":"connect-refused"}`
	r, err = ParseHelperProbe(ExecResult{Stdout: []byte(transport), ExitCode: 3}, false)
	require.NoError(t, err)
	require.Zero(t, r.Status)
}
