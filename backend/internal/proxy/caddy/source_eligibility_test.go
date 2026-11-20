// Source: docklet-next/issues/528
// Revision: 54fd54b6bf259e09c1d4fa66ab1cd7ebdebf09e6
// Retrieved: 2026-11-20
// License: Apache-2.0
// Deviations: Original M28 Caddy setup eligibility through the real Docker seam (U-3, C-DR10).

package caddy

import (
	"testing"

	dto "github.com/florianfinn/docklet-next/types/proxy"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/stretchr/testify/require"
)

func TestM28CaddySourceEligibilityNeedsAnExplicitConfigInsideTheSource(t *testing.T) {
	x := fixture(t)
	x.c.Mounts = append(x.c.Mounts, container.MountPoint{Type: mount.TypeBind, Source: x.root, Destination: "/srv/data", RW: true})
	probe := func(target string) dto.SourceEligibility {
		t.Helper()
		var source dto.Source
		for _, m := range x.c.Mounts {
			if m.Destination == target {
				var err error
				source, err = x.d.Source(t.Context(), x.c, m) // as discovery derives it
				require.NoError(t, err)
			}
		}
		e, err := x.e.SourceEligibility(t.Context(), dto.Instance{Kind: dto.Caddy, EnvironmentID: "0", Identity: x.i.Identity, Source: source})
		require.NoError(t, err)
		return e
	}
	require.Equal(t, dto.SourceEligibility{Selectable: true, RootFile: "/etc/caddy/Caddyfile", RouteDirectory: &dto.RouteDirectoryHint{Value: "docklet"}}, probe("/etc/caddy"))
	require.Equal(t, dto.SourceEligibility{Reason: dto.CaddyConfigImplicit}, probe("/srv/data"), "config outside the source")

	x.c.Config.Cmd = []string{"run", "--adapter", "caddyfile"}
	require.Equal(t, dto.SourceEligibility{Reason: dto.CaddyConfigImplicit}, probe("/etc/caddy"), "no default inference without --config")
	x.c.Config.Cmd = []string{"run", "--config", "/etc/caddy/caddy.json"}
	require.Equal(t, dto.SourceEligibility{Reason: dto.UnsupportedFormat}, probe("/etc/caddy"))
}
