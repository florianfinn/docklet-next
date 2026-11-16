// Source: docklet-next/issues/471
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Original hostile argv evidence through the real Docker exec sender and receiver.

package traefik

import (
	"testing"

	dto "github.com/florianfinn/docklet-next/types/proxy"
	"github.com/stretchr/testify/require"
)

func TestP16HostileArgumentsReachRealDockerExec(t *testing.T) {
	x := fixture(t)
	file := "/etc/caddy/$(command); 'quote' `command`/Caddyfile"
	host := "app'`;$(command).example.com"
	url := "http://127.0.0.1:8080/$(command);'quote'?value=a;b"
	for _, kind := range []dto.Kind{dto.Caddy, dto.Traefik} {
		for _, action := range []string{"version", "validate", "adapt", "reload", "probe", "admin"} {
			if kind == dto.Traefik && (action == "validate" || action == "adapt" || action == "reload") {
				continue
			}
			t.Run(string(kind)+"/"+action, func(t *testing.T) {
				i := x.i
				i.Kind = kind
				before := len(x.calls)
				_, err := x.d.Exec(t.Context(), i, action, file, host, url)
				require.NoError(t, err)
				require.Len(t, x.calls, before+1)
				var expected []string
				switch action {
				case "version":
					expected = []string{string(kind), "version"}
				case "validate", "adapt", "reload":
					expected = []string{"caddy", action, "--config", file, "--adapter", "caddyfile"}
					if action == "adapt" {
						expected = append(expected, "--pretty")
					}
				default:
					expected = []string{"wget", "-Y", "off", "-S", "-T", "10", "-O", "-", "--header", "Host: " + host, url}
				}
				require.Equal(t, expected, x.calls[before], "the Docker JSON payload must preserve argument boundaries")
			})
		}
	}
}
