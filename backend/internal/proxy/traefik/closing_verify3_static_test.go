// Source: docklet-next/issues/471
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Adopted from the M27 closing verify 3 counterexamples (C-B2: static read at publication).

package traefik

import (
	"context"
	"github.com/florianfinn/docklet-next/backend/internal/proxy"
	"github.com/florianfinn/docklet-next/backend/internal/proxy/hostio"
	dto "github.com/florianfinn/docklet-next/types/proxy"
	"github.com/stretchr/testify/require"
	"net/http"
	"testing"
)

type verify3ArmAfterValidate struct {
	proxy.Engine
	arm, disarm     func()
	validated       bool
	reads, failRead int
}

func (e *verify3ArmAfterValidate) Validate(ctx context.Context, in proxy.Intent) error {
	if err := e.Engine.Validate(ctx, in); err != nil {
		return err
	}
	e.validated = true
	return nil
}

func (e *verify3ArmAfterValidate) Read(ctx context.Context, i dto.Instance) (dto.ReadResult, error) {
	if e.validated {
		e.reads++
		if e.reads == e.failRead {
			e.arm()
			defer e.disarm()
		}
	}
	return e.Engine.Read(ctx, i)
}

func TestM27StaticReadFailureAtPublicationStaysAnError(t *testing.T) {
	for _, mode := range []string{"explicit-static", "default-finder"} {
		for _, fence := range []struct {
			name string
			n    int
		}{{"agent-fence", 1}, {"writer-fence", 2}} {
			t.Run(mode+"/"+fence.name, func(t *testing.T) {
				x := fixture(t)
				configPath := "/etc/traefik/traefik.toml"
				if mode == "explicit-static" {
					configPath = "/etc/traefik/traefik.yml"
					x.inspect.Config.Cmd = []string{"--configfile=" + configPath}
					x.put("traefik.yml", "providers:\n  file:\n    directory: /etc/traefik/dynamic\nentryPoints:\n  web:\n    address: ':80'\n")
				}
				wrapped := &verify3ArmAfterValidate{Engine: x.e, failRead: fence.n, arm: func() { x.archiveStatus = map[string]int{configPath: http.StatusServiceUnavailable} }, disarm: func() { x.archiveStatus = nil }}
				x.d.Engines[dto.Traefik] = wrapped
				a, err := proxy.NewAgent(x.db.DB, x.d, proxy.Engines{dto.Traefik: wrapped})
				require.NoError(t, err)
				q := x.request("edit-route")
				q.Generation = a.Generation()
				q.Instance.Generation = q.Generation
				q.Instance.Registration = q.Registration
				q.Change.Upstreams = []string{"http://192.0.2.2:8080"}
				n := 0
				x.probeFunc = func([]string) hostio.ExecResult {
					n++
					if n <= 2 {
						return response(200, "old")
					}
					return response(200, "new")
				}
				got, applyErr := a.Apply(t.Context(), q)
				x.archiveStatus = nil
				require.GreaterOrEqual(t, wrapped.reads, fence.n, "the selected fence must run")
				stored, queryErr := a.Query(t.Context(), dto.Query{ID: q.ID, Instance: q.Instance, Generation: q.Generation, Registration: q.Registration})
				require.NoError(t, queryErr)
				t.Logf("returned=%s/%s error=%v persisted=%s/%s targetChanged=%v", got.State, got.Reason, applyErr, stored.State, stored.Reason, x.read("dynamic/routes.yml") != tiny)
				require.Error(t, applyErr, "a failed static read in the publishing fence must not be consumed as successful graph evidence")
				require.Equal(t, tiny, x.read("dynamic/routes.yml"), "must stop before publication")
				require.NotEqual(t, dto.Verified, stored.State, "the unfinished phase settles through recovery, not as verified")
			})
		}
	}
}

// The display keeps the incomplete-inventory presentation of a failed static
// read; the publication graph of the same read stays an error.
func TestM27StaticReadFailureDisplaysIncompleteInventory(t *testing.T) {
	x := fixture(t)
	a, err := proxy.NewAgent(x.db.DB, x.d, x.d.Engines)
	require.NoError(t, err)
	m := &proxy.Module{Agent: a, Runtime: x.d}
	x.i.Generation = a.Generation()
	x.archiveStatus = map[string]int{"/etc/traefik/traefik.toml": http.StatusServiceUnavailable}
	r, err := m.Read(t.Context(), x.i)
	require.NoError(t, err, "the display presents the failed read as incomplete inventory")
	require.False(t, r.Eligibility.Writable)
	require.Equal(t, dto.IncompleteInventory, r.Eligibility.Reason)
	_, err = x.d.Graph(t.Context(), x.i)
	require.Error(t, err)
	require.True(t, proxy.Transient(err), "a publication check keeps the read failure unfinished")
}
