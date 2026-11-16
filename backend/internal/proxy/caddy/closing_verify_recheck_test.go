// Source: docklet-next/issues/474
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Adopted from the M27 closing verify counterexamples (C-B2/C-B4).

package caddy

import (
	"github.com/florianfinn/docklet-next/backend/internal/proxy/hostio"
	dto "github.com/florianfinn/docklet-next/types/proxy"
	"github.com/stretchr/testify/require"
	"net/http"
	"strings"
	"testing"
)

func TestVerifyTransientValidationAndFinalFenceReadsAreNotInvalidGraph(t *testing.T) {
	for _, stage := range []string{"validation-graph", "final-check", "final-target", "final-graph"} {
		t.Run(stage, func(t *testing.T) {
			x := fixture(t)
			site := baseSite()
			text, err := site.Render()
			require.NoError(t, err)
			x.owned("routes/app.caddy", text)
			next := site
			next.Name = "renamed"
			q := x.request("edit-route", next)
			validated, probed, failed := false, false, false
			x.execFn = func(args []string) hostio.ExecResult {
				if len(args) > 1 && args[1] == "validate" {
					validated = true
				}
				if args[0] == "wget" {
					probed = true
					return httpResult(200, "")
				}
				return hostio.ExecResult{}
			}
			x.fault = func(w http.ResponseWriter, r *http.Request) bool {
				if failed {
					return false
				}
				target := r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/archive") && r.URL.Query().Get("path") == "/etc/caddy/routes/app.caddy"
				graph := r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/archive") && r.URL.Query().Get("path") == "/etc/caddy/Caddyfile"
				inspect := strings.Contains(r.URL.Path, "/containers/") && strings.HasSuffix(r.URL.Path, "/json")
				fail := stage == "validation-graph" && probed && !validated && graph || validated && (stage == "final-check" && inspect || stage == "final-target" && target || stage == "final-graph" && graph)
				if fail {
					failed = true
					http.Error(w, `{"message":"temporary daemon failure"}`, http.StatusServiceUnavailable)
					return true
				}
				return false
			}
			got, err := x.a.Apply(t.Context(), q)
			require.True(t, failed)
			require.Equal(t, text, x.bytes("routes/app.caddy"))
			stored, queryErr := x.a.Query(t.Context(), dto.Query{ID: q.ID, Instance: q.Instance, Generation: q.Generation, Registration: q.Registration})
			require.NoError(t, queryErr)
			t.Logf("stage=%s state=%s reason=%s error=%v persisted-state=%s persisted-reason=%s", stage, got.State, got.Reason, err, stored.State, stored.Reason)
			require.Error(t, err, "an unreadable validation graph or final fence is not an observed invalid graph")
			// Recovery settles the unchanged target as rejected without the false graph reason.
			require.Equal(t, dto.Rejected, stored.State)
			require.NotEqual(t, dto.InvalidGraph, stored.Reason)
		})
	}
}
