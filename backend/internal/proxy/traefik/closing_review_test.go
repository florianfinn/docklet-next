// Source: docklet-next/issues/473
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Adopted from the M27 closing review (C-B1 Traefik producer controls).

package traefik

import (
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func TestClosingReviewerEmptyReferenceProducer(t *testing.T) {
	x := fixture(t)
	x.put("dynamic/routes.yml", strings.Replace(tiny, "      service: app", "      middlewares: [\"\"]\n      service: app", 1))
	got, err := x.readResult()
	require.NoError(t, err)
	require.True(t, got.Eligibility.Writable)
	require.Equal(t, "editable", got.Routes[0].Class)
	require.Equal(t, "", got.Routes[0].References[0].Name)
}
func TestClosingReviewerEmptyDefinitionProducer(t *testing.T) {
	x := fixture(t)
	x.put("dynamic/routes.yml", tiny+"  middlewares:\n    \"\":\n      headers:\n        frameDeny: true\n")
	got, err := x.readResult()
	require.NoError(t, err)
	require.True(t, got.Eligibility.Writable)
	found := false
	for _, m := range got.Middlewares {
		if m.Name == "" {
			found = true
		}
	}
	require.True(t, found)
}
func TestClosingReviewerPreserveResolverProducer(t *testing.T) {
	x := fixture(t)
	x.inspect.Config.Cmd = append(x.inspect.Config.Cmd, "--certificatesresolvers.preserve.acme.email=operator@example.com")
	inv, err := x.e.Inventory(t.Context(), x.i)
	require.NoError(t, err)
	require.Contains(t, inv.Resolvers, "preserve")
	r := x.request("edit-route")
	r.Change.Resolver = "preserve"
	in, err := x.e.Prepare(t.Context(), r)
	require.NoError(t, err)
	next, err := parseDocument(string(in.New.Content), r.Target)
	require.NoError(t, err)
	require.Equal(t, "preserve", str(at(next.root, "http", "routers", "app", "tls", "certResolver")))
}
