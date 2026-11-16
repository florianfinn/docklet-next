// Source: docklet-next/issues/471
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Original hostile-input capture for every proxy command builder.

package hostio

import (
	"context"
	"testing"

	dto "github.com/florianfinn/docklet-next/types/proxy"
	"github.com/stretchr/testify/require"
)

func TestP16EveryCommandCapturesHostileValuesAsSingleArguments(t *testing.T) {
	file := "/etc/caddy/$(touch injected); 'quoted' `command`/Caddyfile"
	host := "app'`;$(command).example.com"
	rawURL := "http://127.0.0.1:8080/$(command);'quoted'?value=a;b"
	for _, kind := range []dto.Kind{dto.Traefik, dto.Caddy} {
		for _, action := range []string{"version", "validate", "adapt", "reload", "probe", "admin"} {
			if kind == dto.Traefik && (action == "validate" || action == "adapt" || action == "reload") {
				continue
			}
			t.Run(string(kind)+"/"+action, func(t *testing.T) {
				var captured []string
				calls := 0
				_, err := Exec(t.Context(), func(_ context.Context, args []string, _ int) (ExecResult, error) {
					calls++
					captured = append([]string(nil), args...)
					return ExecResult{}, nil
				}, kind, action, file, host, rawURL)
				require.NoError(t, err)
				require.Equal(t, 1, calls)
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
					expected = []string{"wget", "-Y", "off", "-S", "-T", "10", "-O", "-", "--header", "Host: " + host, rawURL}
				}
				require.Equal(t, expected, captured)
			})
		}
	}
	for _, action := range []string{"probe", "admin"} {
		t.Run(action+"/https", func(t *testing.T) {
			var captured []string
			_, err := Exec(t.Context(), func(_ context.Context, args []string, _ int) (ExecResult, error) {
				captured = args
				return ExecResult{}, nil
			}, dto.Caddy, action, "", host, "https://127.0.0.1:8443/hostile;path")
			require.NoError(t, err)
			require.Equal(t, []string{"wget", "-Y", "off", "-S", "-T", "10", "-O", "-", "--header", "Host: " + host, "--no-check-certificate", "https://127.0.0.1:8443/hostile;path"}, captured)
		})
	}
}

func TestP16InvalidCommandValuesNeverReachExec(t *testing.T) {
	for _, bad := range []string{"-option", "bad\nvalue", "bad\rvalue", "bad\x00value", "bad\tvalue"} {
		for _, action := range []string{"validate", "adapt", "reload", "probe", "admin"} {
			t.Run(action+"/"+bad, func(t *testing.T) {
				file, host, rawURL := "/etc/caddy/Caddyfile", "app.example.com", "http://127.0.0.1:8080/"
				if action == "probe" || action == "admin" {
					host = bad
				} else if bad[0] == '-' {
					file = bad
				} else {
					file = "/etc/caddy/" + bad
				}
				calls := 0
				run := func(context.Context, []string, int) (ExecResult, error) { calls++; return ExecResult{}, nil }
				_, err := Exec(t.Context(), run, dto.Caddy, action, file, host, rawURL)
				require.ErrorContains(t, err, "invalid-input")
				require.Zero(t, calls)
				if action == "probe" || action == "admin" {
					_, err = Exec(t.Context(), run, dto.Caddy, action, "", "app.example.com", bad)
					require.ErrorContains(t, err, "invalid-input")
					require.Zero(t, calls)
				}
			})
		}
	}
}
