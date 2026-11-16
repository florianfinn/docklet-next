// Source: docklet-next/issues/473
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Adopted from the M27 closing verify counterexamples (C-B2).

package traefik

import (
	"github.com/florianfinn/docklet-next/backend/internal/proxy"
	dto "github.com/florianfinn/docklet-next/types/proxy"
	"github.com/stretchr/testify/require"
	"net/http"
	"testing"
)

func TestVerifyTransientPrepareArchiveReadIsNotInvalidGraph(t *testing.T) {
	x := fixture(t)
	a, err := proxy.NewAgent(x.db.DB, x.d, proxy.Engines{dto.Traefik: x.e})
	require.NoError(t, err)
	q := x.request("edit-route")
	q.Generation = a.Generation()
	q.Instance.Generation = q.Generation
	q.Instance.Registration = q.Registration
	q.Change.Name = "renamed"
	x.archiveStatus = map[string]int{"/etc/traefik/dynamic": http.StatusServiceUnavailable}
	got, err := a.Apply(t.Context(), q)
	require.Positive(t, x.archiveFaults, "the injected archive fault was actually served")
	require.Equal(t, tiny, x.read("dynamic/routes.yml"))
	stored, queryErr := a.Query(t.Context(), dto.Query{ID: q.ID, Instance: q.Instance, Generation: q.Generation, Registration: q.Registration})
	require.NoError(t, queryErr)
	t.Logf("state=%s reason=%s error=%v persisted-state=%s persisted-reason=%s", got.State, got.Reason, err, stored.State, stored.Reason)
	require.Error(t, err, "failed baseline archive read must remain an error, not a durable invalid graph refusal")
	require.NotEqual(t, dto.InvalidGraph, stored.Reason)
	require.NotEqual(t, dto.IncompleteInventory, stored.Reason)
}
