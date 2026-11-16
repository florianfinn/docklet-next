// Source: docklet-next/issues/471
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Adopted from the M27 closing verify 2 counterexamples (C-B2).

package caddy

import (
	"github.com/florianfinn/docklet-next/backend/internal/proxy/hostio"
	dto "github.com/florianfinn/docklet-next/types/proxy"
	"github.com/stretchr/testify/require"
	"net/http"
	"strings"
	"testing"
)

func TestReviewerVerify2TransientResumeAdminExecCreate(t *testing.T) {
	x := fixture(t)
	x.c.Config.Cmd = append(x.c.Config.Cmd, "--resume")
	site := baseSite()
	text, err := site.Render()
	require.NoError(t, err)
	x.owned("routes/app.caddy", text)
	x.execFn = func(args []string) hostio.ExecResult {
		if args[0] == "wget" {
			return httpResult(200, "{}")
		}
		if len(args) > 1 && args[1] == "adapt" {
			return hostio.ExecResult{Stdout: []byte("{}")}
		}
		return hostio.ExecResult{}
	}
	inv, err := x.e.Inventory(t.Context(), x.i)
	require.NoError(t, err)
	require.True(t, inv.Eligibility.Writable)
	next := site
	next.Name = "renamed"
	q := x.request("edit-route", next)
	failed := false
	x.fault = func(w http.ResponseWriter, r *http.Request) bool {
		if !failed && r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/exec") {
			failed = true
			http.Error(w, `{"message":"temporary exec-create failure"}`, 503)
			return true
		}
		return false
	}
	got, applyErr := x.a.Apply(t.Context(), q)
	require.True(t, failed)
	require.Equal(t, text, x.bytes("routes/app.caddy"))
	stored, err := x.a.Query(t.Context(), dto.Query{ID: q.ID, Instance: q.Instance, Generation: q.Generation, Registration: q.Registration})
	require.NoError(t, err)
	t.Logf("returned=%s/%s error=%v persisted=%s/%s", got.State, got.Reason, applyErr, stored.State, stored.Reason)
	inv, err = x.e.Inventory(t.Context(), x.i)
	require.NoError(t, err)
	require.True(t, inv.Eligibility.Writable)
	require.Error(t, applyErr, "failed Docker exec creation during the resume readback must remain an error")
}
