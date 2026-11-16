// Source: docklet-next/issues/474
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Adopted from the M27 closing verify counterexamples (C-B2/C-B4).

package caddy

import (
	dto "github.com/florianfinn/docklet-next/types/proxy"
	"github.com/stretchr/testify/require"
	"net/http"
	"strings"
	"testing"
)

func TestVerifyTransientPrepareArchiveReadIsNotInvalidGraph(t *testing.T) {
	x := fixture(t)
	site := baseSite()
	text, err := site.Render()
	require.NoError(t, err)
	x.owned("routes/app.caddy", text)
	next := site
	next.Name = "renamed"
	q := x.request("edit-route", next)
	failed := false
	x.fault = func(w http.ResponseWriter, r *http.Request) bool {
		if !failed && r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/archive") && r.URL.Query().Get("path") == "/etc/caddy/Caddyfile" {
			failed = true
			http.Error(w, `{"message":"temporary archive failure"}`, http.StatusServiceUnavailable)
			return true
		}
		return false
	}
	got, err := x.a.Apply(t.Context(), q)
	require.True(t, failed)
	require.Equal(t, text, x.bytes("routes/app.caddy"))
	stored, queryErr := x.a.Query(t.Context(), dto.Query{ID: q.ID, Instance: q.Instance, Generation: q.Generation, Registration: q.Registration})
	require.NoError(t, queryErr)
	t.Logf("state=%s reason=%s error=%v persisted-state=%s persisted-reason=%s", got.State, got.Reason, err, stored.State, stored.Reason)
	require.Error(t, err, "failed baseline archive read must remain an error, not a durable invalid graph refusal")
	require.NotEqual(t, dto.InvalidGraph, stored.Reason)
}
