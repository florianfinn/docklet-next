// Source: docklet-next/issues/471
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Adopted from the M27 closing verify 2 counterexamples (C-B2).

package traefik

import (
	"github.com/florianfinn/docklet-next/backend/internal/proxy"
	"github.com/florianfinn/docklet-next/backend/internal/proxy/hostio"
	dto "github.com/florianfinn/docklet-next/types/proxy"
	"github.com/stretchr/testify/require"
	"net/http"
	"testing"
)

func TestReviewerVerify2TransientStaticArchiveRead(t *testing.T) {
	for _, mode := range []string{"explicit-static", "default-finder"} {
		for _, stage := range []string{"prepare", "validate"} {
			t.Run(mode+"/"+stage, func(t *testing.T) {
				x := fixture(t)
				configPath := "/etc/traefik/traefik.toml"
				if mode == "explicit-static" {
					configPath = "/etc/traefik/traefik.yml"
					x.inspect.Config.Cmd = []string{"--configfile=" + configPath}
					x.put("traefik.yml", "providers:\n  file:\n    directory: /etc/traefik/dynamic\nentryPoints:\n  web:\n    address: ':80'\n")
				}
				healthy, err := x.e.Inventory(t.Context(), x.i)
				require.NoError(t, err)
				require.True(t, healthy.Eligibility.Writable)
				a, err := proxy.NewAgent(x.db.DB, x.d, proxy.Engines{dto.Traefik: x.e})
				require.NoError(t, err)
				q := x.request("edit-route")
				q.Generation = a.Generation()
				q.Instance.Generation = q.Generation
				q.Instance.Registration = q.Registration
				q.Change.Name = "renamed"
				if stage == "prepare" {
					x.archiveStatus = map[string]int{configPath: http.StatusServiceUnavailable}
				} else {
					x.probeFunc = func([]string) hostio.ExecResult {
						x.archiveStatus = map[string]int{configPath: http.StatusServiceUnavailable}
						return response(200, "old")
					}
				}
				got, applyErr := a.Apply(t.Context(), q)
				x.archiveStatus = nil
				x.probeFunc = nil
				stored, err := a.Query(t.Context(), dto.Query{ID: q.ID, Instance: q.Instance, Generation: q.Generation, Registration: q.Registration})
				require.NoError(t, err)
				t.Logf("%s: returned=%s/%s error=%v persisted=%s/%s", mode+"/"+stage+"/"+string(got.Phase), got.State, got.Reason, applyErr, stored.State, stored.Reason)
				require.Equal(t, tiny, x.read("dynamic/routes.yml"))
				recovered, err := x.e.Inventory(t.Context(), x.i)
				require.NoError(t, err)
				require.True(t, recovered.Eligibility.Writable)
				require.Error(t, applyErr, "a transient Docker read during preparation must remain an error")
			})
		}
	}
}
