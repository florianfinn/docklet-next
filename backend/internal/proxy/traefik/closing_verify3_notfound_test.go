// Source: docklet-next/issues/471
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Adopted from the M27 closing verify 3 counterexamples (C-B4: observed missing explicit static file).

package traefik

import (
	"github.com/florianfinn/docklet-next/backend/internal/proxy"
	dto "github.com/florianfinn/docklet-next/types/proxy"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestM27MissingExplicitStaticFileIsATypedRefusal(t *testing.T) {
	x := fixture(t)
	x.inspect.Config.Cmd = []string{"--configfile=/etc/traefik/missing.yml"}
	a, err := proxy.NewAgent(x.db.DB, x.d, x.d.Engines)
	require.NoError(t, err)
	q := x.request("edit-route")
	q.Generation = a.Generation()
	q.Instance.Generation = q.Generation
	q.Instance.Registration = q.Registration
	q.Change.Name = "renamed"
	got, err := a.Apply(t.Context(), q)
	require.NoError(t, err, "an observed missing explicit file must remain a typed eligibility refusal")
	require.Equal(t, dto.Rejected, got.State)
	require.Equal(t, dto.IncompleteInventory, got.Reason)
	require.Equal(t, tiny, x.read("dynamic/routes.yml"))
}
