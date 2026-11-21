// Source: docklet-next/issues/549
// Revision: 1d8ef389b0a40f3ba27fb09f8f59d8ac993180ff
// Retrieved: 2026-11-21
// License: Apache-2.0
// Deviations: Original UI TLS contract proof through the real Traefik Prepare patcher.

package traefik

import (
	"strings"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/proxy"
	dto "github.com/florianfinn/docklet-next/types/proxy"
	"github.com/stretchr/testify/require"
)

func TestUITLSNoRouteCertificateHasNoTLSKey(t *testing.T) {
	for _, kind := range []string{"plain", "static-tls"} {
		t.Run(kind, func(t *testing.T) {
			x := fixture(t)
			if kind == "static-tls" {
				x.inspect.Config.Cmd = append(x.inspect.Config.Cmd, "--entrypoints.web.http.tls=true")
			}
			q := x.request("create-route")
			q.Target = "dynamic/fresh.yml"
			q.Expected = dto.FileState{}
			q.Change = dto.Change{Kind: "create-route", Name: "fresh", Rule: "Host(`fresh.example.com`)", Upstreams: []string{"http://192.0.2.2:8080"}}
			in := x.prepare(q)
			d, err := parseDocument(string(in.New.Content), q.Target)
			require.NoError(t, err)
			require.Nil(t, at(d.root, "http", "routers", "fresh", "tls"), "no resolver must not introduce any route TLS key")
		})
	}

}

func TestUITLSKeepCurrentPreservesEntryPointsAndTLSBytes(t *testing.T) {
	x := fixture(t)
	// Listener TLS and the complete router TLS subtree are deliberately independent.
	tls := "      tls: # route settings\n        domains:\n          - main: example.com\n            sans: [app.example.com]\n"
	old := strings.Replace(tiny, "      entryPoints: [web]\n", "      entryPoints: [web] # explicit listener\n"+tls, 1)
	x.put("dynamic/routes.yml", old)
	q := x.request("edit-route")
	q.Change.Upstreams = []string{"http://192.0.2.2:8080"}
	in := x.prepare(q)
	require.Equal(t, strings.Replace(old, "http://192.0.2.1:8080", "http://192.0.2.2:8080", 1), string(in.New.Content), "Keep current must leave entryPoints and the entire TLS subtree byte-identical")
	require.Equal(t, proxy.Snapshot([]byte(old), true), in.Old)
}
